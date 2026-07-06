package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func decodeYakRequest(arguments map[string]any, req any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decodeHook,
		Result:     req,
	})
	if err != nil {
		return utils.Wrap(err, "BUG: new map structure decoder error")
	}
	if err := decoder.Decode(arguments); err != nil {
		return utils.Wrap(err, "invalid argument")
	}
	return nil
}

type execResultReceiver interface {
	Recv() (*ypb.ExecResult, error)
}

type collectExecStreamOptions struct {
	progressToken mcp.ProgressToken
	maxMessages   int
}

func collectExecResultStream(
	ctx context.Context,
	s *MCPServer,
	stream execResultReceiver,
	opts collectExecStreamOptions,
) ([]any, error) {
	results := make([]any, 0, 4)
	count := 0
	for {
		rsp, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("[Error] %v", err),
				})
			}
			break
		}
		count++
		if opts.maxMessages > 0 && count > opts.maxMessages {
			break
		}

		exec := rsp
		if exec == nil {
			continue
		}
		content := string(exec.Message)
		content = handleExecMessage(content)
		msgContent := gjson.GetBytes(exec.Message, "content")
		level := msgContent.Get("level").String()
		if content == "" {
			continue
		}
		if level == "json-risk" {
			newContent, err := sjson.Set(content, "Request", strconv.Quote(msgContent.Get("Request").String()))
			if err == nil {
				content = newContent
			}
			newContent, err = sjson.Set(content, "Response", strconv.Quote(msgContent.Get("Response").String()))
			if err == nil {
				content = newContent
			}
		}
		results = append(results, mcp.TextContent{
			Type: "text",
			Text: content,
		})
		if opts.progressToken != nil {
			s.notificationServer(ctx).SendNotificationToClient("notifications/message", map[string]any{
				"level": "info",
				"data":  content,
			})
		}
	}
	return results, nil
}

type backgroundStreamStatus struct {
	Name      string         `json:"name"`
	StartedAt time.Time      `json:"startedAt"`
	Summary   map[string]any `json:"summary"`
	Logs      []string       `json:"logs,omitempty"`
}

var backgroundStreams sync.Map

func storeBackgroundStreamStatus(name string, summary map[string]any) {
	backgroundStreams.Store(name, &backgroundStreamStatus{
		Name:      name,
		StartedAt: time.Now(),
		Summary:   summary,
	})
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
	bgCtx := context.Background()
	stream, err := startFn(bgCtx)
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
			if rsp == nil {
				continue
			}
			content := handleExecMessage(string(rsp.Message))
			if content != "" {
				appendBackgroundStreamLog(name, content)
			}
		}
	}()

	resp := map[string]any{
		"status":  "started",
		"name":    name,
		"summary": summary,
	}
	return NewCommonCallToolResult(resp)
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
