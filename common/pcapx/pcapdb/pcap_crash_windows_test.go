package pcapdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestCrashCheckpointWindowsExclusiveHandle(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "checkpoint.json")
	expected := crashCheckpoint{Committed: PCAPFileDBMetadata{PacketCount: 17}}
	data, err := json.Marshal(expected)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(marker, data, 0600))
	path, err := windows.UTF16PtrFromString(marker)
	require.NoError(t, err)
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	defer func() {
		if handle != windows.InvalidHandle {
			windows.CloseHandle(handle)
		}
	}()
	_, err = os.ReadFile(marker)
	require.ErrorIs(t, err, windows.ERROR_SHARING_VIOLATION, "exercise a real exclusive Windows handle")
	_, ready, err := readCrashCheckpoint(marker)
	require.NoError(t, err)
	require.False(t, ready, "an unreadable checkpoint cannot authorize killing the child")
	require.NoError(t, windows.CloseHandle(handle))
	handle = windows.InvalidHandle
	actual, ready, err := readCrashCheckpoint(marker)
	require.NoError(t, err)
	require.True(t, ready)
	require.Equal(t, expected, actual)
	require.NoError(t, os.WriteFile(marker, []byte("invalid checkpoint"), 0600))
	_, ready, err = readCrashCheckpoint(marker)
	require.Error(t, err, "malformed published JSON must never be retried as sharing")
	require.False(t, ready)
}
