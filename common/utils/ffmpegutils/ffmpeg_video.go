package ffmpegutils

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// formatDuration converts a time.Duration to ffmpeg's HH:MM:SS.ms format.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Millisecond)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// FrameExtractionResult holds information about a single extracted frame.
type FrameExtractionResult struct {
	// FilePath is the path to the extracted image file.
	FilePath string
	// Timestamp is the exact time of the frame in the video.
	Timestamp time.Duration
	// Error captures any issue that occurred while processing this specific frame.
	Error error
}

// ExtractImageFramesFromVideo extracts frames from a video and streams the results.
// It returns a channel that provides FrameExtractionResult for each frame created.
func ExtractImageFramesFromVideo(inputFile string, opts ...Option) (<-chan *FrameExtractionResult, error) {
	// 1. Validate input file
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("input file does not exist: %s", inputFile)
	}
	if ffmpegBinaryPath == "" {
		return nil, fmt.Errorf("ffmpeg binary path is not configured")
	}

	// 2. Apply options and perform validation
	o := newDefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	if o.mode == modeUnset && o.customVideoFilter == "" {
		return nil, fmt.Errorf("frame extraction mode not set; use WithSceneThreshold(), WithFramesPerSecond(), or WithCustomVideoFilter()")
	}

	// Ensure output directory exists and is safe
	if o.outputDir == "" {
		tempDir, err := ioutil.TempDir("", "extracted-frames-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary output directory: %w", err)
		}
		o.outputDir = tempDir
	} else {
		// Basic security check: clean the path to prevent traversal
		cleanedPath := filepath.Clean(o.outputDir)
		if err := os.MkdirAll(cleanedPath, 0750); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
		o.outputDir = cleanedPath
	}

	// 3. Construct command
	args := []string{
		"-i", inputFile,
		"-nostdin",
		"-threads", strconv.Itoa(o.threads),
	}
	if o.startTime > 0 {
		args = append(args, "-ss", formatDuration(o.startTime))
	}
	if o.endTime > 0 {
		args = append(args, "-to", formatDuration(o.endTime))
	}

	outputPattern := filepath.Join(o.outputDir, o.outputFramePattern)

	// Apply video filter
	if o.customVideoFilter != "" {
		args = append(args, "-vf", o.customVideoFilter)
		// custom filter might need vsync settings, user should specify if needed
	} else {
		switch o.mode {
		case modeSceneChange:
			// select='eq(n,0)+gt(scene,THRESHOLD)' ensures the very first frame is always included,
			// preventing loss of context for segments that have no other major scene changes.
			args = append(args, "-vf", fmt.Sprintf("select='eq(n,0)+gt(scene,%.2f)'", o.sceneThreshold), "-vsync", "vfr")
		case modeFixedRate:
			args = append(args, "-r", fmt.Sprintf("%f", o.framesPerSecond))
		}
	}

	args = append(args, "-q:v", strconv.Itoa(o.frameQuality), outputPattern)

	// 4. Execute and stream results
	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	resultsChan := make(chan *FrameExtractionResult)

	go func() {
		defer close(resultsChan)
		if o.debug {
			cmd.Stderr = log.NewLogWriter(log.DebugLevel)
			log.Debugf("executing ffmpeg frame extraction: %s", cmd.String())
		} else {
			// Even if not in debug, we need to consume stderr to prevent the pipe from filling up
			cmd.Stderr = ioutil.Discard
		}

		if err := cmd.Run(); err != nil {
			resultsChan <- &FrameExtractionResult{Error: fmt.Errorf("ffmpeg execution failed: %w", err)}
			return
		}

		// After successful execution, list files and create results
		// This is a simplified approach. A better one would parse ffmpeg output.
		files, err := ioutil.ReadDir(o.outputDir)
		if err != nil {
			resultsChan <- &FrameExtractionResult{Error: fmt.Errorf("failed to read output directory: %w", err)}
			return
		}

		for _, file := range files {
			if !file.IsDir() {
				// We don't have timestamp info here, which is a limitation of this approach.
				// A more advanced solution would be needed to parse timestamps.
				resultsChan <- &FrameExtractionResult{
					FilePath:  filepath.Join(o.outputDir, file.Name()),
					Timestamp: -1, // Placeholder
				}
			}
		}
	}()

	return resultsChan, nil
}

// BurnInSubtitles hard-codes subtitles from an SRT file into a video.
func BurnInSubtitles(inputFile string, opts ...Option) error {
	// 1. Validate inputs
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input video file does not exist: %s", inputFile)
	}
	if ffmpegBinaryPath == "" {
		return fmt.Errorf("ffmpeg binary path is not configured")
	}

	o := newDefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	if o.subtitleFile == "" {
		return fmt.Errorf("subtitle file is required; use WithSubtitleFile()")
	}
	if _, err := os.Stat(o.subtitleFile); os.IsNotExist(err) {
		return fmt.Errorf("subtitle file does not exist: %s", o.subtitleFile)
	}
	if o.outputVideoFile == "" {
		return fmt.Errorf("output video file is required; use WithOutputVideoFile()")
	}

	// 2. Construct command.
	// The filter `subtitles=FILENAME` will burn the SRT file onto the video.
	// NOTE: This requires ffmpeg to be compiled with --enable-libass.
	// The paths in the filter need to be escaped for ffmpeg.
	escapedSubtitlePath := filepath.ToSlash(o.subtitleFile)
	vfFilter := fmt.Sprintf("subtitles='%s'", escapedSubtitlePath)

	args := []string{
		"-i", inputFile,
		"-nostdin",
		"-y",
		"-c:v", "libx264", // Re-encode the video to apply the filter
		"-c:a", "copy", // Copy the audio stream without re-encoding
		"-vf", vfFilter,
		o.outputVideoFile,
	}

	// 3. Execute command
	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Debugf("executing subtitle burn-in: %s", cmd.String())
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg execution failed during subtitle burn-in: %w", err)
	}

	return nil
}
