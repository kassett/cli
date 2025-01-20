package commit

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       commitOptions
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name:        "no argument",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"branch\", \"message\" not set",
		},
		{
			name:     "match multiple patterns",
			cli:      "-m \"commit message\" -b main file1 file2",
			wantsErr: false,
			wants: commitOptions{
				CommitMessage:  "commit message",
				PatternMatches: []string{"file1", "file2"},
				Branch:         "main",
			},
		},
		{
			name:     "use commit all",
			cli:      "-m \"commit message\" -b main --all",
			wantsErr: false,
			wants: commitOptions{
				CommitMessage: "commit message",
				Branch:        "main",
				CommitAll:     true,
			},
		},
		{
			name:        "fail on commit all and force",
			cli:         "-m \"commit message\" -b main --all --force",
			wantsErr:    true,
			wantsErrMsg: "specify only one of `--all`, `--force`",
		},
		{
			name: "test dry run",
			cli:  "-m \"commit message\" -b main --dry-run",
			wants: commitOptions{
				CommitMessage: "commit message",
				Branch:        "main",
				DryRun:        true,
			},
			wantsErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts commitOptions
			cmd := NewCmdCommit(f, func(config *commitOptions) error {
				gotOpts = *config
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Branch, gotOpts.Branch)
			assert.Equal(t, tt.wants.CommitMessage, gotOpts.CommitMessage)
			assert.Equal(t, tt.wants.CommitAll, gotOpts.CommitAll)
			assert.Equal(t, tt.wants.Force, gotOpts.Force)
			assert.Equal(t, tt.wants.SyncWithRemote, gotOpts.SyncWithRemote)
			assert.Equal(t, tt.wants.DryRun, gotOpts.DryRun)
		})
	}
}

func TestSetupContext(t *testing.T) {

}

func TestCreateBlob(t *testing.T) {

}

func TestCreateTree(t *testing.T) {

}

func TestLatestCommit(t *testing.T) {

}

// Test_copyFilesToTempDir tests the copyFilesToTempDir function
func Test_copyFilesToTempDir(t *testing.T) {
	// Arrange: Create temporary files to act as input files
	inputFiles := []string{
		"testdir/file1.txt",
		"testdir/nested/file2.txt",
	}

	// Create test files with sample content
	for _, file := range inputFiles {
		_ = os.MkdirAll(filepath.Dir(file), os.ModePerm)
		err := os.WriteFile(file, []byte("test content"), os.ModePerm)
		assert.NoError(t, err, "Failed to create test file: %s", file)
	}
	defer func() {
		for _, file := range inputFiles {
			_ = os.Remove(file)
		}
		_ = os.RemoveAll("testdir")
	}()

	// Act: Call the function
	tempDir, err := copyFilesToTempDir(inputFiles)

	assert.NoError(t, err, "Expected no error from copyFilesToTempDir")
	assert.NotEmpty(t, tempDir, "Temp directory path should not be empty")

	for _, file := range inputFiles {
		tempFilePath := filepath.Join(tempDir, file)
		assert.FileExists(t, tempFilePath, "Expected file to exist in temp directory: %s", tempFilePath)

		// Verify file content
		content, err := os.ReadFile(tempFilePath)
		assert.NoError(t, err, "Failed to read temp file: %s", tempFilePath)
		assert.Equal(t, "test content", string(content), "File content mismatch for: %s", tempFilePath)
	}

	_ = os.RemoveAll(tempDir)
}

func Test_copyFile(t *testing.T) {
	sourceFileContent := []byte("Hello, world!")
	sourceFileName := "source_test_file.txt"
	destFileName := "dest_test_file.txt"

	err := ioutil.WriteFile(sourceFileName, sourceFileContent, 0644)
	assert.NoError(t, err, "Failed to create source file")

	defer func() {
		_ = os.Remove(sourceFileName)
		_ = os.Remove(destFileName)
	}()

	err = copyFile(sourceFileName, destFileName)

	assert.NoError(t, err, "copyFile should not return an error")
	_, err = os.Stat(destFileName)
	assert.NoError(t, err, "Destination file should exist")

	// Check the content of the destination file
	destFileContent, err := ioutil.ReadFile(destFileName)
	assert.NoError(t, err, "Failed to read destination file")
	assert.Equal(t, sourceFileContent, destFileContent, "File content should match")
}
