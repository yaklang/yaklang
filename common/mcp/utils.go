package mcp

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
)

func NewCommonCallToolResult(data any) (*mcp.CallToolResult, error) {
	rspBytes, err := json.Marshal(data)
	if err != nil {
		return nil, utils.Wrap(err, "failed to marshal response")
	}
	return &mcp.CallToolResult{
		Content: []any{
			mcp.TextContent{
				Type: "text",
				Text: string(rspBytes),
			},
		},
	}, nil
}
