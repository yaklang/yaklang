package imageutils

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
)

func ExtractVideoFrameContext(ctx context.Context, input string) (chan *ImageResult, error) {
	if utils.GetFirstExistedFile(input) == "" {
		return nil, utils.Errorf("%s file not existed", input)
	}

	if ctx == nil {
		ctx = context.Background()
	}

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

	token := utils.RandStringBytes(10)
	outputFmt := filepath.Join(outputTmp, "core-"+token+"-%04d.jpeg")

	var ch = make(chan *ImageResult)
	go func() {
		finishedCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		var output bytes.Buffer

		cmd := exec.CommandContext(
			ctx,
			ffmpegBinaryPath, "-i", input,
			`-vf`, `scdet=threshold=20,select='eq(n,0) + gt(floor(t), floor(prev_t)) + gt(scene, 0.2)',drawtext=fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf:text='offset-timestamp\: %{eif\:t*1000\:d}ms':fontcolor=white:fontsize=24:box=1:boxcolor=black@0.5:x=(w-tw)/2:y=h-th-10,setpts=N/FR/TB`,
			`-fps_mode`, `vfr`,
			outputFmt,
		)
		cmd.Stdout = &output
		cmd.Stderr = &output

		go func() {
			defer close(ch)

			outputIdx := 0
			filter := map[string]bool{}
			for {
				select {
				case <-time.After(1 * time.Second):
					// read dir and get all files
					files, err := os.ReadDir(outputTmp)
					if err != nil {
						log.Errorf("read dir failed: %v", err)
						continue
					}
					for _, file := range files {
						if file.IsDir() {
							continue
						}
						fileName := file.Name()
						if _, ok := filter[fileName]; ok {
							continue
						}
						filter[fileName] = true
						outputIdx++
						data, err := os.ReadFile(filepath.Join(outputTmp, fileName))
						if err != nil {
							log.Errorf("read file failed: %v", err)
							continue
						}
						mime := mimetype.Detect(data)
						ch <- &ImageResult{
							RawImage: data,
							MIMEType: mime,
						}
					}
					select {
					case <-finishedCtx.Done():
						return
					default:
					}
					continue
				}
			}
		}()

		err := cmd.Run()
		if err != nil {
			log.Errorf("ffmpeg command failed: %v", err)
			log.Errorf("ffmpeg output: %s", output.String())
			return
		}
	}()
	return ch, nil
}
