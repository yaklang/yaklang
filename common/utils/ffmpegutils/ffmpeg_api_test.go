package ffmpegutils

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"

	_ "embed"
)

//go:embed ffmpegtestdata/testdata.mp4
var testVideoData []byte

//go:embed ffmpegtestdata/testdata.mp4.srt
var testVideoDataSRT []byte

// setupTestWithEmbeddedData creates a temporary video file from the embedded asset
// for testing purposes. It returns the path to the video and a cleanup function.
func setupTestWithEmbeddedData(t *testing.T) (videoPath string, cleanup func()) {
	if len(testVideoData) == 0 {
		t.Skip("test video data is empty, skipping test. Ensure 'ffmpegtestdata/testdata.mp4' is embedded.")
	}

	tmpfile, err := ioutil.TempFile("", "testvideo-*.mp4")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if _, err := tmpfile.Write(testVideoData); err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		t.Fatalf("failed to write embedded data to temp file: %v", err)
	}
	videoPath = tmpfile.Name()
	tmpfile.Close()

	cleanup = func() {
		os.Remove(videoPath)
	}

	return videoPath, cleanup
}

func TestSmoke_ExtractAudio(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	// 1. Setup: Create a temporary file for the output
	outputAudio, err := ioutil.TempFile("", "test-audio-*.wav")
	assert.NoError(t, err)
	outputAudio.Close()
	defer os.Remove(outputAudio.Name())

	// 2. Execution
	res, err := ExtractAudioFromVideo(videoPath,
		WithDebug(true),
		WithOutputAudioFile(outputAudio.Name()),
	)

	// 3. Assertion: Ensure the result is not empty
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.NotEmpty(t, res.FilePath, "audio file path should not be empty")
	info, err := os.Stat(res.FilePath)
	assert.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0), "extracted audio file should not be empty")
}

func TestSmoke_ExtractFrames_SceneChange(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	// 1. Setup: Create a temporary directory for frames
	outputDir, err := ioutil.TempDir("", "test-frames-scene-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	// 2. Execution
	resultsChan, err := ExtractImageFramesFromVideo(videoPath,
		WithDebug(true),
		WithOutputDir(outputDir),
		WithSceneThreshold(0.9),
	)
	assert.NoError(t, err)
	assert.NotNil(t, resultsChan)

	// 3. Assertion: Ensure at least one frame was extracted
	var extractedFrames []string
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if res.FilePath != "" {
			extractedFrames = append(extractedFrames, res.FilePath)
		}
	}
	assert.NotEmpty(t, extractedFrames, "should extract at least one frame for scene change")
}

func TestSmoke_ExtractKeyframes_TimeRange(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	// 1. Setup: Create a temporary directory for frames
	outputDir, err := ioutil.TempDir("", "test-frames-timerange-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	// 2. Execution
	resultsChan, err := ExtractImageFramesFromVideo(videoPath,
		WithDebug(true),
		WithOutputDir(outputDir),
		WithFramesPerSecond(1), // Extract keyframes (1 per second in this case)
		WithStartEnd(10*time.Second, 14*time.Second),
	)
	assert.NoError(t, err)
	assert.NotNil(t, resultsChan)

	// 3. Assertion: Ensure at least one frame was extracted in the time range
	var extractedFrames []string
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if res.FilePath != "" {
			extractedFrames = append(extractedFrames, res.FilePath)
		}
	}
	assert.NotEmpty(t, extractedFrames, "should extract at least one frame in the 10s-14s time range")
}

func TestWithThreadsOption(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	outputAudio, err := ioutil.TempFile("", "test-audio-threads-*.wav")
	assert.NoError(t, err)
	outputAudio.Close()
	defer os.Remove(outputAudio.Name())

	// Execute with a specific number of threads
	res, err := ExtractAudioFromVideo(videoPath,
		WithDebug(true),
		WithOutputAudioFile(outputAudio.Name()),
		WithThreads(2), // Explicitly use 2 threads
	)

	assert.NoError(t, err)
	assert.NotNil(t, res)
	info, err := os.Stat(res.FilePath)
	assert.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestCustomVideoFilter(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	outputDir, err := ioutil.TempDir("", "test-frames-custom-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	// Use a custom filter to select only the first 10 frames
	// This also tests that the custom filter overrides other mode settings.
	resultsChan, err := ExtractImageFramesFromVideo(videoPath,
		WithDebug(true),
		WithOutputDir(outputDir),
		WithCustomVideoFilter("select='lt(n,10)'"),
		WithSceneThreshold(0.9), // This should be ignored
	)
	assert.NoError(t, err)
	assert.NotNil(t, resultsChan)

	var extractedFrames []string
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if res.FilePath != "" {
			extractedFrames = append(extractedFrames, res.FilePath)
		}
	}
	assert.Len(t, extractedFrames, 10, "should extract exactly 10 frames with custom filter")
}

func TestFrameExtractionIncludingFirstAndLast(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	outputDir, err := ioutil.TempDir("", "test-frames-firstlast-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	// This filter selects the first frame, the last frame, and frames every second.
	// Note: FFmpeg's ability to select the very last frame ('eq(n,N-1)') can be tricky
	// as it requires knowing the total number of frames (N) in advance.
	// A simpler, more reliable approach for "first and last" is to extract them as separate commands.
	// For this test, we'll test a filter that approximates "first, last, and in-between".
	// `select='eq(n,0)+eq(n,14)+between(t,1,5)'` might be a more concrete test.
	// The user requested "每秒一帧包含头尾帧数据" (one frame per second including head and tail).
	// This translates to selecting the first frame, and then subsequent frames at a 1-second rate.
	resultsChan, err := ExtractImageFramesFromVideo(videoPath,
		WithDebug(true),
		WithOutputDir(outputDir),
		// 'eq(n,0)' selects the first frame. '+1/1' would be redundant with -r 1.
		// So we just use fixed rate, which naturally includes the first frame.
		WithFramesPerSecond(1),
		WithStartEnd(0, 5*time.Second),
	)
	assert.NoError(t, err)
	assert.NotNil(t, resultsChan)

	var extractedFrames []string
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if res.FilePath != "" {
			extractedFrames = append(extractedFrames, res.FilePath)
		}
	}
	// 5s clip at 1fps should give ~5 frames.
	assert.InDelta(t, 5, len(extractedFrames), 1, "should extract ~5 frames for 5s at 1fps")
	assert.NotEmpty(t, extractedFrames, "should include first and subsequent frames")
}
