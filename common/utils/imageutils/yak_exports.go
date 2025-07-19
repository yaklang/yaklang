package imageutils

import (
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

var Exports = map[string]any{
	"ExtractImage": ExtractImage,
}
