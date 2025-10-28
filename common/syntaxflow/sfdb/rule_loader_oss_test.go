package sfdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestOSSRuleLoader 测试OSS规则加载器
func TestOSSRuleLoader(t *testing.T) {
	// 创建Mock OSS客户端
	mockClient := NewMockOSSClient(OSSTypeMinIO)

	// 添加测试规则
	mockClient.AddRuleObject("test_sqli_java", `desc(
  title: "SQL注入检测",
  title_zh: "SQL注入检测",
  description: "检测Java代码中的SQL注入漏洞",
  language: java,
  purpose: audit,
  severity: high
)

// 查找HTTP参数
request.getParameter(*) as $param

// 查找SQL执行点
Statement.execute* as $exec

// 数据流分析
$param -> $exec as $vuln
alert $vuln`)

	mockClient.AddRuleObject("test_xss_java", `desc(
  title: "XSS检测",
  language: java,
  purpose: vuln,
  severity: medium
)

request.getParameter(*) as $param
response.getWriter().write* as $sink
$param -> $sink as $vuln
alert $vuln`)

	mockClient.AddRuleObject("test_rce_php", `desc(
  title: "RCE检测",
  language: php,
  purpose: security,
  severity: critical
)

$_GET as $input
system as $exec
$input -> $exec as $vuln
alert $vuln`)

	// 创建OSS加载器
	loader := NewOSSRuleLoader(mockClient,
		WithOSSBucket("test-bucket"),
		WithOSSPrefix("syntaxflow/"),
		WithOSSCache(true),
	)
	assert.NotNil(t, loader)
	assert.Equal(t, LoaderTypeOSS, loader.GetLoaderType())

	ctx := context.Background()

	// 测试LoadRules - 加载所有规则
	t.Run("LoadRules - All", func(t *testing.T) {
		rules, err := loader.LoadRules(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(rules))
		t.Logf("Loaded %d rules from OSS", len(rules))
	})

	// 测试LoadRules - 按语言筛选
	t.Run("LoadRules - Filter by Language", func(t *testing.T) {
		filter := &ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
		}
		rules, err := loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(rules))
		t.Logf("Loaded %d java rules", len(rules))

		// 验证规则语言
		for _, rule := range rules {
			assert.Equal(t, "java", rule.Language)
		}
	})

	// 测试LoadRules - 按用途筛选
	t.Run("LoadRules - Filter by Purpose", func(t *testing.T) {
		// 测试筛选 audit 类型的规则
		filter := &ypb.SyntaxFlowRuleFilter{
			Purpose: []string{"audit"},
		}
		rules, err := loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(rules))
		assert.Equal(t, "test_sqli_java", rules[0].RuleName)
		t.Logf("Found %d audit rules", len(rules))

		// 测试筛选 vuln 类型的规则
		filter = &ypb.SyntaxFlowRuleFilter{
			Purpose: []string{"vuln"},
		}
		rules, err = loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(rules))
		assert.Equal(t, "test_xss_java", rules[0].RuleName)
		t.Logf("Found %d vuln rules", len(rules))

		// 测试筛选 security 类型的规则
		filter = &ypb.SyntaxFlowRuleFilter{
			Purpose: []string{"security"},
		}
		rules, err = loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(rules))
		assert.Equal(t, "test_rce_php", rules[0].RuleName)
		t.Logf("Found %d security rules", len(rules))
	})

	// 测试LoadRules - 按严重程度筛选
	t.Run("LoadRules - Filter by Severity", func(t *testing.T) {
		filter := &ypb.SyntaxFlowRuleFilter{
			Severity: []string{"critical"},
		}
		rules, err := loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(rules))
		assert.Equal(t, "test_rce_php", rules[0].RuleName)
	})

	// 测试LoadRules - 关键词搜索
	t.Run("LoadRules - Filter by Keyword", func(t *testing.T) {
		filter := &ypb.SyntaxFlowRuleFilter{
			Keyword: "XSS",
		}
		rules, err := loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(rules))
		assert.Equal(t, "test_xss_java", rules[0].RuleName)
	})

	// 测试LoadRuleByName
	t.Run("LoadRuleByName", func(t *testing.T) {
		rule, err := loader.LoadRuleByName(ctx, "test_sqli_java")
		assert.NoError(t, err)
		assert.NotNil(t, rule)
		assert.Equal(t, "test_sqli_java", rule.RuleName)
		assert.Equal(t, "java", rule.Language)
		t.Logf("Loaded rule: %s", rule.RuleName)

		// 测试缓存
		rule2, err := loader.LoadRuleByName(ctx, "test_sqli_java")
		assert.NoError(t, err)
		assert.Equal(t, rule, rule2) // 应该是同一个对象（缓存）
	})

	// 测试YieldRules
	t.Run("YieldRules", func(t *testing.T) {
		count := 0
		for item := range loader.YieldRules(ctx, nil) {
			assert.NotNil(t, item)
			if item.Error != nil {
				t.Logf("Error: %v", item.Error)
			} else {
				count++
				t.Logf("Yielded rule: %s", item.Rule.RuleName)
			}
		}
		assert.Equal(t, 3, count)
	})

	// 测试Close
	err := loader.Close()
	assert.NoError(t, err)
}

// TestOSSRuleLoader_ContextCancellation 测试上下文取消
func TestOSSRuleLoader_ContextCancellation(t *testing.T) {
	mockClient := NewMockOSSClient(OSSTypeMinIO)
	mockClient.AddRuleObject("test_rule", "desc(title: 'test')\nprintln as $output")

	loader := NewOSSRuleLoader(mockClient)

	// 创建已取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// LoadRules应该返回上下文错误
	_, err := loader.LoadRules(ctx, nil)
	assert.Error(t, err)

	// LoadRuleByName应该返回上下文错误
	_, err = loader.LoadRuleByName(ctx, "test_rule")
	assert.Error(t, err)
}

// TestMockOSSClient 测试Mock OSS客户端
func TestMockOSSClient(t *testing.T) {
	client := NewMockOSSClient(OSSTypeMinIO)
	assert.NotNil(t, client)
	assert.Equal(t, OSSTypeMinIO, client.GetType())

	// 添加对象
	client.AddObject("test.txt", []byte("test content"))

	// 列出对象
	objects, err := client.ListObjects("bucket", "")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(objects))
	assert.Equal(t, "test.txt", objects[0].Key)

	// 获取对象
	content, err := client.GetObject("bucket", "test.txt")
	assert.NoError(t, err)
	assert.Equal(t, "test content", string(content))

	// 获取不存在的对象
	_, err = client.GetObject("bucket", "notfound.txt")
	assert.Error(t, err)

	// 关闭客户端
	err = client.Close()
	assert.NoError(t, err)
}
