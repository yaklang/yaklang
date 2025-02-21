package mcp

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func handleExecMessage(exec *ypb.ExecResult) string {
	content := string(exec.Message)
	// handle complex message
	msgContent := gjson.GetBytes(exec.Message, "content")
	level := msgContent.Get("level").String()
	switch level {
	case "feature-status-card-data":
		data := msgContent.Get("data").String()
		// ignore empty risk message
		if gjson.Get(data, "id").String() == "漏洞/风险/指纹" {
			cardCount := int(gjson.Get(data, "data").Int())
			if cardCount == 0 {
				return ""
			}
		}
	case "info", "json":
		// use content directly
		content = msgContent.Get("data").String()
	}
	return content
}

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
