package main

import (
	"fmt"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/rag/generate_index_tool"
)

// clearCommand clear 命令处理函数
func clearCommand(c *cli.Context) error {
	cacheDir := c.String("cache")
	confirm := c.Bool("confirm")

	if !confirm {
		fmt.Printf("即将清空缓存目录: %s\n", cacheDir)
		fmt.Print("确认继续吗? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("操作已取消")
			return nil
		}
	}

	fmt.Printf("清空缓存目录: %s\n", cacheDir)

	cacheManager := generate_index_tool.NewFileCacheManager(cacheDir)
	if err := cacheManager.Clear(); err != nil {
		return fmt.Errorf("清空缓存失败: %v", err)
	}

	fmt.Println("缓存已清空")
	return nil
}
