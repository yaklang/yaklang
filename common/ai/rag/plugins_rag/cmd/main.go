package main

import (
	"os"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func main() {
	yakit.LoadGlobalNetworkConfig()
	app := cli.NewApp()
	app.Name = "plugins_rag"
	app.Usage = "Yaklang 插件 RAG 系统：索引和搜索插件"
	app.Version = "1.0.0"

	app.Flags = []cli.Flag{}

	app.Commands = []cli.Command{
		*getIndexPluginCommand(),
		*getSearchPluginCommand(),
		*getListCollectionCommand(),
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("运行失败: %v", err)
		os.Exit(1)
	}
}
