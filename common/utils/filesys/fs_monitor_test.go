package filesys_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// TestHandlerFileMonitor_FileChanges tests that the monitor detects file changes
func TestHandlerFileMonitor_FileChanges(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "yak_monitor_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Channel to collect events
	eventsChan := make(chan *filesys.EventSet, 10)
	eventsHandler := func(eventSet *filesys.EventSet) {
		eventsChan <- eventSet
	}

	// Manually create a monitor (bypassing the handler for this test)
	monitor, err := filesys.WatchPath(ctx, tmpDir, eventsHandler)
	require.NoError(t, err)
	defer func() {
		monitor.CancelFunc()
	}()

	// Wait a bit for the monitor to initialize
	time.Sleep(2 * time.Second)

	// Create a new file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Wait for the event
	select {
	case events := <-eventsChan:
		assert.NotNil(t, events)
		found := false
		for _, event := range events.CreateEvents {
			if event.Path == testFile {
				found = true
				assert.Equal(t, filesys.FsMonitorCreate, event.Op)
				assert.False(t, event.IsDir)
				break
			}
		}
		assert.True(t, found, "Should detect file creation")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for create event")
	}

	// Delete the file
	err = os.Remove(testFile)
	require.NoError(t, err)

	// Wait for the delete event
	select {
	case events := <-eventsChan:
		assert.NotNil(t, events)
		found := false
		for _, event := range events.DeleteEvents {
			if event.Path == testFile {
				found = true
				assert.Equal(t, filesys.FsMonitorDelete, event.Op)
				break
			}
		}
		assert.True(t, found, "Should detect file deletion")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for delete event")
	}
}

// TestHandlerFileMonitor_DirectoryChanges tests that the monitor detects directory changes
func TestHandlerFileMonitor_DirectoryChanges(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "yak_monitor_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Channel to collect events
	eventsChan := make(chan *filesys.EventSet, 10)
	eventsHandler := func(eventSet *filesys.EventSet) {
		eventsChan <- eventSet
	}

	// Manually create a monitor
	monitor, err := filesys.WatchPath(ctx, tmpDir, eventsHandler)
	require.NoError(t, err)
	defer func() {
		monitor.CancelFunc()
	}()

	// Create a new directory
	testDir := filepath.Join(tmpDir, "testdir")
	err = os.Mkdir(testDir, 0755)
	require.NoError(t, err)

	// Wait for the event
	select {
	case events := <-eventsChan:
		assert.NotNil(t, events)
		found := false
		for _, event := range events.CreateEvents {
			if event.Path == testDir {
				found = true
				assert.Equal(t, filesys.FsMonitorCreate, event.Op)
				assert.True(t, event.IsDir)
				break
			}
		}
		assert.True(t, found, "Should detect directory creation")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for create event")
	}
}

// TestHandlerFileMonitor_NestedFiles tests monitoring nested directory structures
func TestHandlerFileMonitor_NestedFiles(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "yak_monitor_test_*")
	require.NoError(t, err)

	// Create nested directory structure
	subDir := filepath.Join(tmpDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// context
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Channel to collect events
	eventsChan := make(chan *filesys.EventSet, 10)
	eventsHandler := func(eventSet *filesys.EventSet) {
		eventsChan <- eventSet
	}

	// Create a monitor
	monitor, err := filesys.WatchPath(ctx, tmpDir, eventsHandler)
	require.NoError(t, err)
	defer func() {
		monitor.CancelFunc()
	}()

	// Create file in subdirectory
	nestedFile := filepath.Join(subDir, "nested.txt")
	err = os.WriteFile(nestedFile, []byte("nested content"), 0644)
	require.NoError(t, err)

	// Wait for file creation event
	select {
	case events := <-eventsChan:
		assert.NotNil(t, events)
		found := false
		for _, event := range events.CreateEvents {
			if event.Path == nestedFile {
				found = true
				assert.False(t, event.IsDir)
				break
			}
		}
		assert.True(t, found, "Should detect nested file creation")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for nested file creation event")
	}
}
