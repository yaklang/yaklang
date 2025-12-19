package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/rag/generate_index_tool"
	"github.com/yaklang/yaklang/common/ai/rag/rag_search_tool"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/depinjector"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	_ "github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	_ "github.com/yaklang/yaklang/common/aiforge"
)

func init() {
	depinjector.DependencyInject()
	yakit.LoadGlobalNetworkConfig()
}
func main() {
	app := &cli.App{
		Name:        "build-vector-index",
		Usage:       "构建AITool和Forge向量索引",
		Description: "用于为AITool和AIForge创建向量索引，支持RAG语义搜索",
		Version:     "1.0.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "cache",
				Usage: "缓存目录路径",
			},
			&cli.IntFlag{
				Name:  "workers",
				Value: 1,
				Usage: "并发工作协程数",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "强制绕过缓存，重新处理所有数据",
			},
		},
		Commands: []cli.Command{
			{
				Name:    "aitool",
				Aliases: []string{"tool"},
				Usage:   "构建AITool向量索引",
				Description: `为所有AITool构建向量索引，用于支持AI工具的语义搜索功能。
索引会存储在Profile数据库中，集合名称为: ` + rag_search_tool.AIToolVectorIndexName,
				Action: buildAIToolIndex,
			},
			{
				Name:    "forge",
				Aliases: []string{"f"},
				Usage:   "构建Forge向量索引",
				Description: `为所有AIForge构建向量索引，用于支持Forge的语义搜索功能。
索引会存储在Profile数据库中，集合名称为: ` + rag_search_tool.ForgeVectorIndexName,
				Action: buildForgeIndex,
			},
			{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   "构建所有向量索引（AITool和Forge）",
				Description: `一次性构建AITool和Forge的向量索引。
这会依次执行两个索引的构建过程。`,
				Action: buildAllIndex,
			},
			{
				Name:    "export",
				Aliases: []string{"e"},
				Usage:   "导出向量索引到文件",
				Description: `将向量索引导出为二进制文件，便于备份或迁移。
支持导出 AITool 或 Forge 索引。`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "type",
						Usage:    "索引类型：aitool 或 forge",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "output,o",
						Usage: "输出文件路径（如果未指定，将生成默认文件名）",
					},
				},
				Action: exportIndex,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// buildAIToolIndex 构建AITool向量索引
func buildAIToolIndex(c *cli.Context) error {
	cacheDir := c.GlobalString("cache")
	if cacheDir == "" {
		cacheDir = os.TempDir()
	}
	workers := c.GlobalInt("workers")
	force := c.GlobalBool("force")

	fmt.Printf("开始构建AITool向量索引...\n")
	fmt.Printf("集合名称: %s\n", rag_search_tool.AIToolVectorIndexName)
	fmt.Printf("缓存目录: %s\n", cacheDir)
	fmt.Printf("并发工作数: %d\n", workers)
	fmt.Printf("强制更新: %t\n", force)
	fmt.Println()

	db := consts.GetGormProfileDatabase()

	// 创建索引管理器
	opts := []generate_index_tool.OptionFunc{
		generate_index_tool.WithCacheDir(cacheDir + "/aitool"),
		generate_index_tool.WithConcurrentWorkers(workers),
		generate_index_tool.WithForceBypassCache(force),
		generate_index_tool.WithDefaultAIProcessor(), // 使用默认 AI 处理器，通过 aicommon.InvokeLiteForge 调用
		generate_index_tool.WithProgressCallback(func(current, total int, message string) {
			fmt.Printf("进度: %d/%d - %s\n", current, total, message)
		}),
	}
	fmt.Println("使用默认AI处理器")

	manager, err := generate_index_tool.CreateIndexManager(
		db,
		rag_search_tool.AIToolVectorIndexName,
		"AITool向量索引",
		opts...,
	)
	if err != nil {
		return fmt.Errorf("创建索引管理器失败: %v", err)
	}

	// 获取所有AITool
	allTools := buildinaitools.GetAllTools()
	fmt.Printf("找到 %d 个AITool\n", len(allTools))

	// 转换为可索引项
	allToolItems := []generate_index_tool.IndexableItem{}
	for _, tool := range allTools {
		content := fmt.Sprintf("名称: %s\n描述: %s\n关键词: %s\n参数: %s",
			tool.GetName(),
			tool.GetDescription(),
			strings.Join(tool.GetKeywords(), ", "),
			tool.Params().String())

		allToolItems = append(allToolItems, generate_index_tool.NewCommonIndexableItem(
			tool.GetName(),
			content,
			map[string]interface{}{
				"name": tool.GetName(),
			},
			tool.GetName()))
	}

	// 执行索引
	result, err := manager.IndexItems(context.Background(), allToolItems)
	if err != nil {
		return fmt.Errorf("索引失败: %v", err)
	}

	fmt.Printf("\n✅ AITool索引完成！\n")
	fmt.Printf("成功: %d, 失败: %d, 跳过: %d, 耗时: %s\n",
		result.SuccessCount, len(result.FailedItems), result.SkippedCount, result.Duration)

	if len(result.FailedItems) > 0 {
		fmt.Printf("\n失败的项目:\n")
		for _, failedItem := range result.FailedItems {
			fmt.Printf("  - %s: %s\n", failedItem.Key, failedItem.Error)
		}
	}

	return nil
}

// buildForgeIndex 构建Forge向量索引
func buildForgeIndex(c *cli.Context) error {
	cacheDir := c.GlobalString("cache")
	if cacheDir == "" {
		cacheDir = os.TempDir()
	}
	workers := c.GlobalInt("workers")
	force := c.GlobalBool("force")

	fmt.Printf("开始构建Forge向量索引...\n")
	fmt.Printf("集合名称: %s\n", rag_search_tool.ForgeVectorIndexName)
	fmt.Printf("缓存目录: %s\n", cacheDir)
	fmt.Printf("并发工作数: %d\n", workers)
	fmt.Printf("强制更新: %t\n", force)
	fmt.Println()

	db := consts.GetGormProfileDatabase()

	// 创建索引管理器
	opts := []generate_index_tool.OptionFunc{
		generate_index_tool.WithCacheDir(cacheDir + "/forge"),
		generate_index_tool.WithConcurrentWorkers(workers),
		generate_index_tool.WithForceBypassCache(force),
		generate_index_tool.WithDefaultAIProcessor(), // 使用默认 AI 处理器，通过 aicommon.InvokeLiteForge 调用
		generate_index_tool.WithProgressCallback(func(current, total int, message string) {
			fmt.Printf("进度: %d/%d - %s\n", current, total, message)
		}),
	}
	fmt.Println("使用默认AI处理器")

	manager, err := generate_index_tool.CreateIndexManager(
		db,
		rag_search_tool.ForgeVectorIndexName,
		"Forge向量索引",
		opts...,
	)
	if err != nil {
		return fmt.Errorf("创建索引管理器失败: %v", err)
	}

	// 获取所有Forge
	allForges, err := yakit.GetAllAIForge(db)
	if err != nil {
		return fmt.Errorf("获取Forge列表失败: %v", err)
	}
	fmt.Printf("找到 %d 个Forge\n", len(allForges))

	// 转换为可索引项
	allForgeItems := []generate_index_tool.IndexableItem{}
	for _, forge := range allForges {
		content := fmt.Sprintf("名称: %s\n显示名称: %s\n描述: %s\n标签: %s\n工具关键词: %s",
			forge.ForgeName,
			forge.ForgeVerboseName,
			forge.Description,
			forge.Tags,
			forge.ToolKeywords)

		allForgeItems = append(allForgeItems, generate_index_tool.NewCommonIndexableItem(
			forge.ForgeName,
			content,
			map[string]interface{}{
				"name":         forge.ForgeName,
				"verbose_name": forge.ForgeVerboseName,
				"description":  forge.Description,
			},
			forge.ForgeName))
	}

	// 执行索引
	result, err := manager.IndexItems(context.Background(), allForgeItems)
	if err != nil {
		return fmt.Errorf("索引失败: %v", err)
	}

	fmt.Printf("\n✅ Forge索引完成！\n")
	fmt.Printf("成功: %d, 失败: %d, 跳过: %d, 耗时: %s\n",
		result.SuccessCount, len(result.FailedItems), result.SkippedCount, result.Duration)

	if len(result.FailedItems) > 0 {
		fmt.Printf("\n失败的项目:\n")
		for _, failedItem := range result.FailedItems {
			fmt.Printf("  - %s: %s\n", failedItem.Key, failedItem.Error)
		}
	}

	return nil
}

// buildAllIndex 构建所有索引
func buildAllIndex(c *cli.Context) error {
	fmt.Println("==================== 构建所有向量索引 ====================")
	fmt.Println()

	// 构建AITool索引
	fmt.Println("【1/2】构建AITool索引")
	fmt.Println("======================================================")
	if err := buildAIToolIndex(c); err != nil {
		return fmt.Errorf("构建AITool索引失败: %v", err)
	}

	fmt.Println()
	fmt.Println()

	// 构建Forge索引
	fmt.Println("【2/2】构建Forge索引")
	fmt.Println("======================================================")
	if err := buildForgeIndex(c); err != nil {
		return fmt.Errorf("构建Forge索引失败: %v", err)
	}

	fmt.Println()
	fmt.Println("==================== 所有索引构建完成 ====================")
	return nil
}

// exportIndex 导出向量索引
func exportIndex(c *cli.Context) error {
	indexType := c.String("type")
	outputPath := c.String("output")

	if indexType == "" {
		return fmt.Errorf("必须指定索引类型 (--type aitool 或 --type forge)")
	}

	var collectionName string
	var defaultFileName string

	switch indexType {
	case "aitool", "tool":
		collectionName = rag_search_tool.AIToolVectorIndexName
		defaultFileName = "aitool_index.bin"
	case "forge", "f":
		collectionName = rag_search_tool.ForgeVectorIndexName
		defaultFileName = "forge_index.bin"
	default:
		return fmt.Errorf("未知的索引类型: %s (支持: aitool, forge)", indexType)
	}

	// 如果未指定输出路径，使用默认路径
	if outputPath == "" {
		timestamp := time.Now().Format("20060102_150405")
		outputPath = filepath.Join(".", fmt.Sprintf("%s_%s", timestamp, defaultFileName))
	}

	// 确保输出目录存在
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("创建输出目录失败: %v", err)
		}
	}

	fmt.Printf("开始导出向量索引...\n")
	fmt.Printf("集合名称: %s\n", collectionName)
	fmt.Printf("输出路径: %s\n", outputPath)
	fmt.Println()

	db := consts.GetGormProfileDatabase()

	// 导出索引
	reader, err := vectorstore.ExportRAGToBinary(
		collectionName,
		vectorstore.WithImportExportDB(db),
		vectorstore.WithProgressHandler(func(percent float64, message string, messageType string) {
			fmt.Printf("[%.1f%%] %s\n", percent, message)
		}),
	)
	if err != nil {
		return fmt.Errorf("导出失败: %v", err)
	}

	// 创建输出文件
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %v", err)
	}
	defer outFile.Close()

	// 复制数据到文件
	written, err := io.Copy(outFile, reader)
	if err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Println()
	fmt.Printf("导出成功！\n")
	fmt.Printf("文件大小: %.2f MB\n", float64(written)/(1024*1024))
	fmt.Printf("保存位置: %s\n", outputPath)

	return nil
}
