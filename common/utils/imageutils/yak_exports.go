package imageutils

import (
	"context"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
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
func ExtractImageFromFile(filePath string) (chan *ImageResult, error) {
	ctx := context.Background()

	mt, mtErr := mimetype.DetectFile(filePath)
	if mtErr != nil {
		return nil, utils.Errorf("cannot fetch mimetype for file %s: %v", filePath, mtErr)
	}

	var result chan *ImageResult
	var err error

	if mt.IsVideo() {
		result, err = ExtractVideoFrameContext(ctx, filePath)
		if err != nil {
			return nil, utils.Errorf("cannot extract video frames for file %s: %v", filePath, err)
		}
		return result, nil
	} else {
		result, err = ExtractDocumentPagesContext(ctx, filePath)
		if err != nil {
			return nil, utils.Errorf("cannot extract document pages for file %s: %v", filePath, err)
		}
		return result, nil
	}
}

var Exports = map[string]any{
	"ExtractImage":         ExtractImage,
	"ExtractImageFromFile": ExtractImageFromFile,
}
