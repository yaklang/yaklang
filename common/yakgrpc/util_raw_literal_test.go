package yakgrpc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestYamlMapBuilder_RawLiteralBlockScalar 测试 raw 字段的 literal block scalar 格式
func TestYamlMapBuilder_RawLiteralBlockScalar(t *testing.T) {
	tests := []struct {
		name     string
		requests []string
		checks   []func(t *testing.T, yaml string)
	}{
		{
			name: "单个 HTTP 请求 - 应使用 | 格式",
			requests: []string{
				"GET /test HTTP/1.1\nHost: example.com\n\n",
			},
			checks: []func(t *testing.T, yaml string){
				func(t *testing.T, yaml string) {
					assert.Contains(t, yaml, "- |", "应该使用 literal block scalar (|) 格式")
					assert.NotContains(t, yaml, "\\n", "不应该包含转义的换行符")
					assert.Contains(t, yaml, "GET /test HTTP/1.1", "应该包含原始请求内容")
				},
			},
		},
		{
			name: "多个 HTTP 请求 - 应在请求之间有空行",
			requests: []string{
				"POST /login HTTP/1.1\nHost: {{Hostname}}\nContent-Type: application/x-www-form-urlencoded\n\nuser=admin&pass=admin",
				"GET /admin HTTP/1.1\nHost: {{Hostname}}\nCookie: session={{session}}",
				"GET /data HTTP/1.1\nHost: {{Hostname}}\nCookie: session={{session}}",
			},
			checks: []func(t *testing.T, yaml string){
				func(t *testing.T, yaml string) {
					// 检查使用 | 格式
					assert.Contains(t, yaml, "- |", "应该使用 literal block scalar 格式")

					// 检查不包含转义换行
					assert.NotContains(t, yaml, "\\n", "不应该包含转义的换行符")

					// 检查所有请求都存在
					assert.Contains(t, yaml, "POST /login", "应该包含第一个请求")
					assert.Contains(t, yaml, "GET /admin", "应该包含第二个请求")
					assert.Contains(t, yaml, "GET /data", "应该包含第三个请求")

					// 检查请求之间有空行分隔
					lines := strings.Split(yaml, "\n")
					var rawSection []string
					inRaw := false
					for _, line := range lines {
						if strings.Contains(line, "raw:") {
							inRaw = true
							continue
						}
						if inRaw {
							// 遇到下一个字段或缩进变小则退出
							if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.TrimSpace(line) != "" && !strings.Contains(line, "- |") {
								break
							}
							rawSection = append(rawSection, line)
						}
					}

					// 计算空行数量（应该是请求数-1）
					emptyLineCount := 0
					for _, line := range rawSection {
						if strings.TrimSpace(line) == "" {
							emptyLineCount++
						}
					}
					// 三个请求之间应该有两个空行（在请求之间）
					assert.GreaterOrEqual(t, emptyLineCount, 2, "请求之间应该有空行分隔")
				},
			},
		},
		{
			name: "包含特殊字符的请求 - 引号和大括号",
			requests: []string{
				`POST /api HTTP/1.1
Host: {{Hostname}}
Content-Type: application/json
User-Agent: "Test/1.0"

{"key": "value", "data": "{{variable}}"}`,
			},
			checks: []func(t *testing.T, yaml string){
				func(t *testing.T, yaml string) {
					assert.Contains(t, yaml, "- |", "应该使用 literal block scalar 格式")
					assert.Contains(t, yaml, `"Test/1.0"`, "应该保留引号")
					assert.Contains(t, yaml, `{{Hostname}}`, "应该保留模板变量")
					assert.Contains(t, yaml, `{"key": "value"`, "应该保留 JSON 内容")
				},
			},
		},
		{
			name: "包含空行的 HTTP body",
			requests: []string{
				"POST /test HTTP/1.1\nHost: example.com\nContent-Type: text/plain\n\nline1\n\nline2\nline3",
			},
			checks: []func(t *testing.T, yaml string){
				func(t *testing.T, yaml string) {
					assert.Contains(t, yaml, "- |", "应该使用 literal block scalar 格式")
					assert.Contains(t, yaml, "line1", "应该包含 body 第一行")
					assert.Contains(t, yaml, "line2", "应该包含 body 第二行")
					assert.Contains(t, yaml, "line3", "应该包含 body 第三行")
				},
			},
		},
		{
			name: "nuclei 标准格式 - 带 timeout",
			requests: []string{
				"@timeout: 30s\nGET /api/test HTTP/1.1\nHost: {{Hostname}}\n\n",
				"@timeout: 30s\nPOST /api/login HTTP/1.1\nHost: {{Hostname}}\nContent-Type: application/json\n\n{\"user\":\"admin\"}",
			},
			checks: []func(t *testing.T, yaml string){
				func(t *testing.T, yaml string) {
					assert.Contains(t, yaml, "@timeout: 30s", "应该保留 timeout 指令")
					assert.Contains(t, yaml, "- |", "应该使用 literal block scalar 格式")

					// 检查格式符合 Nuclei 标准
					lines := strings.Split(yaml, "\n")
					foundFirstPipe := false
					foundSecondPipe := false
					foundEmptyLineBetween := false

					for i, line := range lines {
						if strings.Contains(line, "- |") {
							if !foundFirstPipe {
								foundFirstPipe = true
							} else {
								foundSecondPipe = true
								// 检查两个 | 之间是否有空行
								for j := i - 1; j >= 0; j-- {
									if strings.Contains(lines[j], "- |") {
										break
									}
									if strings.TrimSpace(lines[j]) == "" {
										foundEmptyLineBetween = true
										break
									}
								}
							}
						}
					}

					assert.True(t, foundFirstPipe, "应该找到第一个 literal block scalar 标记")
					assert.True(t, foundSecondPipe, "应该找到第二个 literal block scalar 标记")
					assert.True(t, foundEmptyLineBetween, "两个请求之间应该有空行")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewYamlMapBuilder()
			builder.Set("raw", tt.requests)

			yaml, err := builder.MarshalToString()
			require.NoError(t, err, "MarshalToString 不应该返回错误")

			t.Logf("Generated YAML:\n%s", yaml)

			// 运行所有检查
			for _, check := range tt.checks {
				check(t, yaml)
			}
		})
	}
}

// TestYamlMapBuilder_RawFieldOnly 测试只有 raw 字段被处理为 literal block scalar
func TestYamlMapBuilder_RawFieldOnly(t *testing.T) {
	builder := NewYamlMapBuilder()

	// 设置 raw 字段（应该使用 literal block scalar）
	builder.Set("raw", []string{"GET /test HTTP/1.1\nHost: example.com"})

	// 设置其他字符串数组字段（不包含换行，使用正常数组格式）
	builder.Set("other_array", []string{"value1", "value2"})

	yaml, err := builder.MarshalToString()
	require.NoError(t, err)

	t.Logf("Generated YAML:\n%s", yaml)

	// raw 字段应该使用 literal block scalar
	assert.Contains(t, yaml, "raw:\n  - |", "raw 字段应该使用 literal block scalar")

	// other_array 字段应该使用正常的数组格式
	assert.Contains(t, yaml, "other_array:", "应该包含 other_array 字段")
	assert.Contains(t, yaml, "- value1", "other_array 应该使用正常数组格式")
	assert.Contains(t, yaml, "- value2", "other_array 应该使用正常数组格式")
}

// TestYamlMapBuilder_EmptyRequests 测试空请求数组
func TestYamlMapBuilder_EmptyRequests(t *testing.T) {
	builder := NewYamlMapBuilder()
	builder.Set("raw", []string{})

	yaml, err := builder.MarshalToString()
	require.NoError(t, err)

	t.Logf("Generated YAML:\n%s", yaml)

	if strings.Contains(yaml, "raw:") {
		t.Fatal("empty should be filtered")
	}
}

// TestYamlMapBuilder_SingleLineRequest 测试单行请求（不包含换行）
func TestYamlMapBuilder_SingleLineRequest(t *testing.T) {
	builder := NewYamlMapBuilder()
	// 注意：即使没有换行，由于是 raw 字段，也应该使用 literal block scalar
	builder.Set("raw", []string{"GET /test HTTP/1.1"})

	yaml, err := builder.MarshalToString()
	require.NoError(t, err)

	t.Logf("Generated YAML:\n%s", yaml)

	// 应该使用 literal block scalar 格式
	assert.Contains(t, yaml, "- |", "即使是单行，raw 字段也应该使用 literal block scalar")
}
