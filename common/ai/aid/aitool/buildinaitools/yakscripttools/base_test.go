package yakscripttools

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"gotest.tools/v3/assert"
)

//go:embed yakscriptforai
var testYakScriptFS embed.FS

func TestBuiltinAIToolMetadataMatchesDynamicAnalysis(t *testing.T) {
	err := fs.WalkDir(testYakScriptFS, "yakscriptforai", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".yak") {
			return err
		}
		content, err := testYakScriptFS.ReadFile(path)
		assert.NilError(t, err)
		name := strings.TrimSuffix(d.Name(), ".yak")
		t.Run(name, func(t *testing.T) {
			_, hit := builtinmetadata.Lookup("tool:"+name, string(content))
			assert.Equal(t, hit, builtinmetadata.Enabled, "metadata activation for %s", name)
			want, got := LoadYakScriptToAiTools(name, string(content)), loadBuiltinAITool(name, string(content))
			assert.Assert(t, want != nil && got != nil)
			var wantSchema, gotSchema any
			assert.NilError(t, json.Unmarshal([]byte(want.Params), &wantSchema))
			assert.NilError(t, json.Unmarshal([]byte(got.Params), &gotSchema))
			assert.DeepEqual(t, gotSchema, wantSchema)
			want.Params, got.Params = "", ""
			assert.DeepEqual(t, got, want)
		})
		return nil
	})
	assert.NilError(t, err)
}

func TestBuiltinAIToolChangedSourceUsesDynamicAnalysis(t *testing.T) {
	// The name deliberately matches a builtin, but its source and parameter
	// schema have changed. A source-hash miss must use the new source.
	code := `cli.String("new_user_parameter", cli.setRequired(true))`
	_, hit := builtinmetadata.Lookup("tool:do_http_request", code)
	assert.Assert(t, !hit)
	tool := loadBuiltinAITool("do_http_request", code)
	assert.Assert(t, tool != nil)
	assert.Assert(t, strings.Contains(tool.Params, "new_user_parameter"))
	assert.Assert(t, !strings.Contains(tool.Params, "raw_http_request"))
}

func TestBuiltinAIToolSyncPreservesFavorites(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.NilError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)
	tool := &schema.AIYakTool{Name: "do_http_request", Content: "old content", IsFavorite: true, Author: "user"}
	assert.NilError(t, db.Create(tool).Error)
	assert.NilError(t, overrideYakScriptAiTools(db))
	stored, err := yakit.GetAIYakTool(db, tool.Name)
	assert.NilError(t, err)
	assert.Equal(t, stored.ID, tool.ID)
	assert.Equal(t, stored.IsFavorite, true)
	assert.Equal(t, stored.Author, "user")
	assert.Assert(t, stored.Content != "old content")
}

func TestLoadedAIToolPersistsAIOutputMode(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.NilError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)
	for _, tc := range []struct {
		name, code string
		mode       int
	}{
		{"plain", `println("hello")`, 1},
		{"output", `yakit.AIOutput("hello")`, 2},
		{"alias", `output = yakit.AIOutput; output("hello")`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := LoadYakScriptToAiTools(tc.name, tc.code)
			assert.Assert(t, tool != nil)
			assert.Equal(t, tool.EnableAIOutputLog, tc.mode)
			_, err := yakit.SaveAIYakTool(db, tool)
			assert.NilError(t, err)
			stored, err := yakit.GetAIYakTool(db, tc.name)
			assert.NilError(t, err)
			assert.Equal(t, stored.EnableAIOutputLog, ParseAIToolEnableAIOutputLog(tc.code))
		})
	}
	var pending int
	assert.NilError(t, db.Model(&schema.AIYakTool{}).Where("enable_ai_output_log = ?", 0).Count(&pending).Error)
	assert.Equal(t, pending, 0, "new tools must not need the legacy startup backfill")
}

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
		{Name: "url_content_summary", Author: schema.AIResourceAuthorBuiltin, IsBuiltin: true},
		{Name: "send_http_request_by_url", IsBuiltin: true},
		{Name: "send_http_request_packet", Author: "user", IsBuiltin: false},
		{Name: "do_http_request", IsBuiltin: true},
	}
	for _, row := range rows {
		assert.NilError(t, db.Create(&row).Error)
	}
	// The real edit path clears IsBuiltin but keeps the builtin author.
	edited, err := yakit.GetAIYakTool(db, "url_content_summary")
	assert.NilError(t, err)
	edited.IsBuiltin = false
	edited.Content = `println("user customized content")`
	_, err = yakit.UpdateAIYakToolByID(db, edited)
	assert.NilError(t, err)
	assert.NilError(t, removeRetiredBuiltInAITools(db))

	var remaining []schema.AIYakTool
	assert.NilError(t, db.Find(&remaining).Error)
	assert.Equal(t, len(remaining), 3)
	names := map[string]bool{}
	for _, row := range remaining {
		names[row.Name] = true
	}
	assert.Assert(t, names["send_http_request_packet"])
	assert.Assert(t, names["do_http_request"])
	preserved, err := yakit.GetAIYakTool(db, edited.Name)
	assert.NilError(t, err)
	assert.Equal(t, preserved.Author, schema.AIResourceAuthorBuiltin)
	assert.Equal(t, preserved.Content, edited.Content)
	assert.Equal(t, preserved.IsBuiltin, false)
}

func TestPCAPCoreToolRename(t *testing.T) {
	tools, err := loadAllYakScriptFromEmbedFS()
	assert.NilError(t, err)
	loaded := make(map[string]bool)
	for _, tool := range tools {
		loaded[tool.Name] = true
	}
	for _, name := range []string{"load_pcapfile", "query_pcapdb", "manage_pcapdb"} {
		assert.Assert(t, loaded[name], "missing PCAP core tool %s", name)
	}
	for _, name := range []string{"query_pcap_in_db", "pcapdb"} {
		assert.Assert(t, !loaded[name], "retired tool %s should not be embedded", name)
	}
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NilError(t, err)
	defer db.Close()
	assert.NilError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)
	assert.NilError(t, db.Create(&schema.AIYakTool{Name: "query_pcapdb", IsBuiltin: true}).Error)
	for _, name := range []string{"query_pcap_in_db", "pcapdb"} {
		assert.NilError(t, db.Create(&schema.AIYakTool{Name: name, IsBuiltin: true}).Error)
	}
	assert.NilError(t, removeRetiredBuiltInAITools(db))
	var remaining []schema.AIYakTool
	assert.NilError(t, db.Find(&remaining).Error)
	assert.Equal(t, len(remaining), 1)
	assert.Equal(t, remaining[0].Name, "query_pcapdb")
	// Edited/user-owned scripts with either old name must not be removed on sync.
	for _, name := range []string{"query_pcap_in_db", "pcapdb"} {
		assert.NilError(t, db.Create(&schema.AIYakTool{Name: name, Author: schema.AIResourceAuthorBuiltin, IsBuiltin: false}).Error)
	}
	assert.NilError(t, removeRetiredBuiltInAITools(db))
	var count int64
	assert.NilError(t, db.Model(&schema.AIYakTool{}).Count(&count).Error)
	assert.Equal(t, count, int64(3))
}

type failingAIToolFS struct {
	FileSystemWithHash
	content []byte
	err     error
}

func (f *failingAIToolFS) ReadFile(name string) ([]byte, error) {
	return f.content, f.err
}

func TestOverrideYakScriptAiToolsRollsBackAndRetries(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.NilError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)
	retired := &schema.AIYakTool{Name: "http_response_diff", IsBuiltin: true, Content: "original"}
	assert.NilError(t, db.Create(retired).Error)
	assert.NilError(t, db.Exec(`CREATE TRIGGER reject_ai_tool_insert BEFORE INSERT ON ai_yak_tools WHEN NEW.name = 'do_http_request' BEGIN SELECT RAISE(FAIL, 'injected tool write failure'); END`).Error)
	err = overrideYakScriptAiTools(db)
	assert.Assert(t, err != nil && strings.Contains(err.Error(), "injected tool write failure"), "save failure must propagate: %v", err)
	var count int
	assert.NilError(t, db.Model(&schema.AIYakTool{}).Count(&count).Error)
	assert.Equal(t, count, 1, "failed sync must roll back tool deletion and partial inserts")
	preserved, err := yakit.GetAIYakTool(db, retired.Name)
	assert.NilError(t, err)
	assert.Equal(t, preserved.Content, "original")
	assert.NilError(t, db.Exec(`DROP TRIGGER reject_ai_tool_insert`).Error)
	assert.NilError(t, overrideYakScriptAiTools(db))
	_, err = yakit.GetAIYakTool(db, retired.Name)
	assert.Assert(t, err != nil, "successful retry should remove retired builtins")
	_, err = yakit.GetAIYakTool(db, "do_http_request")
	assert.NilError(t, err)
}

func TestOverrideYakScriptAiToolsLoadFailureLeavesDatabaseIntact(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.NilError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)
	retired := &schema.AIYakTool{Name: "http_response_diff", IsBuiltin: true}
	assert.NilError(t, db.Create(retired).Error)
	originalFS := yakScriptFS
	t.Cleanup(func() { yakScriptFS = originalFS })
	readErr := errors.New("injected embedded read failure")
	yakScriptFS = &failingAIToolFS{FileSystemWithHash: originalFS, err: readErr}
	err = overrideYakScriptAiTools(db)
	assert.Assert(t, errors.Is(err, readErr), "embedded read failure must propagate: %v", err)
	_, err = yakit.GetAIYakTool(db, retired.Name)
	assert.NilError(t, err)

	yakScriptFS = &failingAIToolFS{FileSystemWithHash: originalFS, content: []byte("func {")}
	err = overrideYakScriptAiTools(db)
	assert.Assert(t, err != nil && strings.Contains(err.Error(), "parse builtin AI tool") && strings.Contains(err.Error(), ".yak"), "parse failure must name the broken file: %v", err)
	_, err = yakit.GetAIYakTool(db, retired.Name)
	assert.NilError(t, err)
}
