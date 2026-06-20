package mediautils

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
)

// ExtractAudioFromVideo 从视频中提取音频（导出名为 ffmpeg.ExtractAudioFromVideo）
//
// 参数:
//   - i: 输入视频文件路径
//   - opts: 可选项，如 ffmpeg.withOutputFile / ffmpeg.withStartEnd / ffmpeg.withDebug 等
//
// 返回值:
//   - 音频提取结果对象（含输出文件路径等信息）
//   - 错误信息（提取失败时返回）
//
// example:
// ```
// result, err = ffmpeg.ExtractAudioFromVideo("video.mp4")
// die(err)
// println(result.FilePath)
// // ------------
// dump(ffmpeg.ExtractAudioFromVideo("video.mp4")~)
// ```
func _extractAudioFromVideo(i string, opts ...ffmpegutils.Option) (*ffmpegutils.AudioExtractionResult, error) {
	//  go run common/yak/cmd/yak.go -c 'dump(ffmpeg.ExtractAudioFromVideo("vtestdata/demo1.mp4")~)'
	return ffmpegutils.ExtractAudioFromVideo(i, opts...)
}

// ExtractFineGrainedFramesFromVideo 以细粒度（低场景阈值）从视频中提取关键帧（导出名为 ffmpeg.ExtractFineGrainedFramesFromVideo）
// 默认启用时间戳叠加与智能场景检测，适合需要较密集采样的场景
//
// 参数:
//   - i: 输入视频文件路径
//   - opts: 可选项，如 ffmpeg.withStartEnd / ffmpeg.withTimestampOverlay 等
//
// 返回值:
//   - 帧结果管道，可用 for range 逐帧消费（每帧可 SaveToFile）
//   - 错误信息（提取失败时返回）
//
// example:
// ```
//
//	result, err = ffmpeg.ExtractFineGrainedFramesFromVideo("video.mp4");
//
//	for i in result {
//	    filename, err = i.SaveToFile();
//		if err != nil { continue }
//	}
//
// result = ffmpeg.ExtractFineGrainedFramesFromVideo("video.mp4", ffmpeg.withStartEnd(10, 20.5))~ // 10-20.5 seconds frames
// ```
func _extractFineGrainedFramesFromVideo(i string, opts ...ffmpegutils.Option) (<-chan *ffmpegutils.FfmpegStreamResult, error) {
	opts = append(opts, ffmpegutils.WithSceneThreshold(0.05))
	opts = append(opts, ffmpegutils.WithTimestampOverlay(true))
	opts = append(opts, ffmpegutils.WithIgnoreBottomPaddingInSceneDetection(true)) // 默认启用智能场景检测
	return ffmpegutils.ExtractImageFramesFromVideo(i, opts...)
}

// ExtractBroadGrainedFramesFromVideo 以粗粒度（较高场景阈值）从视频中提取关键帧（导出名为 ffmpeg.ExtractBroadGrainedFramesFromVideo）
// 相比细粒度采样更稀疏，适合快速概览视频内容
//
// 参数:
//   - i: 输入视频文件路径
//   - opts: 可选项，如 ffmpeg.withStartEnd / ffmpeg.withTimestampOverlay 等
//
// 返回值:
//   - 帧结果管道，可用 for range 逐帧消费（每帧可 SaveToFile）
//   - 错误信息（提取失败时返回）
//
// example:
// ```
//
//	result, err = ffmpeg.ExtractBroadGrainedFramesFromVideo("video.mp4");
//
//	for i in result {
//	    filename, err = i.SaveToFile();
//		if err != nil { continue }
//	}
//
// ```
func _extractBroadGrainedFramesFromVideo(i string, opts ...ffmpegutils.Option) (<-chan *ffmpegutils.FfmpegStreamResult, error) {
	opts = append(opts, ffmpegutils.WithSceneThreshold(0.2))
	opts = append(opts, ffmpegutils.WithTimestampOverlay(true))
	opts = append(opts, ffmpegutils.WithIgnoreBottomPaddingInSceneDetection(true)) // 默认启用智能场景检测
	return ffmpegutils.ExtractImageFramesFromVideo(i, opts...)
}

// BurnSRTIntoVideo 将 SRT 字幕烧录进视频（导出名为 ffmpeg.BurnSRTIntoVideo）
// 默认启用字幕内边距与时间戳，输出为新的 mp4 文件并返回其路径
//
// 参数:
//   - inputVideo: 输入视频文件路径
//   - srtFile: SRT 字幕文件路径
//   - opts: 可选项，如 ffmpeg.withOutputFile / ffmpeg.withSubtitlePadding 等
//
// 返回值:
//   - 输出视频文件路径
//   - 错误信息（烧录失败或输出无效时返回）
//
// example:
// ```
// outputFile, err = ffmpeg.BurnSRTIntoVideo("video.mp4", "subtitles.srt")
// die(err)
// println(outputFile)
//
// // With simple mode (wavy calling)
// outputFile = ffmpeg.BurnSRTIntoVideo("video.mp4", "subtitles.srt")~
//
// ```
func _burnInSubtitles(inputVideo string, srtFile string, opts ...ffmpegutils.Option) (string, error) {
	// Generate output filename based on input
	// go run common/yak/cmd/yak.go -c 'result = ffmpeg.BurnSRTIntoVideo("vtestdata/demo1.mp4", "vtestdata/demo1.mp3.srt"); println(result)'
	baseName := strings.TrimSuffix(filepath.Base(inputVideo), filepath.Ext(inputVideo))
	outputFile, err := consts.TempFile(baseName + "_with_subtitles_*.mp4")
	if err != nil {
		// Fallback to ioutil.TempFile if consts.TempFile is not available
		outputFile, err = ioutil.TempFile("", baseName+"_with_subtitles_*.mp4")
		if err != nil {
			return "", err
		}
	}
	outputFile.Close() // Close the file, ffmpeg will write to the path
	outputPath := outputFile.Name()
	os.Remove(outputPath) // Remove the temp file before ffmpeg creates it

	// Prepare options with smart defaults
	finalOpts := []ffmpegutils.Option{
		ffmpegutils.WithSubtitleFile(srtFile),
		ffmpegutils.WithOutputVideoFile(outputPath),
		ffmpegutils.WithSubtitlePadding(true), // Enable padding by default for better UX
		ffmpegutils.WithSubtitleTimestamp(true),
	}

	// Append user options (they can override defaults)
	finalOpts = append(finalOpts, opts...)

	// Execute the subtitle burning
	err = ffmpegutils.BurnInSubtitles(inputVideo, finalOpts...)
	if err != nil {
		return "", err
	}

	if !utils.FileExists(outputPath) {
		return "", utils.Error("failed to burn subtitles into video")
	}

	// Check file size to ensure the output is reasonable
	if fileInfo, err := os.Stat(outputPath); err == nil {
		if fileInfo.Size() <= 0 {
			return "", utils.Error("generated video file is empty or invalid")
		}
		log.Infof("Successfully burned subtitles into video: %s (size: %d bytes)", outputPath, fileInfo.Size())
	} else {
		log.Infof("Successfully burned subtitles into video: %s", outputPath)
	}

	return outputPath, nil
}

// ExtractUserScreenshot 截取用户屏幕截图，支持多显示器自动拼接（导出名为 ffmpeg.ExtractUserScreenshot）
//
// 参数:
//   - opts: 可选项，如 ffmpeg.withScreenCaptureDebug / ffmpeg.withScreenCaptureQuality 等
//
// 返回值:
//   - 截图结果对象（可调用 SaveToFile 保存）
//   - 错误信息（截图失败时返回）
//
// example:
// ```
// result, err = ffmpeg.ExtractUserScreenshot()
// die(err)
// filename, err = result.SaveToFile()
// println(filename)
//
// // With debug mode
// result = ffmpeg.ExtractUserScreenshot(ffmpeg.withScreenCaptureDebug(true))~
//
// // With high quality (无损压缩)
// result = ffmpeg.ExtractUserScreenshot(ffmpeg.withScreenCaptureQuality(ffmpeg.QualityHigh))~
//
// // With normal quality (平衡质量和大小)
// result = ffmpeg.ExtractUserScreenshot(ffmpeg.withScreenCaptureQuality(ffmpeg.QualityNormal))~
//
// // With low quality (快速截图，小文件)
// result = ffmpeg.ExtractUserScreenshot(ffmpeg.withScreenCaptureQuality(ffmpeg.QualityLow))~
//
// // Screenshot is automatically concatenated if multiple screens are detected
// ```
func _extractUserScreenShot(opts ...ffmpegutils.Option) (*ffmpegutils.FfmpegStreamResult, error) {
	return ffmpegutils.ExtractUserScreenShot(opts...)
}

// ExtractVideoSliceFromVideo 将长视频按时间切分为多个 mp4 切片（导出名为 ffmpeg.ExtractVideoSliceFromVideo）
// 默认流复制（不重编码、保持源分辨率与 FPS）；通过 ffmpeg.withSliceReencode(true) 启用重编码。
// 切片实时通过 channel 与可选回调 ffmpeg.withSliceCallback(cb) 下发。
//
// 参数:
//   - i: 输入视频文件路径
//   - opts: 可选项，如 ffmpeg.withSliceDurationSeconds / ffmpeg.withSlicePresetForOmni / ffmpeg.withSliceCallback 等
//
// 返回值:
//   - 视频切片结果管道，可用 for range 逐个消费（每个含 FilePath/Error 等）
//   - 错误信息（启动切片失败时返回）
//
// example:
// ```
//
//	ch, err = ffmpeg.ExtractVideoSliceFromVideo("video.mp4",
//	    ffmpeg.withSlicePresetForOmni("flash"),
//	    ffmpeg.withSliceCallback(r => log.info("slice ready: %v", r.FilePath)),
//	)
//	for slice in ch {
//	    if slice.Error != nil { continue }
//	    // upload slice.FilePath to omni model
//	}
//
// ```
//
// 关键词: ffmpeg 切片, segment muxer, omni 视频切片
func _extractVideoSliceFromVideo(i string, opts ...ffmpegutils.Option) (<-chan *ffmpegutils.VideoSliceResult, error) {
	return ffmpegutils.ExtractVideoSliceFromVideo(i, opts...)
}

var FfmpegExports = map[string]any{
	"ExtractAudioFromVideo":              _extractAudioFromVideo,
	"ExtractFineGrainedFramesFromVideo":  _extractFineGrainedFramesFromVideo,
	"ExtractBroadGrainedFramesFromVideo": _extractBroadGrainedFramesFromVideo,
	"BurnSRTIntoVideo":                   _burnInSubtitles,
	"ExtractUserScreenshot":              _extractUserScreenShot,
	"ExtractVideoSliceFromVideo":         _extractVideoSliceFromVideo,

	"withStartEnd":                            ffmpegutils.WithStartEndSeconds,
	"withTimestampOverlay":                    ffmpegutils.WithTimestampOverlay,
	"withSubtitlePadding":                     ffmpegutils.WithSubtitlePadding,
	"withOutputFile":                          ffmpegutils.WithOutputVideoFile,
	"withIgnoreBottomPaddingInSceneDetection": ffmpegutils.WithIgnoreBottomPaddingInSceneDetection,
	"withScreenCaptureDebug":                  ffmpegutils.WithScreenCaptureDebug,
	"withScreenCaptureQuality":                ffmpegutils.WithScreenCaptureQuality,
	"QualityLow":                              ffmpegutils.QualityLow,
	"QualityNormal":                           ffmpegutils.QualityNormal,
	"QualityHigh":                             ffmpegutils.QualityHigh,

	// 视频切片关键词: ExtractVideoSliceFromVideo, withSlice
	"withSliceDurationSeconds": ffmpegutils.WithSliceDurationSeconds,
	"withSliceReencode":        ffmpegutils.WithSliceReencode,
	"withSliceMaxHeight":       ffmpegutils.WithSliceMaxHeight,
	"withSliceTargetFPS":       ffmpegutils.WithSliceTargetFPS,
	"withSliceLoadRawData":     ffmpegutils.WithSliceLoadRawData,
	"withSliceCallback":        ffmpegutils.WithSliceCallback,
	"withSliceOutputDir":       ffmpegutils.WithSliceOutputDir,
	"withSlicePresetForOmni":   ffmpegutils.WithSlicePresetForOmni,
	"withDebug":                ffmpegutils.WithDebug,
}
