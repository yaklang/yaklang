package ffmpegutils

import (
	"github.com/yaklang/yaklang/common/consts"
	"os/exec"
)

var (
	// ffmpegBinaryPath holds the path to the ffmpeg executable.
	// It is initialized by checking the system's configuration.
	ffmpegBinaryPath = consts.GetFfmpegPath()
)

func resolveFFmpegBinary() string {
	if ffmpegBinaryPath != "" {
		return ffmpegBinaryPath
	}
	if path := consts.GetFfmpegPath(); path != "" {
		return path
	}
	path, _ := exec.LookPath("ffmpeg")
	return path
}
