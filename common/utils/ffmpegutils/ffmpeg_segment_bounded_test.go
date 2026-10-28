package ffmpegutils

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVideoSlicesFinalFlushDoesNotDropSegments(t *testing.T) {
	binary := os.Getenv("YAK_OMNI_TEST_FFMPEG")
	if binary == "" {
		binary, _ = exec.LookPath("ffmpeg")
	}
	if binary == "" {
		t.Skip("ffmpeg is required")
	}
	previous := ffmpegBinaryPath
	ffmpegBinaryPath = binary
	t.Cleanup(func() { ffmpegBinaryPath = previous })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	input := filepath.Join(dir, "input.mp4")
	output, err := exec.CommandContext(ctx, binary, "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=2", "-t", "70", "-c:v", "libx264", input).CombinedOutput()
	require.NoError(t, err, string(output))
	ch, err := ExtractVideoSliceFromVideo(input, WithContext(ctx), WithSliceDurationSeconds(1), WithSliceReencode(true), WithSliceMaxHeight(64), WithSliceTargetFPS(2), WithSliceOutputDir(filepath.Join(dir, "slices")))
	require.NoError(t, err)
	// Let ffmpeg finish before reading, exercising the final flush beyond the
	// channel's buffer capacity. Every complete segment must still be delivered.
	time.Sleep(300 * time.Millisecond)
	count := 0
	for result := range ch {
		require.NoError(t, result.Error)
		require.Equal(t, count, result.Index)
		count++
	}
	require.Equal(t, 70, count)
}
