package vectorstore

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isCI 检测是否在 CI 环境中运行
func isCI() bool {
	// 检测常见的 CI 环境变量
	ciEnvVars := []string{
		"CI",             // 通用 CI 标识
		"GITHUB_ACTIONS", // GitHub Actions
		"GITLAB_CI",      // GitLab CI
		"CIRCLECI",       // CircleCI
		"TRAVIS",         // Travis CI
		"JENKINS_HOME",   // Jenkins
		"BUILDKITE",      // Buildkite
	}

	for _, envVar := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}
	return false
}

// TestAIBalanceFreeEmbedding_Basic 测试基本的 embedding 功能
func TestAIBalanceFreeEmbedding_Basic(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 创建服务实例
	service, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err, "failed to create aibalance free embedder")
	require.NotNil(t, service, "service should not be nil")

	// 检查服务是否可用
	assert.True(t, service.IsAvailable(), "service should be available")

	// 测试生成 embedding
	testText := "这是一个测试文本，用于生成嵌入向量"
	embedding, err := service.Embedding(testText)
	require.NoError(t, err, "failed to generate embedding")
	require.NotNil(t, embedding, "embedding should not be nil")
	assert.Greater(t, len(embedding), 0, "embedding vector should not be empty")

	// 验证向量不全为零
	hasNonZero := false
	for _, val := range embedding {
		if val != 0 {
			hasNonZero = true
			break
		}
	}
	assert.True(t, hasNonZero, "embedding vector should contain non-zero values")

	t.Logf("embedding dimension: %d", len(embedding))
}

// TestAIBalanceFreeEmbedding_EmbeddingRaw 测试原始 embedding 功能
func TestAIBalanceFreeEmbedding_EmbeddingRaw(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 创建服务实例
	service, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err, "failed to create aibalance free embedder")
	require.NotNil(t, service, "service should not be nil")

	// 测试生成原始 embedding
	testText := "test embedding raw functionality"
	embeddings, err := service.EmbeddingRaw(testText)
	require.NoError(t, err, "failed to generate raw embedding")
	require.NotNil(t, embeddings, "embeddings should not be nil")
	assert.Greater(t, len(embeddings), 0, "should return at least one embedding vector")

	// 检查第一个向量
	if len(embeddings) > 0 {
		firstVector := embeddings[0]
		assert.Greater(t, len(firstVector), 0, "first vector should not be empty")

		// 验证向量不全为零
		hasNonZero := false
		for _, val := range firstVector {
			if val != 0 {
				hasNonZero = true
				break
			}
		}
		assert.True(t, hasNonZero, "embedding vector should contain non-zero values")
	}

	t.Logf("returned %d embedding vectors", len(embeddings))
}

// TestAIBalanceFreeEmbedding_Singleton 测试单例模式
func TestAIBalanceFreeEmbedding_Singleton(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 多次获取服务实例，应该返回相同的实例
	service1, err1 := NewAIBalanceFreeEmbedder()
	require.NoError(t, err1)
	require.NotNil(t, service1)

	service2, err2 := NewAIBalanceFreeEmbedder()
	require.NoError(t, err2)
	require.NotNil(t, service2)

	// 验证是同一个实例
	assert.Equal(t, service1, service2, "should return the same singleton instance")

	// 使用 GetAIBalanceFreeEmbeddingService 也应该返回相同实例
	service3, err3 := GetAIBalanceFreeEmbeddingService()
	require.NoError(t, err3)
	require.NotNil(t, service3)

	assert.Equal(t, service1, service3, "GetAIBalanceFreeEmbeddingService should return the same instance")
}

// TestAIBalanceFreeEmbedding_ServiceInfo 测试服务信息获取
func TestAIBalanceFreeEmbedding_ServiceInfo(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 创建服务实例
	service, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.NotNil(t, service)

	// 获取服务信息
	domain, model, available := service.GetServiceInfo()

	assert.Equal(t, aibalanceDomain, domain, "domain should match")
	assert.Equal(t, aibalanceFreeModel, model, "model should match")
	assert.True(t, available, "service should be available")

	t.Logf("service info: domain=%s, model=%s, available=%v", domain, model, available)
}

// TestAIBalanceFreeEmbedding_IsAvailableFunction 测试服务可用性检查函数
func TestAIBalanceFreeEmbedding_IsAvailableFunction(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 检查服务是否可用
	available := IsAIBalanceFreeServiceAvailable()
	assert.True(t, available, "service should be available")
}

// TestAIBalanceFreeEmbedding_GlobalFunction 测试全局 embedding 函数
func TestAIBalanceFreeEmbedding_GlobalFunction(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 使用全局函数生成 embedding
	testText := "global function test"
	embedding, err := AIBalanceFreeEmbeddingFunc(testText)
	require.NoError(t, err, "failed to generate embedding using global function")
	require.NotNil(t, embedding, "embedding should not be nil")
	assert.Greater(t, len(embedding), 0, "embedding vector should not be empty")

	t.Logf("global function embedding dimension: %d", len(embedding))
}

// TestAIBalanceFreeEmbedding_MultipleTexts 测试多个文本的 embedding
func TestAIBalanceFreeEmbedding_MultipleTexts(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 创建服务实例
	service, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.NotNil(t, service)

	// 测试多个不同的文本
	testTexts := []string{
		"这是第一段测试文本",
		"This is the second test text",
		"これは三番目のテストテキストです",
		"短文本",
		"这是一段比较长的测试文本，用于验证 AIBalance 免费 Embedding 服务是否能够正确处理不同长度的文本内容。",
	}

	embeddings := make([][]float32, len(testTexts))
	for i, text := range testTexts {
		embedding, err := service.Embedding(text)
		require.NoError(t, err, "failed to generate embedding for text %d", i)
		require.NotNil(t, embedding, "embedding should not be nil for text %d", i)
		assert.Greater(t, len(embedding), 0, "embedding should not be empty for text %d", i)
		embeddings[i] = embedding
		t.Logf("text %d embedding dimension: %d", i, len(embedding))
	}

	// 验证所有 embedding 的维度相同
	firstDim := len(embeddings[0])
	for i, emb := range embeddings[1:] {
		assert.Equal(t, firstDim, len(emb), "all embeddings should have the same dimension (text %d)", i+1)
	}
}

// TestAIBalanceFreeEmbedding_EmptyText 测试空文本处理
func TestAIBalanceFreeEmbedding_EmptyText(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 创建服务实例
	service, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.NotNil(t, service)

	// 测试空文本（根据服务器实现，可能返回错误或者处理为默认向量）
	_, err = service.Embedding("")
	// 空文本可能会被服务器拒绝，这是正常的
	if err != nil {
		t.Logf("empty text handling: %v (this is expected)", err)
	}
}

// TestAIBalanceFreeEmbedding_InterfaceImplementation 测试接口实现
func TestAIBalanceFreeEmbedding_InterfaceImplementation(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 重置单例以确保干净的测试环境
	ResetAIBalanceFreeService()

	// 创建服务实例
	service, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.NotNil(t, service)

	// 验证实现了 EmbeddingClient 接口
	var _ EmbeddingClient = service

	// 测试接口方法
	testText := "interface implementation test"

	// 测试 Embedding 方法
	embedding, err := service.Embedding(testText)
	require.NoError(t, err)
	require.NotNil(t, embedding)
	assert.Greater(t, len(embedding), 0)

	// 测试 EmbeddingRaw 方法
	embeddingsRaw, err := service.EmbeddingRaw(testText)
	require.NoError(t, err)
	require.NotNil(t, embeddingsRaw)
	assert.Greater(t, len(embeddingsRaw), 0)
}

// TestAIBalanceFreeEmbedding_Reset 测试重置功能
func TestAIBalanceFreeEmbedding_Reset(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	// 创建第一个实例
	ResetAIBalanceFreeService()
	service1, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.NotNil(t, service1)

	// 重置服务
	ResetAIBalanceFreeService()

	// 创建新实例（应该是一个新的实例）
	service2, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.NotNil(t, service2)

	// 两个实例应该都可以正常工作
	testText := "reset test"

	_, err = service2.Embedding(testText)
	require.NoError(t, err, "new instance should work after reset")
}

// TestNormalizeEmbeddingModelName 测试模型名称归一化
func TestNormalizeEmbeddingModelName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		{"embedding-free", "Qwen3-Embedding-0.6B", "AIBalance 免费模型"},
		{"Qwen3-Embedding-0.6B-Q4_K_M", "Qwen3-Embedding-0.6B", "本地量化模型"},
		{"Qwen3-Embedding-0.6B", "Qwen3-Embedding-0.6B", "基础模型名"},
		{"unknown-model", "unknown-model", "未知模型保持不变"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := NormalizeEmbeddingModelName(tc.input)
			assert.Equal(t, tc.expected, result, "模型名称归一化失败: %s", tc.desc)
		})
	}
}

// TestIsCompatibleEmbeddingModel 测试模型兼容性检查
func TestIsCompatibleEmbeddingModel(t *testing.T) {
	testCases := []struct {
		model1     string
		model2     string
		compatible bool
		desc       string
	}{
		{"embedding-free", "Qwen3-Embedding-0.6B-Q4_K_M", true, "免费模型与本地模型兼容"},
		{"embedding-free", "Qwen3-Embedding-0.6B", true, "免费模型与基础模型兼容"},
		{"Qwen3-Embedding-0.6B-Q4_K_M", "Qwen3-Embedding-0.6B", true, "量化模型与基础模型兼容"},
		{"embedding-free", "unknown-model", false, "免费模型与未知模型不兼容"},
		{"Qwen3-Embedding-0.6B", "text-embedding-3-small", false, "不同模型系列不兼容"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := IsCompatibleEmbeddingModel(tc.model1, tc.model2)
			assert.Equal(t, tc.compatible, result, "模型兼容性检查失败: %s", tc.desc)
		})
	}
}

// TestAIBalanceFreeEmbedding_ModelInfo 测试模型信息获取
func TestAIBalanceFreeEmbedding_ModelInfo(t *testing.T) {
	if isCI() {
		t.Skip("skip aibalance free embedding test in CI environment")
	}

	ResetAIBalanceFreeService()
	service, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.NotNil(t, service)

	// 测试归一化的模型名称
	modelName := service.GetModelName()
	assert.Equal(t, "Qwen3-Embedding-0.6B", modelName, "should return normalized model name")

	// 测试模型维度
	dimension := service.GetModelDimension()
	assert.Equal(t, 1024, dimension, "should return correct dimension")

	t.Logf("normalized model name: %s, dimension: %d", modelName, dimension)
}
