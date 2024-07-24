package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCopyDirectory(t *testing.T) {
	test := func(t *testing.T, copyCallback func(src, dst string) error) {
		// Create a temporary directory
		tempDir, err := os.MkdirTemp("", "test-src")
		require.NoError(t, err, "Failed to create temporary directory")
		defer os.RemoveAll(tempDir)

		// Create a temporary file
		tempFile, err := os.CreateTemp(tempDir, "testfile")
		require.NoError(t, err, "Failed to create temporary file")

		// Write a random string to the temporary file
		randomString := uuid.NewString()
		_, err = tempFile.WriteString(randomString)
		require.NoError(t, err, "Failed to write to temporary file")

		// Copy the directory
		destDir := tempDir + "-copy"
		err = copyCallback(tempDir, destDir)
		defer os.RemoveAll(destDir)
		require.NoError(t, err, "Failed to copy directory")

		// Verify that the destination directory exists
		_, err = os.Stat(destDir)
		require.NoError(t, err, "Destination directory stat error")

		// Verify that the destination file exists
		destFile := filepath.Join(destDir, filepath.Base(tempFile.Name()))
		_, err = os.Stat(destFile)
		require.NoError(t, err, "Destination file stat error")

		// Read the content of the destination file
		destContent, err := os.ReadFile(destFile)
		require.NoError(t, err, "Failed to read destination file")

		// Compare the content of the source and destination files
		require.Equal(t, randomString, string(destContent), "Content of source and destination files do not match")

		// Verify that the source directory exists
		_, err = os.Stat(tempDir)
		require.NoError(t, err, "Source directory stat error")

		// Verify that the source file exists
		_, err = os.Stat(tempFile.Name())
		require.NoError(t, err, "Source file stat error")
	}

	t.Run("CopyDirectory", func(t *testing.T) {
		test(t, func(src, dst string) error {
			return CopyDirectory(src, dst, false)
		})
	})

	// t.Run("CopyDirectoryEx", func(t *testing.T) {
	// 	test(t, func(src, dst string) error {
	// 		return CopyDirectoryEx(src, dst, )
	// 	})
	// })

	t.Run("ConcurrentCopyDirectory", func(t *testing.T) {
		test(t, func(src, dst string) error {
			return ConcurrentCopyDirectory(src, dst, 10, false)
		})
	})
}
