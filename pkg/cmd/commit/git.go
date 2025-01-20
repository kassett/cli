package commit

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/cli/cli/v2/api"
	"os"
	"path/filepath"
	"strings"
)

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
	return getGitOutputRef(command[1:])
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
	commandOutput, err := getGitOutputRef(command[1:])
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
	_, err := getGitOutputRef([]string{"fetch"})
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
	_, err = getGitOutputRef([]string{"stash", "push", "--include-untracked"})
	if err != nil {
		return err
	}
	_, err = getGitOutputRef([]string{"checkout", branchName})
	if err != nil {
		return err
	}
	_, err = getGitOutputRef([]string{"pull"})
	if err != nil {
		return err
	}

	// Verify branches are not diverging by comparing local and remote HEAD
	localHash, err := getGitOutputRef([]string{"rev-parse", "HEAD"})
	if err != nil {
		return fmt.Errorf("failed to get local HEAD commit: %w", err)
	}

	remoteHash, err := getGitOutputRef([]string{"rev-parse", fmt.Sprintf("origin/%s", branchName)})
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
	_, err = getGitOutputRef([]string{"stash", "drop"})
	if err != nil {
		return err
	}
	return nil
}

// getTreeTip returns the sha of the tree tip based on the latest commit
func getTreeTip(latestCommit string) string {
	path := fmt.Sprintf("/git/trees/%s", latestCommit)

	// Ignore the error explicitly because we are certain it exists
	output, _ := makeRequestRef(path, "GET", nil, nil)
	return output["sha"].(string)
}

// getLatestCommit returns whether the branch exists, the sha of the latest commit (either to the branch if it exists, or the default branch), and any errors
func getLatestCommit(defaultBranch string, branch string) (bool, string, error) {
	var commitResponse struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}

	_, err := makeRequestRef(fmt.Sprintf("/branches/%s", branch), "GET", nil, &commitResponse)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) && (httpError.StatusCode != 404 || httpError.Message != "Branch not found") {
			return false, "", err
		}
	} else {
		return true, commitResponse.Commit.SHA, nil
	}

	var defaultCommitResponse struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	_, err = makeRequestRef(fmt.Sprintf("/branches/%s", defaultBranch), "GET", nil, &defaultCommitResponse)
	return false, defaultCommitResponse.Commit.SHA, nil
}

// createBlobs creates blobs for the files provided
func createBlobs(files []string) ([]map[string]interface{}, error) {
	blobs := make([]map[string]interface{}, 0)
	for _, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			blobs = append(blobs, map[string]interface{}{
				"path": file,
				"mode": "100644",
				"type": "blob",
				"sha":  nil,
			})
		} else {
			data, _ := os.ReadFile(file)
			encoded := base64.StdEncoding.EncodeToString(data)

			var blobStruct struct {
				SHA string `json:"sha"`
			}

			body := map[string]interface{}{
				"content":  encoded,
				"encoding": "base64",
			}
			_, err = makeRequestRef("/git/blobs", "POST", body, &blobStruct)
			if err != nil {
				return nil, err
			}

			blobs = append(blobs, map[string]interface{}{
				"path": file,
				"mode": "100644",
				"type": "blob",
				"sha":  blobStruct.SHA,
			})
		}
	}
	return blobs, nil
}

// createNewTree creates a new tree based on the provided treeSha and blobs
func createNewTree(treeSha string, blobs []map[string]interface{}) (string, error) {
	tree := map[string]interface{}{
		"base_tree": treeSha,
		"tree":      blobs,
	}

	var treeStruct struct {
		SHA string `json:"sha"`
	}
	_, err := makeRequestRef("/git/trees", "POST", tree, &treeStruct)
	if err != nil {
		return "", err
	}

	return treeStruct.SHA, nil
}

// commitTree commits a tree based on the provided treeSha, latestCommit, and commitMessage
func commitTree(treeSha string, latestCommit string, commitMessage string) (string, error) {
	body := map[string]interface{}{
		"message": commitMessage,
		"tree":    treeSha,
		"parents": []string{latestCommit},
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	_, err := makeRequestRef("/git/commits", "POST", body, &commit)
	if err != nil {
		return "", err
	}

	return commit.SHA, nil
}

// createNewBranch creates a new branch based on the provided commitSha and branchName
func createNewBranch(commitSha string, branchName string) error {
	body := map[string]interface{}{
		"ref": fmt.Sprintf("refs/heads/%s", branchName),
		"sha": commitSha,
	}
	_, err := makeRequestRef("/git/refs", "POST", body, nil)
	return err
}

func updateBranch(commitSha string, branchName string) error {
	body := map[string]interface{}{
		"sha": commitSha,
	}
	_, err := makeRequestRef(fmt.Sprintf("/git/refs/heads/%s", branchName), "POST", body, nil)
	return err
}
