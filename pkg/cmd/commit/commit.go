package commit

import (
	"errors"
	"fmt"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"net/http"
	"os"
)

var (
	gitClient     *git.Client
	apiClient     *api.Client
	repo          ghrepo.Interface
	host          string
	defaultBranch string
)

// commitOptions the options for the commit command
type commitOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser
	GitClient  *git.Client

	// CommitMessage the message designated for the particular commit
	CommitMessage string
	// PatternMatches pattern matches for the files to be committed
	PatternMatches []string
	// Branch the name of the branch the commit will be made to
	Branch string

	// CommitAll commit all changed files
	CommitAll bool
	// Force commit traditionally ignored files
	Force bool
	// IncludeUntracked include untracked files in the commit
	IncludeUntracked bool
	// IncludeStagedFiles include staged files in the commit
	IncludeStagedFiles bool

	// SyncWithRemote will ensure that the local branch is up to date with the remote branch
	SyncWithRemote bool

	// DryRun get a description of the commit that would be made
	DryRun bool
}

func NewCmdCommit(f *cmdutil.Factory, runF func(options *commitOptions) error) *cobra.Command {
	opts := &commitOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "commit [<files>...] -b [<branch>]",
		Short:                 "Create a new commit.",
		PreRun: func(c *cobra.Command, args []string) {
			opts.BaseRepo = cmdutil.OverrideBaseRepoFunc(f, "")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set pattern matches from arguments
			opts.PatternMatches = args

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--all`, `--force`",
				opts.CommitAll,
				opts.Force,
			); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return createCommit(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "The name of the branch to commit to.")
	cmd.Flags().StringVarP(&opts.CommitMessage, "message", "m", "", "Commit message for the new commit.")
	cmd.Flags().BoolVarP(&opts.CommitAll, "all", "a", false, "Commit all changed files.")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Force the commit of traditionally ignored files.")
	cmd.Flags().BoolVar(&opts.IncludeUntracked, "include-untracked", false, "Include untracked files in the commit.")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview the commit without actually creating it.")
	cmd.Flags().BoolVar(&opts.IncludeStagedFiles, "include-staged", false, "Include staged files in the commit.")
	cmd.Flags().BoolVar(&opts.SyncWithRemote, "sync-local", false, "Ensure that the local branch is up to date with the remote branch.")

	// Mark --message as required
	_ = cmd.MarkFlagRequired("message")
	_ = cmd.MarkFlagRequired("branch")

	cmdutil.EnableRepoOverride(cmd, f)
	return cmd
}

// setupContext sets up the command for the global variables to be used in the commit
func setupContext(opts *commitOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient = api.NewClientFromHTTP(httpClient)
	gitClient = opts.GitClient
	repo, err = opts.BaseRepo()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host, _ = cfg.Authentication().DefaultHost()
	defaultBranch, err = api.RepoDefaultBranch(apiClient, repo)
	if err != nil {
		return err
	}
	return nil
}

// createCommit is the main function for the commit command
func createCommit(opts *commitOptions) error {
	err := setupContext(opts)
	if err != nil {
		return nil
	}

	alreadyStagedFiles, err := listStagedFiles()
	if len(alreadyStagedFiles) > 0 && !opts.IncludeStagedFiles {
		return errors.New("staged files found, use --include-staged to include them in the commit")
	}

	filesToCommit, err := listFilesForCommit(opts)
	filesToCommit = append(filesToCommit, alreadyStagedFiles...)

	if err != nil {
		return err
	}

	branchExists, latestCommit, err := getLatestCommit(defaultBranch, opts.Branch)
	if err != nil {
		return err
	}
	if !branchExists {
		err = createNewBranch(latestCommit, opts.Branch)
		if err != nil {
			return err
		}
	}
	treeTip := getTreeTip(latestCommit)
	if err != nil {
		return err
	}
	blobs, err := createBlobs(filesToCommit)
	if err != nil {
		return err
	}
	newTreeSha, err := createNewTree(treeTip, blobs)
	if err != nil {
		return err
	}
	newCommitSha, err := commitTree(newTreeSha, latestCommit, opts.CommitMessage)
	if err != nil {
		return err
	}
	err = updateBranch(newCommitSha, opts.Branch)
	if err != nil {
		return err
	}

	if opts.SyncWithRemote {
		err = syncWithRemote(opts.Branch)
		if err != nil {
			return err
		}
	}

	return nil
}

// listFilesForCommit returns a list of files to be committed based on the options provided
func listFilesForCommit(opts *commitOptions) ([]string, error) {
	if !opts.CommitAll && (opts.PatternMatches != nil || len(opts.PatternMatches) == 0) {
		return nil, errors.New("no files to commit")
	}

	if opts.CommitAll {
		return listFilesUsingPatterns([]string{"."}, opts.Force, !opts.IncludeUntracked)
	}
	return listFilesUsingPatterns(opts.PatternMatches, opts.Force, !opts.IncludeUntracked)
}

// makeRequest makes a request to the GitHub API, using a temporary file for the body of the message.
func makeRequest(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
	// Construct the endpoint URL
	endpoint = fmt.Sprintf("repos/%s/%s", repo.RepoOwner(), repo.RepoName()) + endpoint

	// Prepare the request body
	var ioBody *os.File
	if body != nil {
		tmpFile, err := writeToTempFile(body)
		if err != nil {
			return nil, err
		}
		defer func(name string) {
			_ = os.Remove(name)
		}(tmpFile.Name())
		ioBody, err = os.Open(tmpFile.Name())
		if err != nil {
			return nil, err
		}
		defer func(ioBody *os.File) {
			_ = ioBody.Close()
		}(ioBody)
	}

	// Determine the target for the response
	target := data
	if target == nil {
		target = &map[string]interface{}{}
	}

	// Make the API request
	err := apiClient.REST(host, method, endpoint, ioBody, target)
	if err != nil {
		return nil, err
	}

	// Return the response if no error occurred
	if responseMap, ok := target.(*map[string]interface{}); ok {
		return *responseMap, nil
	}
	return nil, nil
}
