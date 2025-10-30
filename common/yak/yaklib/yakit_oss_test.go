package yaklib

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMockOSSClient 测试 Mock OSS 客户端
func TestMockOSSClient(t *testing.T) {
	client := NewMockOSSClient(OSSTypeAliyun)

	// 添加一些测试对象
	client.AddObject("test1.txt", []byte("content1"))
	client.AddObject("test2.txt", []byte("content2"))
	client.AddObject("syntaxflow/rule1.sf", []byte("rule content 1"))
	client.AddObject("syntaxflow/rule2.sf", []byte("rule content 2"))

	// 测试 GetType
	assert.Equal(t, OSSTypeAliyun, client.GetType())

	// 测试 ListObjects - 列出所有对象
	objects, err := client.ListObjects("test-bucket", "")
	require.NoError(t, err)
	assert.Len(t, objects, 4)

	// 测试 ListObjects - 带前缀过滤
	objects, err = client.ListObjects("test-bucket", "syntaxflow/")
	require.NoError(t, err)
	assert.Len(t, objects, 2)

	// 测试 GetObject
	content, err := client.GetObject("test-bucket", "test1.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("content1"), content)

	// 测试 GetObjectStream
	stream, err := client.GetObjectStream("test-bucket", "test2.txt")
	require.NoError(t, err)
	require.NotNil(t, stream)
	_ = stream.Close()

	// 测试 GetObject - 不存在的对象
	_, err = client.GetObject("test-bucket", "not-exist.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// 测试 Close
	err = client.Close()
	assert.NoError(t, err)
}

// TestOSSType 测试 OSS 类型
func TestOSSType(t *testing.T) {
	testCases := []struct {
		ossType     OSSType
		shouldValid bool
	}{
		{OSSTypeAliyun, true},
		{OSSTypeS3, true},
		{OSSTypeMinIO, true},
		{OSSTypeCustom, true},
		{OSSType("invalid"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.ossType.String(), func(t *testing.T) {
			isValid := tc.ossType.IsValid()
			assert.Equal(t, tc.shouldValid, isValid)
			assert.Equal(t, string(tc.ossType), tc.ossType.String())
		})
	}
}

// TestMockOSSClientAddRuleObject 测试添加规则对象
func TestMockOSSClientAddRuleObject(t *testing.T) {
	client := NewMockOSSClient(OSSTypeAliyun)

	// 添加规则对象
	client.AddRuleObject("sql-injection", "rule content here")
	client.AddRuleObject("xss-check", "another rule content")

	// 验证对象已添加
	objects, err := client.ListObjects("test-bucket", "")
	require.NoError(t, err)
	assert.Len(t, objects, 2)

	// 验证内容
	content, err := client.GetObject("test-bucket", "syntaxflow/sql-injection.sf")
	require.NoError(t, err)
	assert.Equal(t, "rule content here", string(content))
}

// TestDownloadOSSSyntaxFlowRuleFiles 测试从 OSS 下载 SyntaxFlow 规则文件
func TestDownloadOSSSyntaxFlowRuleFiles(t *testing.T) {
	client := NewMockOSSClient(OSSTypeAliyun)

	// 添加多个规则文件（使用 AddRuleObject 会自动添加 .sf 后缀）
	client.AddRuleObject("sql-injection", "rule sql-injection content")
	client.AddRuleObject("xss-check", "rule xss-check content")
	client.AddRuleObject("path-traversal", "rule path-traversal content")

	// 添加一个非 .sf 文件
	client.AddObject("syntaxflow/noop-file.txt", []byte("not a rule file"))

	// 下载规则文件
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream := DownloadOSSSyntaxFlowRuleFiles(ctx, client, "test-bucket", "syntaxflow/")

	// 收集结果
	results := make([]*OSSRuleFileItem, 0)
	for item := range stream.Chan {
		results = append(results, item)
	}

	// 验证结果
	// 应该只返回 .sf 文件，排除其他类型文件
	assert.Equal(t, int64(3), stream.Total)
	assert.Len(t, results, 3)

	// 验证每个规则文件
	for _, item := range results {
		require.NoError(t, item.Error)
		assert.NotEmpty(t, item.RuleName)
		assert.NotEmpty(t, item.Content)
		assert.NotEmpty(t, item.Key)
		assert.Contains(t, item.Key, ".sf")
	}

	// 验证特定规则
	foundSQLInject := false
	foundXSS := false
	foundPathTrav := false
	for _, item := range results {
		if item.RuleName == "sql-injection" {
			foundSQLInject = true
			assert.Contains(t, item.Content, "sql-injection")
		}
		if item.RuleName == "xss-check" {
			foundXSS = true
			assert.Contains(t, item.Content, "xss-check")
		}
		if item.RuleName == "path-traversal" {
			foundPathTrav = true
			assert.Contains(t, item.Content, "path-traversal")
		}
	}
	assert.True(t, foundSQLInject, "should find sql-injection rule")
	assert.True(t, foundXSS, "should find xss-check rule")
	assert.True(t, foundPathTrav, "should find path-traversal rule")
}

// TestDownloadOSSRules 测试下载 OSS 规则
func TestDownloadOSSRules(t *testing.T) {
	client := NewMockOSSClient(OSSTypeAliyun)

	// 添加规则文件
	client.AddRuleObject("rule1", "content1")
	client.AddRuleObject("rule2", "content2")
	client.AddRuleObject("rule3", "content3")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 下载规则
	err := DownloadOSSRules(ctx, client, "test-bucket", "syntaxflow/")
	require.NoError(t, err)
}

// TestDownloadOSSRulesWithEmptyResult 测试空结果
func TestDownloadOSSRulesWithEmptyResult(t *testing.T) {
	client := NewMockOSSClient(OSSTypeAliyun)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 没有规则文件，应该返回错误
	err := DownloadOSSRules(ctx, client, "test-bucket", "syntaxflow/")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download any rules")
}

// TestExtractRuleNameFromKey 测试从 key 中提取规则名称
func TestExtractRuleNameFromKey(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		prefix   string
		expected string
	}{
		{
			name:     "simple rule",
			key:      "syntaxflow/java/sql-injection.sf",
			prefix:   "syntaxflow/",
			expected: "sql-injection",
		},
		{
			name:     "with subdirectory",
			key:      "syntaxflow/php/xss-check.sf",
			prefix:   "syntaxflow/",
			expected: "xss-check",
		},
		{
			name:     "directly under prefix",
			key:      "syntaxflow/rule.sf",
			prefix:   "syntaxflow/",
			expected: "rule",
		},
		{
			name:     "complex path",
			key:      "rules/syntaxflow/java/security/sql-injection.sf",
			prefix:   "rules/syntaxflow/",
			expected: "sql-injection",
		},
		{
			name:     "no subdirectory",
			key:      "syntaxflow/test-rule.sf",
			prefix:   "syntaxflow/",
			expected: "test-rule",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractRuleNameFromKey(tc.key, tc.prefix)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestOSSObject 测试 OSS 对象结构
func TestOSSObject(t *testing.T) {
	obj := OSSObject{
		Key:          "test-key",
		Size:         1024,
		LastModified: 1234567890,
		ETag:         "etag-value",
	}

	assert.Equal(t, "test-key", obj.Key)
	assert.Equal(t, int64(1024), obj.Size)
	assert.Equal(t, int64(1234567890), obj.LastModified)
	assert.Equal(t, "etag-value", obj.ETag)
}

// TestOSSConfig 测试 OSS 配置
func TestOSSConfig(t *testing.T) {
	config := &OSSConfig{
		Endpoint:        "oss-cn-hangzhou.aliyuncs.com",
		AccessKeyID:     "test-access-key",
		AccessKeySecret: "test-secret",
		Bucket:          "test-bucket",
		Prefix:          "syntaxflow/",
		Region:          "cn-hangzhou",
		OSSType:         OSSTypeAliyun,
		EnableSSL:       true,
	}

	assert.Equal(t, "oss-cn-hangzhou.aliyuncs.com", config.Endpoint)
	assert.Equal(t, "test-access-key", config.AccessKeyID)
	assert.Equal(t, "syntaxflow/", config.Prefix)
	assert.True(t, config.EnableSSL)
}

// TestDownloadOSSSyntaxFlowRuleFilesWithContext 测试上下文取消
func TestDownloadOSSSyntaxFlowRuleFilesWithContext(t *testing.T) {
	client := NewMockOSSClient(OSSTypeAliyun)

	// 添加大量规则文件
	for i := 0; i < 100; i++ {
		client.AddRuleObject("rule"+string(rune(i)), "content")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := DownloadOSSSyntaxFlowRuleFiles(ctx, client, "test-bucket", "syntaxflow/")

	// 读取前几个就取消
	count := 0
	for item := range stream.Chan {
		count++
		if count >= 5 {
			cancel()
			break
		}
		if item.Error != nil {
			break
		}
	}

	// 验证只读取了部分数据
	assert.LessOrEqual(t, count, 10, "should stop early due to context cancellation")
}

// BenchmarkListObjects 性能测试：列出对象
func BenchmarkListObjects(b *testing.B) {
	client := NewMockOSSClient(OSSTypeAliyun)

	// 添加大量对象
	for i := 0; i < 1000; i++ {
		client.AddObject("object"+string(rune(i))+".txt", []byte("content"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.ListObjects("test-bucket", "")
	}
}

// BenchmarkGetObject 性能测试：获取对象
func BenchmarkGetObject(b *testing.B) {
	client := NewMockOSSClient(OSSTypeAliyun)
	client.AddObject("test.txt", []byte("content"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetObject("test-bucket", "test.txt")
	}
}
