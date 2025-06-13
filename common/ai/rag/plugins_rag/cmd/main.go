package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func init() {
	plugins_rag.GenerateYakScriptMetadata = func(script string) (*plugins_rag.GenerateResult, error) {
		res, err := metadata.GenerateYakScriptMetadata(script)
		if err != nil {
			return nil, err
		}
		return &plugins_rag.GenerateResult{
			Language:    res.Language,
			Description: res.Description,
			Keywords:    res.Keywords,
		}, nil
	}
}
func main() {
	yakit.LoadGlobalNetworkConfig()
	app := cli.NewApp()
	app.Name = "plugins_rag"
	app.Usage = "Yaklang 插件 RAG 系统：索引和搜索插件"
	app.Version = "1.0.0"

	var apiKey, model, query string
	var baseURL string
	var limit int
	var collectionName string
	var dimension int
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "api-key",
			Usage:       "OpenAI API Key",
			EnvVar:      "OPENAI_API_KEY",
			Destination: &apiKey,
		},
		cli.StringFlag{
			Name:        "model,m",
			Usage:       "OpenAI 嵌入模型名称",
			Value:       "Qwen3-Embedding-0.6B-Q8_0",
			Destination: &model,
		},
		cli.StringFlag{
			Name:        "query, q",
			Usage:       "搜索查询",
			Destination: &query,
		},
		cli.IntFlag{
			Name:        "limit, l",
			Usage:       "结果数量限制",
			Value:       5,
			Destination: &limit,
		},
		cli.StringFlag{
			Name:        "collection, c",
			Usage:       "SQLite 集合名称",
			Value:       plugins_rag.DefaultCollectionName,
			Destination: &collectionName,
		},
		cli.StringFlag{
			Name:        "baseurl,u",
			Usage:       "Base URL",
			Value:       "http://127.0.0.1:8080",
			Destination: &baseURL,
		},
		cli.IntFlag{
			Name:        "dimension, d",
			Usage:       "向量维度",
			Value:       1024,
			Destination: &dimension,
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "index",
			Usage: "索引所有插件",
			Action: func(c *cli.Context) error {
				// 配置选项
				opts := []aispec.AIConfigOption{aispec.WithModel(model)}
				if apiKey != "" {
					opts = append(opts, aispec.WithAPIKey(apiKey))
				}
				if baseURL != "" {
					opts = append(opts, aispec.WithBaseURL(baseURL))
				}
				manager, err := plugins_rag.CreateSQLiteManager(collectionName, model, dimension, opts...)
				if err != nil {
					log.Errorf("创建插件 RAG 管理器失败: %v", err)
					return err
				}

				// 索引所有插件
				log.Infof("开始索引插件...")
				err = manager.IndexAllPlugins()
				if err != nil {
					log.Errorf("索引插件失败: %v", err)
					return err
				}

				log.Infof("成功索引 %d 个插件", manager.GetIndexedPluginsCount())
				return nil
			},
		},
		{
			Name:  "search",
			Usage: "搜索插件",
			Action: func(c *cli.Context) error {
				if query == "" {
					return fmt.Errorf("请提供搜索查询，使用 --query 参数")
				}

				// 配置选项
				opts := []aispec.AIConfigOption{aispec.WithModel(model)}
				if apiKey != "" {
					opts = append(opts, aispec.WithAPIKey(apiKey))
				}
				if baseURL != "" {
					opts = append(opts, aispec.WithBaseURL(baseURL))
				}
				manager, err := plugins_rag.CreateSQLiteManager(collectionName, model, dimension, opts...)
				if err != nil {
					log.Errorf("创建插件 RAG 管理器失败: %v", err)
					return err
				}

				// 索引所有插件（确保已索引）
				// log.Infof("确保插件已索引...")
				// err = manager.IndexAllPlugins()
				// if err != nil {
				// 	log.Errorf("索引插件失败: %v", err)
				// 	return err
				// }

				// 搜索插件
				log.Infof("搜索查询: %s", query)
				results, err := manager.SearchPlugins(query, limit)
				if err != nil {
					log.Errorf("搜索失败: %v", err)
					return err
				}

				// 显示结果
				log.Infof("找到 %d 个结果:", len(results))
				for i, item := range results {
					script := item.Script
					fmt.Printf("\n--- 结果 #%d ---\n", i+1)
					fmt.Printf("名称: %s\n", script.ScriptName)
					fmt.Printf("类型: %s\n", script.Type)
					fmt.Printf("作者: %s\n", script.Author)
					fmt.Printf("标签: %s\n", script.Tags)
					fmt.Printf("描述: %s\n", script.Help)
					fmt.Printf("----------------------\n")
				}

				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("运行失败: %v", err)
		os.Exit(1)
	}
}
