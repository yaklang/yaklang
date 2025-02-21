package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *MCPServer) registerPortScanTool() {
	s.server.AddTool(mcp.NewTool("port_scan",
		mcp.WithDescription("Scan ports on targets"),
		mcp.WithStringArray("targets",
			mcp.Description("Targets to scan, allow netmask (e.g. 192.168.1.0/24)"),
			mcp.Required(),
		),
		mcp.WithNumberArray("ports",
			mcp.Description("Ports to scan, allow ranges (e.g. 5-10)"),
			mcp.Required(),
		),
		mcp.WithString("mode",
			mcp.Description("Scan mode: fingerprint, syn, or all"),
			mcp.Enum("fingerprint", "syn", "all"),
			mcp.Default("all"),
			mcp.Required(),
		),
		mcp.WithStringArray("proto",
			mcp.Description("Protocols to scan: tcp, udp"),
			mcp.Enum("tcp", "udp"),
			mcp.Default([]string{"tcp"}),
			mcp.Required(),
		),
		mcp.WithNumber("concurrent",
			mcp.Description("Number of concurrent scans"),
		),
		mcp.WithBool("active",
			mcp.Description("Active mode, whether to send extra packets to detect fingerprints"),
			mcp.Default(true),
			mcp.Required(),
		),
		mcp.WithString("fingerprintMode",
			mcp.Description("Fingerprint mode: service, web, or all"),
			mcp.Enum("service", "web", "all"),
			mcp.Default("all"),
			mcp.Required(),
		),
		mcp.WithBool("saveToDB",
			mcp.Description("Whether to save the results to the database"),
			mcp.Default(true),
			mcp.Required(),
		),
		mcp.WithString("targetsFile",
			mcp.Description("File containing targets to scan"),
		),
		mcp.WithStringArray("proxy",
			mcp.Description("HTTP proxies (e.g. host:port)"),
		),
		mcp.WithNumber("probeTimeout",
			mcp.Description("Timeout for a single probe"),
		),
		mcp.WithNumber("probeMax",
			mcp.Description("Maximum number of probes for fingerprinting"),
		),
		mcp.WithBool("enableCClassScan",
			mcp.Description("Enable C-class scan, expanding targets to the entire C-class network"),
		),
		mcp.WithBool("skippedHostAliveScan",
			mcp.Description("Whether to skip host alive scan"),
		),
		mcp.WithNumber("hostAliveTimeout",
			mcp.Description("Timeout for host alive scan"),
		),
		mcp.WithNumber("hostAliveConcurrent",
			mcp.Description("Number of concurrent host alive scans"),
		),
		mcp.WithStringArray("hostAlivePorts",
			mcp.Description("Ports to ping for host alive scan"),
		),
		mcp.WithStringArray("excludeHosts",
			mcp.Description("Hosts to exclude from the scan"),
		),
		mcp.WithNumberArray("excludePorts",
			mcp.Description("Ports to exclude from the scan"),
		),
		mcp.WithNumber("synConcurrent",
			mcp.Description("Number of concurrent SYN scans"),
		),
		mcp.WithBool("enableBrute",
			mcp.Description("Enable brute force when specific ports are detected with corresponding fingerprints"),
		),
	), s.handlePortScan)
}

func (s *MCPServer) handlePortScan(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var req ypb.PortScanRequest
	ArrayToStringHook := func(from reflect.Type, to reflect.Type, v any) (any, error) {
		if to.Kind() == reflect.String {
			if from.Kind() == reflect.Slice {
				slice := utils.InterfaceToSliceInterface(v)
				stringSlice := lo.Map(slice, func(item any, _ int) string {
					return utils.InterfaceToString(item)
				})
				return strings.Join(stringSlice, ","), nil
			}
		}
		return v, nil
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: ArrayToStringHook,
		Result:     &req,
	})
	if err != nil {
		return nil, utils.Wrap(err, "BUG: new map structure decoder error")
	}
	err = decoder.Decode(request.Params.Arguments)
	if err != nil {
		return nil, utils.Wrap(err, "invalid argument")
	}

	var progressToken mcp.ProgressToken
	meta := request.Params.Meta
	if meta != nil {
		progressToken = meta.ProgressToken
	}

	stream, err := s.grpcClient.PortScan(ctx, &req)
	if err != nil {
		return nil, utils.Wrap(err, "failed to query yak script")
	}
	results := make([]any, 0, 4)
	for {
		exec, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("error: %v", err),
				})
			}
			break
		}
		if !exec.IsMessage {
			continue
		}

		content := handleExecMessage(exec)
		if content == "" {
			continue
		}
		results = append(results, mcp.TextContent{
			Type: "text",
			Text: content,
		})
		s.server.SendNotificationToClient("port_scan/info", map[string]any{
			"content":       content,
			"progressToken": progressToken,
		})
	}
	if len(results) == 0 {
		results = append(results, mcp.TextContent{
			Type: "text",
			Text: "[System] Port scan completed with no results",
		})
	}

	return &mcp.CallToolResult{
		Content: results,
	}, nil
}
