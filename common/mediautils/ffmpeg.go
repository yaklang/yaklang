package mediautils

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
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
	}

	// Append user options (they can override defaults)
	finalOpts = append(finalOpts, opts...)

	// Execute the subtitle burning
	err = ffmpegutils.BurnInSubtitles(inputVideo, finalOpts...)
	if err != nil {
		return "", err
	}

	return outputPath, nil
}

var FfmpegExports = map[string]any{
	"ExtractAudioFromVideo":              _extractAudioFromVideo,
	"ExtractFineGrainedFramesFromVideo":  _extractFineGrainedFramesFromVideo,
	"ExtractBroadGrainedFramesFromVideo": _extractBroadGrainedFramesFromVideo,
	"BurnSRTIntoVideo":                   _burnInSubtitles,

	"withStartEnd":         ffmpegutils.WithStartEndSeconds,
	"withTimestampOverlay": ffmpegutils.WithTimestampOverlay,
}
