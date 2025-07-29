package ffmpegutils

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bytes"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
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

// ExtractImageFramesFromVideo extracts frames from a video and streams the results.
// It returns a channel that provides FfmpegStreamResult for each frame created.
func ExtractImageFramesFromVideo(inputFile string, opts ...Option) (<-chan *FfmpegStreamResult, error) {
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
	if o.fontFile != "" {
		if _, err := os.Stat(o.fontFile); os.IsNotExist(err) {
			return nil, fmt.Errorf("font file does not exist: %s", o.fontFile)
		}
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
		var vfParts []string
		// Frame selection part
		switch o.mode {
		case modeSceneChange:
			vfParts = append(vfParts, fmt.Sprintf("select='eq(n,0)+gt(scene,%.2f)'", o.sceneThreshold))
		case modeFixedRate:
			// -r option is used instead of a select filter for fixed rate
		}

		// Text drawing part
		if o.fontFile != "" {
			escapedFontPath := filepath.ToSlash(o.fontFile)
			drawtext := fmt.Sprintf("drawtext=fontfile='%s':text='timestamp: %%{pts\\:hms}':fontcolor=white:fontsize=24:box=1:boxcolor=black@0.5:x=(w-tw)/2:y=h-th-10", escapedFontPath)
			vfParts = append(vfParts, drawtext)
		}

		if len(vfParts) > 0 {
			args = append(args, "-vf", strings.Join(vfParts, ","))
		}

		// Add other necessary flags based on mode
		switch o.mode {
		case modeSceneChange:
			args = append(args, "-vsync", "vfr")
		case modeFixedRate:
			args = append(args, "-r", fmt.Sprintf("%f", o.framesPerSecond))
		}
	}

	args = append(args, "-q:v", strconv.Itoa(o.frameQuality), outputPattern)

	// 4. Execute and stream results
	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	resultsChan := make(chan *FfmpegStreamResult)

	go func() {
		defer close(resultsChan)
		cmdCtx, cancel := context.WithCancel(o.ctx)
		defer cancel()

		if o.debug {
			cmd.Stderr = log.NewLogWriter(log.DebugLevel)
			log.Infof("executing ffmpeg frame extraction: %s", cmd.String())
		} else {
			// Even if not in debug, we need to consume stderr to prevent the pipe from filling up
			cmd.Stderr = ioutil.Discard
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			processedFiles := make(map[string]bool)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-cmdCtx.Done():
					// Final check after command finishes
					files, _ := ioutil.ReadDir(o.outputDir)
					for _, file := range files {
						if !file.IsDir() && !processedFiles[file.Name()] {
							sendFrame(file.Name(), o.outputDir, resultsChan)
						}
					}
					return
				case <-ticker.C:
					files, err := ioutil.ReadDir(o.outputDir)
					if err != nil {
						continue // Ignore transient errors
					}
					for _, file := range files {
						if !file.IsDir() && !processedFiles[file.Name()] {
							processedFiles[file.Name()] = true
							sendFrame(file.Name(), o.outputDir, resultsChan)
						}
					}
				}
			}
		}()

		err := cmd.Run()
		cancel()  // Signal the poller to finish
		wg.Wait() // Wait for the poller to do its final read

		if err != nil {
			resultsChan <- &FfmpegStreamResult{Error: fmt.Errorf("ffmpeg execution failed: %w", err)}
			return
		}
	}()

	return resultsChan, nil
}

func sendFrame(filename, dir string, ch chan<- *FfmpegStreamResult) {
	filePath := filepath.Join(dir, filename)
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		ch <- &FfmpegStreamResult{Error: fmt.Errorf("failed to read frame file %s: %w", filename, err)}
		return
	}

	mimeObj := mimetype.Detect(data)
	ch <- &FfmpegStreamResult{
		RawData:     data,
		MIMEType:    mimeObj.String(),
		MIMETypeObj: mimeObj,
	}
	os.Remove(filePath) // Clean up immediately
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
		log.Infof("executing subtitle burn-in: %s", cmd.String())
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg execution failed during subtitle burn-in: %w", err)
	}

	return nil
}

// StartScreenRecording starts a non-blocking ffmpeg process for screen recording.
// It returns the command instance, allowing the caller to manage its lifecycle (e.g., wait or kill).
func StartScreenRecording(outputFile string, opts ...Option) (*exec.Cmd, error) {
	if ffmpegBinaryPath == "" {
		return nil, fmt.Errorf("ffmpeg binary path is not configured")
	}

	o := newDefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	if o.recordFormat == "" {
		return nil, fmt.Errorf("screen recording format is required; use WithScreenRecordFormat()")
	}
	if o.recordInput == "" {
		return nil, fmt.Errorf("screen recording input is required; use WithScreenRecordInput()")
	}

	args := []string{
		"-f", o.recordFormat,
		"-r", strconv.Itoa(o.recordFramerate),
		"-i", o.recordInput,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-an",                                    // No audio
		"-movflags", "+frag_keyframe+empty_moov", // Make the mp4 streamable and fix moov atom not found error
	}
	if o.captureCursor {
		// This option is specific to certain formats like avfoundation
		if o.recordFormat == "avfoundation" {
			args = append(args, "-capture_cursor", "1")
		}
	}
	args = append(args, outputFile)

	if utils.FileExists(outputFile) {
		os.RemoveAll(outputFile)
	}

	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stdout = log.NewLogWriter(log.InfoLevel)
		cmd.Stderr = log.NewLogWriter(log.InfoLevel)
		log.Infof("starting ffmpeg screen recording: %s", cmd.String())
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg screen recording: %w", err)
	}

	return cmd, nil
}

// ExtractSpecificFrame extracts a single frame at a specific frame number from a video.
func ExtractSpecificFrame(inputFile string, frameNum int) ([]byte, error) {
	if ffmpegBinaryPath == "" {
		return nil, fmt.Errorf("ffmpeg binary path is not configured")
	}

	if frameNum < 0 {
		return nil, fmt.Errorf("frame number must be non-negative")
	}

	// Using a pipe to get the output directly into a buffer
	var out bytes.Buffer
	var stderr bytes.Buffer

	// -vf select filter is 0-indexed, so we use the number directly
	// scale=-1:600 resizes the height to 600px, keeping aspect ratio
	args := []string{
		"-i", inputFile,
		"-vf", fmt.Sprintf("select=gte(n\\,%d),scale=-1:600", frameNum),
		"-frames:v", "1",
		"-f", "image2",
		"-codec:v", "mjpeg",
		"pipe:1", // Output to stdout
	}

	cmd := exec.CommandContext(context.Background(), ffmpegBinaryPath, args...)
	log.Infof("executing ffmpeg to extract specific frame: %s", cmd.String())
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log.Warnf("ffmpeg exec failed, reason(stdout): \n%s", out.String())
		log.Warnf("ffmpeg exec failed, reason(stderr): \n%s", stderr.String())
		return nil, fmt.Errorf("ffmpeg execution failed when extracting specific frame: %w", err)
	}

	if out.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced no output; the video may be too short or the frame number too high")
	}

	return out.Bytes(), nil
}
