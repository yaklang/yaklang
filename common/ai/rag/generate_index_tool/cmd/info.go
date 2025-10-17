package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/rag/generate_index_tool"
)

// infoCommand info 命令处理函数
func infoCommand(c *cli.Context) error {
	db, err := getDatabase(c)
	if err != nil {
		return err
	}
	defer db.Close()

	collectionName := c.String("collection")
	cacheDir := c.String("cache")

	fmt.Printf("向量库信息 - 集合: %s\n", collectionName)
	fmt.Println(strings.Repeat("=", 50))

	// 创建索引管理器
	manager, err := generate_index_tool.CreateIndexManager(db, collectionName, "信息查询")
	if err != nil {
		return fmt.Errorf("创建索引管理器失败: %v", err)
	}

	// 获取文档总数
	total, err := manager.GetTotalCount()
	if err != nil {
		fmt.Printf("无法获取文档总数: %v\n", err)
	} else {
		fmt.Printf("文档总数: %d\n", total)
	}

	// 检查缓存状态
	cacheManager := generate_index_tool.NewFileCacheManager(cacheDir)

	rawCache, err := cacheManager.LoadRawCache()
	if err != nil {
		fmt.Printf("原始内容缓存: 无法加载 (%v)\n", err)
	} else {
		fmt.Printf("原始内容缓存: %d 项\n", len(rawCache))
	}

	processedCache, err := cacheManager.LoadProcessedCache()
	if err != nil {
		fmt.Printf("处理后缓存: 无法加载 (%v)\n", err)
	} else {
		fmt.Printf("处理后缓存: %d 项\n", len(processedCache))
	}

	fmt.Printf("缓存目录: %s\n", cacheDir)

	return nil
}
