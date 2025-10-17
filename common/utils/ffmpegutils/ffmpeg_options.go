package ffmpegutils

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
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
	audioBitrate    string // e.g., "128k"

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

	// Image-specific options
	targetImageSize int64 // Target size in bytes

	// Subtitle-specific options
	subtitleFile                        string
	outputVideoFile                     string
	fontFile                            string // Path to a font file for drawtext filter
	showTimestamp                       bool   // Whether to show timestamp overlay on frames
	subtitlePadding                     bool   // Whether to add black padding for subtitles instead of overlaying on content
	ignoreBottomPaddingInSceneDetection bool   // Whether to ignore bottom padding area when doing scene detection
	showSubtitleTimestamp               bool   // Whether to show timestamp information within subtitle text

	// Screen recording options
	recordFormat    string // e.g., "avfoundation" on macOS, "gdigrab" on Windows
	recordInput     string // e.g., "1" for screen, "desktop", "video=Integrated Camera"
	recordFramerate int
	captureCursor   bool
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
		targetImageSize:    200 * 1024, // Default 200KB
		recordFramerate:    10,
		captureCursor:      true,
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

// WithAudioBitrate sets the target bitrate for audio compression (e.g., "128k").
func WithAudioBitrate(bitrate string) Option {
	return func(o *options) {
		o.audioBitrate = bitrate
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

// WithStartEndSeconds specifies the time range for extraction in seconds.
func WithStartEndSeconds(start, end float64) Option {
	return func(o *options) {
		o.startTime = time.Duration(start * float64(time.Second))
		o.endTime = time.Duration(end * float64(time.Second))
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

// --- Image Options ---

// WithTargetImageSize sets the target file size in bytes for image compression.
func WithTargetImageSize(sizeInBytes int64) Option {
	return func(o *options) {
		if sizeInBytes > 0 {
			o.targetImageSize = sizeInBytes
		}
	}
}

// --- Subtitle/Video Options ---

// WithSubtitleFile specifies the path to the SRT subtitle file to burn in.
func WithSubtitleFile(filepath string) Option {
	return func(o *options) {
		o.subtitleFile = filepath
	}
}

// WithOutputVideoFile specifies the path for the final output video.
func WithOutputVideoFile(filepath string) Option {
	return func(o *options) {
		o.outputVideoFile = filepath
	}
}

// WithFontFile specifies the path to a TTF font file for text overlays.
func WithFontFile(filepath string) Option {
	return func(o *options) {
		o.fontFile = filepath
	}
}

// WithTimestampOverlay enables or disables timestamp overlay on extracted frames.
// When enabled, a black bar will be added at the bottom of each frame displaying the timestamp.
func WithTimestampOverlay(show bool) Option {
	return func(o *options) {
		o.showTimestamp = show
	}
}

// WithSubtitlePadding enables or disables adding black padding for subtitles.
// When enabled, black padding will be added to the bottom of the video where subtitles are displayed,
// ensuring subtitles don't cover the original video content.
func WithSubtitlePadding(enable bool) Option {
	return func(o *options) {
		o.subtitlePadding = enable
	}
}

// WithIgnoreBottomPaddingInSceneDetection controls whether scene detection should ignore the bottom padding area.
// When enabled, scene detection will only analyze the original video content, ignoring changes in timestamp/subtitle areas.
// This is particularly useful when extracting frames with timestamps or subtitles to avoid false scene changes.
func WithIgnoreBottomPaddingInSceneDetection(enable bool) Option {
	return func(o *options) {
		o.ignoreBottomPaddingInSceneDetection = enable
	}
}

// WithSubtitleTimestamp sets whether to show timestamp information within subtitle text.
// When enabled, each subtitle line will include timing information like "(start: 00:01:23 ---> end: 00:01:26)".
func WithSubtitleTimestamp(enable bool) Option {
	return func(o *options) {
		o.showSubtitleTimestamp = enable
	}
}

// --- Screen Recording Options ---

// WithScreenRecordFormat sets the input format for screen recording.
// Common values: "avfoundation" (macOS), "gdigrab" (Windows), "x11grab" (Linux).
func WithScreenRecordFormat(format string) Option {
	return func(o *options) {
		o.recordFormat = format
	}
}

// WithScreenRecordInput specifies the input source for recording.
// e.g., "1" (screen), "desktop", "video=Integrated Camera:audio=Built-in Microphone".
func WithScreenRecordInput(input string) Option {
	return func(o *options) {
		o.recordInput = input
	}
}

// WithScreenRecordFramerate sets the capture framerate.
func WithScreenRecordFramerate(rate int) Option {
	return func(o *options) {
		if rate > 0 {
			o.recordFramerate = rate
		}
	}
}

// WithScreenRecordCaptureCursor enables or disables capturing the mouse cursor.
func WithScreenRecordCaptureCursor(capture bool) Option {
	return func(o *options) {
		o.captureCursor = capture
	}
}

// --- Screen Capture Options ---

// ScreenCaptureMode 定义屏幕截图模式
type ScreenCaptureMode int

const (
	CaptureSingle   ScreenCaptureMode = iota // 捕获单个屏幕
	CaptureAll                               // 捕获所有屏幕并拼接
	CaptureMultiple                          // 捕获多个屏幕作为单独文件
)

// ScreenCaptureQuality 定义屏幕截图质量级别
type ScreenCaptureQuality int

const (
	QualityLow    ScreenCaptureQuality = iota // 低质量，快速截图
	QualityNormal                             // 正常质量
	QualityHigh                               // 高质量，无损截图
)

// WithScreenCaptureMode 设置屏幕截图模式
func WithScreenCaptureMode(mode ScreenCaptureMode) Option {
	return func(o *options) {
		// 为了向后兼容，这里暂时不在 options 结构体中添加新字段
		// 实际实现中会根据需要调整
	}
}

// WithScreenCaptureDebug 启用屏幕截图调试信息
func WithScreenCaptureDebug(enable bool) Option {
	return func(o *options) {
		o.debug = enable
	}
}

// WithScreenCaptureQuality 设置屏幕截图质量
func WithScreenCaptureQuality(quality ScreenCaptureQuality) Option {
	return func(o *options) {
		// 根据质量级别设置对应的frameQuality值
		switch quality {
		case QualityLow:
			o.frameQuality = 20 // 低质量，快速截图
		case QualityNormal:
			o.frameQuality = 10 // 正常质量
		case QualityHigh:
			o.frameQuality = 1 // 高质量，无损截图
		default:
			o.frameQuality = 1 // 默认高质量
		}
	}
}

// --- Result Types ---

// FfmpegStreamResult holds the result of a single data unit from a stream,
// typically an image frame.
type FfmpegStreamResult struct {
	// MIMEType is the detected MIME type of the raw data.
	MIMEType    string
	MIMETypeObj *mimetype.MIME
	// RawData is the raw byte content of the result.
	RawData []byte
	// Timestamp is the exact time of the frame in the video.
	Timestamp time.Duration
	// Error captures any issue that occurred while processing this specific result.
	Error error
}

var frameCounter = new(int64)

func getFrameCounter() int64 {
	// Increment the frame counter atomically
	return atomic.AddInt64(frameCounter, 1)
}

func (f *FfmpegStreamResult) SaveToFile() (string, error) {
	if f.RawData == nil || len(f.RawData) == 0 {
		return "", utils.Errorf("no data to save for frame at %s", f.Timestamp)
	}

	filefp, err := consts.TempFile("ffmpeg_frame_" + fmt.Sprint(getFrameCounter()) + "_*" + f.MIMETypeObj.Extension())
	if err != nil {
		return "", utils.Errorf("failed to save frame data to temporary file: %w", err)
	}
	defer filefp.Close()
	_, err = filefp.Write(f.RawData)
	if err != nil {
		return "", err
	}
	return filefp.Name(), nil
}
