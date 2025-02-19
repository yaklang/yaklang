package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/go-viper/mapstructure/v2"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec/codegrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	codecMethodDoc = new(sync.Map)
)

func (s *MCPServer) registerCodecTool() {
	// fuzztag
	s.server.AddTool(mcp.NewTool("render_fuzztag",
		mcp.WithDescription("Render fuzztag(a DSL for web fuzzer). e.g. {{int(1-10)}} will be render as [1,2,3,4,5,6,7,8,9,10]"),
		mcp.WithString("template",
			mcp.Description("Input fuzztag"),
			mcp.Required(),
		),
		mcp.WithNumber("limit",
			mcp.Description("The limit number for the result"),
		),
		mcp.WithNumber("timeoutSeconds",
			mcp.Description("The timeout in seconds for the rendering"),
		),
	), s.handleRenderFuzztag)

	// codec
	s.server.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"file://doc/codec_methods/{method}",
			"codec method details, include codec name, description and params, should be used with exec_codec",
		),
		s.handleCodecMethodDetailResource,
	)

	s.server.AddTool(mcp.NewTool("codec_method_details",
		mcp.WithDescription("Get codec method details, include codec name, description and params. Should be use with exec_codec tool"),
		mcp.WithStringArray("method",
			mcp.Description("Name of codec method"),
			mcp.Required(),
		),
	), s.handleCodecMethodDetails)

	s.server.AddTool(mcp.NewTool("exec_codec",
		mcp.WithDescription("Codec processing workflow with multiple encoding/decoding steps."),
		mcp.WithRequireTool("codec_method_details"),
		mcp.WithString("text",
			mcp.Description("Input text for codec processing"),
		),
		mcp.WithStructArray("workFlow",
			[]mcp.PropertyOption{
				mcp.Description("Sequence of codec operations, each item contains codec type, script, plugin name and parameters"),
			},
			mcp.WithString("codecType",
				mcp.Description("Name of codec method"),
				mcp.Required(),
				mcp.Enum(codegrpc.GetCodecLibsDocMethodNames()),
			),
			mcp.WithKVPairs("params",
				mcp.Description("Parameters for codec operation."),
				mcp.RequireTool("codec_method_details"),
			),
		),
	), s.handleExecCodec)
}

func (s *MCPServer) handleExecCodec(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var req ypb.CodecRequestFlow
	err := mapstructure.Decode(request.Params.Arguments, &req)
	if err != nil {
		return nil, utils.Wrap(err, "invalid argument")
	}
	rsp, err := s.grpcClient.NewCodec(ctx, &req)
	if err != nil {
		return nil, utils.Wrap(err, "failed to exec codec")
	}
	return NewCommonCallToolResult(rsp)
}

func getCodecMethodDetail(methodName string) (string, error) {
	doc := ""
	if iDoc, ok := codecMethodDoc.Load(methodName); !ok {
		method, ok := codegrpc.CodecLibsDoc[methodName]
		if !ok {
			return "", utils.Errorf("method[%s] not found", methodName)
		}
		if bytes, err := json.Marshal(method); err != nil {
			return "", utils.Wrap(err, "failed to encode codec method to document")
		} else {
			doc = string(bytes)
			codecMethodDoc.Store(methodName, doc)
		}
	} else {
		doc = iDoc.(string)
	}
	return doc, nil
}

func (s *MCPServer) handleCodecMethodDetails(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	iMethods, ok := request.Params.Arguments["method"]
	if !ok {
		return nil, utils.Error("missing argument: method")
	}
	methods := utils.InterfaceToStringSlice(iMethods)
	results := lo.SliceToMap(methods, func(methodName string) (string, string) {
		doc, _ := getCodecMethodDetail(methodName)
		return methodName, doc
	})
	return NewCommonCallToolResult(results)
}

func (s *MCPServer) handleCodecMethodDetailResource(
	ctx context.Context,
	request mcp.ReadResourceRequest,
) ([]any, error) {
	u := utils.ParseStringToUrl(request.Params.URI)
	p := strings.Split(u.Path, "/")
	methodName := p[len(p)-1]

	doc, err := getCodecMethodDetail(methodName)
	if err != nil {
		return nil, utils.Wrap(err, "failed to get codec method detail")
	}

	return []any{
		mcp.TextResourceContents{
			ResourceContents: mcp.ResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
			},
			Text: doc,
		},
	}, nil
}

func (s *MCPServer) handleRenderFuzztag(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var req ypb.StringFuzzerRequest
	err := mapstructure.Decode(request.Params.Arguments, &req)
	if err != nil {
		return nil, utils.Wrap(err, "invalid argument")
	}
	rsp, err := s.grpcClient.StringFuzzer(ctx, &req)
	if err != nil {
		return nil, utils.Wrap(err, "failed to render fuzztag")
	}
	results := make([]string, 0, len(rsp.Results))
	for _, result := range rsp.Results {
		results = append(results, string(result))
	}
	return NewCommonCallToolResult(results)
}
