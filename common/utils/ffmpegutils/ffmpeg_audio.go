package ffmpegutils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"io/ioutil"
	"os"
	"os/exec"
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
		tmpFile, err := ioutil.TempFile(consts.GetDefaultYakitBaseTempDir(), "extracted-audio-*.mp3")
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
		"-c:a", "libmp3lame",
		"-q:a", "7",
		o.outputAudioFile,
	)

	// 4. Execute the command
	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Infof("executing audio extraction: %s", cmd.String())
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

// CompressAudio compresses an audio file to a specified format and bitrate.
func CompressAudio(inputFile, outputFile string, opts ...Option) error {
	// 1. Validate input file
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input audio file does not exist: %s", inputFile)
	}
	if ffmpegBinaryPath == "" {
		return fmt.Errorf("ffmpeg binary path is not configured")
	}

	// 2. Apply options and perform validation
	o := newDefaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	if o.audioBitrate == "" {
		return fmt.Errorf("audio bitrate not specified; use WithAudioBitrate()")
	}

	// 3. Construct ffmpeg command arguments safely
	args := []string{
		"-i", inputFile,
		"-nostdin",
		"-threads", strconv.Itoa(o.threads),
		"-y",                 // Overwrite output file
		"-c:a", "libmp3lame", // A common, high-compatibility codec
		"-b:a", o.audioBitrate,
		outputFile,
	}

	// 4. Execute the command
	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Infof("executing audio compression: %s", cmd.String())
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg audio compression failed: %w", err)
	}

	return nil
}
