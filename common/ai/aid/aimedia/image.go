// Package aimedia owns validated image inputs independently of prompts and model
// providers. Binary data must never be serialized into ordinary tool logs.
package aimedia

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

// These are decoding limits, not token estimates or compression targets.
const (
	MaxImageBytes  = 20 * 1024 * 1024
	MaxImagePixels = 40_000_000
)

// Source identifies an observation, not the image's content identity. Two tool
// calls can observe identical pixels without being the same event.
type Source struct {
	Kind             string    `json:"kind,omitempty"`
	ToolName         string    `json:"tool_name,omitempty"`
	ServerName       string    `json:"server_name,omitempty"`
	ToolCallID       string    `json:"tool_call_id,omitempty"`
	TaskIndex        string    `json:"task_index,omitempty"`
	TurnID           string    `json:"turn_id,omitempty"`
	CapturedAt       time.Time `json:"captured_at,omitempty"`
	ObservedAt       time.Time `json:"observed_at,omitempty"`
	CoordinateSpace  string    `json:"coordinate_space,omitempty"`
	DevicePixelRatio float64   `json:"device_pixel_ratio,omitempty"`
	Ordinal          int       `json:"ordinal,omitempty"`
	OriginalName     string    `json:"original_name,omitempty"`
	OriginalImageID  string    `json:"original_image_id,omitempty"`
	OriginalWidth    int       `json:"original_width,omitempty"`
	OriginalHeight   int       `json:"original_height,omitempty"`
	CropX            int       `json:"crop_x,omitempty"`
	CropY            int       `json:"crop_y,omitempty"`
	CropWidth        int       `json:"crop_width,omitempty"`
	CropHeight       int       `json:"crop_height,omitempty"`
}

// Image is a model-independent resource reference. Only metadata is serialized;
// inline bytes live in memory until the owning runtime persists the resource.
type Image struct {
	ID        string `json:"id"`
	MIMEType  string `json:"mime_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	Path      string `json:"path,omitempty"`
	Source    Source `json:"source"`
	data      []byte
}

func (i *Image) String() string {
	if i == nil {
		return "<nil image>"
	}
	return fmt.Sprintf("[Image %s: %s, %dx%d, %d bytes]", i.ID, i.MIMEType, i.Width, i.Height, i.SizeBytes)
}

// GoString also keeps %#v diagnostic output from leaking inline image bytes.
func (i *Image) GoString() string { return i.String() }

func FromBytes(ctx context.Context, raw []byte, mimeType string) (*Image, error) {
	if ctx == nil {
		return nil, fmt.Errorf("image input context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) == 0 || len(raw) > MaxImageBytes {
		return nil, fmt.Errorf("image must contain 1..%d bytes", MaxImageBytes)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid image header: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width) > MaxImagePixels/int64(config.Height) {
		return nil, fmt.Errorf("image dimensions exceed %d pixels", MaxImagePixels)
	}
	actualMIME := "image/" + format
	if mimeType != "" && !strings.EqualFold(strings.TrimSpace(mimeType), actualMIME) {
		return nil, fmt.Errorf("image MIME type %q does not match %s", mimeType, actualMIME)
	}
	// Check the complete image before saving or sending it; a valid header alone
	// is not evidence of a decodable image. The pixel limit bounds this decode.
	if _, _, err := image.Decode(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("invalid image data: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	return &Image{
		ID: "image-" + hash, MIMEType: actualMIME,
		Width: config.Width, Height: config.Height, SizeBytes: int64(len(raw)),
		SHA256: hash, data: bytes.Clone(raw),
	}, nil
}

func FromBase64(ctx context.Context, encoded, mimeType string) (*Image, error) {
	if ctx == nil {
		return nil, fmt.Errorf("image input context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(MaxImageBytes) {
		return nil, fmt.Errorf("encoded image exceeds the %d-byte image limit", MaxImageBytes)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Do not include the encoded payload in error messages.
		return nil, fmt.Errorf("invalid image base64: %w", err)
	}
	return FromBytes(ctx, raw, mimeType)
}

// FromFile reads an explicitly supplied, local file. It never interprets URLs
// or remote MCP paths as local filesystem authority.
func FromFile(ctx context.Context, path string) (*Image, error) {
	raw, err := readImageFile(ctx, path)
	if err != nil {
		return nil, err
	}
	img, err := FromBytes(ctx, raw, "")
	if err != nil {
		return nil, err
	}
	img.Path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	img.data = nil
	return img, nil
}

func readImageFile(ctx context.Context, path string) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("image input context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("image resource is not a regular file")
	}
	if info.Size() > MaxImageBytes {
		return nil, fmt.Errorf("image file exceeds %d bytes", MaxImageBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) > MaxImageBytes {
		return nil, fmt.Errorf("image file exceeds %d bytes", MaxImageBytes)
	}
	return raw, nil
}

// Bytes returns an independent buffer and verifies persisted resources have not
// changed. A metadata-only reference is not proof the image is still available.
func (i *Image) Bytes(ctx context.Context) ([]byte, error) {
	if i == nil {
		return nil, fmt.Errorf("image is nil")
	}
	if ctx == nil {
		return nil, fmt.Errorf("image input context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw := i.data
	if raw == nil {
		if i.Path == "" {
			return nil, fmt.Errorf("image %s has no available data", i.ID)
		}
		var err error
		raw, err = readImageFile(ctx, i.Path)
		if err != nil {
			return nil, err
		}
	}
	digest := sha256.Sum256(raw)
	if int64(len(raw)) != i.SizeBytes || hex.EncodeToString(digest[:]) != i.SHA256 {
		return nil, fmt.Errorf("image %s content does not match its reference", i.ID)
	}
	return bytes.Clone(raw), nil
}
