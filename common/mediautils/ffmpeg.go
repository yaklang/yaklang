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

// ffmpeg.ExtractAudioFromVideo can extract audio from video
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

// ffmpeg.ExtractFineGrainedFramesFromVideo can extract fine-grained frames from video
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

// ffmpeg.ExtractBroadGrainedFramesFromVideo can extract fine-grained frames from video
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

// BurnSRTIntoVideo can burn subtitles into a video
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

// ffmpeg.ExtractUserScreenshot can capture user screen screenshots with multi-monitor support
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

var FfmpegExports = map[string]any{
	"ExtractAudioFromVideo":              _extractAudioFromVideo,
	"ExtractFineGrainedFramesFromVideo":  _extractFineGrainedFramesFromVideo,
	"ExtractBroadGrainedFramesFromVideo": _extractBroadGrainedFramesFromVideo,
	"BurnSRTIntoVideo":                   _burnInSubtitles,
	"ExtractUserScreenshot":              _extractUserScreenShot,

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
}
