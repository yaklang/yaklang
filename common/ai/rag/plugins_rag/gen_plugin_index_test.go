package plugins_rag_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/generate_index_tool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
)

func TestGenPluginIndex(t *testing.T) {
	profileDB := consts.GetGormProfileDatabase()
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatal(err)
	}

	tempDir := consts.TempAIDir()
	defer os.RemoveAll(tempDir)

	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	manager, err := generate_index_tool.CreateIndexManager(
		db, "test_plugin_index",
		"脚本向量库",
		generate_index_tool.WithConcurrentWorkers(10),
		generate_index_tool.WithCacheDir(tempDir),
	)
	if err != nil {
		t.Fatal(err)
	}

	var allScripts []*schema.YakScript

	if err := profileDB.Where("ignored = ?", false).Find(&allScripts).Error; err != nil {
		t.Fatal(err)
	}

	// 转换为可索引项
	items := generate_index_tool.ConvertScriptsToIndexableItems(allScripts)

	// 执行索引
	result, err := manager.IndexItems(context.Background(), items)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("索引完成，成功: %d, 失败: %d, 跳过: %d, 耗时: %s\n", result.SuccessCount, len(result.FailedItems), result.SkippedCount, result.Duration)
	fmt.Printf("失败的项目: %v\n", result.FailedItems)
	fmt.Printf("跳过的项目: %v\n", result.SkippedCount)
	fmt.Printf("耗时: %s\n", result.Duration)
}
