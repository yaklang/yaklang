package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
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

func (s *MCPServer) downloadAndExecYakScript(ctx context.Context, taskName, scriptName, scriptUUID string, request mcp.CallToolRequest, handler func(ypb.Yak_DebugPluginClient, string) (*mcp.CallToolResult, error)) (*mcp.CallToolResult, error) {
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

// normalizeMCPArguments fixes common MCP argument shapes before mapstructure decode.
func normalizeMCPArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return nil
	}
	args := maps.Clone(arguments)

	if nested, ok := args["request"].(map[string]any); ok {
		for k, v := range nested {
			if _, exists := args[k]; !exists {
				args[k] = v
			}
		}
		delete(args, "request")
	}
	if group, ok := args["group"].(map[string]any); ok {
		for k, v := range group {
			if _, exists := args[k]; !exists {
				args[k] = v
			}
		}
		delete(args, "group")
	}
	if rule, ok := args["rule"]; ok {
		if _, exists := args["syntaxFlowInput"]; !exists {
			args["syntaxFlowInput"] = rule
		}
	}
	if fp, ok := args["fingerprint"]; ok {
		if _, exists := args["rule"]; !exists {
			args["rule"] = fp
		}
		delete(args, "fingerprint")
	}
	if obj, ok := args["object"].(map[string]any); ok {
		if data, ok := obj["data"]; ok {
			if _, exists := args["data"]; !exists {
				args["data"] = data
			}
		}
		delete(args, "object")
	}
	if utils.InterfaceToString(args["gadget"]) == "" {
		args["gadget"] = "URLDNS"
	}
	if rules, ok := args["rules"].(map[string]any); ok && len(rules) == 0 {
		args["rules"] = []any{}
	}
	if raw, ok := args["jsonRaw"]; ok {
		switch v := raw.(type) {
		case string:
			args["jsonRaw"] = []byte(v)
		case []any:
			args["jsonRaw"] = utils.InterfaceToBytes(v)
		}
	}
	if data, ok := args["data"].(string); ok {
		if raw, err := base64.StdEncoding.DecodeString(data); err == nil && len(raw) > 0 {
			args["data"] = raw
		}
	}
	if _, ok := args["filter"]; !ok {
		if _, hasPagination := args["pagination"]; hasPagination {
			args["filter"] = map[string]any{}
		}
	}
	return args
}

func decodeYakRequest(arguments map[string]any, req any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decodeHook,
		Result:     req,
	})
	if err != nil {
		return utils.Wrap(err, "BUG: new map structure decoder error")
	}
	if err := decoder.Decode(normalizeMCPArguments(arguments)); err != nil {
		return utils.Wrap(err, "invalid argument")
	}
	return nil
}

type execResultReceiver interface {
	Recv() (*ypb.ExecResult, error)
}

type backgroundStreamStatus struct {
	Name      string         `json:"name"`
	StartedAt time.Time      `json:"startedAt"`
	Summary   map[string]any `json:"summary"`
	Logs      []string       `json:"logs,omitempty"`
}

var backgroundStreams sync.Map

func storeBackgroundStreamStatus(name string, summary map[string]any) {
	backgroundStreams.Store(name, &backgroundStreamStatus{Name: name, StartedAt: time.Now(), Summary: summary})
}

func appendBackgroundStreamLog(name, line string) {
	if v, ok := backgroundStreams.Load(name); ok {
		if st, ok := v.(*backgroundStreamStatus); ok {
			st.Logs = append(st.Logs, line)
			backgroundStreams.Store(name, st)
		}
	}
}

func startBackgroundExecStream(
	s *MCPServer,
	name string,
	summary map[string]any,
	startFn func(context.Context) (execResultReceiver, error),
) (*mcp.CallToolResult, error) {
	stream, err := startFn(context.Background())
	if err != nil {
		return nil, err
	}
	storeBackgroundStreamStatus(name, summary)
	go func() {
		for {
			rsp, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					log.Warnf("background stream %s recv: %v", name, err)
				}
				return
			}
			if rsp != nil {
				if content := handleExecMessage(string(rsp.Message)); content != "" {
					appendBackgroundStreamLog(name, content)
				}
			}
		}
	}()
	return NewCommonCallToolResult(map[string]any{"status": "started", "name": name, "summary": summary})
}

func unaryToolHandler[T any](
	call func(context.Context, *MCPServer, *T) (any, error),
	errMsg string,
) ToolHandlerWrapperFunc {
	return func(s *MCPServer) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var req T
			if request.Params.Arguments != nil {
				if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
					return nil, err
				}
			}
			rsp, err := call(ctx, s, &req)
			if err != nil {
				return nil, utils.Wrap(err, errMsg)
			}
			return NewCommonCallToolResult(rsp)
		}
	}
}

func unaryEmptyToolHandler(
	call func(context.Context, *MCPServer) (any, error),
	errMsg string,
) ToolHandlerWrapperFunc {
	return func(s *MCPServer) server.ToolHandlerFunc {
		return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			rsp, err := call(ctx, s)
			if err != nil {
				return nil, utils.Wrap(err, errMsg)
			}
			return NewCommonCallToolResult(rsp)
		}
	}
}
