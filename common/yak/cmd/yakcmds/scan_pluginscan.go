package yakcmds

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	"github.com/segmentio/ksuid"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
		cli.StringSliceFlag{
			Name:  "vars,var",
			Usage: "Inject template variable (KEY=VALUE). Can be specified multiple times",
		},
		cli.StringFlag{
			Name:  "vars-file,varfile,var-file",
			Usage: "Load template variables from file (each line 'Key: Value')",
		},
		cli.StringFlag{
			Name: "plugin", Usage: "Exec Single Plugin by Name",
		},
		cli.StringFlag{
			Name: "type", Usage: `Type of Plugins in Yakit, port-scan / mitm / yaml-poc`,
		},
		cli.StringFlag{Name: "proxy", Usage: "Proxy Server, like http://127.0.0.1:8083"},
		cli.IntFlag{Name: "concurrent,thread", Usage: "(Thread)Concurrent Scan Number", Value: 50},
		cli.Float64Flag{Name: "poc-timeout", Usage: "Scan Timeout in Which Sub-Task", Value: 30},
		cli.Float64Flag{Name: "total-timeout", Usage: "Scan Timeout for all", Value: 7200},
	},

	Before: func(c *cli.Context) error {
		// in this mode, the log will be short and limited
		os.Setenv(`YAK_IN_TERMINAL_MODE`, "1")
		return nil
	},
	Action: func(c *cli.Context) error {
		fmt.Print(yakitScanBanner)

		db := consts.GetGormProfileDatabase()
		if db == nil {
			return utils.Error("yak profile/plugin database is nil")
		}

		// fix plugin
		plugins := utils.PrettifyListFromStringSplitEx(c.String("type"), ",", "|", "\n")
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

		pluginName := c.String("plugin")

		if len(plugins) == 0 || len(pluginName) != 0 {
			plugins = []string{"mitm", "nuclei", "port-scan"}
		}

		keyword := c.String("k")

		// remove temporary plugin
		if c.String("templates") != "" {
			log.Infof("start to load templates: %v", c.String("templates"))
		}
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
			log.Fatalf("unsupported file type: %s", filename)
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
		clearFuncs := make([]func(), 0, len(templatesCodes))
		if len(templatesCodes) > 0 {
			for _, temp := range templatesCodes {
				pluginName, clearFunc, err := yakit.CreateTemporaryYakScriptEx(`nuclei`, temp, uid)
				if err != nil {
					return utils.Errorf("create temporary nuclei template failed: %s", err)
				}
				clearFuncs = append(clearFuncs, clearFunc)
				handledUUID = true
				if pluginName != "" {
					log.Infof("Generate Temporary Yaml PoC Plugin: %s", pluginName)
				}
				if !utils.StringArrayContains(plugins, "nuclei") {
					plugins = append(plugins, "nuclei")
				}
			}
		}
		if len(clearFuncs) > 0 {
			defer func() {
				for _, f := range clearFuncs {
					f()
				}
			}()
		}

		if len(portScan) > 0 {
			log.Warn("portscan plugin is unfinished supporting")
		}
		if len(yakMITM) > 0 {
			log.Warn("mitm plugin is unfinished supporting")
		}

		db = db.Model(&schema.YakScript{}).Where("type IN (?)", plugins)
		if handledUUID {
			db = db.Where("script_name LIKE ?", "%"+uid)
		}
		if keyword != "" {
			db = bizhelper.FuzzSearchEx(db, []string{
				"script_name", "tags", "content", "help",
			}, keyword, false)
		}
		db = db.Order("updated_at desc")

		pluginList := omap.NewOrderedMap(map[string]*schema.YakScript{})
		for result := range yakit.YieldYakScripts(db, context.Background()) {
			if pluginName != "" && pluginName != result.ScriptName {
				continue
			}
			if !c.Bool("list") {
				log.Infof("start to load plugin: %s", result.ScriptName)
			}
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
				name := []rune(result.ScriptName)
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
		if targets.GetInput() == "" && targets.GetInputFile() == nil && targets.GetHTTPRequestTemplate() == nil {
			log.Fatal("no target/target-file/raw-packet-file input")
		}
		varEntries := c.StringSlice("vars")
		varFile := c.String("vars-file")
		customVars, err := collectCustomVars(varEntries, varFile)
		if err != nil {
			return err
		}

		gen, err := yakgrpc.TargetGenerator(context.Background(), consts.GetGormProjectDatabase(), targets)
		if err != nil {
			return utils.Errorf("generate target failed: %s", err)
		}

		if len(customVars) > 0 {
			origGen := gen
			genWithVars := make(chan *yakgrpc.HybridScanTarget)
			go func() {
				defer close(genWithVars)
				for target := range origGen {
					if target.Vars == nil {
						target.Vars = map[string]any{}
					}
					injected, _ := target.Vars["INJECTED_VARS"].(map[string]any)
					if injected == nil {
						injected = make(map[string]any, len(customVars))
						target.Vars["INJECTED_VARS"] = injected
					}
					for k, v := range customVars {
						injected[k] = v
					}
					genWithVars <- target
				}
			}()
			gen = genWithVars
		}

		runtimeId := uuid.New().String()

		rootCtx := context.Background()
		if ret := c.Float64("total-timeout"); ret > 0 {
			rootCtx = utils.TimeoutContextSeconds(ret)
		}
		ctx, cancelAll := context.WithCancel(rootCtx)
		_ = cancelAll

		historyShowCtx, cancelHistory := context.WithCancel(ctx)
		defer cancelHistory()
		go func() {
			showHistoryFlow := utils.NewCoolDown(2 * time.Second)
			var last int64
			for {
				select {
				case <-historyShowCtx.Done():
					return
				default:
					showHistoryFlow.DoOr(func() {
						last = ShowHistoryHTTPFlowByRuntimeId(last, runtimeId)
					}, func() {
						time.Sleep(500 * time.Millisecond)
					})
				}
			}
		}()

		thread := c.Int("thread")
		if thread <= 0 {
			thread = 50
		}
		swg := utils.NewSizedWaitGroup(thread)
		publicFilter := filter.NewCuckooFilter()
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
			target := target
			log.Infof("start to scan target: %v with plugins list cap: %v", target.Url, pluginList.Len())
			for _, plugin := range pluginList.Values() {
				log.Debugf("prepare target: %p in: %v", target, plugin.ScriptName)
				swg.Add()
				plugin := plugin
				du := 30 * time.Second
				if ret := c.Float64("poc-timeout"); ret > 0 {
					du = utils.FloatSecondDuration(ret)
				}
				singleTaskCtx, singleTaskCancel := context.WithTimeout(ctx, du)
				go func() {
					defer swg.Done()
					defer singleTaskCancel()
					defer func() {
						if err := recover(); err != nil {
							log.Errorf("scan target failed(panic): %s", err)
						}
					}()

					err := yakgrpc.ScanHybridTargetWithPlugin(
						runtimeId, singleTaskCtx, target, plugin, c.String("proxy"),
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
		cancelHistory()

		log.Infof("start to checking runtimeId: %s", runtimeId)
		for riskInfo := range yakit.YieldRisksByRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId) {
			log.Infof("match risk: %s", riskInfo.Title)
			riskInfo.ColorizedShow()
		}

		return nil
	},
}

func ShowHistoryHTTPFlowByRuntimeId(last int64, runtimeId string) int64 {
	db := consts.GetGormProjectDatabase()
	db = db.Model(&schema.HTTPFlow{}).Where("runtime_id = ?", runtimeId)
	var count int64
	if db.Count(&count).Error == nil {
		if count != last {
			log.Infof("runtime_id: %v cause %v http flows", runtimeId, count)
			return count
		}
	}
	return last
}

func collectCustomVars(entries []string, varsFile string) (map[string]any, error) {
	result := make(map[string]any)
	if varsFile != "" {
		fileVars, err := loadVarsFromFile(varsFile)
		if err != nil {
			return nil, err
		}
		for k, v := range fileVars {
			result[k] = v
		}
	}
	for _, entry := range entries {
		key, value, err := parseKeyValue(entry)
		if err != nil {
			return nil, err
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func parseKeyValue(raw string) (string, string, error) {
	parts := strings.SplitN(raw, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid vars format: %s (expected KEY=VALUE)", raw)
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if key == "" {
		return "", "", fmt.Errorf("empty variable name in %s", raw)
	}
	return key, value, nil
}

func loadVarsFromFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	res := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid vars-file line %d: %s", lineNum, line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid vars-file line %d: empty key", lineNum)
		}
		res[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
