package main

import (
	"fmt"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/rag/generate_index_tool"
)

// searchCommand search 命令处理函数
func searchCommand(c *cli.Context) error {
	db, err := getDatabase(c)
	if err != nil {
		return err
	}
	defer db.Close()

	collectionName := c.String("collection")
	query := c.String("query")
	limit := c.Int("limit")
	page := c.Int("page")
	verbose := c.Bool("verbose")

	fmt.Printf("在集合 %s 中搜索: %s\n", collectionName, query)
	fmt.Printf("页码: %d, 限制: %d\n", page, limit)
	fmt.Println()

	// 创建索引管理器
	manager, err := generate_index_tool.CreateIndexManager(db, collectionName, "脚本搜索")
	if err != nil {
		return fmt.Errorf("创建索引管理器失败: %v", err)
	}

	// 执行搜索
	results, err := manager.SearchItems(query, page, limit)
	if err != nil {
		return fmt.Errorf("搜索失败: %v", err)
	}

	fmt.Printf("搜索结果 (共 %d 个):\n", len(results))
	for i, result := range results {
		fmt.Printf("\n结果 %d:\n", i+1)
		fmt.Printf("  ID: %s\n", result.Document.ID)
		fmt.Printf("  得分: %.4f\n", result.Score)

		if verbose {
			fmt.Printf("  内容预览: %s\n", truncateString(result.Document.Content, 200))

			if result.Document.Metadata != nil {
				fmt.Printf("  元数据:\n")
				for k, v := range result.Document.Metadata {
					fmt.Printf("    %s: %v\n", k, v)
				}
			}
		} else {
			fmt.Printf("  内容预览: %s\n", truncateString(result.Document.Content, 100))
		}
	}

	// 获取总数
	total, err := manager.GetTotalCount()
	if err == nil {
		fmt.Printf("\n集合中总文档数: %d\n", total)
	}

	return nil
}
