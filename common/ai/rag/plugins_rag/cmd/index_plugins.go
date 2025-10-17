package main

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

func getIndexPluginCommand() *cli.Command {
	return &cli.Command{
		Name:  "index",
		Usage: "索引插件",
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
				Name:  "baseurl,u",
				Usage: "Base URL",
				Value: "http://127.0.0.1:8080",
			},
			cli.IntFlag{
				Name:  "dimension, d",
				Usage: "向量维度",
				Value: 1024,
			},
			cli.StringFlag{
				Name:  "metadata-file,f",
				Usage: "元数据文件",
				Value: "",
			},
			cli.BoolFlag{
				Name:  "reset,r",
				Usage: "重置并重新生成索引",
			},
		},
		Action: indexPlugins,
	}
}
func indexPlugins(c *cli.Context) error {
	db := consts.GetGormProfileDatabase()
	if c.Bool("reset") {
		log.Infof("重置并重新生成索引...")
		rag.DeleteCollection(db, plugins_rag.PLUGIN_RAG_COLLECTION_NAME)
	}

	model := c.String("model")
	apiKey := c.String("api-key")
	baseURL := c.String("base-url")
	dimension := c.Int("dimension")
	metadataFile := c.String("metadata-file")

	// 配置选项
	opts := []aispec.AIConfigOption{aispec.WithModel(model)}
	if apiKey != "" {
		opts = append(opts, aispec.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, aispec.WithBaseURL(baseURL))
	}
	manager, err := plugins_rag.NewSQLitePluginsRagManager(db, plugins_rag.PLUGIN_RAG_COLLECTION_NAME, model, dimension, metadataFile, opts...)
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

	return nil
}
