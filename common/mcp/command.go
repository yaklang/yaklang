package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var MCPCommandUsage = `Start a mcp server for providing mcp service.

Available ToolSets: codec, cve, httpflow, hybrid_scan, payload, port_scan, yak_document, yak_script, reverse_shell, http_fuzzer, brute, subdomain, crawler, dynamic, ssa, project_database, global_hotpatch, system_proxy

Available ResourceSets: codec`

var MCPCommand = &cli.Command{
	Name:  "mcp",
	Usage: MCPCommandUsage,
	Flags: []cli.Flag{
		cli.StringFlag{Name: "transport", Usage: "transport protocol, e.g. sse/stdio/streamable_http", Value: "stdio"},
		cli.StringFlag{Name: "host", Usage: "if transport is http-based, listen host", Value: "localhost"},
		cli.IntFlag{Name: "port", Usage: "if transport is http-based, listen port", Value: 11432},
		cli.StringFlag{Name: "t,tool", Usage: "enable tool sets, split by ','"},
		cli.StringFlag{Name: "dt,disable-tool", Usage: "disable tool sets, split by ','"},
		cli.StringFlag{Name: "r,resource", Usage: "enable resource sets, split by ','"},
		cli.StringFlag{Name: "dr,disable-resource", Usage: "disable resource sets, split by ','"},
		cli.StringSliceFlag{Name: "script", Usage: "add the dynamic Yak script as a tool to the MCP server"},
		cli.StringFlag{Name: "base-url", Usage: "if transport is http-based, the base url of the MCP server"},
		cli.BoolFlag{Name: "enable-aitool-framework", Usage: "expose built-in aitool-framework tools (fs, ssa, yakscript, etc.)"},
		cli.BoolFlag{Name: "enable-bridge-external-mcp", Usage: "bridge external MCP servers already enabled in AI Agent"},
	},
	Action: func(c *cli.Context) error {
		yakit.CallPostInitDatabase()

		var err error
		transport := c.String("transport")
		host := c.String("host")
		port := c.Int("port")
		tool, disableTool := c.String("tool"), c.String("disable-tool")
		script := c.StringSlice("script")
		baseURL := c.String("base-url")
		enableAIToolFramework := c.Bool("enable-aitool-framework")
		enableBridgeExternalMCP := c.Bool("enable-bridge-external-mcp")
		toolSets := lo.FilterMap(strings.Split(tool, ","), func(item string, _ int) (string, bool) {
			item = strings.TrimSpace(item)
			return item, item != ""
		})
		disableToolSets := lo.FilterMap(strings.Split(disableTool, ","), func(item string, _ int) (string, bool) {
			item = strings.TrimSpace(item)
			return item, item != ""
		})
		resource, disableResource := c.String("resource"), c.String("disable-resource")
		resourceSets := lo.FilterMap(strings.Split(resource, ","), func(item string, _ int) (string, bool) {
			item = strings.TrimSpace(item)
			return item, item != ""
		})
		disableResourceSets := lo.FilterMap(strings.Split(disableResource, ","), func(item string, _ int) (string, bool) {
			item = strings.TrimSpace(item)
			return item, item != ""
		})

		opts := make([]McpServerOption, 0, len(toolSets)+len(disableToolSets)+len(resourceSets)+len(disableResourceSets))
		for _, toolSet := range toolSets {
			opts = append(opts, WithEnableToolSet(toolSet))
		}
		for _, toolSet := range disableToolSets {
			opts = append(opts, WithDisableToolSet(toolSet))
		}
		for _, resourceSet := range resourceSets {
			opts = append(opts, WithEnableResourceSet(resourceSet))
		}
		for _, resourceSet := range disableResourceSets {
			opts = append(opts, WithDisableResourceSet(resourceSet))
		}
		if len(script) > 0 {
			opts = append(opts, WithDynamicScript(script))
		}

		if enableAIToolFramework {
			db := consts.GetGormProfileDatabase()

			// Built-in framework tools: fs, ssa, yakscript, etc.
			builtinTools := buildinaitools.GetAllToolsDynamically(db)
			if len(builtinTools) > 0 {
				opts = append(opts, WithAITools(builtinTools...))
				log.Infof("loaded %d built-in aitool-framework tools", len(builtinTools))
			}
		}

		if enableBridgeExternalMCP {
			db := consts.GetGormProfileDatabase()
			externalTools, mcpErr := aitool.LoadAllEnabledAIToolsFromMCPServers(db, context.Background())
			if mcpErr != nil {
				log.Warnf("load external mcp tools via bridge failed: %v", mcpErr)
			} else if len(externalTools) > 0 {
				opts = append(opts, WithAITools(externalTools...))
				log.Infof("loaded %d external mcp tools via bridge", len(externalTools))
			}
		}

		// Apply per-tool enable/disable state from the profile DB, matching the
		// behaviour of grpc_mcp.go launchMcpServer.
		disabledTools, dbErr := yakit.GetDisabledMCPClientToolNames(consts.GetGormProfileDatabase())
		if dbErr != nil {
			log.Warnf("mcp command: failed to load disabled tool list from DB: %v", dbErr)
		}
		if len(disabledTools) > 0 {
			opts = append(opts, WithDisabledToolNames(disabledTools))
		}

		s, err := NewMCPServer(opts...)
		if err != nil {
			return err
		}
		switch transport {
		case "stdio":
			log.SetLevel(log.FatalLevel)
			err = s.ServeStdio()
		case "sse":
			if port == 0 {
				port = utils.GetRandomAvailableTCPPort()
			}
			hostPort := utils.HostPort(host, port)
			if baseURL == "" {
				baseURL = fmt.Sprintf("http://%s", hostPort)
			}
			log.Infof("start to listen reverse(mcp) on: %s", hostPort)
			log.Infof("mcp server endpoint: %s", baseURL)
			err = s.ServeSSE(hostPort, baseURL)
		case "streamable_http", "streamable-http", "http":
			if port == 0 {
				port = utils.GetRandomAvailableTCPPort()
			}
			hostPort := utils.HostPort(host, port)
			if baseURL == "" {
				baseURL = fmt.Sprintf("http://%s", hostPort)
			}
			log.Infof("start to listen streamable http(mcp) on: %s", hostPort)
			log.Infof("mcp streamable http endpoint: %s/mcp", baseURL)
			err = s.ServeStreamableHTTP(hostPort, baseURL)
		default:
			return utils.Errorf("invalid transport: %v", transport)
		}
		if err != nil {
			return err
		}

		return nil
	},
}
