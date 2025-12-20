package main

import (
	"fmt"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/rag/generate_index_tool"

	_ "github.com/yaklang/yaklang/common/ai/aid"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
)

// indexCommand index 命令处理函数
func indexCommand(c *cli.Context) error {
	db, err := getDatabase(c)
	if err != nil {
		return err
	}
	defer db.Close()

	collectionName := c.String("collection")
	cacheDir := c.String("cache")
	useAI := c.Bool("ai")
	forceBypass := c.Bool("force")
	batchSize := c.Int("batch-size")
	workers := c.Int("workers")
	includeMetadata := !c.Bool("no-metadata")

	fmt.Printf("开始索引脚本到集合: %s\n", collectionName)
	fmt.Printf("缓存目录: %s\n", cacheDir)
	fmt.Printf("使用AI处理: %t\n", useAI)
	fmt.Printf("强制绕过缓存: %t\n", forceBypass)
	fmt.Printf("批处理大小: %d\n", batchSize)
	fmt.Printf("并发工作数: %d\n", workers)
	fmt.Printf("包含元数据: %t\n", includeMetadata)
	fmt.Println()

	// 创建测试脚本数据
	testScripts := createTestScripts(db)

	// 配置选项
	var opts []generate_index_tool.OptionFunc
	opts = append(opts,
		generate_index_tool.WithCacheDir(cacheDir),
		generate_index_tool.WithForceBypassCache(forceBypass),
		generate_index_tool.WithIncludeMetadata(includeMetadata),
		generate_index_tool.WithBatchSize(batchSize),
		generate_index_tool.WithConcurrentWorkers(workers),
		generate_index_tool.WithProgressCallback(func(current, total int, message string) {
			fmt.Printf("进度: %d/%d - %s\n", current, total, message)
		}),
	)

	if useAI {
		opts = append(opts, generate_index_tool.WithDefaultAIProcessor())
	}

	// 执行索引
	result, err := generate_index_tool.QuickIndexScripts(db, collectionName, testScripts, opts...)
	if err != nil {
		return fmt.Errorf("索引失败: %v", err)
	}

	fmt.Printf("\n索引完成!\n")
	fmt.Printf("成功: %d\n", result.SuccessCount)
	fmt.Printf("失败: %d\n", len(result.FailedItems))
	fmt.Printf("跳过: %d\n", result.SkippedCount)
	fmt.Printf("耗时: %s\n", result.Duration)

	if len(result.FailedItems) > 0 {
		fmt.Printf("\n失败的项目:\n")
		for _, item := range result.FailedItems {
			fmt.Printf("  - %s: %s\n", item.Key, item.Error)
		}
	}

	return nil
}
