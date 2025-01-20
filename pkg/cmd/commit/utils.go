package commit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(sourceFile *os.File) {
		_ = sourceFile.Close()
	}(sourceFile)

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func(destinationFile *os.File) {
		_ = destinationFile.Close()
	}(destinationFile)

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}

// copyFilesToTempDir copies files to a temporary directory and returns the temp directory path
func copyFilesToTempDir(files []string) (string, error) {
	tempDir, err := os.MkdirTemp("", "git-sync")
	if err != nil {
		return "", err
	}

	for _, file := range files {
		// Ensure directories are created in the temp dir
		relativePath := filepath.Dir(file)
		if err := os.MkdirAll(filepath.Join(tempDir, relativePath), os.ModePerm); err != nil {
			return "", err
		}

		// Copy file to temp dir
		if err := copyFile(file, filepath.Join(tempDir, file)); err != nil {
			return "", err
		}
	}

	return tempDir, nil
}

// writeToTempFile writes a map[string]interface{} to a temporary file in JSON format.
func writeToTempFile(data map[string]interface{}) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", "body-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	encoder := json.NewEncoder(tmpFile)
	if err := encoder.Encode(data); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("failed to write JSON to temp file: %w", err)
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("failed to reset file pointer: %w", err)
	}

	return tmpFile, nil
}
