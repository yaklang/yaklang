package yakcmds

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	"github.com/segmentio/ksuid"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"strconv"
	"strings"
)

const yakitScanBanner = `
             _    _ _
 _   _  __ _| | _(_) |_      ___  ___ __ _ _ __
| | | |/ _` + "`" + ` | |/ / | __|____/ __|/ __/ _` + "`" + ` | '_ \
| |_| | (_| |   <| | ||_____\__ \ (_| (_| | | | |
 \__, |\__,_|_|\_\_|\__|    |___/\___\__,_|_| |_|
 |___/
						--- Powered by yaklang.io
`

var hybridScanCommand = &cli.Command{
	Name:    "scan",
	Aliases: []string{"poc"},
	Usage:   "Use plugins (UI in Yakit) to scan",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "templates,t",
			Usage: "plugin-file templates (file / dir), split by 'comma', like './templates/1.yaml,./templates/vulns/'",
		},
		cli.StringFlag{
			Name:  "target,host",
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
		cli.StringFlag{Name: "proxy", Usage: "Proxy Server, like http://127.0.0.1:8083"},
		cli.IntFlag{Name: "concurrent,thread", Usage: "(Thread)Concurrent Scan Number", Value: 50},
	},

	Action: func(c *cli.Context) error {
		fmt.Println(yakitScanBanner)

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

		// remove temporary plugin
		log.Infof("start to load templates: %v", c.String("templates"))
		uid := ksuid.New().String()
		var templatesCodes []string
		var yakMITM []string
		var portScan []string
		handleTempFileTemplate := func(filename string) error {
			log.Infof("start to handle file: %s", filename)
			if strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") {
				raw, err := os.ReadFile(filename)
				if err != nil {
					return err
				}
				log.Infof("handle yaml template finished: %s", filename)
				templatesCodes = append(templatesCodes, string(raw))
				return nil
			} else if strings.HasSuffix(filename, ".yak") {
				raw, err := os.ReadFile(filename)
				if err != nil {
					return err
				}
				yakMITM = append(yakMITM, string(raw))
				return nil
			}
			return nil
		}
		for _, file := range utils.PrettifyListFromStringSplitEx(c.String("templates"), ",", "|", "\n") {
			if file == "" {
				continue
			}

			if utils.IsDir(file) {
				files, err := utils.ReadFilesRecursively(file)
				if err != nil {
					return utils.Errorf("handle path(dir) %v failed: %s", file, err.Error())
				}
				for _, f := range files {
					if f.IsDir {
						continue
					}
					if strings.HasSuffix(f.Path, ".yaml") || strings.HasSuffix(f.Path, ".yml") || strings.HasSuffix(f.Path, ".yak") {
						err := handleTempFileTemplate(f.Path)
						if err != nil {
							return err
						}
					}
				}
			} else {
				err := handleTempFileTemplate(file)
				if err != nil {
					return err
				}
			}
		}

		handledUUID := false
		if len(templatesCodes) > 0 {
			for _, temp := range templatesCodes {
				pluginName, err := yakit.CreateTemporaryYakScript(`nuclei`, temp, uid)
				if err != nil {
					return utils.Errorf("create temporary nuclei template failed: %s", err)
				}
				handledUUID = true
				if pluginName != "" {
					log.Infof("Generate Temporary Yaml PoC Plugin: %s", pluginName)
				}
				if !utils.StringArrayContains(plugins, "nuclei") {
					plugins = append(plugins, "nuclei")
				}
			}
		}

		if len(portScan) > 0 {
			log.Warn("portscan plugin is unfinished supporting")
		}
		if len(yakMITM) > 0 {
			log.Warn("mitm plugin is unfinished supporting")
		}

		db = db.Model(&yakit.YakScript{})
		db.Where("type IN ?", plugins)
		if handledUUID {
			db = db.Where("script_name LIKE ?", "%"+uid)
		}
		if keyword != "" {
			db = bizhelper.FuzzSearchEx(db, []string{
				"script_name", "tags", "content", "help",
			}, keyword, false)
		}
		db = db.Order("updated_at desc")

		pluginList := omap.NewOrderedMap(map[string]*yakit.YakScript{})
		for result := range yakit.YieldYakScripts(db, context.Background()) {
			log.Infof("start to load plugin: %s", result.ScriptName)
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

		// build targets
		targetsLine := []string{c.String("target")}
		targetsLine = append(targetsLine, c.Args()...)
		targets := &ypb.HybridScanInputTarget{
			Input:     strings.Join(targetsLine, "\n"),
			InputFile: utils.PrettifyListFromStringSplitEx(c.String("target-file"), ","),
		}
		if ret := c.String("raw"); ret != "" {
			raw, err := os.ReadFile(ret)
			if err != nil {
				return utils.Errorf("read raw packet file failed: %s", err)
			}
			targets.HTTPRequestTemplate = &ypb.HTTPRequestBuilderParams{
				IsRawHTTPRequest: true,
				IsHttps:          c.Bool("https"),
				RawHTTPRequest:   raw,
			}
		}

		gen, err := yakgrpc.TargetGenerator(context.Background(), consts.GetGormProjectDatabase(), targets)
		if err != nil {
			return utils.Errorf("generate target failed: %s", err)
		}

		runtimeId := uuid.New().String()

		thread := c.Int("thread")
		if thread <= 0 {
			thread = 50
		}
		swg := utils.NewSizedWaitGroup(thread)
		publicFilter := filter.NewFilter()
		public := yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
			if risk, ok := yakit.IsRiskExecResult(i); ok {
				log.Infof("risk: %s", risk.Title)
			}
			return nil
		})
		if err != nil {
			return utils.Errorf("create local client failed: %s", err)
		}
		for target := range gen {
			log.Infof("start to scan target: %p with plugins list cap: %v", target, pluginList.Len())
			for _, plugin := range pluginList.Values() {
				log.Debugf("prepare target: %p in: %v", target, plugin.ScriptName)
				swg.Add()
				plugin := plugin
				go func() {
					defer swg.Done()

					defer func() {
						if err := recover(); err != nil {
							log.Errorf("scan target failed(panic): %s", err)
						}
					}()
					err := yakgrpc.ScanHybridTargetWithPlugin(
						runtimeId, context.Background(), target, plugin, c.String("proxy"),
						public, publicFilter,
					)
					if err != nil {
						log.Errorf("scan target failed: %s", err)
					}
				}()
			}
		}
		log.Infof("start to waiting for all scan finished")
		swg.Wait()

		log.Infof("start to checking runtimeId: %s", runtimeId)
		for riskInfo := range yakit.YieldRisksByRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId) {
			log.Infof("match risk: %s", riskInfo.Title)
			riskInfo.ColorizedShow()
		}

		return nil
	},
}
