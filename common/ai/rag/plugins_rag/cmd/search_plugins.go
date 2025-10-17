package main

import (
	"fmt"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

func getSearchPluginCommand() *cli.Command {
	return &cli.Command{
		Name:   "search",
		Usage:  "搜索插件",
		Action: searchPlugins,
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:   "api-key",
				Usage:  "OpenAI API Key",
				EnvVar: "OPENAI_API_KEY",
			},
			cli.StringFlag{
				Name:  "model,m",
				Usage: "OpenAI 嵌入模型名称",
				Value: "Qwen3-Embedding-0.6B-Q4_K_M",
			},
			cli.StringFlag{
				Name:  "query, q",
				Usage: "搜索查询",
			},
			cli.IntFlag{
				Name:  "limit, l",
				Usage: "结果数量限制",
				Value: 5,
			},
			cli.StringFlag{
				Name:  "baseurl,u",
				Usage: "Base URL",
				Value: "http://127.0.0.1:8080",
			},
			cli.IntFlag{
				Name:  "dimension, d",
				Usage: "向量维度",
				Value: 1024,
			},
		},
	}
}
func searchPlugins(c *cli.Context) error {

	model := c.String("model")
	apiKey := c.String("api-key")
	baseURL := c.String("base-url")
	dimension := c.Int("dimension")
	limit := c.Int("limit")

	query := c.String("query")
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
	db := consts.GetGormProfileDatabase()
	manager, err := plugins_rag.NewSQLitePluginsRagManager(db, plugins_rag.PLUGIN_RAG_COLLECTION_NAME, model, dimension, "", opts...)
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
}
