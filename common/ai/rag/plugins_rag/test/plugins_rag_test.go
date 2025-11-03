package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// MockEmbedder 是一个模拟的嵌入客户端，用于测试
type MockEmbedder struct{}

// Embedding 模拟实现 EmbeddingClient 接口
func (m *MockEmbedder) Embedding(text string) ([]float32, error) {
	// 根据文本内容生成不同的向量，用于模拟嵌入
	if utils.MatchAllOfSubString(text, "sql", "injection") {
		return []float32{1.0, 0.0, 0.0}, nil
	} else if utils.MatchAllOfSubString(text, "xss") {
		return []float32{0.0, 1.0, 0.0}, nil
	} else if utils.MatchAllOfSubString(text, "port", "scan") {
		return []float32{0.0, 0.0, 1.0}, nil
	} else if utils.MatchAllOfSubString(text, "web", "vuln") {
		return []float32{0.5, 0.5, 0.0}, nil
	} else if utils.MatchAllOfSubString(text, "网站", "漏洞") {
		return []float32{0.5, 0.5, 0.0}, nil
	} else if utils.MatchAllOfSubString(text, "安全") {
		return []float32{0.3, 0.3, 0.3}, nil
	}
	return []float32{0.1, 0.1, 0.1}, nil
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
	store := vectorstore.NewMemoryVectorStore(mockEmbedder)

	// 创建RAG系统
	ragSystem, err := rag.NewRAGSystem(rag.WithVectorStore(store), rag.WithEmbeddingClient(mockEmbedder))
	if err != nil {
		t.Fatalf("failed to create rag system: %v", err)
	}

	// 创建插件RAG管理器
	manager := plugins_rag.NewPluginsRagManager(consts.GetGormProfileDatabase(), ragSystem, collectionName, "")

	return manager
}

// 测试创建插件RAG管理器
func TestCreatePluginsRagManager(t *testing.T) {
	// 创建基于MockEmbedder的管理器
	manager := createMockRagManager(t, "test_collection")
	assert.NotNil(t, manager)
}

// 辅助函数：列出向量存储集合
func TestListVectorStore(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collections, err := yakit.GetAllRAGCollectionInfos(db)
	assert.NoError(t, err)
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
