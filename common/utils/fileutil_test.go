package utils_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
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
			return utils.CopyDirectory(src, dst, false)
		})
	})

	t.Run("CopyDirectoryEx", func(t *testing.T) {
		test(t, func(src, dst string) error {
			return utils.CopyDirectoryEx(src, dst, false, filesys.NewLocalFs())
		})
	})

	t.Run("ConcurrentCopyDirectory", func(t *testing.T) {
		test(t, func(src, dst string) error {
			return utils.ConcurrentCopyDirectory(src, dst, 10, false)
		})
	})
}

func TestIsSubPath(t *testing.T) {
	testCases := []struct {
		name     string
		sub      string
		parent   string
		expected bool
	}{
		{"SubPath is a direct child",
			"/a/b/c", "/a/b", true},
		{"SubPath is the same as parent",
			"/a/b/c", "/a/b/c", false},
		{"SubPath is not a child",
			"/a/b/c", "/a/b/c/d", false},
		{"Parent path with ..",
			"/a/b/c", "/a/b/..", true},
		{"Parent path with .. and same sub",
			"/a/b/c", "/a/b/../b", true},
		{"Parent path with .. and same sub with child",
			"/a/b/c", "/a/b/../b/c", false},
		{"Parent path with .. and different sub",
			"/a/b/c", "/a/b/../d", false},
		{"Parent path with .. and different sub with child",
			"/a/b/c", "/a/b/../d/e", false},
		{"Relative path sub is a direct child",
			"./a/b/c", "./a/b", true},
		{"Relative path sub is the same as parent",
			"./a/b/c", "./a/b/c", false},
		{"Relative path sub is not a child",
			"./a/b/c", "./a/b/c/d", false},
		{"Relative path parent with ..",
			"./a/b/c", "./a/b/..", true},
		{"Relative path parent with .. and same sub",
			"./a/b/c", "./a/b/../b", true},
		{"Relative path parent with .. and same sub with child",
			"./a/b/c", "./a/b/../b/c", false},
		{"Relative path parent with .. and different sub",
			"./a/b/c", "./a/b/../d", false},
		{"Relative path parent with .. and different sub with child",
			"./a/b/c", "./a/b/../d/e", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing sub: %s, parent: %s, expected: %v", tc.sub, tc.parent, tc.expected)
			result := utils.IsSubPath(tc.sub, tc.parent)
			assert.Equal(t, tc.expected, result)
		})
	}
}
