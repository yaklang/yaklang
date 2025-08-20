package imageutils

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
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

	token := utils.RandStringBytes(10)
	outputFmt := filepath.Join(outputTmp, "image-"+token+"-%d.jpeg")
	if err := os.MkdirAll(outputTmp, os.ModePerm); err != nil {
		return nil, utils.Errorf("create output dir failed: %v", err)
	}

	var ch = make(chan *ImageResult)
	go func() {
		// defer os.RemoveAll(outputTmp)

		finishedCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		var outBuf, errBuf bytes.Buffer
		cmd := exec.CommandContext(ctx, page2imgPath, "-i", input, "-o", outputFmt, "-s", "200")
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		go func() {
			defer close(ch)

			filter := map[string]bool{}
			for {
				time.Sleep(1 * time.Second)
				// read dir and get all files
				files, err := os.ReadDir(outputTmp)
				if err != nil {
					log.Errorf("read dir failed: %v", err)
					select {
					case <-finishedCtx.Done():
						return
					default:
					}
					continue
				}

				var orderedFiles = make([]*orderedFile, 0, len(files))

				for _, file := range files {
					if file.IsDir() {
						continue
					}
					fileName := file.Name()
					if _, ok := filter[fileName]; ok {
						continue
					}
					filter[fileName] = true

					_, filenameWithoutDir := filepath.Split(fileName)
					extName := filepath.Ext(filenameWithoutDir)
					filenameWithoutExt := strings.TrimSuffix(filenameWithoutDir, extName)
					imageOrderStr := strings.TrimPrefix(filenameWithoutExt, "image-"+token+"-")
					imageOrderInt := utils.InterfaceToInt(imageOrderStr)
					if imageOrderInt <= 0 {
						continue
					}
					orderedFiles = append(orderedFiles, &orderedFile{
						idx:      imageOrderInt,
						filename: fileName,
					})
				}
				for _, of := range sortOrderedFile(orderedFiles) {
					log.Infof("find page image idx[%v]: %v", of.idx, of.filename)
					fileName := of.filename
					filePath := filepath.Join(outputTmp, fileName)

					// compress
					compressedFile := filepath.Join(outputTmp, "compressed_"+fileName)
					err := ffmpegutils.CompressImage(filePath, compressedFile)
					if err == nil {
						var originalSize int64
						var nowSize int64
						if s, err := os.Stat(filePath); err == nil {
							originalSize = s.Size()
						}
						if s, err := os.Stat(compressedFile); err == nil {
							nowSize = s.Size()
						}
						log.Infof("compressed page image %s, from: %v -> %v", fileName, originalSize, nowSize)
						filePath = compressedFile
					}

					data, err := os.ReadFile(filePath)
					if err != nil {
						log.Errorf("read file failed: %v", err)
						continue
					}
					mime := mimetype.Detect(data)
					ch <- &ImageResult{
						RawImage: data,
						MIMEType: mime,
					}
					// os.Remove(filepath.Join(outputTmp, fileName))
				}
				select {
				case <-finishedCtx.Done():
					return
				default:
				}
			}
		}()

		err := cmd.Run()
		if err != nil {
			log.Errorf("page2img command failed: %v", err)
			log.Errorf("page2img stdout: %s", outBuf.String())
			log.Errorf("page2img stderr: %s", errBuf.String())
			return
		}
	}()
	return ch, nil
}
