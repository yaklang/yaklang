package imageutils

import (
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"os/exec"
	"path/filepath"
)

func ExtractVideoFrameContext(ctx context.Context, input string) (*chunkmaker.ChunkMaker, error) {
	if utils.GetFirstExistedFile(input) == "" {
		return nil, utils.Errorf("%s file not existed", input)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	/*
		ffmpeg -i vtestdata/demo.mp4 \
		-vf "scdet=threshold=20,select='eq(n,0) + gt(floor(t), floor(prev_t)) + gt(scene, 0.2)',drawtext=fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf:text='tffset-timestamp\: %{eif\:t*1000\:d}ms':fontcolor=white:fontsize=24:box=1:boxcolor=black@0.5:x=(w-tw)/2:y=h-th-10,setpts=N/FR/TB" \
		-fps_mode vfr \
		/tmp/core-%04d.jpeg
	*/
	ffmpegBinaryPath := consts.GetFfmpegPath()
	if ffmpegBinaryPath == "" {
		return nil, errors.New("ffmpeg path is empty")
	}

	outputTmp := consts.GetDefaultYakitBaseTempDir()
	outputTmp = filepath.Join(outputTmp, "video-frame-temp-dir-"+utils.RandStringBytes(12))
	_ = os.MkdirAll(outputTmp, os.ModePerm)
	if _, err := os.Stat(ffmpegBinaryPath); os.IsNotExist(err) {
		return nil, errors.New("ffmpeg binary not found at " + ffmpegBinaryPath)
	}

	outputFmt := filepath.Join(outputTmp, "core-%04d.jpeg")

	cmd := exec.CommandContext(
		ctx,
		ffmpegBinaryPath, "-i", input,
		`-vf`, `scdet=threshold=20,select='eq(n,0) + gt(floor(t), floor(prev_t)) + gt(scene, 0.2)',drawtext=fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf:text='tffset-timestamp\: %{eif\:t*1000\:d}ms':fontcolor=white:fontsize=24:box=1:boxcolor=black@0.5:x=(w-tw)/2:y=h-th-10,setpts=N/FR/TB`,
		`-fps_mode`, `vfr`,
		outputFmt,
	)
	err := cmd.Run()
	if err != nil {
		return nil, utils.Errorf("ffmpeg command failed: %v", err)
	}
	return nil, nil
}
