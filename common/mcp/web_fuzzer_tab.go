package mcp

import (
	"context"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func handleCreateWebFuzzerTab(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if err := s.ensureLocalClient(); err != nil {
			return nil, err
		}

		var req ypb.FuzzerRequest
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: decodeHook,
			Result:     &req,
		})
		if err != nil {
			return nil, utils.Wrap(err, "BUG: new map structure decoder error")
		}
		if err := decoder.Decode(request.Params.Arguments); err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}

		var extra struct {
			OpenFlag *bool  `mapstructure:"openFlag"`
			TabName  string `mapstructure:"tabName"`
			PageId   string `mapstructure:"pageId"`
		}
		if err := mapstructure.Decode(request.Params.Arguments, &extra); err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}

		openFlag := true
		if extra.OpenFlag != nil {
			openFlag = *extra.OpenFlag
		}

		fuzzerConfig, err := yakit.BuildWebFuzzerConfig(&req, func(opts *yakit.WebFuzzerPageBuildOptions) {
			if extra.PageId != "" {
				opts.PageID = extra.PageId
			}
			if extra.TabName != "" {
				opts.TabName = extra.TabName
			}
		})
		if err != nil {
			return nil, err
		}

		if _, err = s.grpcClient.SaveFuzzerConfig(ctx, &ypb.SaveFuzzerConfigRequest{
			Data: []*ypb.FuzzerConfig{fuzzerConfig},
		}); err != nil {
			return nil, utils.Wrap(err, "failed to save web fuzzer config")
		}

		yakit.BroadcastWebFuzzerTab(openFlag, fuzzerConfig)

		return NewCommonCallToolResult(map[string]any{
			"status":   "ok",
			"openFlag": openFlag,
			"pageId":   fuzzerConfig.GetPageId(),
			"type":     fuzzerConfig.GetType(),
			"config":   fuzzerConfig.GetConfig(),
		})
	}
}
