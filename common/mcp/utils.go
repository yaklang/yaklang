package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

func decodeHookGenerator(sep string) func(from reflect.Type, to reflect.Type, v any) (any, error) {
	return func(from reflect.Type, to reflect.Type, v any) (any, error) {
		if to.Kind() == reflect.String {
			if from.Kind() == reflect.Slice {
				slice := utils.InterfaceToSliceInterface(v)
				stringSlice := lo.Map(slice, func(item any, _ int) string {
					return utils.InterfaceToString(item)
				})
				return strings.Join(stringSlice, sep), nil
			}
		} else if to.Kind() == reflect.Slice && to.Elem().Kind() == reflect.Uint8 {
			if from.Kind() == reflect.Slice {
				slice := utils.InterfaceToSliceInterface(v)
				bytesSlice := lo.Map(slice, func(item any, _ int) []byte {
					return utils.InterfaceToBytes(item)
				})
				return bytes.Join(bytesSlice, []byte(sep)), nil
			}
		}
		return v, nil
	}
}

func decodeHook(from reflect.Type, to reflect.Type, v any) (any, error) {
	return decodeHookGenerator("\n")(from, to, v)
}

func handleExecMessage(content string) string {
	if content == "" {
		return ""
	}
	// handle complex message
	msgContent := gjson.Get(content, "content")
	level := msgContent.Get("level").String()
	switch level {
	case "feature-status-card-data", "json-feature":
		return ""
	case "info", "json", "feature-table-data", "json-risk":
		// use content directly
		content = msgContent.Get("data").String()
	}
	return content
}

func (s *MCPServer) commonExecYakScript(ctx context.Context, taskName, scriptName, scriptUUID string, request mcp.CallToolRequest, handler func(ypb.Yak_DebugPluginClient, string) (*mcp.CallToolResult, error)) (*mcp.CallToolResult, error) {
	var script *ypb.YakScript

	rsp, err := s.grpcClient.QueryYakScript(ctx, &ypb.QueryYakScriptRequest{
		UUID: scriptUUID,
	})
	if err != nil {
		return nil, utils.Wrapf(err, "failed to query yak script[%s]", scriptName)
	}

	if rsp.Total == 0 {
		// if not exist, download
		script, err = s.grpcClient.DownloadOnlinePluginByUUID(ctx, &ypb.DownloadOnlinePluginByUUIDRequest{
			UUID: scriptUUID,
		})
		if err != nil {
			return nil, utils.Wrapf(err, "failed to download yak script[%s]", scriptName)
		}
	} else {
		script = rsp.Data[0]
	}

	req := ypb.DebugPluginRequest{
		PluginType: script.Type,
		PluginName: script.ScriptName,
	}
	args := request.Params.Arguments
	req.ExecParams = make([]*ypb.KVPair, 0, len(args))
	for k, v := range args {
		req.ExecParams = append(req.ExecParams, &ypb.KVPair{
			Key:   k,
			Value: utils.InterfaceToString(v),
		})
	}

	stream, err := s.grpcClient.DebugPlugin(ctx, &req)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to exec %s", taskName)
	}

	return handler(stream, taskName)
}
