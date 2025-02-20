package mcp

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
)

func NewCommonCallToolResult(data any) (*mcp.CallToolResult, error) {
	var result string
	switch r := data.(type) {
	case string:
		result = r
	case []any:
		return &mcp.CallToolResult{
			Content: r,
		}, nil
	default:
		resultBytes, err := json.Marshal(data)
		if err != nil {
			return nil, utils.Wrap(err, "failed to marshal response")
		}
		result = string(resultBytes)
	}
	return &mcp.CallToolResult{
		Content: []any{
			mcp.TextContent{
				Type: "text",
				Text: string(result),
			},
		},
	}, nil
}
