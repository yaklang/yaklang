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

	// Video slicing options (segment muxer based, time-based slicing)
	// 用于按时间段把长视频切成若干段独立 mp4，配合 omni 模型做端到端理解
	sliceDurationSeconds float64                       // 每段时长（秒），默认 120
	sliceReencode        bool                          // 是否重编码（默认 false：流复制最快）
	sliceMaxHeight       int                           // 重编码模式下最大高度，默认 720
	sliceTargetFPS       float64                       // 重编码模式下目标 FPS，默认 2
	sliceLoadRawData     bool                          // 是否随 channel 回吐字节内容，默认 false
	sliceCallback        func(*VideoSliceResult)       // 实时回调，与 channel 双轨
	sliceOutputDir       string                        // 切片输出目录（不指定则用临时目录）
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

		// Video slicing defaults: stream copy, 120 秒/段，与 omni Flash 安全限制兼容
		sliceDurationSeconds: 120,
		sliceReencode:        false,
		sliceMaxHeight:       720,
		sliceTargetFPS:       2,
		sliceLoadRawData:     false,
	}
}

// WithContext sets the context for the command.
func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

// withDebug 启用 ffmpeg 命令的详细日志输出（导出名为 ffmpeg.withDebug）
// 作为各 ffmpeg.Extract*/Burn* 接口的可选项使用，便于排查问题
//
// 参数:
//   - debug: 是否启用详细日志
//
// 返回值:
//   - 可传入 ffmpeg 各接口的选项
//
// Example:
// ```
// // 开启调试日志提取音频（需要真实视频文件，示意性示例）
// result = ffmpeg.ExtractAudioFromVideo("video.mp4", ffmpeg.withDebug(true))~
// ```
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

// withStartEnd 以秒为单位指定提取/处理的时间区间（导出名为 ffmpeg.withStartEnd）
// 作为各 ffmpeg.Extract* 接口的可选项使用，仅处理 [start, end] 区间内的内容
//
// 参数:
//   - start: 起始时间（秒）
//   - end: 结束时间（秒）
//
// 返回值:
//   - 可传入 ffmpeg 各接口的选项
//
// Example:
// ```
// // 仅提取 10 到 20.5 秒之间的帧（需要真实视频文件，示意性示例）
// result = ffmpeg.ExtractFineGrainedFramesFromVideo("video.mp4", ffmpeg.withStartEnd(10, 20.5))~
// ```
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

// withOutputFile 指定最终输出文件的路径（导出名为 ffmpeg.withOutputFile）
// 作为 ffmpeg.BurnSRTIntoVideo / ffmpeg.ExtractAudioFromVideo 等接口的可选项；不设置时使用临时文件
//
// 参数:
//   - filepath: 输出文件路径
//
// 返回值:
//   - 可传入 ffmpeg 各接口的选项
//
// Example:
// ```
// // 指定输出路径烧录字幕（需要真实视频与字幕文件，示意性示例）
// out = ffmpeg.BurnSRTIntoVideo("video.mp4", "subtitles.srt", ffmpeg.withOutputFile("/tmp/out.mp4"))~
// ```
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

// withTimestampOverlay 设置是否在提取的帧上叠加时间戳（导出名为 ffmpeg.withTimestampOverlay）
// 启用后会在每帧底部加一条黑条显示时间戳；作为 ffmpeg.Extract*Frames* 接口的可选项使用
//
// 参数:
//   - show: 是否叠加时间戳
//
// 返回值:
//   - 可传入 ffmpeg 各接口的选项
//
// Example:
// ```
// // 提取帧并叠加时间戳（需要真实视频文件，示意性示例）
// result = ffmpeg.ExtractBroadGrainedFramesFromVideo("video.mp4", ffmpeg.withTimestampOverlay(true))~
// ```
func WithTimestampOverlay(show bool) Option {
	return func(o *options) {
		o.showTimestamp = show
	}
}

// withSubtitlePadding 设置烧录字幕时是否在底部添加黑色内边距（导出名为 ffmpeg.withSubtitlePadding）
// 启用后会在视频底部加黑边用于显示字幕，避免字幕遮挡原始画面；作为 ffmpeg.BurnSRTIntoVideo 的可选项使用
//
// 参数:
//   - enable: 是否添加字幕内边距
//
// 返回值:
//   - 可传入 ffmpeg 各接口的选项
//
// Example:
// ```
// // 烧录字幕并添加底部黑边（需要真实视频与字幕文件，示意性示例）
// out = ffmpeg.BurnSRTIntoVideo("video.mp4", "subtitles.srt", ffmpeg.withSubtitlePadding(true))~
// ```
func WithSubtitlePadding(enable bool) Option {
	return func(o *options) {
		o.subtitlePadding = enable
	}
}

// withIgnoreBottomPaddingInSceneDetection 设置场景检测时是否忽略底部内边距区域（导出名为 ffmpeg.withIgnoreBottomPaddingInSceneDetection）
// 启用后场景检测只分析原始画面，忽略时间戳/字幕区域的变化，避免误判场景切换
//
// 参数:
//   - enable: 是否在场景检测中忽略底部内边距
//
// 返回值:
//   - 可传入 ffmpeg 各接口的选项
//
// Example:
// ```
// // 叠加时间戳的同时避免其干扰场景检测（需要真实视频文件，示意性示例）
// result = ffmpeg.ExtractFineGrainedFramesFromVideo("video.mp4",
//     ffmpeg.withTimestampOverlay(true),
//     ffmpeg.withIgnoreBottomPaddingInSceneDetection(true),
// )~
// ```
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

// withScreenCaptureDebug 启用屏幕截图的调试信息（导出名为 ffmpeg.withScreenCaptureDebug）
// 作为 ffmpeg.ExtractUserScreenshot 的可选项使用
//
// 参数:
//   - enable: 是否启用调试信息
//
// 返回值:
//   - 可传入 ffmpeg.ExtractUserScreenshot 的选项
//
// Example:
// ```
// // 以调试模式截屏（需要桌面环境，示意性示例）
// result = ffmpeg.ExtractUserScreenshot(ffmpeg.withScreenCaptureDebug(true))~
// ```
func WithScreenCaptureDebug(enable bool) Option {
	return func(o *options) {
		o.debug = enable
	}
}

// withScreenCaptureQuality 设置屏幕截图质量（导出名为 ffmpeg.withScreenCaptureQuality）
// 作为 ffmpeg.ExtractUserScreenshot 的可选项使用；可选 ffmpeg.QualityLow/QualityNormal/QualityHigh
//
// 参数:
//   - quality: 质量级别（QualityLow 快速小文件 / QualityNormal 平衡 / QualityHigh 无损）
//
// 返回值:
//   - 可传入 ffmpeg.ExtractUserScreenshot 的选项
//
// Example:
// ```
// // 以高质量（无损）截屏（需要桌面环境，示意性示例）
// result = ffmpeg.ExtractUserScreenshot(ffmpeg.withScreenCaptureQuality(ffmpeg.QualityHigh))~
// ```
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

// --- Video Slicing Options ---
// 视频切片关键词: ExtractVideoSliceFromVideo, segment muxer, omni preset

// withSliceDurationSeconds 设置每段切片的时长（秒），默认 120 秒（导出名为 ffmpeg.withSliceDurationSeconds）
// 作为 ffmpeg.ExtractVideoSliceFromVideo 的可选项使用
// 关键词: 切片段长, segment_time
//
// 参数:
//   - seconds: 每段切片时长（秒）
//
// 返回值:
//   - 可传入 ffmpeg.ExtractVideoSliceFromVideo 的选项
//
// Example:
// ```
// // 每 30 秒切一段（需要真实视频文件，示意性示例）
// ch = ffmpeg.ExtractVideoSliceFromVideo("video.mp4", ffmpeg.withSliceDurationSeconds(30))~
// ```
func WithSliceDurationSeconds(seconds float64) Option {
	return func(o *options) {
		if seconds > 0 {
			o.sliceDurationSeconds = seconds
		}
	}
}

// withSliceReencode 设置切片是否重编码（导出名为 ffmpeg.withSliceReencode）
// false（默认）使用 -c copy 流复制，速度极快、保持源分辨率与 FPS；true 则重编码到指定分辨率与 FPS
// 作为 ffmpeg.ExtractVideoSliceFromVideo 的可选项使用
// 关键词: 流复制 stream copy, 重编码 reencode
//
// 参数:
//   - enable: 是否启用重编码
//
// 返回值:
//   - 可传入 ffmpeg.ExtractVideoSliceFromVideo 的选项
//
// Example:
// ```
// // 启用重编码切片以统一分辨率/帧率（需要真实视频文件，示意性示例）
// ch = ffmpeg.ExtractVideoSliceFromVideo("video.mp4",
//     ffmpeg.withSliceReencode(true),
//     ffmpeg.withSliceMaxHeight(720),
//     ffmpeg.withSliceTargetFPS(2),
// )~
// ```
func WithSliceReencode(enable bool) Option {
	return func(o *options) {
		o.sliceReencode = enable
	}
}

// withSliceMaxHeight 重编码模式下设置切片最大高度（保持宽高比），默认 720（导出名为 ffmpeg.withSliceMaxHeight）
// 仅在启用 ffmpeg.withSliceReencode(true) 时生效；作为 ffmpeg.ExtractVideoSliceFromVideo 的可选项使用
// 关键词: 切片分辨率, scale
//
// 参数:
//   - h: 最大高度（像素）
//
// 返回值:
//   - 可传入 ffmpeg.ExtractVideoSliceFromVideo 的选项
//
// Example:
// ```
// // 重编码并限制高度为 480（需要真实视频文件，示意性示例）
// ch = ffmpeg.ExtractVideoSliceFromVideo("video.mp4", ffmpeg.withSliceReencode(true), ffmpeg.withSliceMaxHeight(480))~
// ```
func WithSliceMaxHeight(h int) Option {
	return func(o *options) {
		if h > 0 {
			o.sliceMaxHeight = h
		}
	}
}

// withSliceTargetFPS 重编码模式下设置切片目标 FPS，默认 2（导出名为 ffmpeg.withSliceTargetFPS）
// 仅在启用 ffmpeg.withSliceReencode(true) 时生效；默认值贴合 omni 默认抽样率
// 作为 ffmpeg.ExtractVideoSliceFromVideo 的可选项使用
// 关键词: 切片帧率, target fps
//
// 参数:
//   - fps: 目标帧率
//
// 返回值:
//   - 可传入 ffmpeg.ExtractVideoSliceFromVideo 的选项
//
// Example:
// ```
// // 重编码并设置目标帧率为 1（需要真实视频文件，示意性示例）
// ch = ffmpeg.ExtractVideoSliceFromVideo("video.mp4", ffmpeg.withSliceReencode(true), ffmpeg.withSliceTargetFPS(1))~
// ```
func WithSliceTargetFPS(fps float64) Option {
	return func(o *options) {
		if fps > 0 {
			o.sliceTargetFPS = fps
		}
	}
}

// withSliceLoadRawData 设置是否在切片结果 channel 中携带分片原始字节，默认 false（导出名为 ffmpeg.withSliceLoadRawData）
// 默认仅返回文件路径；启用会显著增加内存与 IO 开销，建议确需 base64 推送时再开启
// 作为 ffmpeg.ExtractVideoSliceFromVideo 的可选项使用
// 关键词: 携带原始字节, raw data
//
// 参数:
//   - enable: 是否在结果中携带原始字节
//
// 返回值:
//   - 可传入 ffmpeg.ExtractVideoSliceFromVideo 的选项
//
// Example:
// ```
// // 切片并携带原始字节用于直接上传（需要真实视频文件，示意性示例）
// ch = ffmpeg.ExtractVideoSliceFromVideo("video.mp4", ffmpeg.withSliceLoadRawData(true))~
// ```
func WithSliceLoadRawData(enable bool) Option {
	return func(o *options) {
		o.sliceLoadRawData = enable
	}
}

// withSliceCallback 注册切片实时回调，每段切片落盘后立即触发（与 channel 同时发送）（导出名为 ffmpeg.withSliceCallback）
// 作为 ffmpeg.ExtractVideoSliceFromVideo 的可选项使用，便于边切片边处理（如上传到模型）
// 关键词: 切片回调, slice callback
//
// 参数:
//   - cb: 回调函数 func(result)，每段切片就绪时调用
//
// 返回值:
//   - 可传入 ffmpeg.ExtractVideoSliceFromVideo 的选项
//
// Example:
// ```
// // 边切片边记录路径（需要真实视频文件，示意性示例）
// ch = ffmpeg.ExtractVideoSliceFromVideo("video.mp4",
//     ffmpeg.withSliceCallback(func(r) { log.info("slice ready: %v", r.FilePath) }),
// )~
// ```
func WithSliceCallback(cb func(*VideoSliceResult)) Option {
	return func(o *options) {
		o.sliceCallback = cb
	}
}

// withSliceOutputDir 指定切片输出目录，不指定则使用临时目录（导出名为 ffmpeg.withSliceOutputDir）
// 作为 ffmpeg.ExtractVideoSliceFromVideo 的可选项使用
// 关键词: 切片输出目录, slice output dir
//
// 参数:
//   - dir: 切片文件输出目录
//
// 返回值:
//   - 可传入 ffmpeg.ExtractVideoSliceFromVideo 的选项
//
// Example:
// ```
// // 将切片输出到指定目录（需要真实视频文件，示意性示例）
// ch = ffmpeg.ExtractVideoSliceFromVideo("video.mp4", ffmpeg.withSliceOutputDir("/tmp/slices"))~
// ```
func WithSliceOutputDir(dir string) Option {
	return func(o *options) {
		o.sliceOutputDir = dir
	}
}

// withSlicePresetForOmni 按目标 omni 模型一键预设切片段长（导出名为 ffmpeg.withSlicePresetForOmni）
// turbo => 30 秒、flash => 120 秒、plus => 120 秒（保守值，避免触发 "video file is too long"；
// 如需更长可改用 ffmpeg.withSliceDurationSeconds）。作为 ffmpeg.ExtractVideoSliceFromVideo 的可选项使用
// 关键词: omni 预设, slice preset
//
// 参数:
//   - preset: 预设名称（turbo / flash / plus）
//
// 返回值:
//   - 可传入 ffmpeg.ExtractVideoSliceFromVideo 的选项
//
// Example:
// ```
// // 使用 flash 预设切片（需要真实视频文件，示意性示例）
// ch = ffmpeg.ExtractVideoSliceFromVideo("video.mp4", ffmpeg.withSlicePresetForOmni("flash"))~
// ```
func WithSlicePresetForOmni(preset string) Option {
	return func(o *options) {
		switch preset {
		case "turbo":
			o.sliceDurationSeconds = 30
		case "flash":
			o.sliceDurationSeconds = 120
		case "plus":
			o.sliceDurationSeconds = 120
		default:
			// 未知预设保持默认值，仅记录
		}
	}
}

// --- Result Types ---

// VideoSliceResult 描述一段时间维度切片的产物。
// 关键词: 视频切片结果, VideoSliceResult
type VideoSliceResult struct {
	// FilePath 切片文件路径
	FilePath string
	// Index 切片序号（从 0 开始）
	Index int
	// StartTime 切片起始时间（基于段长估算，因流复制按 keyframe 实际可能略有偏差）
	StartTime time.Duration
	// EndTime 切片结束时间（估算）
	EndTime time.Duration
	// SizeBytes 文件字节数
	SizeBytes int64
	// MIMEType 一般是 "video/mp4"
	MIMEType string
	// RawData 仅当 WithSliceLoadRawData(true) 时填充
	RawData []byte
	// Error 处理过程中产生的错误（不致命错误也会通过该字段传出）
	Error error
}

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
