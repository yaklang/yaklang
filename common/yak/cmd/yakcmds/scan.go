package yakcmds

import (
	"context"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
)

var ScanCommands = []*cli.Command{
	{
		Name:    "pull-plugins",
		Aliases: []string{"pull"},
		Usage:   "pull plugins from yaklang.io and nuclei-templates",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:   "proxy",
				Usage:  "Proxy Server(http/socks5...)",
				EnvVar: "http_proxy",
			},
			cli.StringFlag{
				Name:  "base-url,u",
				Usage: "yaklang / yakit plugin server url",
				Value: `https://www.yaklang.com/`,
			},
			cli.StringFlag{
				Name:  "nuclei-templates-url,n",
				Usage: "Nuclei Templates URL",
				Value: `https://github.com/projectdiscovery/nuclei-templates`,
			},
		},
		Action: func(c *cli.Context) error {
			client := yaklib.NewOnlineClient(c.String("base-url"))
			if c.String("proxy") != "" {
				consts.SetOnlineBaseUrlProxy(c.String("proxy"))
			}
			stream := client.DownloadYakitPluginAll(context.Background())
			count := 0
			for result := range stream.Chan {
				count++
				log.Infof("start to save plugin(%v/%v): %v", count, result.Total, result.Plugin.ScriptName)
				err := client.Save(consts.GetGormProfileDatabase(), result.Plugin)
				if err != nil {
					log.Errorf("save plugin failed: %s", err)
				}
			}

			tools.UpdatePoCWithUrl(c.String(`nuclei-templates-url`), c.String("proxy"))
			return nil
		},
	},
	{
		Name:  "update-nuclei-database",
		Usage: "Load Nuclei-Template into Local Yak Plugin Database",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  "no-cache",
				Usage: "do not use local file cache will not download from git",
			},
			cli.StringFlag{
				Name:  "url",
				Usage: "which url to download?",
				Value: `https://github.com/projectdiscovery/nuclei-templates`,
			},
		},
		Action: func(c *cli.Context) error {
			var err error
			err = yak.NewScriptEngine(1).ExecuteMain(`
loglevel("info")
log.info("start to load local database"); 
die(nuclei.UpdateDatabase())`, "main")
			if err != nil {
				log.Errorf("execute nuclei.UpdateDatabase() failed: %s", err)
				return err
			}
			return nil
		},
	},
	{
		Name: "remove-nuclei-database", Usage: "Remove Nuclei-Template from Local Yak Plugin Database",
		Action: func(c *cli.Context) error {
			err := tools.RemovePoCDatabase()
			if err != nil {
				log.Errorf("remove pocs failed: %s", err)
			}
			return nil
		},
	},
	&synscanCommand,
	&servicescanCommand,
	hybridScanCommand,
	&crawlerxCommand,
}
