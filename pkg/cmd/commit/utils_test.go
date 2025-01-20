package commit

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

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

	// The target function
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

// Test_copyFile tests the copyFile function
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

	// The target function
	err = copyFile(sourceFileName, destFileName)

	assert.NoError(t, err, "copyFile should not return an error")
	_, err = os.Stat(destFileName)
	assert.NoError(t, err, "Destination file should exist")

	// Check the content of the destination file
	destFileContent, err := ioutil.ReadFile(destFileName)
	assert.NoError(t, err, "Failed to read destination file")
	assert.Equal(t, sourceFileContent, destFileContent, "File content should match")
}

func Test_writeToTempFile(t *testing.T) {
	tests := []struct {
		name        string
		inputData   map[string]interface{}
		expectError bool
	}{
		{
			name: "Valid JSON data",
			inputData: map[string]interface{}{
				"key1": "value1",
				"key2": 23.1,
				"key3": true,
			},
			expectError: false,
		},
		{
			name:        "Empty map",
			inputData:   map[string]interface{}{},
			expectError: false,
		},
		{
			name:        "Nil data",
			inputData:   nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call writeToTempFile
			tmpFile, err := writeToTempFile(tt.inputData)

			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Ensure the file was created and is not nil
			assert.NotNil(t, tmpFile)
			defer func(name string) {
				_ = os.Remove(name)
			}(tmpFile.Name())
			defer func(tmpFile *os.File) {
				_ = tmpFile.Close()
			}(tmpFile)

			// Read the file content
			content, err := ioutil.ReadAll(tmpFile)
			assert.NoError(t, err)

			// Parse the file content back into a map
			var result map[string]interface{}
			err = json.Unmarshal(content, &result)
			assert.NoError(t, err)

			// Compare the result with the input data
			assert.Equal(t, tt.inputData, result)
		})
	}
}
