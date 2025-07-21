package imageutils

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
)

func ExtractDocumentPagesContext(ctx context.Context, input string) (chan *ImageResult, error) {
	if utils.GetFirstExistedFile(input) == "" {
		return nil, utils.Errorf("%s file not existed", input)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	page2imgPath := consts.GetPage2ImgBinaryPath()
	if page2imgPath == "" {
		return nil, errors.New("page2img path is empty")
	}
	if _, err := os.Stat(page2imgPath); os.IsNotExist(err) {
		return nil, errors.New("page2img binary not found at " + page2imgPath)
	}

	outputTmp, err := os.MkdirTemp(consts.GetDefaultYakitBaseTempDir(), "page2img-")
	if err != nil {
		return nil, utils.Errorf("create temp dir failed: %v", err)
	}

	outputFmt := filepath.Join(outputTmp, "image-%d.jpeg")
	if err := os.MkdirAll(outputTmp, os.ModePerm); err != nil {
		return nil, utils.Errorf("create output dir failed: %v", err)
	}

	var ch = make(chan *ImageResult)
	go func() {
		defer close(ch)
		defer os.RemoveAll(outputTmp)

		var outBuf, errBuf bytes.Buffer
		cmd := exec.CommandContext(ctx, page2imgPath, "-i", input, "-o", outputFmt)
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		err := cmd.Run()
		if err != nil {
			log.Errorf("page2img command failed: %v", err)
			log.Errorf("page2img stdout: %s", outBuf.String())
			log.Errorf("page2img stderr: %s", errBuf.String())
			return
		}

		files, err := os.ReadDir(outputTmp)
		if err != nil {
			log.Errorf("read output dir failed: %v", err)
			return
		}

		var fileNames []string
		for _, file := range files {
			if !file.IsDir() {
				fileNames = append(fileNames, file.Name())
			}
		}
		sort.Strings(fileNames)

		for _, fileName := range fileNames {
			select {
			case <-ctx.Done():
				return
			default:
				fp := filepath.Join(outputTmp, fileName)
				data, err := os.ReadFile(fp)
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
		}
	}()

	return ch, nil
}
