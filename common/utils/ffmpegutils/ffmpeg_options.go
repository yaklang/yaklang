package ffmpegutils

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// --- General Options ---

// Option defines the functional option type.
type Option func(*options)

// options holds all the configurable parameters for an ffmpeg operation.
type options struct {
	ctx   context.Context
	debug bool

	// General execution options
	threads int // Number of threads to use

	// Audio-specific options
	outputAudioFile string
	audioSampleRate int
	audioChannels   int

	// Frame-specific options
	outputDir          string
	outputFramePattern string
	startTime          time.Duration
	endTime            time.Duration
	mode               frameExtractionMode
	customVideoFilter  string // Custom video filter to override default behavior
	sceneThreshold     float64
	framesPerSecond    float64
	frameQuality       int
}

// frameExtractionMode defines the method for frame extraction.
type frameExtractionMode int

const (
	modeUnset frameExtractionMode = iota
	// modeSceneChange extracts frames based on scene change detection.
	modeSceneChange
	// modeFixedRate extracts frames at a fixed rate (fps).
	modeFixedRate
)

func newDefaultOptions() *options {
	return &options{
		ctx:                context.Background(),
		debug:              false,
		threads:            0, // 0 lets ffmpeg decide, which is often optimal
		audioSampleRate:    16000,
		audioChannels:      1,
		outputFramePattern: "frame_%04d.jpg",
		frameQuality:       2,
		sceneThreshold:     0.4, // A sensible default
		framesPerSecond:    1,
	}
}

// WithContext sets the context for the command.
func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

// WithDebug enables verbose logging for the ffmpeg command.
func WithDebug(debug bool) Option {
	return func(o *options) {
		o.debug = debug
	}
}

// WithThreads sets the number of threads for ffmpeg to use.
// A value of 0 (the default) allows ffmpeg to choose the best value.
func WithThreads(n int) Option {
	return func(o *options) {
		if n >= 0 {
			o.threads = n
		}
	}
}

// --- Audio Options ---

// WithOutputAudioFile specifies the output file path for the extracted audio.
// If not set, a temporary file will be created.
func WithOutputAudioFile(filename string) Option {
	return func(o *options) {
		o.outputAudioFile = filename
	}
}

// WithSampleRate sets the audio sample rate. Defaults to 16000.
func WithSampleRate(rate int) Option {
	return func(o *options) {
		if rate > 0 {
			o.audioSampleRate = rate
		}
	}
}

// WithChannels sets the number of audio channels. Defaults to 1 (mono).
func WithChannels(channels int) Option {
	return func(o *options) {
		if channels > 0 {
			o.audioChannels = channels
		}
	}
}

// --- Frame Options ---

// WithOutputDir specifies the directory to save extracted frames.
// If not set, a temporary directory will be created.
func WithOutputDir(dir string) Option {
	return func(o *options) {
		o.outputDir = dir
	}
}

// WithOutputFramePattern sets the output filename pattern for frames (e.g., "frame-%03d.jpg").
func WithOutputFramePattern(pattern string) Option {
	return func(o *options) {
		o.outputFramePattern = pattern
	}
}

// WithStartEnd specifies the time range for extraction.
func WithStartEnd(start, end time.Duration) Option {
	return func(o *options) {
		o.startTime = start
		o.endTime = end
	}
}

// WithSceneThreshold sets the scene change detection sensitivity (0.0 to 1.0).
// Using this option sets the extraction mode to scene change detection.
// It is mutually exclusive with WithFramesPerSecond.
func WithSceneThreshold(threshold float64) Option {
	return func(o *options) {
		if o.mode != modeUnset && o.mode != modeSceneChange {
			log.Warnf("ffmpeg option conflict: WithSceneThreshold is overwriting a previously set frame extraction mode.")
		}
		o.mode = modeSceneChange
		if threshold >= 0.0 && threshold <= 1.0 {
			o.sceneThreshold = threshold
		}
	}
}

// WithFramesPerSecond sets a fixed rate for frame extraction.
// Using this option sets the extraction mode to fixed rate.
// It is mutually exclusive with WithSceneThreshold.
func WithFramesPerSecond(fps float64) Option {
	return func(o *options) {
		if o.mode != modeUnset && o.mode != modeFixedRate {
			log.Warnf("ffmpeg option conflict: WithFramesPerSecond is overwriting a previously set frame extraction mode.")
		}
		o.mode = modeFixedRate
		if fps > 0 {
			o.framesPerSecond = fps
		}
	}
}

// WithFrameQuality sets the quality for extracted frames (1-31, where 2 is high quality).
func WithFrameQuality(quality int) Option {
	return func(o *options) {
		if quality >= 1 && quality <= 31 {
			o.frameQuality = quality
		}
	}
}

// WithCustomVideoFilter provides a way to specify a custom -vf filter string.
// This will OVERRIDE any filter settings from WithSceneThreshold or WithFramesPerSecond.
// Use with caution.
func WithCustomVideoFilter(filter string) Option {
	return func(o *options) {
		o.customVideoFilter = filter
	}
}
