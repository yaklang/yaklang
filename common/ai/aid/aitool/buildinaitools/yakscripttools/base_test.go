package yakscripttools

import (
	"embed"
	"io/fs"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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

	for _, tool := range tools {
		assert.Equal(t, tool.Author, schema.AIResourceAuthorBuiltin)
		assert.Equal(t, tool.IsBuiltin, true)
	}
}

func TestRetiredHTTPToolsNotEmbedded(t *testing.T) {
	tools, err := loadAllYakScriptFromEmbedFS()
	assert.NilError(t, err)
	loaded := make(map[string]bool, len(tools))
	for _, tool := range tools {
		loaded[tool.Name] = true
	}
	for _, name := range []string{"http_response_diff", "url_content_summary", "send_http_request_by_url", "send_http_request_packet"} {
		if loaded[name] {
			t.Fatalf("retired tool %q is still embedded", name)
		}
		if _, err := testYakScriptFS.ReadFile("yakscriptforai/http/" + name + ".yak"); err == nil {
			t.Fatalf("retired tool file %q is still embedded", name)
		}
	}
}

func TestRemoveRetiredBuiltInAIToolsPreservesCustomTools(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.NilError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)

	rows := []schema.AIYakTool{
		{Name: "http_response_diff", IsBuiltin: true},
		{Name: "url_content_summary", Author: schema.AIResourceAuthorBuiltin},
		{Name: "send_http_request_by_url", IsBuiltin: true},
		{Name: "send_http_request_packet", Author: "user", IsBuiltin: false},
		{Name: "do_http_request", IsBuiltin: true},
	}
	for _, row := range rows {
		assert.NilError(t, db.Create(&row).Error)
	}
	assert.NilError(t, removeRetiredBuiltInAITools(db))

	var remaining []schema.AIYakTool
	assert.NilError(t, db.Find(&remaining).Error)
	assert.Equal(t, len(remaining), 2)
	names := map[string]bool{}
	for _, row := range remaining {
		names[row.Name] = true
	}
	assert.Assert(t, names["send_http_request_packet"])
	assert.Assert(t, names["do_http_request"])
}
