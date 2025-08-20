package ffmpegutils

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/yaklang/yaklang/common/log"
)

const (
	maxCompressIterations = 10
	jpegMinQuality        = 2
	jpegMaxQuality        = 31
)

// CompressImage resizes an image to be under a target size, saving it to outputFile.
// It iteratively adjusts the JPEG quality to meet the size constraint.
func CompressImage(inputFile, outputFile string, opts ...Option) error {
	// 1. Validate input file
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input image file does not exist: %s", inputFile)
	}
	if ffmpegBinaryPath == "" {
		return fmt.Errorf("ffmpeg binary path is not configured")
	}

	// 2. Apply options
	o := newDefaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	if o.targetImageSize <= 0 {
		return fmt.Errorf("target image size must be positive")
	}

	// 3. Iteratively find the best quality setting
	var lastErr error
	currentQuality := (jpegMinQuality + jpegMaxQuality) / 2 // Start in the middle
	lowerBound := jpegMinQuality
	upperBound := jpegMaxQuality

	for i := 0; i < maxCompressIterations; i++ {
		args := []string{
			"-i", inputFile,
			"-nostdin",
			"-y",
			"-q:v", strconv.Itoa(currentQuality),
			outputFile,
		}

		cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
		if o.debug {
			cmd.Stderr = log.NewLogWriter(log.DebugLevel)
			log.Infof("executing image compression: %s", cmd.String())
		}

		lastErr = cmd.Run()
		if lastErr != nil {
			// If ffmpeg fails, we can't continue
			return fmt.Errorf("ffmpeg execution failed during image compression: %w", lastErr)
		}

		info, err := os.Stat(outputFile)
		if err != nil {
			return fmt.Errorf("could not stat output image file: %w", err)
		}

		fileSize := info.Size()
		if o.debug {
			log.Debugf("iteration %d: quality=%d, size=%d bytes, target=%d bytes", i+1, currentQuality, fileSize, o.targetImageSize)
		}

		if fileSize <= o.targetImageSize {
			// Success! The file is small enough.
			// We could try to get a slightly better quality, but this is good enough for now.
			log.Infof("image compressed successfully to %d bytes (under %d) with quality %d", fileSize, o.targetImageSize, currentQuality)
			return nil
		}

		// File is too big, we need to lower the quality (increase q:v)
		// Adjust search bounds for binary search
		lowerBound = currentQuality + 1
		currentQuality = (currentQuality + upperBound) / 2
		if currentQuality <= lowerBound {
			currentQuality = lowerBound
		}
	}

	// 最后尝试使用 scale 滤镜进行压缩
	log.Infof("quality-based compression failed, trying scale filter as fallback")

	args := []string{
		"-i", inputFile,
		"-nostdin",
		"-y",
		"-vf", "scale=1080:1080:force_original_aspect_ratio=decrease",
		"-q:v", strconv.Itoa(jpegMaxQuality), // Use maximum quality setting for scale compression
		outputFile,
	}

	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Infof("executing fallback image compression with scale filter: %s", cmd.String())
	}

	lastErr = cmd.Run()
	if lastErr != nil {
		return fmt.Errorf("fallback ffmpeg execution with scale filter failed: %w", lastErr)
	}

	// Check final file size
	info, err := os.Stat(outputFile)
	if err != nil {
		return fmt.Errorf("could not stat final output image file: %w", err)
	}

	fileSize := info.Size()
	if o.debug {
		log.Debugf("fallback compression result: size=%d bytes, target=%d bytes", fileSize, o.targetImageSize)
	}

	if fileSize <= o.targetImageSize {
		log.Infof("image compressed successfully with scale filter to %d bytes (under %d)", fileSize, o.targetImageSize)
		return nil
	}

	log.Warnf("even with scale filter, image size %d bytes still exceeds target %d bytes", fileSize, o.targetImageSize)
	return fmt.Errorf("failed to compress image under target size %d even with scale filter fallback", o.targetImageSize)
}
