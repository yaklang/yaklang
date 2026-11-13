package aitool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

// resolveMCPImageResources only asks the originating MCP server to read an
// explicitly declared image resource. It never opens a remote engine's path or
// fetches a URL using the agent's own filesystem/network authority.
type MCPImageResourceReader interface {
	ReadResource(context.Context, mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
}

// mcpToolExecutionError carries a remote tool failure and its decoded result.
// Transport and invocation errors have no such result.
type mcpToolExecutionError struct {
	message string
	result  any
}

func (e *mcpToolExecutionError) Error() string { return e.message }

func MCPToolMediaResult(ctx context.Context, c MCPImageResourceReader, result *mcp.CallToolResult, source aimedia.Source) (any, error) {
	value, _, err := convertMCPToolResult(ctx, c, result, source)
	if _, ok := value.(string); ok {
		return result, nil
	}
	return value, err
}

func convertMCPToolResult(ctx context.Context, c MCPImageResourceReader, result *mcp.CallToolResult, source aimedia.Source) (any, string, error) {
	if result == nil {
		return nil, "", fmt.Errorf("MCP tool returned no result")
	}
	contents, err := resolveMCPImageResources(ctx, c, result.Content)
	var converted mcpToolContent
	if err == nil {
		converted, err = convertMCPToolContent(ctx, contents, source)
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		if !result.IsError {
			return nil, "", err
		}
		// Preserve the original failure and its structured recovery data when
		// a screenshot cannot be decoded or a resource cannot be read.
		for _, block := range result.Content {
			converted.blocks = append(converted.blocks, mcp.NewTextContent(mcpTextBlock(block)))
		}
		converted.blocks = append(converted.blocks, mcp.NewTextContent("MCP recovery media unavailable."))
	}
	value := any(converted.text)
	if result.StructuredContent != nil || result.IsError {
		summary := *result
		summary.Content = converted.blocks
		value = &summary
	}
	if len(converted.images) != 0 {
		value = &MediaResult{Result: value, Images: converted.images}
	}
	if result.IsError {
		message := firstMCPTextFromContent(result.Content)
		if message == "" {
			message = "remote MCP tool failed"
		}
		return value, converted.text, &mcpToolExecutionError{message: message, result: value}
	}
	return value, converted.text, nil
}

func resolveMCPImageResources(ctx context.Context, c MCPImageResourceReader, contents []any) ([]any, error) {
	var out []any
	reads := 0
	fileReference := func(text string) (string, string) {
		var metadata struct {
			Delivery string `json:"delivery"`
			URI      string `json:"resourceUri"`
			MIME     string `json:"mimeType"`
		}
		if json.Unmarshal([]byte(text), &metadata) == nil && metadata.Delivery == "file" {
			return metadata.URI, metadata.MIME
		}
		return "", ""
	}
	for _, content := range contents {
		out = append(out, content)
		var uri, mimeType string
		switch v := content.(type) {
		case mcp.TextContent:
			uri, mimeType = fileReference(v.Text)
		case *mcp.TextContent:
			if v != nil {
				uri, mimeType = fileReference(v.Text)
			}
		case mcp.EmbeddedResource:
			uri, mimeType = v.Resource.URI, v.Resource.MIMEType
		case *mcp.EmbeddedResource:
			if v != nil {
				uri, mimeType = v.Resource.URI, v.Resource.MIMEType
			}
		case map[string]any:
			typ, _ := v["type"].(string)
			if typ == "text" {
				text, _ := v["text"].(string)
				uri, mimeType = fileReference(text)
			}
			if typ == "resource_link" {
				uri, _ = v["uri"].(string)
				mimeType, _ = v["mimeType"].(string)
			}
			if typ == "resource" {
				if resource, ok := v["resource"].(map[string]any); ok {
					blob, _ := resource["blob"].(string)
					mime, _ := resource["mimeType"].(string)
					if strings.HasPrefix(mime, "image/") {
						if blob != "" {
							out[len(out)-1] = mcp.NewImageContent(blob, mime)
						} else {
							uri, _ = resource["uri"].(string)
							mimeType = mime
						}
					}
				}
			}
		}
		if uri == "" || !strings.HasPrefix(mimeType, "image/") {
			continue
		}
		reads++
		if reads > MaxToolImages {
			return nil, fmt.Errorf("too many MCP image resources")
		}
		request := mcp.ReadResourceRequest{}
		request.Params.URI = uri
		resource, err := c.ReadResource(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("read MCP image resource: %w", err)
		}
		if resource == nil {
			return nil, fmt.Errorf("MCP image resource returned no data")
		}
		found := false
		for _, item := range resource.Contents {
			raw, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			var blob struct {
				URI  string `json:"uri"`
				MIME string `json:"mimeType"`
				Blob string `json:"blob"`
			}
			if err := json.Unmarshal(raw, &blob); err != nil {
				return nil, err
			}
			if blob.URI != uri || !strings.HasPrefix(blob.MIME, "image/") || blob.Blob == "" {
				return nil, fmt.Errorf("MCP image resource returned mismatched content")
			}
			out = append(out, fetchedMCPImage{mcp.NewImageContent(blob.Blob, blob.MIME)})
			found = true
		}
		if !found {
			return nil, fmt.Errorf("MCP image resource is empty")
		}
	}
	return out, nil
}
