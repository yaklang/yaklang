package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

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
				mcp.EnumString(bruteutils.GetBuildinAvailableBruteType()...),
				mcp.Required(),
			),
			mcp.WithStringArray("targets",
				mcp.Description("Targets for the brute force attack"),
			),
			mcp.WithString("targetFile",
				mcp.Description("File containing targets for the brute force attack"),
			),
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
			mcp.WithStringArray("usernames",
				mcp.Description("List of usernames to use in the brute force attack"),
			),
			mcp.WithString("usernameFile",
				mcp.Description("File containing usernames for the brute force attack"),
			),
			mcp.WithStringArray("passwords",
				mcp.Description("List of passwords to use in the brute force attack"),
			),
			mcp.WithString("passwordFile",
				mcp.Description("File containing passwords for the brute force attack"),
			),
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
	targets := utils.MapGetStringSlice(args, "targets")
	targetFile := utils.MapGetString(args, "targetFile")
	if len(targets) == 0 && targetFile == "" {
		return nil, utils.Error("invalid argument: target is required (provide targets or targetFile)")
	}
	usernames := utils.MapGetStringSlice(args, "usernames")
	usernameFile := utils.MapGetString(args, "usernameFile")
	passwords := utils.MapGetStringSlice(args, "passwords")
	passwordFile := utils.MapGetString(args, "passwordFile")
	if (len(usernames) == 0 && usernameFile == "") || (len(passwords) == 0 && passwordFile == "") {
		return nil, utils.Error("invalid argument: user-dict and pass-dict are required")
	}

	req := ypb.StartBruteParams{
		Type:                       utils.MapGetString(args, "type"),
		Targets:                    strings.Join(targets, "\n"),
		TargetFile:                 targetFile,
		Usernames:                  usernames,
		UsernameFile:               usernameFile,
		Passwords:                  passwords,
		PasswordFile:               passwordFile,
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
				// Only send progress notification when the client provided a progressToken.
				if progressToken != nil {
					s.notificationServer(ctx).SendNotificationToClient("notifications/progress", map[string]any{
						"progressToken": progressToken,
						"progress":      exec.Progress,
					})
				}
				continue
			}
			content = handleExecMessage(content)
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: content,
			})
			s.notificationServer(ctx).SendNotificationToClient("notifications/message", map[string]any{
				"level": "info",
				"data":  content,
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
