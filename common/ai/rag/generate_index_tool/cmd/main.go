package main

import (
	"log"
	"os"

	"github.com/urfave/cli"
)

func main() {
	app := &cli.App{
		Name:        "generate-index-tool",
		Usage:       "通用向量库生成工具",
		Description: "用于将任意结构的数组生成向量库的命令行工具",
		Version:     "1.0.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "db,d",
				Value: "test.db",
				Usage: "数据库文件路径",
			},
			&cli.StringFlag{
				Name:  "collection,c",
				Value: "test_scripts",
				Usage: "向量库集合名称",
			},
			&cli.StringFlag{
				Name:  "cache",
				Value: "/tmp/index_cache",
				Usage: "缓存目录路径",
			},
		},
		Commands: []cli.Command{
			{
				Name:  "index,i",
				Usage: "索引脚本到向量库",
				Description: `将脚本数据索引到向量库中。支持三步处理流程：
1. 生成原始内容字符串
2. AI清洗和规范化内容
3. 生成向量并存储到RAG系统`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "ai",
						Usage: "是否使用AI处理内容（默认使用简单处理器）",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "强制绕过缓存，重新处理所有数据",
					},
					&cli.IntFlag{
						Name:  "batch-size",
						Value: 50,
						Usage: "批处理大小",
					},
					&cli.IntFlag{
						Name:  "workers",
						Value: 3,
						Usage: "并发工作协程数",
					},
					&cli.BoolFlag{
						Name:  "no-metadata",
						Usage: "不包含元数据信息",
					},
				},
				Action: indexCommand,
			},
			{
				Name:    "search",
				Aliases: []string{"s"},
				Usage:   "在向量库中搜索",
				Description: `使用自然语言在向量库中搜索相关内容。
支持语义搜索，能够理解查询意图并返回相关结果。`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "query",
						Usage:    "搜索查询语句 (必需)",
						Required: true,
					},
					&cli.IntFlag{
						Name:  "limit",
						Value: 10,
						Usage: "搜索结果数量限制",
					},
					&cli.IntFlag{
						Name:  "page",
						Value: 1,
						Usage: "搜索结果页码",
					},
					&cli.BoolFlag{
						Name:  "verbose",
						Usage: "显示详细的搜索结果信息",
					},
				},
				Action: searchCommand,
			},
			{
				Name:    "clear",
				Aliases: []string{"clean"},
				Usage:   "清空缓存",
				Description: `清空指定目录下的所有缓存文件。
这将删除原始内容缓存和处理后内容缓存。`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "confirm",
						Usage: "确认清空缓存（跳过确认提示）",
					},
				},
				Action: clearCommand,
			},
			{
				Name:    "info",
				Aliases: []string{"status"},
				Usage:   "显示向量库信息",
				Description: `显示指定集合的详细信息，包括：
- 文档总数
- 集合配置
- 缓存状态`,
				Action: infoCommand,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
