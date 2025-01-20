package commit

import (
	"context"
	"fmt"
	"github.com/cli/cli/v2/git"
	"path/filepath"
	"strings"
)

type simpleGitClientInterface interface {
	Command(ctx context.Context, args ...string) (git.Command, error)
}

type simpleGitCommand interface {
	Output() ([]byte, error)
}

type gitExecutor struct {
	client simpleGitClientInterface
}

// getGitOutput runs a git command and returns the output as a list of strings.
func getGitOutput(command []string) ([]string, error) {
	cmd, err := gitClient.Command(context.Background(), command...)
	cmdOutput, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	outputStr := strings.TrimSpace(string(cmdOutput))
	cmdOutputAsList := strings.Split(outputStr, "\n")

	prunedList := make([]string, 0)
	for _, item := range cmdOutputAsList {
		if item != "" {
			prunedList = append(prunedList, item)
		}
	}

	return prunedList, nil
}

func listStagedFiles() ([]string, error) {
	command := []string{
		"git",
		"diff",
		"--name-only",
		"--cached",
	}
	return getGitOutput(command[1:])
}

func listFilesUsingPatterns(patterns []string, force bool, excludeUntracked bool) ([]string, error) {
	command := []string{
		"git",
		"add",
		"--dry-run",
	}
	if excludeUntracked {
		command = append(command, "-u")
	} else if force {
		command = append(command, "-f")
	}

	command = append(command, patterns...)
	commandOutput, err := getGitOutput(command[1:])
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	for _, commandResult := range commandOutput {
		// Adding files using git add --dry-run returns "add '<FILENAME>'"
		filename := strings.Trim(strings.Replace(commandResult, "add ", "", 1), "'")
		files = append(files, filename)
	}
	return files, nil
}

func syncWithRemote(branchName string) error {
	// First thing is run git fetch
	_, err := getGitOutput([]string{"fetch"})
	if err != nil {
		return err
	}

	// List changed files
	backupFiles, err := listFilesUsingPatterns([]string{"."}, false, false)
	if err != nil {
		return err
	}

	tempDir, err := copyFilesToTempDir(backupFiles)
	if err != nil {
		return err
	}

	// Now stash the latest changes
	_, err = getGitOutput([]string{"stash", "push", "--include-untracked"})
	if err != nil {
		return err
	}
	_, err = getGitOutput([]string{"checkout", branchName})
	if err != nil {
		return err
	}
	_, err = getGitOutput([]string{"pull"})
	if err != nil {
		return err
	}

	// Verify branches are not diverging by comparing local and remote HEAD
	localHash, err := getGitOutput([]string{"rev-parse", "HEAD"})
	if err != nil {
		return fmt.Errorf("failed to get local HEAD commit: %w", err)
	}

	remoteHash, err := getGitOutput([]string{"rev-parse", fmt.Sprintf("origin/%s", branchName)})
	if err != nil {
		return fmt.Errorf("failed to get remote HEAD commit: %w", err)
	}

	if strings.TrimSpace(localHash[0]) != strings.TrimSpace(remoteHash[0]) {
		return fmt.Errorf("branches have diverged: local HEAD %s, remote HEAD %s",
			strings.TrimSpace(localHash[0]), strings.TrimSpace(remoteHash[0]))
	}

	// Copy files back
	for _, file := range backupFiles {
		src := filepath.Join(tempDir, file)
		dst := file
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("failed to restore file %s: %w", file, err)
		}
	}

	// Apply the stash
	_, err = getGitOutput([]string{"stash", "drop"})
	if err != nil {
		return err
	}
	return nil
}
