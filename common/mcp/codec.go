package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/go-viper/mapstructure/v2"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec/codegrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	codecMethodDoc = new(sync.Map)
)

func init() {
	AddGlobalResourceSet("codec", WithResourceTemplate(
		mcp.NewResourceTemplate(
			"file://doc/codec_methods/{method}",
			"codec method details, include codec name, description and params, should be used with exec_codec",
		),
		handleCodecMethodDetailResource,
	),
	)

	AddGlobalToolSet("codec",
		// fuzztag
		WithTool(mcp.NewTool("render_fuzztag",
			mcp.WithDescription("Render fuzztag(a DSL for http fuzzer). e.g. {{int(1-10)}} will be render as [1,2,3,4,5,6,7,8,9,10]"),
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
		), handleRenderFuzztag),

		// codec
		WithTool(mcp.NewTool("codec_method_details",
			mcp.WithDescription("Get codec method details, include codec name, description and params. Should be use with exec_codec tool"),
			mcp.WithStringArray("method",
				mcp.Description("Name of codec method"),
				mcp.ItemsEnum(codegrpc.GetCodecLibsDocMethodNames()...),
				mcp.Required(),
			),
		), handleCodecMethodDetails),

		WithTool(mcp.NewTool("exec_codec",
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
					mcp.EnumString(codegrpc.GetCodecLibsDocMethodNames()...),
				),
				mcp.WithKVPairs("params",
					mcp.Description("Parameters for codec operation."),
					mcp.RequireTool("codec_method_details"),
				),
			),
		), handleExecCodec),
	)
}

// codecTypeHintsFromArguments 从原始 MCP 参数中尽力提取 codecType 字符串，
// 用于解码失败时构造"方法详情"提示（不依赖解码结果，nil 安全）。
func codecTypeHintsFromArguments(args map[string]any) []string {
	raw, ok := args["workFlow"]
	if !ok {
		return nil
	}
	var out []string
	switch v := raw.(type) {
	case string:
		out = append(out, v)
	case []any:
		for _, item := range v {
			switch it := item.(type) {
			case string:
				out = append(out, it)
			case map[string]any:
				if ct, ok := it["codecType"].(string); ok && ct != "" {
					out = append(out, ct)
				}
			}
		}
	}
	return out
}

// buildCodecMethodHelpError 构造带方法详情/可用方法名提示的 invalid argument 错误。
func buildCodecMethodHelpError(err error, names []string) error {
	methodDetails := make(map[string]string, len(names))
	notFound := false
	for _, name := range names {
		doc, docErr := getCodecMethodDetail(name)
		if docErr != nil {
			methodDetails[name] = docErr.Error()
			notFound = true
		} else {
			methodDetails[name] = doc
		}
	}
	if len(methodDetails) == 0 || notFound {
		// 提取不到方法名、或所提取的名字不存在时，附可用方法名列表
		valid := codegrpc.GetCodecLibsDocMethodNames()
		const maxShow = 60
		shown := valid
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		methodDetails["_available_methods"] = strings.Join(shown, ", ") +
			fmt.Sprintf(" ... (%d total)", len(valid))
	}
	detailBytes, marshalErr := json.Marshal(methodDetails)
	if marshalErr != nil {
		return utils.Wrap(err, "invalid argument")
	}
	return fmt.Errorf("invalid argument: %v\nhere are method details:\n%s", err, string(detailBytes))
}

func handleExecCodec(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {

		var req ypb.CodecRequestFlow
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			// 注意：不能遍历解码失败后的 req.WorkFlow——mapstructure 在元素级
			// 解码失败时会留下 nil 元素，解引用 flow.CodecType 会 panic
			// （曾表现为 "MCP worker failed: exit status 2"）。
			return nil, buildCodecMethodHelpError(err,
				codecTypeHintsFromArguments(request.Params.Arguments))
		}
		// 预校验：workFlow 元素非 nil 且 codecType 为已知方法；未知方法在 MCP 层
		// 返回可读错误，而不是透传 codec 引擎的 reflect 零值错误。
		for _, flow := range req.WorkFlow {
			if flow == nil {
				return nil, buildCodecMethodHelpError(
					utils.Error("workFlow contains a null item"), nil)
			}
			if _, err := getCodecMethodDetail(flow.CodecType); err != nil {
				return nil, buildCodecMethodHelpError(err, []string{flow.CodecType})
			}
		}
		rsp, err := s.grpcClient.NewCodec(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to exec codec")
		}
		result := make(map[string]any, 2)
		result["text"] = rsp.Result
		result["base64_text"] = rsp.RawResult
		return NewCommonCallToolResult(result)
	}
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

func handleCodecMethodDetails(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		iMethods, ok := request.Params.Arguments["method"]
		if !ok {
			return nil, utils.Error("missing argument: method")
		}
		methods := utils.InterfaceToStringSlice(iMethods)
		results := lo.SliceToMap(methods, func(methodName string) (string, string) {
			doc, err := getCodecMethodDetail(methodName)
			if err != nil {
				return methodName, err.Error()
			}
			return methodName, doc
		})
		return NewCommonCallToolResult(results)
	}
}

func handleCodecMethodDetailResource(s *MCPServer) server.ResourceHandlerFunc {
	return func(
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
}

func handleRenderFuzztag(s *MCPServer) server.ToolHandlerFunc {
	return func(
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
}
