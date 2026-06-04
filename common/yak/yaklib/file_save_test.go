package yaklib

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveFile_OverwritesExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.txt")

	require.NoError(t, _saveFile(path, "123"))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "123", string(raw))

	require.NoError(t, _saveFile(path, "a"))
	raw, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "a", string(raw))
}
