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

// AudioExtractionResult holds the results of an audio extraction operation.
type AudioExtractionResult struct {
	// FilePath is the path to the extracted audio file.
	FilePath string
	// Duration is the duration of the extracted audio.
	Duration time.Duration
}

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

// ExtractAudioFromVideo extracts audio from a video file and saves it as a new audio file.
func ExtractAudioFromVideo(inputFile string, opts ...Option) (*AudioExtractionResult, error) {
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

	// If no output file is specified, create a temporary one.
	if o.outputAudioFile == "" {
		tmpFile, err := ioutil.TempFile("", "extracted-audio-*.wav")
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary audio file: %w", err)
		}
		o.outputAudioFile = tmpFile.Name()
		tmpFile.Close() // Close the file, ffmpeg will write to the path
	}

	// 3. Construct ffmpeg command arguments safely
	args := []string{
		"-i", inputFile,
		"-nostdin",
		"-threads", strconv.Itoa(o.threads),
		"-y", // Overwrite output file if it exists
	}
	if o.startTime > 0 {
		args = append(args, "-ss", formatDuration(o.startTime))
	}
	if o.endTime > 0 {
		args = append(args, "-to", formatDuration(o.endTime))
	}
	args = append(args,
		"-ar", strconv.Itoa(o.audioSampleRate),
		"-ac", strconv.Itoa(o.audioChannels),
		"-c:a", "pcm_s16le",
		"-f", "wav",
		o.outputAudioFile,
	)

	// 4. Execute the command
	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Debugf("executing ffmpeg audio extraction: %s", cmd.String())
	}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg execution failed: %w", err)
	}

	// 5. Return result
	result := &AudioExtractionResult{
		FilePath: o.outputAudioFile,
		Duration: o.endTime - o.startTime,
	}
	if result.Duration < 0 {
		result.Duration = 0 // Handle cases where only start or end is provided later
	}

	return result, nil
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

	if o.mode == modeUnset {
		return nil, fmt.Errorf("frame extraction mode not set; use WithSceneThreshold() or WithFramesPerSecond()")
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
