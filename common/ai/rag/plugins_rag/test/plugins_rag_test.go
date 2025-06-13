package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// MockEmbedder 是一个模拟的嵌入客户端，用于测试
type MockEmbedder struct{}

// Embedding 模拟实现 EmbeddingClient 接口
func (m *MockEmbedder) Embedding(text string) ([]float64, error) {
	// 根据文本内容生成不同的向量，用于模拟嵌入
	if utils.MatchAllOfSubString(text, "sql", "injection") {
		return []float64{1.0, 0.0, 0.0}, nil
	} else if utils.MatchAllOfSubString(text, "xss") {
		return []float64{0.0, 1.0, 0.0}, nil
	} else if utils.MatchAllOfSubString(text, "port", "scan") {
		return []float64{0.0, 0.0, 1.0}, nil
	} else if utils.MatchAllOfSubString(text, "web", "vuln") {
		return []float64{0.5, 0.5, 0.0}, nil
	} else if utils.MatchAllOfSubString(text, "网站", "漏洞") {
		return []float64{0.5, 0.5, 0.0}, nil
	} else if utils.MatchAllOfSubString(text, "安全") {
		return []float64{0.3, 0.3, 0.3}, nil
	}
	return []float64{0.1, 0.1, 0.1}, nil
}

func init() {
	plugins_rag.GenerateYakScriptMetadata = func(script string) (*plugins_rag.GenerateResult, error) {
		res, err := metadata.GenerateYakScriptMetadata(script)
		if err != nil {
			return nil, err
		}
		return &plugins_rag.GenerateResult{
			Language:    res.Language,
			Description: res.Description,
			Keywords:    res.Keywords,
		}, nil
	}
}

// 创建测试用插件
func createTestPlugins(t *testing.T) {
	db := consts.GetGormProfileDatabase()

	// 清理之前可能存在的测试插件
	db.Where("script_name LIKE ?", "test_rag_plugin%").Delete(&schema.YakScript{})

	// 创建一个SQL注入测试插件
	sqlInjectionPlugin := &schema.YakScript{
		ScriptName: "test_rag_plugin_sql_injection",
		Type:       "poc",
		Content:    "# SQL注入测试\n\nhttp.Do({url: params.target, poc: true, matcher: 'SQL syntax'})",
		Level:      "medium",
		Help:       "这个插件用于检测SQL注入漏洞，通过发送特殊的SQL语句测试目标是否存在注入点。",
		Author:     "yaklang_test",
		Tags:       "sql,injection,vuln",
		Params:     `[{"Field": "target", "FieldVerbose": "目标URL", "Required": true, "DefaultValue": ""}]`,
	}

	// 创建一个XSS测试插件
	xssPlugin := &schema.YakScript{
		ScriptName: "test_rag_plugin_xss",
		Type:       "poc",
		Content:    "# XSS测试\n\nhttp.Do({url: params.target, poc: true, matcher: '<script>alert(1)</script>'})",
		Level:      "low",
		Help:       "这个插件用于检测跨站脚本(XSS)漏洞，测试网站是否过滤特殊字符。",
		Author:     "yaklang_test",
		Tags:       "xss,web,vuln",
		Params:     `[{"Field": "target", "FieldVerbose": "目标URL", "Required": true, "DefaultValue": ""}]`,
	}

	// 创建一个扫描器插件
	portScanPlugin := &schema.YakScript{
		ScriptName: "test_rag_plugin_port_scanner",
		Type:       "scan",
		Content:    "# 端口扫描\n\nscan.Port({target: params.target, ports: params.ports})",
		Level:      "info",
		Help:       "这个插件用于扫描目标主机的开放端口，支持指定端口范围。",
		Author:     "yaklang_test",
		Tags:       "scan,port,network",
		Params:     `[{"Field": "target", "FieldVerbose": "目标主机", "Required": true, "DefaultValue": ""}, {"Field": "ports", "FieldVerbose": "端口范围", "Required": false, "DefaultValue": "1-1000"}]`,
	}

	// 保存插件到数据库
	db.Create(sqlInjectionPlugin)
	db.Create(xssPlugin)
	db.Create(portScanPlugin)

	// 验证插件已创建
	var count int
	db.Model(&schema.YakScript{}).Where("script_name LIKE ?", "test_rag_plugin%").Count(&count)
	assert.Equal(t, 3, count, "应该创建3个测试插件")
}

// 清理测试用插件
func cleanupTestPlugins(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	db.Where("script_name LIKE ?", "test_rag_plugin%").Delete(&schema.YakScript{})
}

// 创建基于MockEmbedder的RAG管理器
func createMockRagManager(t *testing.T, collectionName string) *plugins_rag.PluginsRagManager {
	// 创建模拟嵌入器
	mockEmbedder := &MockEmbedder{}

	// 创建内存向量存储
	store := rag.NewMemoryVectorStore(mockEmbedder)

	// 创建RAG系统
	ragSystem := rag.NewRAGSystem(mockEmbedder, store)

	// 创建插件RAG管理器
	manager := plugins_rag.NewPluginsRagManager(consts.GetGormProfileDatabase(), ragSystem, collectionName)

	return manager
}

// 测试创建插件RAG管理器
func TestCreatePluginsRagManager(t *testing.T) {
	// 创建基于MockEmbedder的管理器
	manager := createMockRagManager(t, "test_collection")
	assert.NotNil(t, manager)
}

// 测试索引和搜索插件
func TestIndexAndSearchPlugins(t *testing.T) {
	// 创建测试插件
	createTestPlugins(t)
	defer cleanupTestPlugins(t)

	// 创建基于MockEmbedder的管理器
	manager := createMockRagManager(t, "test_mock_rag")

	// 索引所有插件
	err := manager.IndexAllPlugins()
	assert.NoError(t, err)

	// 确认插件已被索引
	count := manager.GetIndexedPluginsCount()
	assert.GreaterOrEqual(t, count, 3, "至少应该索引3个测试插件")

	// 测试各种搜索查询
	testQueries := []struct {
		Query       string
		ExpectedTag string
	}{
		{"如何检测SQL注入", "sql"},
		{"我需要扫描端口", "port"},
		{"有没有检测XSS的工具", "xss"},
		{"网站安全漏洞检测", "vuln"},
	}

	for _, tq := range testQueries {
		results, err := manager.SearchPlugins(tq.Query, 5)
		assert.NoError(t, err)

		if len(results) > 0 {
			t.Logf("查询 '%s' 返回 %d 个结果", tq.Query, len(results))
			t.Logf("首个结果: %s (相关度: %f)", results[0].Script.ScriptName, results[0].Score)

			// 验证返回的插件类型是否符合预期
			found := false
			for _, result := range results {
				if result.Script.Tags != "" && utils.MatchAllOfSubString(result.Script.Tags, tq.ExpectedTag) {
					found = true
					break
				}
			}
			assert.True(t, found, "搜索结果应该包含标签为 %s 的插件", tq.ExpectedTag)
		} else {
			t.Logf("查询 '%s' 没有返回结果", tq.Query)
		}
	}
}

// 测试单个插件的索引和搜索
func TestIndexSinglePlugin(t *testing.T) {
	// 创建测试插件
	createTestPlugins(t)
	defer cleanupTestPlugins(t)

	// 创建基于MockEmbedder的管理器
	manager := createMockRagManager(t, "test_single_plugin")

	// 索引单个插件
	err := manager.IndexPlugin("test_rag_plugin_sql_injection")
	assert.NoError(t, err)

	// 确认插件已被索引
	count := manager.GetIndexedPluginsCount()
	assert.Equal(t, 1, count, "应该只索引了1个插件")

	// 测试搜索
	results, err := manager.SearchPlugins("SQL注入检测", 5)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1, "搜索结果应该至少包含1个插件")

	if len(results) > 0 {
		assert.Equal(t, "test_rag_plugin_sql_injection", results[0].Script.ScriptName)
	}
}

// 测试移除插件
func TestRemovePlugin(t *testing.T) {
	// 创建测试插件
	createTestPlugins(t)
	defer cleanupTestPlugins(t)

	// 创建基于MockEmbedder的管理器
	manager := createMockRagManager(t, "test_remove_plugin")

	// 索引所有插件
	err := manager.IndexAllPlugins()
	assert.NoError(t, err)

	// 确认插件已被索引
	countBefore := manager.GetIndexedPluginsCount()
	assert.GreaterOrEqual(t, countBefore, 3, "至少应该索引3个测试插件")

	// 移除一个插件
	err = manager.RemovePlugin("test_rag_plugin_sql_injection")
	assert.NoError(t, err)

	// 确认插件已被移除
	countAfter := manager.GetIndexedPluginsCount()
	assert.Equal(t, countBefore-1, countAfter, "移除后索引的插件数量应该减少1")

	// 测试搜索
	results, err := manager.SearchPlugins("SQL注入检测", 5)
	assert.NoError(t, err)

	// 验证移除的插件不在搜索结果中
	for _, result := range results {
		assert.NotEqual(t, "test_rag_plugin_sql_injection", result.Script.ScriptName)
	}
}

// 测试清空所有索引
func TestClearAllPlugins(t *testing.T) {
	// 创建测试插件
	createTestPlugins(t)
	defer cleanupTestPlugins(t)

	// 创建基于MockEmbedder的管理器
	manager := createMockRagManager(t, "test_clear_plugins")

	// 索引所有插件
	err := manager.IndexAllPlugins()
	assert.NoError(t, err)

	// 确认插件已被索引
	countBefore := manager.GetIndexedPluginsCount()
	assert.GreaterOrEqual(t, countBefore, 3, "至少应该索引3个测试插件")

	// 清空所有索引
	err = manager.Clear()
	assert.NoError(t, err)

	// 确认所有索引已被清空
	countAfter := manager.GetIndexedPluginsCount()
	assert.Equal(t, 0, countAfter, "清空后索引的插件数量应该为0")
}

// 辅助函数：列出向量存储集合
func TestListVectorStore(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collections := []*schema.VectorStoreCollection{}
	db.Find(&collections)
	t.Logf("共找到 %d 个向量存储集合", len(collections))

	for i, collection := range collections {
		t.Logf("集合 #%d: %s", i+1, collection.Name)
	}
}

// 辅助函数：移除向量存储集合
func TestRemoveVectorStore(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	// 删除测试集合
	db.Where("name LIKE ?", "test_%").Delete(&schema.VectorStoreCollection{})
	t.Log("已删除所有测试向量存储集合")
}
