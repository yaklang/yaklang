package aitool

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

// convertMCPToolContent handles both in-process SDK structs and the maps
// produced by an actual JSON-RPC round trip. Images never fall through to fmt
// or stdout. File metadata stays metadata: a remote path is not a local file.
func convertMCPToolContent(ctx context.Context, content []any, source aimedia.Source) (any, string, error) {
	var text strings.Builder
	var images []*aimedia.Image
	encodedBytes := 0
	addImage := func(encoded, mimeType string) error {
		if len(images) >= MaxToolImages {
			return fmt.Errorf("MCP tool result exceeds the %d-image limit", MaxToolImages)
		}
		if len(encoded) > base64.StdEncoding.EncodedLen(aimedia.MaxImageBytes)-encodedBytes {
			return fmt.Errorf("MCP tool image payload exceeds the image byte budget")
		}
		encodedBytes += len(encoded)
		img, err := aimedia.FromBase64(ctx, encoded, mimeType)
		if err != nil {
			return fmt.Errorf("MCP tool image %d: %w", len(images)+1, err)
		}
		img.Source = source
		images = append(images, img)
		fmt.Fprintf(&text, "[Image %d: %s, %dx%d, id=%s]", len(images), img.MIMEType, img.Width, img.Height, img.ID)
		return nil
	}
	for _, item := range content {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		var err error
		switch c := item.(type) {
		case mcp.TextContent:
			text.WriteString(c.Text)
		case *mcp.TextContent:
			if c != nil {
				text.WriteString(c.Text)
			}
		case mcp.ImageContent:
			err = addImage(c.Data, c.MIMEType)
		case *mcp.ImageContent:
			if c == nil {
				return nil, "", fmt.Errorf("MCP tool returned a nil image")
			}
			err = addImage(c.Data, c.MIMEType)
		case map[string]any:
			if typ, _ := c["type"].(string); typ == "image" {
				encoded, _ := c["data"].(string)
				mimeType, _ := c["mimeType"].(string)
				err = addImage(encoded, mimeType)
			} else if first := firstMCPTextFromContent([]any{c}); first != "" {
				text.WriteString(first)
			} else {
				fmt.Fprint(&text, c)
			}
		default:
			fmt.Fprint(&text, c)
		}
		if err != nil {
			return nil, "", err
		}
	}
	resultText := text.String()
	if len(images) == 0 {
		return resultText, resultText, nil
	}
	return &MediaResult{Result: resultText, Images: images}, resultText, nil
}
