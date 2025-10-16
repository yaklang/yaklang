package yakscripttools

import (
	"embed"
	"io/fs"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

//go:embed yakscriptforai
var testYakScriptFS embed.FS

func TestLoadAllYakScriptFromEmbedFS(t *testing.T) {
	// 统计 embed FS 中所有 .yak 文件的数量，并收集文件名（去掉.yak后缀）
	expectedToolNames := make(map[string]bool)
	err := fs.WalkDir(testYakScriptFS, "yakscriptforai", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".yak") {
			// 获取工具名（文件名去掉.yak后缀）
			toolName := strings.TrimSuffix(d.Name(), ".yak")
			expectedToolNames[toolName] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历 embed FS 失败: %v", err)
	}

	expectedCount := len(expectedToolNames)
	t.Logf("yakscriptforai 目录下共有 %d 个 .yak 文件", expectedCount)

	// 调用 loadAllYakScriptFromEmbedFS 获取加载的工具
	tools, err := loadAllYakScriptFromEmbedFS()
	if err != nil {
		t.Fatalf("load all yak script from embed fs failed: %v", err)
	}
	actualCount := len(tools)

	// 收集实际加载的工具名
	actualToolNames := make(map[string]bool)
	for _, tool := range tools {
		actualToolNames[tool.Name] = true
	}

	t.Logf("从 EmbedFS 加载了 %d 个工具", actualCount)

	// 查找多出来的工具（在 EmbedFS 中但不在目录中）
	extraTools := []string{}
	for toolName := range actualToolNames {
		if !expectedToolNames[toolName] {
			extraTools = append(extraTools, toolName)
		}
	}

	// 查找缺少的工具（在目录中但不在 EmbedFS 中）
	missingTools := []string{}
	for toolName := range expectedToolNames {
		if !actualToolNames[toolName] {
			missingTools = append(missingTools, toolName)
		}
	}

	// 打印差异信息
	if len(extraTools) > 0 {
		t.Logf("多出来的工具（在 EmbedFS 中但不在目录中）共 %d 个:", len(extraTools))
		for _, toolName := range extraTools {
			t.Logf("  + %s", toolName)
		}
	}

	if len(missingTools) > 0 {
		t.Logf("缺少的工具（在目录中但不在 EmbedFS 中）共 %d 个:", len(missingTools))
		for _, toolName := range missingTools {
			t.Logf("  - %s", toolName)
		}
	}

	// 断言数量相等
	assert.Equal(t, expectedCount, actualCount,
		"从 EmbedFS 加载的工具数量(%d)应该等于 yakscriptforai 目录下 .yak 文件的数量(%d)",
		actualCount, expectedCount)

	// 额外检查：确保至少加载了一些工具
	assert.Assert(t, actualCount > 0, "应该至少加载了一些工具")
}
