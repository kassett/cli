package commit

import (
	"encoding/json"
	"errors"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"testing"
)

func Test_getTreeTip(t *testing.T) {
	originalMakeRequest := makeRequestRef
	defer func() { makeRequestRef = originalMakeRequest }()

	t.Run("Successful retrieval", func(t *testing.T) {
		// Mock makeRequest
		makeRequestRef = func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"sha": "abcdef123456",
			}, nil
		}
		latestCommit := "commit123"

		// Act
		sha := getTreeTip(latestCommit)

		// Assert
		assert.Equal(t, "abcdef123456", sha)
	})
}

func Test_getLatestCommit(t *testing.T) {
	originalMakeRequest := makeRequestRef
	defer func() { makeRequestRef = originalMakeRequest }()

	tests := []struct {
		name          string
		defaultBranch string
		branch        string
		mockResponses []struct {
			path   string
			result interface{}
			err    error
		}
		expectedBranchExists bool
		expectedSHA          string
		expectError          bool
	}{
		{
			name:          "Branch exists",
			defaultBranch: "main",
			branch:        "feature",
			mockResponses: []struct {
				path   string
				result interface{}
				err    error
			}{
				{path: "/branches/feature", result: struct {
					Name   string `json:"name"`
					Commit struct {
						SHA string `json:"sha"`
					} `json:"commit"`
				}{
					Name: "feature",
					Commit: struct {
						SHA string `json:"sha"`
					}{SHA: "feature-sha"},
				}, err: nil},
			},
			expectedBranchExists: true,
			expectedSHA:          "feature-sha",
			expectError:          false,
		},
		{
			name:          "Branch does not exist, default branch used",
			defaultBranch: "main",
			branch:        "nonexistent",
			mockResponses: []struct {
				path   string
				result interface{}
				err    error
			}{
				{path: "/branches/nonexistent", result: nil, err: &ghAPI.HTTPError{StatusCode: 404, Message: "Branch not found"}},
				{path: "/branches/main", result: struct {
					Name   string `json:"name"`
					Commit struct {
						SHA string `json:"sha"`
					} `json:"commit"`
				}{
					Name: "main",
					Commit: struct {
						SHA string `json:"sha"`
					}{SHA: "default-sha"},
				}, err: nil},
			},
			expectedBranchExists: false,
			expectedSHA:          "default-sha",
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock makeRequestRef
			callIndex := 0
			makeRequestRef = func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				mock := tt.mockResponses[callIndex]
				callIndex++
				if mock.err != nil {
					return nil, mock.err
				}
				// Simulate filling the response
				r := mock.result
				if r != nil {
					rBytes, _ := json.Marshal(r)
					_ = json.Unmarshal(rBytes, data)
				}
				return nil, nil
			}

			// Call the function
			branchExists, sha, err := getLatestCommit(tt.defaultBranch, tt.branch)

			// Assertions
			assert.Equal(t, tt.expectedBranchExists, branchExists, "Branch existence mismatch")
			assert.Equal(t, tt.expectedSHA, sha, "SHA mismatch")
			if tt.expectError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Expected no error but got one")
			}
		})
	}
}

func Test_createBlobs(t *testing.T) {
	originalMakeRequest := makeRequestRef                   // Backup the original function
	defer func() { makeRequestRef = originalMakeRequest }() // Restore after test

	tests := []struct {
		name          string
		setupFiles    func(tempDir string) []string
		mockRequest   func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error)
		expectedBlobs []map[string]interface{}
		expectError   bool
	}{
		{
			name: "File does not exist",
			setupFiles: func(tempDir string) []string {
				return []string{"missing.txt"} // No file is created
			},
			mockRequest: nil,
			expectedBlobs: []map[string]interface{}{
				{
					"path": "missing.txt",
					"mode": "100644",
					"type": "blob",
					"sha":  nil,
				},
			},
			expectError: false,
		},
		{
			name: "File exists and blob is created",
			setupFiles: func(tempDir string) []string {
				filePath := tempDir + "/file.txt"
				err := ioutil.WriteFile(filePath, []byte("file content"), 0644)
				assert.NoError(t, err)
				return []string{filePath}
			},
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				assert.Equal(t, "/git/blobs", endpoint)
				assert.Equal(t, "POST", method)
				assert.NotNil(t, body)

				// Simulate setting the SHA value
				if blobData, ok := data.(*struct {
					SHA string `json:"sha"`
				}); ok {
					blobData.SHA = "blob123"
				}
				return nil, nil
			},
			expectedBlobs: []map[string]interface{}{
				{
					"path": "", // This will be dynamically updated to the full path
					"mode": "100644",
					"type": "blob",
					"sha":  "blob123",
				},
			},
			expectError: false,
		},
		{
			name: "API error during blob creation",
			setupFiles: func(tempDir string) []string {
				filePath := tempDir + "/file.txt"
				err := ioutil.WriteFile(filePath, []byte("file content"), 0644)
				assert.NoError(t, err)
				return []string{filePath}
			},
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				return nil, errors.New("API error")
			},
			expectedBlobs: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory
			tempDir, err := ioutil.TempDir("", "test-create-blobs")
			assert.NoError(t, err)
			defer os.RemoveAll(tempDir) // Clean up

			// Setup files
			files := tt.setupFiles(tempDir)

			// Mock makeRequestRef
			if tt.mockRequest != nil {
				makeRequestRef = tt.mockRequest
			}

			// Call createBlobs
			blobs, err := createBlobs(files)

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Dynamically update expected paths to full file paths
				for i, file := range files {
					tt.expectedBlobs[i]["path"] = file
				}

				assert.Equal(t, tt.expectedBlobs, blobs)
			}
		})
	}
}

func Test_createNewTree(t *testing.T) {
	originalMakeRequest := makeRequestRef                   // Backup the original function
	defer func() { makeRequestRef = originalMakeRequest }() // Restore after test

	tests := []struct {
		name        string
		treeSha     string
		blobs       []map[string]interface{}
		mockRequest func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error)
		expectedSHA string
		expectError bool
	}{
		{
			name:    "Successful tree creation",
			treeSha: "base-tree-sha",
			blobs: []map[string]interface{}{
				{"path": "file1.txt", "mode": "100644", "type": "blob", "sha": "sha1"},
				{"path": "file2.txt", "mode": "100644", "type": "blob", "sha": "sha2"},
			},
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				assert.Equal(t, "/git/trees", endpoint)
				assert.Equal(t, "POST", method)

				// Validate request body
				assert.Equal(t, "base-tree-sha", body["base_tree"])
				assert.Equal(t, []map[string]interface{}{
					{"path": "file1.txt", "mode": "100644", "type": "blob", "sha": "sha1"},
					{"path": "file2.txt", "mode": "100644", "type": "blob", "sha": "sha2"},
				}, body["tree"])

				// Simulate setting the SHA in the response
				if treeStruct, ok := data.(*struct {
					SHA string `json:"sha"`
				}); ok {
					treeStruct.SHA = "new-tree-sha"
				}
				return nil, nil
			},
			expectedSHA: "new-tree-sha",
			expectError: false,
		},
		{
			name:    "API error during tree creation",
			treeSha: "base-tree-sha",
			blobs: []map[string]interface{}{
				{"path": "file1.txt", "mode": "100644", "type": "blob", "sha": "sha1"},
			},
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				return nil, errors.New("API error")
			},
			expectedSHA: "",
			expectError: true,
		},
		{
			name:        "Empty treeSha",
			treeSha:     "",
			blobs:       []map[string]interface{}{},
			mockRequest: nil,
			expectedSHA: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock makeRequestRef
			if tt.mockRequest != nil {
				makeRequestRef = tt.mockRequest
			}

			// Call createNewTree
			sha, err := createNewTree(tt.treeSha, tt.blobs)

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSHA, sha)
			}
		})
	}
}

func Test_commitTree(t *testing.T) {
	originalMakeRequest := makeRequestRef                   // Backup the original function
	defer func() { makeRequestRef = originalMakeRequest }() // Restore after test

	tests := []struct {
		name          string
		treeSha       string
		latestCommit  string
		commitMessage string
		mockRequest   func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error)
		expectedSHA   string
		expectError   bool
	}{
		{
			name:          "Successful commit creation",
			treeSha:       "tree-sha123",
			latestCommit:  "latest-commit456",
			commitMessage: "Initial commit",
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				assert.Equal(t, "/git/commits", endpoint)
				assert.Equal(t, "POST", method)

				// Validate the request body
				assert.Equal(t, "Initial commit", body["message"])
				assert.Equal(t, "tree-sha123", body["tree"])
				assert.Equal(t, []string{"latest-commit456"}, body["parents"])

				// Simulate setting the SHA in the response
				if commitData, ok := data.(*struct {
					SHA string `json:"sha"`
				}); ok {
					commitData.SHA = "commit-sha789"
				}
				return nil, nil
			},
			expectedSHA: "commit-sha789",
			expectError: false,
		},
		{
			name:          "API error during commit creation",
			treeSha:       "tree-sha123",
			latestCommit:  "latest-commit456",
			commitMessage: "Error commit",
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				return nil, errors.New("API error")
			},
			expectedSHA: "",
			expectError: true,
		},
		{
			name:          "Empty treeSha",
			treeSha:       "",
			latestCommit:  "latest-commit456",
			commitMessage: "Commit with empty treeSha",
			mockRequest:   nil,
			expectedSHA:   "",
			expectError:   true,
		},
		{
			name:          "Empty latestCommit",
			treeSha:       "tree-sha123",
			latestCommit:  "",
			commitMessage: "Commit with empty latestCommit",
			mockRequest:   nil,
			expectedSHA:   "",
			expectError:   true,
		},
		{
			name:          "Empty commitMessage",
			treeSha:       "tree-sha123",
			latestCommit:  "latest-commit456",
			commitMessage: "",
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				assert.Equal(t, "/git/commits", endpoint)
				assert.Equal(t, "POST", method)

				// Validate the request body
				assert.Equal(t, "", body["message"])
				assert.Equal(t, "tree-sha123", body["tree"])
				assert.Equal(t, []string{"latest-commit456"}, body["parents"])

				// Simulate setting the SHA in the response
				if commitData, ok := data.(*struct {
					SHA string `json:"sha"`
				}); ok {
					commitData.SHA = "commit-sha-empty-message"
				}
				return nil, nil
			},
			expectedSHA: "commit-sha-empty-message",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock makeRequestRef
			if tt.mockRequest != nil {
				makeRequestRef = tt.mockRequest
			}

			// Call commitTree
			sha, err := commitTree(tt.treeSha, tt.latestCommit, tt.commitMessage)

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSHA, sha)
			}
		})
	}
}

func Test_updateBranch(t *testing.T) {
	originalMakeRequest := makeRequestRef                   // Backup the original function
	defer func() { makeRequestRef = originalMakeRequest }() // Restore after test

	tests := []struct {
		name        string
		commitSha   string
		branchName  string
		mockRequest func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error)
		expectError bool
	}{
		{
			name:       "Successful branch update",
			commitSha:  "commit-sha123",
			branchName: "main",
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				assert.Equal(t, "/git/refs/heads/main", endpoint)
				assert.Equal(t, "POST", method)

				// Validate the request body
				assert.Equal(t, "commit-sha123", body["sha"])
				return nil, nil
			},
			expectError: false,
		},
		{
			name:       "API error during branch update",
			commitSha:  "commit-sha123",
			branchName: "main",
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				return nil, errors.New("API error")
			},
			expectError: true,
		},
		{
			name:        "Empty commitSha",
			commitSha:   "",
			branchName:  "main",
			mockRequest: nil,
			expectError: true,
		},
		{
			name:        "Empty branchName",
			commitSha:   "commit-sha123",
			branchName:  "",
			mockRequest: nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock makeRequestRef
			if tt.mockRequest != nil {
				makeRequestRef = tt.mockRequest
			}

			// Call updateBranch
			err := updateBranch(tt.commitSha, tt.branchName)

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateNewBranch(t *testing.T) {
	originalMakeRequest := makeRequestRef                   // Backup the original function
	defer func() { makeRequestRef = originalMakeRequest }() // Restore after test

	tests := []struct {
		name        string
		commitSha   string
		branchName  string
		mockRequest func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error)
		expectError bool
	}{
		{
			name:       "Successful branch creation",
			commitSha:  "commit-sha123",
			branchName: "new-branch",
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				assert.Equal(t, "/git/refs", endpoint)
				assert.Equal(t, "POST", method)

				// Validate the request body
				assert.Equal(t, "refs/heads/new-branch", body["ref"])
				assert.Equal(t, "commit-sha123", body["sha"])
				return nil, nil
			},
			expectError: false,
		},
		{
			name:       "API error during branch creation",
			commitSha:  "commit-sha123",
			branchName: "new-branch",
			mockRequest: func(endpoint, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
				return nil, errors.New("API error")
			},
			expectError: true,
		},
		{
			name:        "Empty commitSha",
			commitSha:   "",
			branchName:  "new-branch",
			mockRequest: nil,
			expectError: true,
		},
		{
			name:        "Empty branchName",
			commitSha:   "commit-sha123",
			branchName:  "",
			mockRequest: nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock makeRequestRef
			if tt.mockRequest != nil {
				makeRequestRef = tt.mockRequest
			}

			// Call createNewBranch
			err := createNewBranch(tt.commitSha, tt.branchName)

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_listFilesUsingPatterns(t *testing.T) {
	// Backup the original function
	originalGetGitOutputRef := getGitOutputRef
	defer func() { getGitOutputRef = originalGetGitOutputRef }() // Restore after tests

	tests := []struct {
		name             string
		patterns         []string
		force            bool
		excludeUntracked bool
		mockOutput       []string
		mockError        error
		expectedFiles    []string
		expectError      bool
	}{
		{
			name:             "Successful file list with patterns",
			patterns:         []string{"*.go", "*.md"},
			force:            false,
			excludeUntracked: false,
			mockOutput:       []string{"add 'file1.go'", "add 'file2.md'"},
			mockError:        nil,
			expectedFiles:    []string{"file1.go", "file2.md"},
			expectError:      false,
		},
		{
			name:             "Exclude untracked files",
			patterns:         []string{"*.txt"},
			force:            false,
			excludeUntracked: true,
			mockOutput:       []string{"add 'tracked_file.txt'"},
			mockError:        nil,
			expectedFiles:    []string{"tracked_file.txt"},
			expectError:      false,
		},
		{
			name:             "Force add files",
			patterns:         []string{"*.json"},
			force:            true,
			excludeUntracked: false,
			mockOutput:       []string{"add 'untracked_file.json'"},
			mockError:        nil,
			expectedFiles:    []string{"untracked_file.json"},
			expectError:      false,
		},
		{
			name:             "Git command returns error",
			patterns:         []string{"*.yaml"},
			force:            false,
			excludeUntracked: false,
			mockOutput:       nil,
			mockError:        assert.AnError,
			expectedFiles:    nil,
			expectError:      true,
		},
		{
			name:             "No matching files",
			patterns:         []string{"*.tmp"},
			force:            false,
			excludeUntracked: false,
			mockOutput:       []string{},
			mockError:        nil,
			expectedFiles:    []string{},
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock getGitOutputRef for this test case
			getGitOutputRef = func(command []string) ([]string, error) {
				return tt.mockOutput, tt.mockError
			}

			// Call listFilesUsingPatterns
			files, err := listFilesUsingPatterns(tt.patterns, tt.force, tt.excludeUntracked)

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedFiles, files)
			}
		})
	}
}
