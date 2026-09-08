package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestScreenshotToolResult(t *testing.T) {
	var buffer bytes.Buffer
	require.NoError(t, png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 800, 600))))
	screenshot := &yakit.YakitScreenshot{
		Data:       base64.StdEncoding.EncodeToString(buffer.Bytes()),
		CapturedAt: "2026-09-08 12:34:56 +08:00", Width: 800, Height: 600,
	}
	result, err := screenshotToolResult(screenshot, screenshotOptions{MaxInlineBytes: screenshotInlineLimit})
	require.NoError(t, err)
	require.Len(t, result.Content, 2)
	content, ok := mcp.AsImageContent(result.Content[1])
	require.True(t, ok)
	require.Equal(t, "image/png", content.MIMEType)
	require.Equal(t, screenshot.Data, content.Data)
	screenshot.Width = 1
	_, err = screenshotToolResult(screenshot, screenshotOptions{MaxInlineBytes: screenshotInlineLimit})
	require.ErrorContains(t, err, "dimensions")
	screenshot.Width = 800
	screenshot.CapturedAt = "UTC time"
	_, err = screenshotToolResult(screenshot, screenshotOptions{MaxInlineBytes: screenshotInlineLimit})
	require.ErrorContains(t, err, "timestamp")
	screenshot.Data = "not base64"
	_, err = screenshotToolResult(screenshot, screenshotOptions{MaxInlineBytes: screenshotInlineLimit})
	require.ErrorContains(t, err, "base64")
	screenshot.Data = base64.StdEncoding.EncodeToString([]byte("not png"))
	_, err = screenshotToolResult(screenshot, screenshotOptions{MaxInlineBytes: screenshotInlineLimit})
	require.ErrorContains(t, err, "PNG")
}

func screenshotPNGFixture(t *testing.T, large bool) *yakit.YakitScreenshot {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 80, 60))
	if large {
		img = image.NewNRGBA(image.Rect(0, 0, 1200, 1200))
		_, err := rand.New(rand.NewSource(1)).Read(img.Pix)
		require.NoError(t, err)
	}
	var buffer bytes.Buffer
	require.NoError(t, png.Encode(&buffer, img))
	return &yakit.YakitScreenshot{
		Data:       base64.StdEncoding.EncodeToString(buffer.Bytes()),
		CapturedAt: "2026-09-08 12:34:56 +08:00", Width: img.Bounds().Dx(), Height: img.Bounds().Dy(),
	}
}

func screenshotFileMetadata(t *testing.T, result *mcp.CallToolResult, screenshot *yakit.YakitScreenshot) map[string]any {
	t.Helper()
	require.Len(t, result.Content, 1, "file responses must not include an image content block")
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	require.Less(t, len(content.Text), 2048, "file response must stay small")
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(content.Text), &metadata))
	require.Equal(t, "file", metadata["delivery"])
	require.Equal(t, "mcp_engine", metadata["storageLocation"])
	require.NotContains(t, metadata, "data")
	data, err := os.ReadFile(metadata["path"].(string))
	require.NoError(t, err)
	require.Equal(t, screenshot.Data, base64.StdEncoding.EncodeToString(data), "saved PNG must retain all original bytes")
	require.EqualValues(t, len(data), metadata["sizeBytes"])
	require.EqualValues(t, len(screenshot.Data), metadata["base64Bytes"])
	return metadata
}

func TestScreenshotEncodedSizeBoundary(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	screenshot := screenshotPNGFixture(t, false)
	data, err := base64.StdEncoding.DecodeString(screenshot.Data)
	require.NoError(t, err)
	for _, threshold := range []int{len(screenshot.Data), len(screenshot.Data) - 1, len(data), 0} {
		result, err := screenshotToolResult(screenshot, screenshotOptions{MaxInlineBytes: threshold})
		require.NoError(t, err)
		if threshold == len(screenshot.Data) {
			require.Len(t, result.Content, 2)
			continue
		}
		metadata := screenshotFileMetadata(t, result, screenshot)
		require.Equal(t, filepath.Join(os.Getenv("YAKIT_HOME"), "screenshots", "2026-09-08"), filepath.Dir(metadata["path"].(string)))
		if threshold == 0 {
			require.Equal(t, "file_requested", metadata["reason"])
		} else {
			require.Equal(t, "inline_limit_exceeded", metadata["reason"])
		}
	}
}

func TestScreenshotLargePNGAutomaticallySaved(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	screenshot := screenshotPNGFixture(t, true)
	data, err := base64.StdEncoding.DecodeString(screenshot.Data)
	require.NoError(t, err)
	require.Greater(t, len(data), 4*1024*1024)
	options, err := decodeScreenshotOptions(nil)
	require.NoError(t, err)
	result, err := screenshotToolResult(screenshot, options)
	require.NoError(t, err)
	metadata := screenshotFileMetadata(t, result, screenshot)
	require.Equal(t, "inline_limit_exceeded", metadata["reason"])
}

func TestScreenshotExplicitPathAndNoOverwrite(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	screenshot := screenshotPNGFixture(t, false)
	for _, path := range []string{"idor/request-response.png", filepath.Join(t.TempDir(), "evidence.png")} {
		options, err := decodeScreenshotOptions(map[string]any{"savePath": path})
		require.NoError(t, err)
		result, err := screenshotToolResult(screenshot, options)
		require.NoError(t, err)
		metadata := screenshotFileMetadata(t, result, screenshot)
		require.Equal(t, "savePath", metadata["reason"])
		require.Equal(t, options.SavePath, metadata["path"])
		_, err = screenshotToolResult(screenshot, options)
		require.ErrorIs(t, err, os.ErrExist)
		screenshotFileMetadata(t, result, screenshot)
	}
}

func TestScreenshotInvalidOptionsBeforeCapture(t *testing.T) {
	for _, arguments := range []map[string]any{
		{"maxInlineBytes": -1}, {"maxInlineBytes": screenshotInlineLimit + 1},
		{"maxInlineBytes": 1.5}, {"maxInlineBytes": "1024"},
		{"savePath": ""}, {"savePath": "../escape.png"}, {"savePath": "report.jpg"}, {"savePath": "bad\x00.png"},
	} {
		_, err := CallBuiltinTool(&MCPServer{}, context.Background(), "screenshot", arguments)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "frontend", "invalid options must be rejected before requesting a capture")
	}
}

func TestScreenshotSaveFailure(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "file-as-parent")
	require.NoError(t, os.WriteFile(blocked, []byte("original"), 0o600))
	_, err := screenshotToolResult(screenshotPNGFixture(t, false), screenshotOptions{SavePath: filepath.Join(blocked, "capture.png")})
	require.ErrorContains(t, err, "create screenshot directory")
	contents, err := os.ReadFile(blocked)
	require.NoError(t, err)
	require.Equal(t, "original", string(contents))
}
