package aitool

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

// fetchedMCPImage is derived from an existing resource block. It has no
// separate index in the server's content array.
type fetchedMCPImage struct{ mcp.ImageContent }

type mcpToolContent struct {
	blocks []any
	images []*aimedia.Image
	text   string
}

// convertMCPToolContent handles both in-process SDK structs and the maps
// produced by an actual JSON-RPC round trip. Images never fall through to fmt
// or stdout. File metadata stays metadata: a remote path is not a local file.
func convertMCPToolContent(ctx context.Context, content []any, source aimedia.Source) (mcpToolContent, error) {
	var converted mcpToolContent
	var text strings.Builder
	encodedBytes := 0
	addImage := func(encoded, mimeType string) (*aimedia.Image, error) {
		if len(converted.images) >= MaxToolImages {
			return nil, fmt.Errorf("MCP tool result exceeds the %d-image limit", MaxToolImages)
		}
		if len(encoded) > base64.StdEncoding.EncodedLen(aimedia.MaxImageBytes)-encodedBytes {
			return nil, fmt.Errorf("MCP tool image payload exceeds the image byte budget")
		}
		encodedBytes += len(encoded)
		img, err := aimedia.FromBase64(ctx, encoded, mimeType)
		if err != nil {
			return nil, fmt.Errorf("MCP tool image %d: %w", len(converted.images)+1, err)
		}
		img.Source = source
		converted.images = append(converted.images, img)
		fmt.Fprintf(&text, "[Image %d: %s, %dx%d, id=%s]", len(converted.images), img.MIMEType, img.Width, img.Height, img.ID)
		return img, nil
	}
	for _, item := range content {
		if err := ctx.Err(); err != nil {
			return mcpToolContent{}, err
		}
		block := item
		var image *aimedia.Image
		var imageErr error
		switch c := item.(type) {
		case mcp.TextContent:
			text.WriteString(c.Text)
		case *mcp.TextContent:
			if c != nil {
				text.WriteString(c.Text)
			}
		case fetchedMCPImage:
			if _, err := addImage(c.Data, c.MIMEType); err != nil {
				return mcpToolContent{}, err
			}
			continue
		case mcp.ImageContent:
			image, imageErr = addImage(c.Data, c.MIMEType)
		case *mcp.ImageContent:
			if c == nil {
				return mcpToolContent{}, fmt.Errorf("MCP tool returned a nil image")
			}
			image, imageErr = addImage(c.Data, c.MIMEType)
		case map[string]any:
			if typ, _ := c["type"].(string); typ == "image" {
				encoded, _ := c["data"].(string)
				mimeType, _ := c["mimeType"].(string)
				image, imageErr = addImage(encoded, mimeType)
			} else if blockText := mcpTextBlock(c); blockText != "" {
				text.WriteString(blockText)
			} else {
				fmt.Fprint(&text, c)
			}
		default:
			fmt.Fprint(&text, c)
		}
		if imageErr != nil {
			return mcpToolContent{}, imageErr
		}
		if image != nil {
			// Keep the original slot so indexes in structuredContent still
			// identify the same content block after binary data is removed.
			block = map[string]any{"type": "image", "mimeType": image.MIMEType,
				"image_id": image.ID, "image_index": len(converted.images) - 1}
		}
		converted.blocks = append(converted.blocks, block)
	}
	converted.text = text.String()
	return converted, nil
}
