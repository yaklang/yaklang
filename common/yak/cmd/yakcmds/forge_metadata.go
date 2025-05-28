package yakcmds

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"os"
)

func createBuildInForgeMetadataCommand() *cli.Command {
	command := &cli.Command{}
	command.Name = "update-forge-metadata"
	command.Description = "forge-metadata 是一个用于生成和更新 yaklang buildin forge 的 metadata 的命令行工具"
	command.UsageText = `format: yak forge-metadata --input <yak_tool_dir> --output <output_dir>

yak update-forge-metadata -p <forge_dir> -o <output_dir> -f
`
	command.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "path,p",
			Usage: "forge 目录路径",
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "输出目录路径",
		},
		cli.BoolFlag{
			Name:  "force,f",
			Usage: "强制更新所有脚本的元数据，即使已经有元数据",
		},
		cli.IntFlag{
			Name:  "concurrency,c",
			Usage: "并发处理的数量",
			Value: 20,
		},
	}

	command.Action = func(c *cli.Context) error {
		inputDir := c.String("path")
		outputDir := c.String("output")
		forceUpdate := c.Bool("force")
		concurrency := c.Int("concurrency")

		if inputDir == "" {
			return fmt.Errorf("input directory is required")
		}

		if concurrency <= 0 {
			concurrency = 1 // 默认不并发
		}

		// 检查输入目录是否存在
		if _, err := os.Stat(inputDir); os.IsNotExist(err) {
			return fmt.Errorf("input directory does not exist: %s", inputDir)
		}

		// 处理所有脚本
		err := aiforge.UpdateForgesMetaData(inputDir, outputDir, concurrency, forceUpdate)
		if err != nil {
			return fmt.Errorf("failed to process yak scripts: %v", err)
		}

		log.Infof("Successfully processed yak scripts from %s to %s", inputDir, outputDir)
		return nil
	}

	return command
}
