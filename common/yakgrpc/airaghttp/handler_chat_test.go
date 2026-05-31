package airaghttp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// TestCleanupSessionData 验证会话结束后 session 及关联数据被彻底删除 (防数据爆炸)
// 关键词: cleanup session, DeleteAISession, avoid data explosion
func TestCleanupSessionData(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&schema.AISession{}, &schema.AIAgentRuntime{}, &schema.AiCheckpoint{},
		&schema.AiOutputEvent{}, &schema.AiProcessAndAiEvent{},
	).Error)

	sessionID := "airaghttp-" + uuid.NewString()
	runtimeID := uuid.NewString()

	require.NoError(t, db.Create(&schema.AISession{SessionID: sessionID}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: runtimeID, PersistentSession: sessionID, Name: "run"}).Error)
	require.NoError(t, db.Create(&schema.AiCheckpoint{CoordinatorUuid: runtimeID, Seq: 1, Type: schema.AiCheckpointType_ToolCall}).Error)
	require.NoError(t, db.Create(&schema.AiOutputEvent{EventUUID: uuid.NewString(), SessionId: sessionID}).Error)
	require.NoError(t, db.Create(&schema.AiOutputEvent{EventUUID: uuid.NewString(), SessionId: sessionID}).Error)

	// 执行清理
	cleanupSessionData(db, sessionID)

	var sessionCount int64
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionID).Count(&sessionCount).Error)
	require.Equal(t, int64(0), sessionCount, "session meta should be deleted")

	var runtimeCount int64
	require.NoError(t, db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", sessionID).Count(&runtimeCount).Error)
	require.Equal(t, int64(0), runtimeCount, "runtime should be deleted")

	var eventCount int64
	require.NoError(t, db.Model(&schema.AiOutputEvent{}).Where("session_id = ?", sessionID).Count(&eventCount).Error)
	require.Equal(t, int64(0), eventCount, "events should be deleted")
}

// TestCleanupSessionData_Guards nil db / 空 session 不应 panic
func TestCleanupSessionData_Guards(t *testing.T) {
	cleanupSessionData(nil, "x")
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	cleanupSessionData(db, "")
}

// TestSanitizeTraceMessage 验证本地路径脱敏与内部 prompt 标记屏蔽
// 关键词: sanitize local path, internal marker leak guard
func TestSanitizeTraceMessage(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "搜索完成", "搜索完成"},
		{"unix-path", "saved to /Users/v1ll4n/secret/a.md done", "saved to [local-path] done"},
		{"win-path", `wrote C:\Users\v1\a.txt`, "wrote [local-path]"},
		{"yakit-projects", "path yakit-projects/aispace/x", "[local-path]"},
		{"internal-cache", "<|AI_CACHE_SYSTEM_high-static|> ## rules", ""},
		{"internal-traits", "<|TRAITS|> something", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, sanitizeTraceMessage(c.in))
		})
	}
}

// TestCleanProgressMessage 验证 loading status 双语技术串被裁剪为简洁短语
func TestCleanProgressMessage(t *testing.T) {
	require.Equal(t, "执行搜索中", cleanProgressMessage("执行搜索中 - search_knowledge:semantic / executing search - mode:semantic"))
	require.Equal(t, "初始化", cleanProgressMessage("初始化 / initializing..."))
	require.Equal(t, "压缩搜索结果中", cleanProgressMessage("压缩搜索结果中 - compressing search result"))
}

// TestReferenceMaterialFiltered 验证 reference_material 事件被标记为噪声 (防系统 prompt 泄漏)
func TestReferenceMaterialFiltered(t *testing.T) {
	require.True(t, chatNoiseTypes["reference_material"], "reference_material type must be dropped")
	require.True(t, chatNoiseNodeIds["reference_material"], "reference_material nodeId must be dropped")
}

// TestClassifyEventProgress 验证 loading status 被归类为 progress 步骤而非丢弃
func TestClassifyEventProgress(t *testing.T) {
	kind, _ := classifyEvent("structured", "status")
	require.Equal(t, "progress", kind)
	kind, _ = classifyEvent("structured", "re-act-loading-status-key")
	require.Equal(t, "progress", kind)
}

// TestGenerateConfigTemplate 生成的模板应是合法 yaml 且可被 LoadConfigFromFile 解析
// 关键词: gen-config template, parseable, set apikey hint
func TestGenerateConfigTemplate(t *testing.T) {
	tpl := GenerateConfigTemplate()
	require.Contains(t, tpl, "api_key")
	require.Contains(t, tpl, "route_prefix")
	require.Contains(t, tpl, "ai_lightweight", "template should expose lightweight ai block")
	require.NotContains(t, tpl, "ai_tier", "ai_tier should be removed")

	dir := t.TempDir()
	path := filepath.Join(dir, "rag-server.yaml")

	// 首次写入成功
	require.NoError(t, SaveConfigTemplateToFile(path, false))
	// 不允许覆盖已存在文件
	require.Error(t, SaveConfigTemplateToFile(path, false))
	// 强制覆盖成功
	require.NoError(t, SaveConfigTemplateToFile(path, true))

	// 生成的模板应可被正常加载, 且默认值正确
	cfg, err := LoadConfigFromFile(path)
	require.NoError(t, err)
	require.Equal(t, 9093, cfg.Port)
	require.Equal(t, "/api/rag-server", cfg.RoutePrefix)
	require.True(t, cfg.ServeFrontend)
	// 默认模板未填 api_key -> 未配置 -> 回退轻量模型
	require.False(t, cfg.IsAIConfigured())

	// 空路径报错
	require.Error(t, SaveConfigTemplateToFile("", false))
	_ = os.Remove(path)
}

// TestIsAIConfigured 验证以 api_key 为准的配置检测
func TestIsAIConfigured(t *testing.T) {
	cfg := NewDefaultConfig()
	require.False(t, cfg.IsAIConfigured(), "no api_key -> not configured")

	cfg.AI.Model = "memfit-qwen3.7-max"
	require.False(t, cfg.IsAIConfigured(), "model alone (no api_key) -> still not configured")

	cfg.AI.APIKey = "mf-xxxx"
	require.True(t, cfg.IsAIConfigured(), "api_key set -> configured")
}

// TestIsLightweightAIConfigured 验证轻量(速度优先)通道的独立配置检测
func TestIsLightweightAIConfigured(t *testing.T) {
	cfg := NewDefaultConfig()
	require.False(t, cfg.IsLightweightAIConfigured(), "default -> not configured (use built-in light)")

	cfg.AILightweight.Model = "memfit-light-free"
	require.False(t, cfg.IsLightweightAIConfigured(), "model alone -> still not configured")

	cfg.AILightweight.APIKey = "mf-lite"
	require.True(t, cfg.IsLightweightAIConfigured(), "api_key set -> configured")

	// 高质与轻量通道相互独立
	require.False(t, cfg.IsAIConfigured(), "lightweight key should not flip high-quality channel")
}
