package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/go-viper/mapstructure/v2"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var filterPortsToolOptions = []mcp.ToolOption{
	mcp.WithStruct("pagination",
		[]mcp.PropertyOption{
			mcp.Description("Pagination settings for the query"),
			mcp.Required(),
		},
	),
	mcp.WithString("hosts",
		mcp.Description("(fuzzy search)Filter by host names or IP addresses"),
	),
	mcp.WithString("ports",
		mcp.Description("Filter by ports, comma-separated, allow ranges (e.g. 80,443,5-10)"),
	),
	mcp.WithString("service",
		mcp.Description("(fuzzy search)Filter by service fingerprint, e.g. https"),
	),
	mcp.WithString("state",
		mcp.Description("Filter by port state"),
		mcp.Enum("open", "closed", "unknown"),
	),
	mcp.WithString("title",
		mcp.Description("(fuzzy search)Filter by HTML title of the service"),
	),
	mcp.WithBool("all",
		mcp.Description("Query all data, ignoring pagination"),
	),
	mcp.WithString("keywords",
		mcp.Description("(fuzzy search)Filter by keywords in the service data"),
	),
	mcp.WithBool("titleEffective",
		mcp.Description("Filter by whether the title is effective, not 404 or empty"),
	),
	mcp.WithString("proto",
		mcp.Description("Filter by protocol, e.g., tcp, udp"),
		mcp.Enum("tcp", "udp"),
		mcp.Default("tcp"),
	),
	mcp.WithNumber("beforeUpdatedAt",
		mcp.Description("Filter by records updated before this timestamp"),
	),
	mcp.WithNumber("afterUpdatedAt",
		mcp.Description("Filter by records updated after this timestamp"),
	),
	mcp.WithNumber("afterId",
		mcp.Description("Filter by records with ID greater than this value"),
	),
	mcp.WithNumber("beforeId",
		mcp.Description("Filter by records with ID less than this value"),
	),
}

func init() {
	AddGlobalToolSet("port_scan",
		WithTool(mcp.NewTool("port_scan",
			mcp.WithDescription("Scan ports on targets"),
			mcp.WithStringArray("targets",
				mcp.Description("Targets to scan, one target per array element, allow netmask (e.g. [\"192.168.1.0/24\", \"10.0.0.1\"])"),
				mcp.Required(),
			),
			mcp.WithString("ports",
				mcp.Description("Ports to scan, comma-separated, allow ranges (e.g. 80,443,1000-10000)"),
				mcp.Required(),
			),
			mcp.WithString("mode",
				mcp.Description("Scan mode. \"fingerprint\": TCP connect scan + service fingerprinting — works on all targets including localhost, no special privileges needed (default, most reliable). \"syn\": SYN/raw-socket scan — faster for large port ranges but requires pcap/libpcap privileges, reports open ports without fingerprinting. \"all\": SYN scan then fingerprint each open port — combines speed of syn with fingerprinting."),
				mcp.Enum("fingerprint", "syn", "all"),
				mcp.Default("fingerprint"),
			),
			mcp.WithStringArray("proto",
				mcp.Description("Protocols to scan: tcp, udp"),
				mcp.ItemsEnum("tcp", "udp"),
				mcp.Default([]string{"tcp"}),
			),
			mcp.WithNumber("concurrent",
				mcp.Description("Number of concurrent scans"),
			),
			mcp.WithBool("active",
				mcp.Description("Active mode, whether to send extra packets to detect fingerprints"),
				mcp.Default(true),
			),
			mcp.WithString("fingerprintMode",
				mcp.Description("Fingerprint mode: service, web, or all"),
				mcp.Enum("service", "web", "all"),
				mcp.Default("all"),
			),
			mcp.WithBool("saveToDB",
				mcp.Description("Whether to save the results to the database"),
				mcp.Default(true),
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
			mcp.WithString("hostAlivePorts",
				mcp.Description("Ports to ping for host alive scan, comma-separated (e.g. 22,80,443)"),
			),
			mcp.WithString("excludeHosts",
				mcp.Description("Hosts to exclude from the scan, comma-separated (e.g. 192.168.1.1,10.0.0.1)"),
			),
			mcp.WithString("excludePorts",
				mcp.Description("Ports to exclude from the scan, comma-separated, allow ranges (e.g. 22,80 or 5-10)"),
			),
			mcp.WithNumber("synConcurrent",
				mcp.Description("Number of concurrent SYN scans"),
			),
			mcp.WithBool("enableBrute",
				mcp.Description("Enable brute force when specific ports are detected with corresponding fingerprints"),
			),
		), handlePortScan),

		WithTool(mcp.NewTool("query_ports",
			append([]mcp.ToolOption{
				mcp.WithDescription("Query ports based on flexible filters"),
			},
				filterPortsToolOptions...)...,
		), handleQueryPort),

		WithTool(mcp.NewTool("delete_ports",
			mcp.WithDescription("Delete ports based on flexible filters"),
			mcp.WithNumberArray("id",
				mcp.Description("ID of the port to delete")),
			mcp.WithBool("all",
				mcp.Description("Delete all ports")),
			mcp.WithStruct("filter", nil, filterPortsToolOptions...),
		), handleDeletePort),
	)
}

func handlePortScan(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.PortScanRequest
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: decodeHookComma,
			Result:     &req,
		})
		if err != nil {
			return nil, utils.Wrap(err, "BUG: new map structure decoder error")
		}
		err = decoder.Decode(request.Params.Arguments)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}

		// Apply the mode default. The MCP schema declares Default("fingerprint") but
		// that is only metadata for the client — the server never injects it. When the
		// caller omits "mode", req.Mode stays "" and the gRPC layer passes --mode "" to
		// the yak script, where cli.String("mode", cli.setDefault("fingerprint")) finds
		// --mode in args and returns "" (not the default — defaults only fire when the
		// arg is absent, not empty). The script's three mode branches are independent
		// `if` blocks (not if/else), so "" matches none and the scan silently produces
		// zero results. Apply the default here so the correct branch always runs.
		if req.GetMode() == "" {
			req.Mode = "fingerprint"
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
						Text: fmt.Sprintf("[Error] %v", err),
					})
				}
				break
			}
			if !exec.IsMessage {
				continue
			}

			content := string(exec.Message)
			content = handleExecMessage(content)
			msgContent := gjson.GetBytes(exec.Message, "content")

			// Filter port results by isOpen. The yak script emits results via
			// yakit.Output({host, port, isOpen: bool}), which arrives as a YakitMessage
			// {"type":"log","content":{"level":"json","data":"{...\"isOpen\":true}"}}.
			// handleExecMessage extracts content.data (the inner JSON string) into
			// content, so we check isOpen there — not at the top level of exec.Message.
			isOpenField := gjson.Get(content, "isOpen")
			if isOpenField.Exists() {
				if !isOpenField.Bool() {
					// closed port — skip
					continue
				}
				content = "[Result] " + content
			}

			if content == "" {
				continue
			}
			typ := gjson.GetBytes(exec.Message, "type").String()

			if typ == "progress" {
				// Only send progress notification when the client provided a progressToken.
				// Sending notifications/progress with a null token violates the MCP spec and
				// causes strict SDK validators (e.g. Python MCP SDK) to drop the connection.
				if progressToken != nil {
					s.notificationServer(ctx).SendNotificationToClient("notifications/progress", map[string]any{
						"progressToken": progressToken,
						"progress":      msgContent.Get("progress").Float(),
					})
				}
			} else {
				s.notificationServer(ctx).SendNotificationToClient("notifications/message", map[string]any{
					"level": "info",
					"data":  content,
				})
			}

			if typ != "progress" {
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: content,
				})
			}
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
}

func handleQueryPort(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.QueryPortsRequest
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: decodeHookComma,
			Result:     &req,
		})
		if err != nil {
			return nil, utils.Wrap(err, "BUG: new map structure decoder error")
		}
		err = decoder.Decode(request.Params.Arguments)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		rsp, err := s.grpcClient.QueryPorts(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query ports")
		}
		return NewCommonCallToolResult(rsp.Data)
	}
}

func handleDeletePort(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.DeletePortsRequest
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: decodeHookComma,
			Result:     &req,
		})
		if err != nil {
			return nil, utils.Wrap(err, "BUG: new map structure decoder error")
		}
		err = decoder.Decode(request.Params.Arguments)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		_, err = s.grpcClient.DeletePorts(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to delete ports")
		}
		return NewCommonCallToolResult("delete port(s) success")
	}
}
