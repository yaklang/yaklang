package aimedia

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math"

	"golang.org/x/image/draw"
)

// View records the mapping from model image pixels to original image pixels.
// Browser CSS/device coordinates require the caller's additional device scale.
type View struct {
	Image  *Image `json:"image"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (v View) OriginalPoint(x, y float64) (float64, float64, error) {
	if v.Image == nil || x < 0 || y < 0 || x > float64(v.Image.Width) || y > float64(v.Image.Height) {
		return 0, 0, fmt.Errorf("point is outside the model image")
	}
	return float64(v.X) + x*float64(v.Width)/float64(v.Image.Width), float64(v.Y) + y*float64(v.Height)/float64(v.Image.Height), nil
}

// Derive preserves the original. Empty crop selects the full image; bounds are
// strict, and a tiny crop cannot silently turn into a completely different view.
func Derive(ctx context.Context, original *Image, crop image.Rectangle, maxEdge int) (*View, error) {
	raw, err := original.Bytes(ctx)
	if err != nil {
		return nil, err
	}
	bounds := image.Rect(0, 0, original.Width, original.Height)
	if crop == (image.Rectangle{}) {
		crop = bounds
	}
	if crop.Empty() || !crop.In(bounds) {
		return nil, fmt.Errorf("crop must be inside the original image")
	}
	if maxEdge <= 0 {
		return nil, fmt.Errorf("maximum image edge must be positive")
	}
	w, h := crop.Dx(), crop.Dy()
	scale := math.Min(1, float64(maxEdge)/float64(max(w, h)))
	tw, th := max(1, int(math.Round(float64(w)*scale))), max(1, int(math.Round(float64(h)*scale)))
	if crop == bounds && tw == w && th == h && (original.MIMEType == "image/png" || original.MIMEType == "image/jpeg" || original.MIMEType == "image/webp") {
		return &View{Image: original, Width: w, Height: h}, nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(dst, dst.Bounds(), decoded, crop, draw.Src, nil)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, dst); err != nil {
		return nil, err
	}
	derived, err := FromBytes(ctx, encoded.Bytes(), "image/png")
	if err != nil {
		return nil, err
	}
	derived.Source = original.Source
	return &View{Image: derived, X: crop.Min.X, Y: crop.Min.Y, Width: w, Height: h}, nil
}

// EstimateImageTokens estimates model-visible cost, never PNG/base64 bytes.
// Conservative is deliberately an upper local heuristic, not a billing claim.
// Actual provider input usage is observed separately; its difference from a
// local text estimate is not a measurement of visual tokens.
func EstimateImageTokens(width, height int, estimator string) int64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	patches := ((int64(width) + 31) / 32) * ((int64(height) + 31) / 32)
	tiles := ((int64(width) + 511) / 512) * ((int64(height) + 511) / 512)
	switch estimator {
	case "patch32":
		return patches
	case "tile512":
		return 85 + 170*tiles
	default:
		return max(256+4*patches, 85+170*tiles)
	}
}
