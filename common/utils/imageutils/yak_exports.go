package imageutils

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"os"
)

// ExtractImage extracts images from various input types, such as io.Reader, []byte, or string.
func ExtractImage(i any) chan *ImageResult {
	switch ret := i.(type) {
	case io.Reader:
		bytes, _ := io.ReadAll(ret)
		return ExtractWildStringImage(bytes)
	default:
		return ExtractWildStringImage(codec.AnyToBytes(ret))
	}
}

// ExtractImageFromFile extract images from a file path,
// we can handle some video formats, PDF, and other files that may contain images.
func ExtractImageFromFile(filePath string, options ...ImageExtractorOption) (chan *ImageResult, error) {
	config := &ImageExtractorConfig{
		ctx: context.Background(),
	}
	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	mt, mtErr := mimetype.DetectFile(filePath)
	if mtErr != nil {
		return nil, utils.Errorf("cannot fetch mimetype for file %s: %v", filePath, mtErr)
	}

	var result chan *ImageResult
	var err error

	if mt.IsVideo() {
		result, err = ExtractVideoFrameContext(config.ctx, filePath)
		if err != nil {
			return nil, utils.Errorf("cannot extract video frames for file %s: %v", filePath, err)
		}
		return result, nil
	} else if mt.IsImage() {
		fp, err := os.OpenFile(filePath, os.O_RDONLY, os.ModePerm)
		if err != nil {
			return nil, utils.Errorf("cannot open file %s: %v", filePath, err)
		}
		var ch = make(chan *ImageResult)
		go func() {
			defer close(ch)
			defer fp.Close()
			count := 0
			for i := range ExtractImage(fp) {
				if i == nil {
					continue
				}
				count++
				ch <- i
			}
			if count <= 0 {
				log.Errorf("no images extracted from file %s", filePath)
			}
		}()
		return ch, nil
	} else {
		result, err = ExtractDocumentPagesContext(config.ctx, filePath)
		if err != nil {
			return nil, utils.Errorf("cannot extract document pages for file %s: %v", filePath, err)
		}
		return result, nil
	}
}

type ImageExtractorConfig struct {
	ctx context.Context
}

type ImageExtractorOption func(*ImageExtractorConfig)

func WithCtx(ctx context.Context) ImageExtractorOption {
	return func(o *ImageExtractorConfig) {
		o.ctx = ctx
	}
}

var Exports = map[string]any{
	"ExtractImage":         ExtractImage,
	"ExtractImageFromFile": ExtractImageFromFile,

	"context": WithCtx,
}
