package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("http_fuzzer",
		WithTool(mcp.NewTool("http_fuzzer",
			mcp.WithDescription("Send HTTP packet(s) based on the given parameters, allow use fuzztag directly"),
			mcp.WithString("request",
				mcp.Description("The raw HTTP request packet to be fuzzed, allow fuzztag"),
				mcp.Required(),
			),
			mcp.WithNumber("concurrent",
				mcp.Description("Number of concurrent requests to send"),
				mcp.Default(20),
				mcp.Min(1),
			),
			mcp.WithBool("isHttps",
				mcp.Description("Indicates if the request should use HTTPS"),
				mcp.Required(),
			),
			mcp.WithBool("isGmTls",
				mcp.Description("Indicates if the request should use GM TLS (Chinese cryptographic standard)"),
			),
			mcp.WithBool("randomJA3",
				mcp.Description("Randomize the JA3 TLS fingerprint on each request to bypass WAF/bot detection (e.g. Akamai)"),
			),
			mcp.WithString("sni",
				mcp.Description("Override the TLS SNI (Server Name Indication) hostname"),
			),
			mcp.WithBool("overwriteSNI",
				mcp.Description("Force the specified SNI value even when it conflicts with the Host header"),
			),
			mcp.WithString("fuzzTagMode",
				mcp.Description("The fuzztag mode"),
				mcp.Enum("close", "standard"),
				mcp.Required(),
			),
			mcp.WithString("proxy",
				mcp.Description("Proxy for the request. e.g. http://127.0.0.1:1080"),
			),
			mcp.WithNumber("perRequestTimeoutSeconds",
				mcp.Description("Timeout in seconds for each request"),
				mcp.Min(0),
			),
			mcp.WithBool("noSystemProxy",
				mcp.Description("Disables the use of system proxy"),
			),
			mcp.WithString("actualAddr",
				mcp.Description("Actual Address to send, if not set, use Host Header as target"),
			),
			mcp.WithBool("noFollowRedirect",
				mcp.Description("Disables following redirects"),
			),
			mcp.WithNumber("redirectTimes",
				mcp.Description("Maximum number of redirects to follow"),
				mcp.Min(0),
			),
			mcp.WithBool("noFixContentLength",
				mcp.Description("Disables automatic fixing response, such as Content-Length header"),
			),
			mcp.WithString("responseCharset",
				mcp.Description("Charset to use for the response. e.g. gb18030"),
			),
			mcp.WithStringArray("dnsServers",
				mcp.Description("Custom DNS servers to use for the request. e.g. 8.8.8.8"),
			),
			mcp.WithKVPairs("etcHosts",
				mcp.Description("Custom /etc/hosts entries to use for the request"),
			),
			mcp.WithNumber("repeatTimes",
				mcp.Description("Number of times to repeat the request"),
				mcp.Default(1),
				mcp.Min(1),
			),
			mcp.WithStringArray("batchTarget",
				mcp.Description("Batch target to be used for the request, one target per element (e.g. [\"http://example.com\", \"http://test.com\"])"),
			),
			mcp.WithBool("batchTargetFile",
				mcp.Description("Indicates if the batch target is provided as a file"),
			),
			mcp.WithBool("disableUseConnPool",
				mcp.Description("Disables the use of connection pool"),
			),
		), handleHTTPFuzzer),
		WithTool(mcp.NewTool("create_web_fuzzer_tab",
			mcp.WithDescription("Create one standalone Web Fuzzer tab in Yakit. Use create_web_fuzzer_tabs instead when a vulnerability reproduction or exploit workflow has multiple meaningful HTTP steps; do not call this tool repeatedly for a batch"),
			mcp.WithString("request",
				mcp.Description("The raw HTTP request packet to load into the new Web Fuzzer tab"),
				mcp.Required(),
			),
			mcp.WithBool("isHttps",
				mcp.Description("Indicates if the request should use HTTPS"),
				mcp.Required(),
			),
			mcp.WithBool("openFlag",
				mcp.Description("Whether to switch to the newly created Web Fuzzer tab"),
				mcp.Default(true),
			),
			mcp.WithString("tabName",
				mcp.Description("Display name of the new Web Fuzzer tab"),
			),
			mcp.WithString("pageId",
				mcp.Description("Optional page id. If empty, backend generates one"),
			),
			mcp.WithString("proxy",
				mcp.Description("Proxy for the request. e.g. http://127.0.0.1:1080"),
			),
			mcp.WithNumber("concurrent",
				mcp.Description("Number of concurrent requests"),
				mcp.Min(1),
			),
			mcp.WithString("hotPatchCode",
				mcp.Description("Hot patch yak code for the tab"),
			),
			mcp.WithString("actualAddr",
				mcp.Description("Actual address to send, if different from Host header"),
			),
		), handleCreateWebFuzzerTab),
		WithTool(mcp.NewTool("create_web_fuzzer_tabs",
			mcp.WithDescription("Create multiple Web Fuzzer tabs in one operation and one Yakit push. "+
				"Prefer this tool when users want to reproduce a vulnerability or inspect exploit steps: "+
				"create one clearly named tab per meaningful HTTP step and group the flow for interactive review and "+
				"reruns. Before creating a group, call query_web_fuzzer_tabs, inspect usedGroupColors, then "+
				"independently choose either an unused preset or a distinct custom #RRGGBB color that fits "+
				"the request context"),
			mcp.WithStructArray("tabs", []mcp.PropertyOption{
				mcp.Description("Web Fuzzer tabs to create, in display order"),
				mcp.Required(),
			},
				mcp.WithString("request", mcp.Description("Raw HTTP request packet"), mcp.Required()),
				mcp.WithBool("isHttps", mcp.Description("Whether this request uses HTTPS"), mcp.Required()),
				mcp.WithString("tabName", mcp.Description("Display name; defaults to MCP Web Fuzzer N")),
				mcp.WithString("pageId", mcp.Description("Optional stable page id; normally omit")),
				mcp.WithString("proxy", mcp.Description("Comma-separated proxy URLs")),
				mcp.WithNumber("concurrent", mcp.Description("Number of concurrent requests"), mcp.Min(1)),
				mcp.WithString("hotPatchCode", mcp.Description("Hot patch yak code")),
				mcp.WithString("actualAddr", mcp.Description("Actual address if different from Host header")),
			),
			mcp.WithString("groupName",
				mcp.Description("Short purpose-oriented group label when grouping this batch; prefer 2-12 characters and never use a sentence"),
				mcp.MaxLength(yakit.WebFuzzerGroupNameMaxLength),
			),
			mcp.WithString("groupId", mcp.Description("Optional id for the new group; normally omit and let the backend generate it")),
			mcp.WithString("groupColor",
				mcp.Description("Strongly recommended when groupName is set. Choose the color yourself from the supported preset names or any custom #RRGGBB value; inspect query_web_fuzzer_tabs first and avoid reusing another group's exact color"),
				mcp.Pattern(yakit.WebFuzzerGroupColorPattern()),
				mcp.Default(yakit.WebFuzzerDefaultGroupColor),
			),
			mcp.WithBool("openFlag", mcp.Description("Switch to Web Fuzzer and focus the newly created batch"), mcp.Default(true)),
		), handleCreateWebFuzzerTabs),
		WithTool(mcp.NewTool("query_web_fuzzer_tabs",
			mcp.WithDescription("Discover the Web Fuzzer state currently visible to the user, including tabs, groups, color usage, available colors, and a recommended non-colliding color. Always call this before creating a group or updating, deleting, moving, or regrouping existing items so subsequent operations use current stable ids and fit the existing visual organization"),
			mcp.WithStringArray("pageIds", mcp.Description("Optional exact page or group ids")),
			mcp.WithString("kind", mcp.Description("Filter result kind"), mcp.Enum("all", "tab", "group"), mcp.Default("all")),
			mcp.WithString("nameContains", mcp.Description("Optional case-insensitive display-name filter")),
			mcp.WithString("groupId", mcp.Description("Optional group id; returns tabs currently in that group")),
			mcp.WithBool("includeRequest", mcp.Description("Include complete request and tab execution settings; false keeps the result compact"), mcp.Default(false)),
		), handleQueryWebFuzzerTabs),
		WithTool(mcp.NewTool("update_web_fuzzer_tab",
			mcp.WithDescription("Partially update one existing Web Fuzzer tab while preserving unspecified settings. Use targetGroupId to move it into a group, or \"0\" to ungroup it"),
			mcp.WithString("pageId", mcp.Description("Stable tab id from query_web_fuzzer_tabs"), mcp.Required()),
			mcp.WithString("tabName", mcp.Description("New display name")),
			mcp.WithString("request", mcp.Description("New raw HTTP request packet")),
			mcp.WithBool("isHttps", mcp.Description("Whether the request uses HTTPS")),
			mcp.WithString("proxy", mcp.Description("New comma-separated proxy URLs; empty string clears proxies")),
			mcp.WithNumber("concurrent", mcp.Description("New concurrency; must be at least 1"), mcp.Min(1)),
			mcp.WithString("hotPatchCode", mcp.Description("New hot patch yak code; empty string clears it")),
			mcp.WithString("actualAddr", mcp.Description("New actual address; empty string clears it")),
			mcp.WithString("targetGroupId", mcp.Description("Destination group id, or 0 to remove the tab from its group")),
			mcp.WithNumber("sort", mcp.Description("Optional 1-based display order"), mcp.Min(1)),
			mcp.WithBool("openFlag", mcp.Description("Switch to Web Fuzzer and focus the changed tab"), mcp.Default(false)),
		), handleUpdateWebFuzzerTab),
		WithTool(mcp.NewTool("delete_web_fuzzer_tabs",
			mcp.WithDescription("Delete existing Web Fuzzer tabs by stable pageId. Empty groups left by the deletion are removed automatically. Use manage_web_fuzzer_tab_group to delete a group"),
			mcp.WithStringArray("pageIds", mcp.Description("Tab ids from query_web_fuzzer_tabs"), mcp.Required()),
		), handleDeleteWebFuzzerTabs),
		WithTool(mcp.NewTool("manage_web_fuzzer_tab_group",
			mcp.WithDescription("Create, update, or delete a Web Fuzzer tab group. Call query_web_fuzzer_tabs first, keep group names short and purpose-oriented, and independently choose an unused preset or custom #RRGGBB color that fits the request context. Prefer grouped tabs to present multi-step vulnerability reproduction or exploit details. create groups existing tabIds; update can rename and add/remove/replace membership; delete ungroups children unless deleteTabs is true"),
			mcp.WithString("action", mcp.Description("Group operation"), mcp.Enum("create", "update", "delete"), mcp.Required()),
			mcp.WithString("groupId", mcp.Description("Existing group id for update/delete; optional custom id for create")),
			mcp.WithString("groupName",
				mcp.Description("Required for create and optional for rename; prefer 2-12 characters, describe only the request purpose, and never use a sentence"),
				mcp.MaxLength(yakit.WebFuzzerGroupNameMaxLength),
			),
			mcp.WithStringArray("tabIds", mcp.Description("Existing tab ids used by create or update membership")),
			mcp.WithString("mode", mcp.Description("How update applies tabIds"), mcp.Enum("add", "remove", "replace"), mcp.Default("add")),
			mcp.WithString("color",
				mcp.Description("Recommended for create and optional for update. Choose the color yourself from a supported preset name or any custom #RRGGBB value, after checking usedGroupColors to avoid an exact collision"),
				mcp.Pattern(yakit.WebFuzzerGroupColorPattern()),
			),
			mcp.WithBool("expand", mcp.Description("Whether the group is expanded")),
			mcp.WithBool("deleteTabs", mcp.Description("For delete only: delete child tabs instead of moving them out of the group"), mcp.Default(false)),
			mcp.WithBool("openFlag", mcp.Description("Switch to Web Fuzzer after the operation"), mcp.Default(false)),
		), handleManageWebFuzzerTabGroup),
	)
}

func handleHTTPFuzzer(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.FuzzerRequest
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: decodeHook,
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
		stream, err := s.grpcClient.HTTPFuzzer(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to start http fuzzer")
		}
		req.DisableHotPatch = true

		results := make([]any, 0, 4)
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
			m := map[string]any{
				"host":                rsp.Host,
				"request":             string(rsp.RequestRaw),
				"response":            string(rsp.ResponseRaw),
				"durationMs":          rsp.TotalDurationMs,
				"firstByteDurationMs": rsp.FirstByteDurationMs,
			}
			if rsp.Discard {
				m["discard"] = true
			}
			if !rsp.Ok {
				m["ok"] = false
				m["err"] = rsp.Reason
			}
			if len(rsp.Payloads) > 0 {
				m["payloads"] = rsp.Payloads
			}
			if rsp.IsTooLargeResponse {
				m["isTooLargeResponse"] = true
				m["large_response_header_file"] = rsp.TooLargeResponseHeaderFile
				m["large_response_body_file"] = rsp.TooLargeResponseBodyFile
			}
			contentBytes, err := json.Marshal(m)
			content := string(contentBytes)
			if err == nil {
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: content,
				})
				s.notificationServer(ctx).SendNotificationToClient("http_fuzzer/info", map[string]any{
					"content":       content,
					"progressToken": progressToken,
				})
			}

		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: "[System] HTTP Fuzzer completed with no output",
			})
		}

		return NewCommonCallToolResult(results)
	}
}
