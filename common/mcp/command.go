package mcp

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var MCPCommandUsage = `Start a mcp server for providing mcp service.

Available ToolSets: codec, cve, httpflow, hybrid_scan, payload, port_scan, yak_document, yak_script, reverse_shell, http_fuzzer, brute

Available ResourceSets: codec`

var MCPCommand = &cli.Command{
	Name:  "mcp",
	Usage: MCPCommandUsage,
	Flags: []cli.Flag{
		cli.StringFlag{Name: "transport", Usage: "transport protocol, e.g. sse/stdio", Value: "stdio"},
		cli.StringFlag{Name: "host", Usage: "if transport is sse, listen host", Value: "localhost"},
		cli.IntFlag{Name: "port", Usage: "if transport is sse, listen port"},
		cli.StringFlag{Name: "t,tool", Usage: "enable tool sets, split by ,"},
		cli.StringFlag{Name: "dt,disable-tool", Usage: "disable tool sets, split by ,"},
		cli.StringFlag{Name: "r,resource", Usage: "enable resource sets, split by ,"},
		cli.StringFlag{Name: "dr,disable-resource", Usage: "disable resource sets, split by ,"},
	},
	Action: func(c *cli.Context) error {
		var err error
		transport := c.String("transport")
		host := c.String("host")
		port := c.Int("port")
		tool, disableTool := c.String("tool"), c.String("disable-tool")
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
			err = s.ServeSSE(fmt.Sprintf(":%d", port), fmt.Sprintf("http://%s:%d", host, port))
		default:
			return utils.Errorf("invalid transport: %v", transport)
		}
		if err != nil {
			return err
		}

		return nil
	},
}
