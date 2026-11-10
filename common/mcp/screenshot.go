package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const screenshotInlineLimit = 1024 * 1024        // Limit the encoded image, not the raw PNG.
const screenshotTransferLimit = 30 * 1024 * 1024 // Matches the desktop's Base64 transfer limit.

type screenshotOptions struct {
	SavePath       string `json:"savePath"`
	MaxInlineBytes int    `json:"maxInlineBytes"`
}

func init() {
	AddGlobalToolSet("screenshot", WithTool(mcp.NewTool("screenshot",
		mcp.WithDescription("Capture the visible Yakit page as a PNG with a bottom-left desktop local-time watermark. Requires one capable frontend connected to this engine. Images up to 1 MiB of Base64 are returned inline; larger images are saved under the MCP engine's YAKIT_HOME/screenshots/YYYY-MM-DD/. File results contain an engine-local absolute path and metadata, without image data. Set savePath or maxInlineBytes=0 to always save a file."),
		mcp.WithString("savePath", mcp.Description("Optional PNG file path on the MCP engine machine. Absolute paths are accepted; relative paths are resolved under YAKIT_HOME/screenshots (normally ~/yakit-projects/screenshots). Parent directories are created. Existing files are never overwritten. Specifying a path returns file metadata only.")),
		mcp.WithInteger("maxInlineBytes", mcp.Description("Maximum Base64 image bytes returned inline, from 0 to 1048576. Default 1048576 (1 MiB). Set 0 to always save to a file. Larger images are automatically saved without resizing or losing detail."), mcp.Default(screenshotInlineLimit), mcp.Min(0), mcp.Max(screenshotInlineLimit)),
	), handleScreenshot))
}

func handleScreenshot(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		options, err := decodeScreenshotOptions(request.Params.Arguments)
		if err != nil {
			return nil, err
		}
		result, err := yakit.RequestYakitScreenshot(ctx)
		if err != nil {
			return nil, err
		}
		toolResult, err := screenshotToolResult(result, options)
		if err != nil {
			return nil, err
		}
		for i, content := range toolResult.Content {
			text, ok := content.(mcp.TextContent)
			if !ok {
				continue
			}
			var metadata map[string]any
			if json.Unmarshal([]byte(text.Text), &metadata) != nil || metadata["delivery"] != "file" {
				continue
			}
			path, _ := metadata["path"].(string)
			img, err := aimedia.FromFile(ctx, path)
			if err != nil {
				return nil, err
			}
			now := time.Now()
			count := 0
			s.screenshotResources.Range(func(key, value any) bool {
				entry := value.(screenshotResource)
				if now.After(entry.Expires) {
					s.screenshotResources.Delete(key)
				} else {
					count++
				}
				return true
			})
			if count >= 256 {
				return nil, fmt.Errorf("too many retained screenshot resources; expired handles are cleared after ten minutes")
			}
			uri := "yakit-screenshot://" + uuid.NewString()
			s.screenshotResources.Store(uri, screenshotResource{Image: img, Expires: now.Add(10 * time.Minute)})
			metadata["resourceUri"] = uri
			raw, _ := json.Marshal(metadata)
			text.Text = string(raw)
			toolResult.Content[i] = text
		}
		return toolResult, nil
	}
}

type screenshotResource struct {
	Image   *aimedia.Image
	Expires time.Time
}

func (s *MCPServer) readScreenshotResource(ctx context.Context, request mcp.ReadResourceRequest) ([]interface{}, error) {
	value, ok := s.screenshotResources.Load(request.Params.URI)
	if !ok {
		return nil, fmt.Errorf("screenshot resource is not available")
	}
	entry := value.(screenshotResource)
	if time.Now().After(entry.Expires) {
		s.screenshotResources.Delete(request.Params.URI)
		return nil, fmt.Errorf("screenshot resource expired")
	}
	raw, err := entry.Image.Bytes(ctx)
	if err != nil {
		return nil, err
	}
	return []interface{}{mcp.BlobResourceContents{ResourceContents: mcp.ResourceContents{URI: request.Params.URI, MIMEType: entry.Image.MIMEType}, Blob: base64.StdEncoding.EncodeToString(raw)}}, nil
}

func decodeScreenshotOptions(arguments map[string]any) (screenshotOptions, error) {
	options := screenshotOptions{MaxInlineBytes: screenshotInlineLimit}
	data, err := json.Marshal(arguments)
	if err != nil {
		return options, fmt.Errorf("invalid screenshot arguments: %w", err)
	}
	if err := json.Unmarshal(data, &options); err != nil {
		return options, fmt.Errorf("invalid screenshot arguments: %w", err)
	}
	if options.MaxInlineBytes < 0 || options.MaxInlineBytes > screenshotInlineLimit {
		return options, fmt.Errorf("maxInlineBytes must be an integer between 0 and %d", screenshotInlineLimit)
	}
	if _, supplied := arguments["savePath"]; supplied {
		if strings.TrimSpace(options.SavePath) == "" {
			return options, fmt.Errorf("savePath must be a non-empty PNG file path")
		}
		options.SavePath, err = resolveScreenshotSavePath(options.SavePath)
	}
	return options, err
}

func resolveScreenshotSavePath(name string) (string, error) {
	if strings.ContainsRune(name, 0) || !strings.EqualFold(filepath.Ext(name), ".png") {
		return "", fmt.Errorf("savePath must be a PNG file path without NUL characters")
	}
	if !filepath.IsAbs(name) {
		name = filepath.Clean(name)
		if name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("relative savePath must stay under YAKIT_HOME/screenshots")
		}
		name = filepath.Join(consts.GetDefaultYakitBaseDir(), "screenshots", name)
	}
	return filepath.Abs(name)
}

func screenshotToolResult(result *yakit.YakitScreenshot, options screenshotOptions) (*mcp.CallToolResult, error) {
	if len(result.Data) > screenshotTransferLimit {
		return nil, fmt.Errorf("screenshot exceeds the 30 MiB Base64 transfer limit")
	}
	data, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid screenshot base64: %w", err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid screenshot PNG: %w", err)
	}
	if config.Width != result.Width || config.Height != result.Height {
		return nil, fmt.Errorf("screenshot dimensions do not match the PNG")
	}
	capturedAt, err := time.Parse("2006-01-02 15:04:05 -07:00", result.CapturedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid screenshot local timestamp: %w", err)
	}
	if options.SavePath != "" || len(result.Data) > options.MaxInlineBytes {
		name := options.SavePath
		reason := "savePath"
		if name == "" {
			name = filepath.Join(capturedAt.Format("2006-01-02"), "screenshot-"+capturedAt.Format("150405")+"-"+uuid.NewString()+".png")
			reason = "inline_limit_exceeded"
			if options.MaxInlineBytes == 0 {
				reason = "file_requested"
			}
		}
		name, err = resolveScreenshotSavePath(name)
		if err != nil {
			return nil, err
		}
		if err := saveScreenshotPNG(name, data); err != nil {
			return nil, err
		}
		return NewCommonCallToolResult(map[string]any{
			"delivery": "file", "path": name, "storageLocation": "mcp_engine",
			"mimeType": "image/png", "width": result.Width, "height": result.Height,
			"capturedAt": result.CapturedAt, "sizeBytes": len(data), "base64Bytes": len(result.Data),
			"maxInlineBytes": options.MaxInlineBytes, "reason": reason,
		})
	}
	return mcp.NewToolResultImage(
		fmt.Sprintf("Yakit screenshot (%d × %d). Local capture time: %s. Time watermark is visible at the bottom left.", result.Width, result.Height, result.CapturedAt),
		result.Data, "image/png",
	), nil
}

func saveScreenshotPNG(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return fmt.Errorf("create screenshot directory: %w", err)
	}
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create screenshot file (existing files are not overwritten): %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(name)
		if writeErr != nil {
			return fmt.Errorf("write screenshot file: %w", writeErr)
		}
		return fmt.Errorf("close screenshot file: %w", closeErr)
	}
	return nil
}
