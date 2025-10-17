package ffmpegutils

import "github.com/yaklang/yaklang/common/consts"

var (
	// ffmpegBinaryPath holds the path to the ffmpeg executable.
	// It is initialized by checking the system's configuration.
	ffmpegBinaryPath = consts.GetFfmpegPath()
)
