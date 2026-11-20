package aitool

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
)

const MaxToolImages = 16

// MediaResult is the explicit callback channel for images. Result retains the
// tool's semantic result; images are moved out before ordinary text capture,
// rendering, artifact shrinking or checkpoint serialization.
type MediaResult struct {
	Result any              `json:"result"`
	Images []*aimedia.Image `json:"images,omitempty"`
}

// ImageResult is the explicit image return channel for Go and Yak-backed tools
// (aiagent.ImageResult). Callers supply their invocation context and semantic
// result; ordinary logging remains independent of the image bytes.
func ImageResult(ctx context.Context, raw []byte, mimeType string, result any) (*MediaResult, error) {
	img, err := aimedia.FromBytes(ctx, raw, mimeType)
	if err != nil {
		return nil, err
	}
	return &MediaResult{Result: result, Images: []*aimedia.Image{img}}, nil
}

func (r *MediaResult) String() string {
	if r == nil {
		return "<nil media result>"
	}
	return fmt.Sprintf("[Tool media result: %d images]", len(r.Images))
}

func splitToolMedia(ctx context.Context, value any) (any, []*aimedia.Image, error) {
	var media *MediaResult
	switch result := value.(type) {
	case *MediaResult:
		media = result
	case MediaResult:
		media = &result
	default:
		return value, nil, nil
	}
	if media == nil {
		return nil, nil, fmt.Errorf("tool media result is nil")
	}
	if len(media.Images) > MaxToolImages {
		return nil, nil, fmt.Errorf("tool result exceeds the %d-image limit", MaxToolImages)
	}
	var totalBytes int64
	images := make([]*aimedia.Image, 0, len(media.Images))
	for _, img := range media.Images {
		if img == nil {
			return nil, nil, fmt.Errorf("tool result contains a nil image")
		}
		if img.SizeBytes <= 0 || img.SizeBytes > aimedia.MaxImageBytes-totalBytes {
			return nil, nil, fmt.Errorf("tool image data exceeds %d bytes", aimedia.MaxImageBytes)
		}
		totalBytes += img.SizeBytes
		// Snapshot the validated bytes. A callback-owned reference or buffer may
		// be reused after return; it must not alter an accepted tool result.
		raw, err := img.Bytes(ctx)
		if err != nil {
			return nil, nil, err
		}
		owned, err := aimedia.FromBytes(ctx, raw, img.MIMEType)
		if err != nil {
			return nil, nil, err
		}
		owned.Source = img.Source
		images = append(images, owned)
	}
	return media.Result, images, nil
}
