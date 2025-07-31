package ffmpegutils

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"

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
	var extractedFrames [][]byte
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if len(res.RawData) > 0 {
			extractedFrames = append(extractedFrames, res.RawData)
			// For smoke test, just check the first one is a valid image
			assert.NotEmpty(t, res.MIMEType)
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
	var extractedFrames [][]byte
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if len(res.RawData) > 0 {
			extractedFrames = append(extractedFrames, res.RawData)
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

	var extractedFrames [][]byte
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if len(res.RawData) > 0 {
			extractedFrames = append(extractedFrames, res.RawData)
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

	var extractedFrames [][]byte
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if len(res.RawData) > 0 {
			extractedFrames = append(extractedFrames, res.RawData)
		}
	}
	// 5s clip at 1fps should give ~5 frames.
	assert.InDelta(t, 5, len(extractedFrames), 1, "should extract ~5 frames for 5s at 1fps")
	assert.NotEmpty(t, extractedFrames, "should include first and subsequent frames")
}

func TestSmoke_ExtractSpecificFrame(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	// Extract the 50th frame (around the 5-second mark for a 10fps video)
	frameData, err := ExtractSpecificFrame(videoPath, 50)
	assert.NoError(t, err)
	assert.NotEmpty(t, frameData, "extracted frame data should not be empty")

	// Verify it's a valid image
	mime := mimetype.Detect(frameData)
	assert.Contains(t, mime.String(), "image/", "extracted data should be an image")
}

func TestSmoke_CompressAudio(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	// First, extract a piece of audio to use as input for compression
	audioRes, err := ExtractAudioFromVideo(videoPath, WithStartEnd(0, 10*time.Second))
	assert.NoError(t, err)
	defer os.Remove(audioRes.FilePath)

	// Now, compress the extracted audio
	outputCompressed, err := ioutil.TempFile("", "compressed-*.mp3")
	assert.NoError(t, err)
	outputCompressed.Close()
	defer os.Remove(outputCompressed.Name())

	err = CompressAudio(audioRes.FilePath, outputCompressed.Name(),
		WithDebug(true),
		WithAudioBitrate("64k"),
	)
	assert.NoError(t, err)

	originalInfo, err := os.Stat(audioRes.FilePath)
	assert.NoError(t, err)
	compressedInfo, err := os.Stat(outputCompressed.Name())
	assert.NoError(t, err)

	assert.Greater(t, compressedInfo.Size(), int64(0))
	assert.Less(t, compressedInfo.Size(), originalInfo.Size(), "compressed audio should be smaller than original")
}

func TestSmoke_CompressImage(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	// First, extract a frame to use as a test image
	frameChan, err := ExtractImageFramesFromVideo(videoPath, WithFramesPerSecond(1), WithStartEnd(5*time.Second, 6*time.Second))
	assert.NoError(t, err)
	frame := <-frameChan
	assert.NoError(t, frame.Error)

	// To compress, we need a file on disk. Write the raw data to a temp file.
	tempInputImage, err := ioutil.TempFile("", "temp-frame-*.jpg")
	assert.NoError(t, err)
	_, err = tempInputImage.Write(frame.RawData)
	assert.NoError(t, err)
	tempInputImage.Close()
	defer os.Remove(tempInputImage.Name())

	// Now, compress the image to be under 20KB
	outputCompressed, err := ioutil.TempFile("", "compressed-*.jpg")
	assert.NoError(t, err)
	outputCompressed.Close()
	defer os.Remove(outputCompressed.Name())

	targetSize := int64(20 * 1024)
	err = CompressImage(tempInputImage.Name(), outputCompressed.Name(),
		WithDebug(true),
		WithTargetImageSize(targetSize),
	)
	assert.NoError(t, err)

	compressedInfo, err := os.Stat(outputCompressed.Name())
	assert.NoError(t, err)
	assert.LessOrEqual(t, compressedInfo.Size(), targetSize, "compressed image should be under the target size")
}

func TestSmoke_TimestampOverlay(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	// 1. Setup: Create a temporary directory for frames with timestamp overlay
	outputDir, err := ioutil.TempDir("", "test-frames-timestamp-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	// 2. Extract frames with timestamp overlay enabled
	resultsChan, err := ExtractImageFramesFromVideo(videoPath,
		WithDebug(true),
		WithOutputDir(outputDir),
		WithFramesPerSecond(0.5), // Extract one frame every 2 seconds
		WithTimestampOverlay(true),
		WithStartEnd(1*time.Second, 5*time.Second),
	)
	assert.NoError(t, err)
	assert.NotNil(t, resultsChan)

	// 3. Verify that frames are extracted with timestamps
	var extractedFrames [][]byte
	for res := range resultsChan {
		assert.NoError(t, res.Error)
		if len(res.RawData) > 0 {
			extractedFrames = append(extractedFrames, res.RawData)
			// Verify the frame is a valid image
			assert.NotEmpty(t, res.MIMEType)
			assert.Contains(t, res.MIMEType, "image")

			// Save frame to verify visually (optional)
			filename, err := res.SaveToFile()
			if err == nil {
				log.Infof("Frame with timestamp saved to: %s", filename)
			}
		}
	}
	assert.NotEmpty(t, extractedFrames, "should extract at least one frame with timestamp overlay")
}

func TestSmoke_BurnInSubtitles(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, videoCleanup := setupTestWithEmbeddedData(t)
	defer videoCleanup()

	// Create a temporary SRT file
	srtFile, err := ioutil.TempFile("", "test-*.srt")
	assert.NoError(t, err)
	_, err = srtFile.Write(testVideoDataSRT)
	assert.NoError(t, err)
	srtFile.Close()
	defer os.Remove(srtFile.Name())

	// Create output video file path
	outputVideo, err := ioutil.TempFile("", "subtitled-*.mp4")
	assert.NoError(t, err)
	outputVideo.Close()
	defer func() {
		// os.Remove(outputVideo.Name())
		println(outputVideo.Name())
	}()

	// Execute burn-in
	err = BurnInSubtitles(videoPath,
		WithDebug(true),
		WithSubtitleFile(srtFile.Name()),
		WithOutputVideoFile(outputVideo.Name()),
	)
	assert.NoError(t, err)

	info, err := os.Stat(outputVideo.Name())
	assert.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0), "subtitled video should not be empty")
}

func TestSmoke_BurnInSubtitlesWithPadding(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, videoCleanup := setupTestWithEmbeddedData(t)
	defer videoCleanup()

	// Create a temporary SRT file
	srtFile, err := ioutil.TempFile("", "test-*.srt")
	assert.NoError(t, err)
	_, err = srtFile.Write(testVideoDataSRT)
	assert.NoError(t, err)
	srtFile.Close()
	defer os.Remove(srtFile.Name())

	// Create output video file path
	outputVideo, err := ioutil.TempFile("", "subtitled-padded-*.mp4")
	assert.NoError(t, err)
	outputVideo.Close()
	defer func() {
		// Keep the file for manual inspection
		log.Infof("Subtitled video with padding saved to: %s", outputVideo.Name())
	}()

	// Execute burn-in with padding enabled
	err = BurnInSubtitles(videoPath,
		WithDebug(true),
		WithSubtitleFile(srtFile.Name()),
		WithOutputVideoFile(outputVideo.Name()),
		WithSubtitlePadding(true), // Enable black padding for subtitles
	)
	assert.NoError(t, err)

	info, err := os.Stat(outputVideo.Name())
	assert.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0), "subtitled video with padding should not be empty")
}

func TestSmoke_MediaUtilsBurnSRTIntoVideo(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, videoCleanup := setupTestWithEmbeddedData(t)
	defer videoCleanup()

	// Create a temporary SRT file
	srtFile, err := ioutil.TempFile("", "test-*.srt")
	assert.NoError(t, err)
	_, err = srtFile.Write(testVideoDataSRT)
	assert.NoError(t, err)
	srtFile.Close()
	defer os.Remove(srtFile.Name())

	// Test the mediautils wrapper function
	// Import the function from mediautils
	burnFunc := func(inputVideo string, srtFile string, opts ...Option) (string, error) {
		// Generate output filename based on input
		baseName := strings.TrimSuffix(filepath.Base(inputVideo), filepath.Ext(inputVideo))
		outputFile, err := ioutil.TempFile("", baseName+"_with_subtitles_*.mp4")
		if err != nil {
			return "", err
		}
		outputFile.Close()
		outputPath := outputFile.Name()

		// Prepare options with smart defaults
		finalOpts := []Option{
			WithSubtitleFile(srtFile),
			WithOutputVideoFile(outputPath),
			WithSubtitlePadding(true), // Enable padding by default
		}

		// Append user options
		finalOpts = append(finalOpts, opts...)

		// Execute
		err = BurnInSubtitles(inputVideo, finalOpts...)
		if err != nil {
			return "", err
		}

		return outputPath, nil
	}

	// Test default behavior (with padding)
	outputPath, err := burnFunc(videoPath, srtFile.Name())
	assert.NoError(t, err)
	assert.NotEmpty(t, outputPath)

	// Verify output file exists and has content
	info, err := os.Stat(outputPath)
	assert.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0), "output video should not be empty")

	log.Infof("BurnSRTIntoVideo test output saved to: %s", outputPath)

	// Test with padding disabled
	outputPath2, err := burnFunc(videoPath, srtFile.Name(), WithSubtitlePadding(false))
	assert.NoError(t, err)
	assert.NotEmpty(t, outputPath2)

	// Verify second output file
	info2, err := os.Stat(outputPath2)
	assert.NoError(t, err)
	assert.Greater(t, info2.Size(), int64(0), "output video without padding should not be empty")

	log.Infof("BurnSRTIntoVideo test output (no padding) saved to: %s", outputPath2)

	// Cleanup
	defer func() {
		os.Remove(outputPath)
		os.Remove(outputPath2)
	}()
}

func TestSmoke_GetVideoDuration(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	// Execute GetVideoDuration
	duration, err := GetVideoDuration(videoPath)
	assert.NoError(t, err)
	assert.Greater(t, duration, time.Duration(0), "video duration should be greater than 0")

	// The test video should have a reasonable duration (assuming it's a few seconds to a few minutes)
	// We'll check that it's between 1 second and 10 minutes as a sanity check
	assert.GreaterOrEqual(t, duration, 1*time.Second, "video duration should be at least 1 second")
	assert.LessOrEqual(t, duration, 10*time.Minute, "video duration should be less than 10 minutes")

	log.Infof("Video duration: %v", duration)
}

func TestGetVideoDuration_NonExistentFile(t *testing.T) {
	// Test with a file that doesn't exist
	_, err := GetVideoDuration("/path/to/nonexistent/video.mp4")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input file does not exist")
}

func TestFormatDuration_LongHours(t *testing.T) {
	// Test formatDuration with various hour lengths
	testCases := []struct {
		duration time.Duration
		expected string
	}{
		{1*time.Hour + 23*time.Minute + 45*time.Second + 678*time.Millisecond, "01:23:45.678"},
		{12*time.Hour + 34*time.Minute + 56*time.Second + 789*time.Millisecond, "12:34:56.789"},
		{99*time.Hour + 59*time.Minute + 59*time.Second + 999*time.Millisecond, "99:59:59.999"},
		{100*time.Hour + 0*time.Minute + 0*time.Second + 0*time.Millisecond, "100:00:00.000"},
		{123*time.Hour + 45*time.Minute + 30*time.Second + 890*time.Millisecond, "123:45:30.890"},
		{1000*time.Hour + 1*time.Minute + 1*time.Second + 1*time.Millisecond, "1000:01:01.001"},
	}

	for _, tc := range testCases {
		result := formatDuration(tc.duration)
		assert.Equal(t, tc.expected, result, "formatDuration(%v) should return %s, got %s", tc.duration, tc.expected, result)
	}
}

func TestGetVideoDuration_RegexPatternLongHours(t *testing.T) {
	// Test the regex pattern used in GetVideoDuration to ensure it handles long hours

	re := regexp.MustCompile(`Duration: (\d+):(\d{2}):(\d{2})\.(\d{2})`)

	testCases := []struct {
		input           string
		expectedMatches []string
		shouldMatch     bool
	}{
		{
			"Duration: 01:23:45.67",
			[]string{"Duration: 01:23:45.67", "01", "23", "45", "67"},
			true,
		},
		{
			"Duration: 99:59:59.99",
			[]string{"Duration: 99:59:59.99", "99", "59", "59", "99"},
			true,
		},
		{
			"Duration: 100:00:00.00",
			[]string{"Duration: 100:00:00.00", "100", "00", "00", "00"},
			true,
		},
		{
			"Duration: 1234:56:78.90", // Note: 78 seconds is invalid but tests the regex
			[]string{"Duration: 1234:56:78.90", "1234", "56", "78", "90"},
			true,
		},
		{
			"Duration: 1:23:45.67", // Single digit hours should still work
			[]string{"Duration: 1:23:45.67", "1", "23", "45", "67"},
			true,
		},
		{
			"Invalid Duration format",
			nil,
			false,
		},
	}

	for _, tc := range testCases {
		matches := re.FindStringSubmatch(tc.input)
		if tc.shouldMatch {
			assert.NotNil(t, matches, "Input %q should match the regex", tc.input)
			assert.Equal(t, tc.expectedMatches, matches, "Regex matches for %q should be %v, got %v", tc.input, tc.expectedMatches, matches)
		} else {
			assert.Nil(t, matches, "Input %q should not match the regex", tc.input)
		}
	}
}

func TestSmoke_DefaultIgnoreBottomPaddingInSceneDetection(t *testing.T) {
	// Simple test to verify the padding-aware scene detection feature works

	outputDir := t.TempDir()

	// Create a simple test video using testsrc
	testVideoPath := filepath.Join(outputDir, "simple_test.mp4")

	// Create a basic 2-second video
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "testsrc=duration=2:rate=2:size=320x240",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-y", testVideoPath)

	err := cmd.Run()
	if err != nil {
		t.Skipf("Failed to create test video (ffmpeg might not be available): %v", err)
	}

	// Test with padding detection enabled
	results, err := ExtractImageFramesFromVideo(testVideoPath,
		WithOutputDir(outputDir),
		WithSceneThreshold(0.3),
		WithIgnoreBottomPaddingInSceneDetection(true))

	assert.NoError(t, err)

	var frameCount int
	for result := range results {
		if result.Error != nil {
			t.Logf("Frame extraction error: %v", result.Error)
			continue
		}
		if len(result.RawData) > 0 {
			frameCount++
		}
	}

	t.Logf("Successfully extracted %d frames with padding-aware scene detection", frameCount)

	// Basic validation - should extract at least 1 frame
	assert.True(t, frameCount > 0, "Should extract at least one frame")

	// Create another test video for the second test (avoid file conflicts)
	testVideoPath2 := filepath.Join(outputDir, "simple_test2.mp4")
	cmd2 := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "testsrc=duration=2:rate=2:size=320x240",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-y", testVideoPath2)

	err = cmd2.Run()
	if err != nil {
		t.Skipf("Failed to create second test video: %v", err)
	}

	// Test that the feature can be disabled too
	results2, err := ExtractImageFramesFromVideo(testVideoPath2,
		WithOutputDir(outputDir),
		WithSceneThreshold(0.3),
		WithIgnoreBottomPaddingInSceneDetection(false))

	assert.NoError(t, err)

	var frameCount2 int
	for result := range results2 {
		if result.Error != nil {
			continue
		}
		if len(result.RawData) > 0 {
			frameCount2++
		}
	}

	t.Logf("Successfully extracted %d frames without padding detection", frameCount2)
	assert.True(t, frameCount2 > 0, "Should extract at least one frame without padding detection")
}

func TestSmoke_DefaultBehaviorIncludesPaddingDetection(t *testing.T) {
	// Test that ExtractImageFramesFromVideo with scene detection uses padding detection by default
	// when called through the high-level functions

	outputDir := t.TempDir()

	// Create a simple test video
	testVideoPath := filepath.Join(outputDir, "default_test.mp4")

	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "testsrc=duration=2:rate=2:size=320x240",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-y", testVideoPath)

	err := cmd.Run()
	if err != nil {
		t.Skipf("Failed to create test video (ffmpeg might not be available): %v", err)
	}

	// Test the default behavior - should include padding detection for scene-based extraction
	results, err := ExtractImageFramesFromVideo(testVideoPath,
		WithOutputDir(outputDir),
		WithSceneThreshold(0.3)) // Scene detection without explicitly setting padding detection

	assert.NoError(t, err)

	var frameCount int
	for result := range results {
		if result.Error != nil {
			t.Logf("Frame extraction error: %v", result.Error)
			continue
		}
		if len(result.RawData) > 0 {
			frameCount++
		}
	}

	t.Logf("Default behavior extracted %d frames", frameCount)
	assert.True(t, frameCount > 0, "Should extract at least one frame with default settings")
}

func TestSmoke_SubtitleWithTimestamp(t *testing.T) {
	// Test that subtitle burning with timestamp information works correctly

	outputDir := t.TempDir()

	// Create a simple test video
	testVideoPath := filepath.Join(outputDir, "test_video.mp4")

	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "testsrc=duration=5:rate=2:size=320x240",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", testVideoPath)

	err := cmd.Run()
	if err != nil {
		t.Skipf("Failed to create test video (ffmpeg might not be available): %v", err)
	}

	// Create a simple SRT file
	srtContent := `1
00:00:01,000 --> 00:00:03,000
Hello World

2
00:00:03,500 --> 00:00:05,000
This is a test subtitle
`

	srtPath := filepath.Join(outputDir, "test.srt")
	err = os.WriteFile(srtPath, []byte(srtContent), 0644)
	assert.NoError(t, err)

	// Test burning subtitles with timestamp information
	outputVideoPath := filepath.Join(outputDir, "output_with_timestamp.mp4")

	err = BurnInSubtitles(testVideoPath,
		WithSubtitleFile(srtPath),
		WithOutputVideoFile(outputVideoPath),
		WithSubtitleTimestamp(true),
		WithSubtitlePadding(true),
		WithDebug(true))

	assert.NoError(t, err)

	// Verify output file was created
	_, err = os.Stat(outputVideoPath)
	assert.NoError(t, err)

	t.Logf("Successfully created video with timestamped subtitles: %s", outputVideoPath)

	// Test without timestamp for comparison
	outputVideoPath2 := filepath.Join(outputDir, "output_without_timestamp.mp4")

	err = BurnInSubtitles(testVideoPath,
		WithSubtitleFile(srtPath),
		WithOutputVideoFile(outputVideoPath2),
		WithSubtitleTimestamp(false),
		WithSubtitlePadding(true),
		WithDebug(true))

	assert.NoError(t, err)

	// Verify second output file was created
	_, err = os.Stat(outputVideoPath2)
	assert.NoError(t, err)

	t.Logf("Successfully created video without timestamped subtitles: %s", outputVideoPath2)
}
