package yakcmds

import (
	"context"
	_ "embed"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"strconv"
	"strings"
)

//go:embed scan_yamlpoc.yak
var yamlScanScript string

var yamlpocCommand = &cli.Command{
	Name:    "scan",
	Aliases: []string{"poc"},
	Usage:   "Use plugins (UI in Yakit) to scan",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "target,host,t",
			Usage: "Target Hosts, separated by comma, like (example.com, http://www2.example.com, 192.168.1.2/24)",
		},
		cli.StringFlag{
			Name:  "target-file,f",
			Usage: "Target Hosts File, one host per line",
		},
		cli.StringFlag{
			Name:  "raw-packet-file,raw",
			Usage: "Raw Packet File",
		},
		cli.BoolFlag{
			Name:  "https",
			Usage: "Raw Packet File is HTTPS or default(not set) https config",
		},
		cli.StringFlag{
			Name:  "keyword,fuzz,k",
			Usage: "Fuzz Search Plugin Keyword",
		},
		cli.BoolFlag{
			Name:  "list,l",
			Usage: "Just List Plugin, No Scan",
		},
		cli.StringFlag{
			Name: "plugin", Usage: "Exec Single Plugin by Name",
		},
		cli.StringFlag{
			Name: "type", Usage: `Type of Plugins in Yakit, port-scan / mitm / yaml-poc`,
		},
	},

	Action: func(c *cli.Context) error {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return utils.Error("yak profile/plugin database is nil")
		}

		// fix plugin
		var plugins = utils.PrettifyListFromStringSplitEx(c.String("type"), ",", "|", "\n")
		for i := 0; i < len(plugins); i++ {
			switch ret := strings.ToLower(plugins[i]); ret {
			case "port-scan", "mitm", "nuclei":
				plugins[i] = ret
				continue
			case "yaml-poc", "yaml", "httptpl", "nuclei-template":
				plugins[i] = "nuclei"
				continue
			case "scanport", "scan-port":
				plugins[i] = "port-scan"
				continue
			default:
				log.Errorf("unsupported plugin type: %s", ret)
			}
		}
		if len(plugins) > 0 {
			plugins = utils.RemoveRepeatStringSlice(plugins)
		}

		if len(plugins) == 0 {
			plugins = []string{"mitm", "nuclei", "port-scan"}
		}

		keyword := c.String("k")

		db = db.Model(&yakit.YakScript{})
		db.Where("type IN ?", plugins)
		db = bizhelper.FuzzSearchEx(db, []string{
			"script_name", "tags", "content", "help",
		}, keyword, false)
		db = db.Order("updated_at desc").Debug()

		pluginList := omap.NewOrderedMap(map[string]*yakit.YakScript{})
		for result := range yakit.YieldYakScripts(db, context.Background()) {
			pluginList.Set(result.ScriptName, result)
		}

		// handle --list command
		if c.Bool("list") {
			log.Infof("\nList All Matched Plugins: %v", strconv.Quote(c.String("k")))

			count := 0
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{
				"Plugin Name", "Type", "Author",
			})
			table.SetAutoWrapText(false)
			table.SetColMinWidth(0, 72)
			for _, result := range pluginList.Values() {
				var name = []rune(result.ScriptName)
				if len(name) > 64 {
					name = append(([]rune(name))[:64], []rune("...")...)
				}
				table.Append([]string{
					string(name),
					result.Type,
					result.Author,
				})
				count++
			}
			table.Render()
			log.Info("Total Matched Plugins: ", count)
			return nil
		}

		gen, err := yakgrpc.TargetGenerator(context.Background(), &ypb.HybridScanInputTarget{
			Input:               "",
			InputFile:           nil,
			HTTPRequestTemplate: nil,
		})
		if err != nil {
			return utils.Errorf("generate target failed: %s", err)
		}

		return nil
	},
}
