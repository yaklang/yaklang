package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("brute",
		WithTool(mcp.NewTool("brute",
			mcp.WithDescription("Initiates a brute force attack based on the provided parameters"),
			mcp.WithString("type",
				mcp.Description("Type of the brute force attack"),
				mcp.Enum(bruteutils.GetBuildinAvailableBruteType()),
				mcp.Required(),
			),
			mcp.WithOneOfStruct("target",
				[]mcp.PropertyOption{
					mcp.Required(),
				},
				[]mcp.ToolOption{
					mcp.WithStringArray("targets",
						mcp.Description("Targets for the brute force attack"),
						mcp.Required(),
					), mcp.WithString("targetFile",
						mcp.Description("File containing targets for the brute force attack"),
						mcp.Required(),
					),
				}),
			mcp.WithBool("replaceDefaultUsernameDict",
				mcp.Description("If false, default username dictionary will be added"),
				mcp.Required(),
				mcp.Default(true),
			),
			mcp.WithBool("replaceDefaultPasswordDict",
				mcp.Description("If false, default password dictionary will be added"),
				mcp.Required(),
				mcp.Default(true),
			),
			mcp.WithOneOfStruct("user-dict",
				[]mcp.PropertyOption{
					mcp.Required(),
				},
				[]mcp.ToolOption{
					mcp.WithStringArray("usernames",
						mcp.Description("List of usernames to use in the brute force attack"),
					),
					mcp.WithString("usernameFile",
						mcp.Description("File containing usernames for the brute force attack"),
					),
				}),
			mcp.WithOneOfStruct("pass-dict",
				[]mcp.PropertyOption{
					mcp.Required(),
				},
				[]mcp.ToolOption{
					mcp.WithStringArray("passwords",
						mcp.Description("List of passwords to use in the brute force attack"),
					),
					mcp.WithString("passwordFile",
						mcp.Description("File containing passwords for the brute force attack"),
					),
				}),
			mcp.WithNumber("timeout",
				mcp.Description("Timeout for the brute force attack"),
			),
			mcp.WithNumber("concurrent",
				mcp.Description("Concurrency level between targets"),
			),
			mcp.WithNumber("retry",
				mcp.Description("Number of retries for the brute force attack"),
			),
			mcp.WithNumber("targetTaskConcurrent",
				mcp.Description("Concurrency level within target tasks"),
			),
			mcp.WithBool("okToStop",
				mcp.Description("Whether to stop the brute force attack once successful"),
			),
			mcp.WithNumber("delayMin",
				mcp.Description("Minimum delay second between attempts"),
				mcp.Min(0),
			),
			mcp.WithNumber("delayMax",
				mcp.Description("Maximum delay second between attempts"),
				mcp.Min(0),
			),
		), handleBrute),
	)
}

func handleBrute(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		target := utils.MapGetRaw(args, "target")
		targetMap, ok := target.(map[string]any)
		if !ok {
			return nil, utils.Error("invalid argument: target")
		}
		username := utils.MapGetRaw(args, "user-dict")
		usernameMap, ok := username.(map[string]any)
		if !ok {
			return nil, utils.Error("invalid argument: user-dict")
		}
		password := utils.MapGetRaw(args, "pass-dict")
		passwordMap, ok := password.(map[string]any)
		if !ok {
			return nil, utils.Error("invalid argument: pass-dict")
		}
		req := ypb.StartBruteParams{
			Type:                       utils.MapGetString(args, "type"),
			Targets:                    strings.Join(utils.MapGetStringSlice(targetMap, "targets"), "\n"),
			TargetFile:                 utils.MapGetString(targetMap, "targetFile"),
			Usernames:                  utils.MapGetStringSlice(usernameMap, "usernames"),
			UsernameFile:               utils.MapGetString(usernameMap, "usernameFile"),
			Passwords:                  utils.MapGetStringSlice(passwordMap, "passwords"),
			PasswordFile:               utils.MapGetString(passwordMap, "passwordFile"),
			ReplaceDefaultUsernameDict: utils.MapGetBool(args, "replaceDefaultUsernameDict"),
			ReplaceDefaultPasswordDict: utils.MapGetBool(args, "replaceDefaultPasswordDict"),
			Timeout:                    float32(utils.InterfaceToFloat64(utils.MapGetRaw(args, "timeout"))),
			Concurrent:                 utils.MapGetInt64(args, "concurrent"),
			Retry:                      utils.MapGetInt64(args, "retry"),
			TargetTaskConcurrent:       utils.MapGetInt64(args, "targetTaskConcurrent"),
			OkToStop:                   utils.MapGetBool(args, "okToStop"),
			DelayMin:                   utils.MapGetInt64(args, "delayMin"),
			DelayMax:                   utils.MapGetInt64(args, "delayMax"),
		}

		var progressToken mcp.ProgressToken
		meta := request.Params.Meta
		if meta != nil {
			progressToken = meta.ProgressToken
		}
		stream, err := s.grpcClient.StartBrute(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to start brute")
		}

		results := make([]any, 0, 4)
		for {
			exec, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					results = append(results, mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("[Error] %v", err),
					})
				}
				break
			}

			content := string(exec.Message)
			if content == "" {
				s.server.SendNotificationToClient("brute/progress", map[string]any{
					"content":  content,
					"progress": exec.Progress,
				})
				continue
			}
			// handle complex message
			msgContent := gjson.GetBytes(exec.Message, "content")
			level := msgContent.Get("level").String()
			switch level {
			case "feature-status-card-data", "feature-table-data", "json-feature":
				continue
			case "info", "json", "json-risk":
				// use content directly
				content = msgContent.Get("data").String()
			}

			results = append(results, mcp.TextContent{
				Type: "text",
				Text: content,
			})
			s.server.SendNotificationToClient("brute/info", map[string]any{
				"content":       content,
				"progress":      exec.Progress,
				"progressToken": progressToken,
			})
		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: "[System] Brute completed with no output",
			})
		}

		return NewCommonCallToolResult(results)
	}
}
