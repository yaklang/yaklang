package aireact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestAttachedFileMonitorWritesTimelineOnContentChange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	react, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true),
	)
	require.NoError(t, err)
	defer react.CloseAttachedFileMonitors()

	path := filepath.Join(t.TempDir(), "pinned.txt")
	require.NoError(t, os.WriteFile(path, []byte("VERSION_ONE\n"), 0o600))

	react.ensureAttachedFileMonitors([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeFile, aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT, "VERSION_ONE\n"),
	}, filesys.WithPollInterval(20*time.Millisecond))
	react.attachedFileMonitorsMu.Lock()
	require.Empty(t, react.attachedFileMonitors, "literal file content has no filesystem path to monitor")
	react.attachedFileMonitorsMu.Unlock()

	resources := []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeFile, aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, path),
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeFile, aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, path),
	}
	react.ensureAttachedFileMonitors(resources, filesys.WithPollInterval(20*time.Millisecond))

	react.attachedFileMonitorsMu.Lock()
	require.Len(t, react.attachedFileMonitors, 1, "the same pinned path must have one session monitor")
	react.attachedFileMonitorsMu.Unlock()

	time.Sleep(100 * time.Millisecond)
	require.NotContains(t, react.DumpTimeline(), "attached_file_changed")

	tempPath := path + ".next"
	require.NoError(t, os.WriteFile(tempPath, []byte("VERSION_TWO\n"), 0o600))
	require.NoError(t, os.Rename(tempPath, path))

	require.Eventually(t, func() bool {
		timeline := react.DumpTimeline()
		return strings.Contains(timeline, "attached_file_changed") &&
			strings.Contains(timeline, path) &&
			strings.Contains(timeline, "changed on disk")
	}, 5*time.Second, 20*time.Millisecond)

	time.Sleep(150 * time.Millisecond)
	timeline := react.DumpTimeline()
	require.Equal(t, 1, strings.Count(timeline, "[text/attached_file_changed]"), "one content version must produce one timeline event")

	touched := time.Now().Add(time.Second)
	require.NoError(t, os.Chtimes(path, touched, touched))
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, strings.Count(react.DumpTimeline(), "[text/attached_file_changed]"), "metadata-only touches must not report a content change")

	require.NoError(t, os.Remove(path))
	require.Eventually(t, func() bool {
		return strings.Contains(react.DumpTimeline(), "was removed after it was attached")
	}, 5*time.Second, 20*time.Millisecond)

	require.NoError(t, os.WriteFile(path, []byte("VERSION_THREE\n"), 0o600))
	require.Eventually(t, func() bool {
		return strings.Contains(react.DumpTimeline(), "reappeared after it was attached")
	}, 5*time.Second, 20*time.Millisecond)

	require.Equal(t, 3, strings.Count(react.DumpTimeline(), "[text/attached_file_changed]"))
}
