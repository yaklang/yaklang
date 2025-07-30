package mediautils

import (
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

var FfmpegExports = map[string]any{
	"ExtractAudioFromVideo": _extractAudioFromVideo,
}
