package yakgrpc

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 场景：用户在编辑器里动态输入 yak 代码，代码往往处于"半成品/不完整"状态，
// 此时即时补全的即时性与鲁棒性非常关键。这里覆盖一批"用户正在输入、尚未写完"
// 的不完整代码，验证 `xxx.` 成员补全都能给出合理结果，且不会因不完整语法把
// SSA 编译打崩(历史上 `for item := range poc.` 会触发空指针 panic)。
func TestGRPCMUSTPASS_LANGUAGE_SuggestionCompletion_Incomplete(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	getCompletionLabels := func(t *testing.T, code string, rng *ypb.Range) ([]string, error) {
		resp, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
			InspectType:   COMPLETION,
			YakScriptType: "yak",
			YakScriptCode: code,
			Range:         rng,
			ModelID:       uuid.NewString(),
		})
		if err != nil {
			return nil, err
		}
		return lo.Map(resp.SuggestionMessage, func(item *ypb.SuggestionDescription, _ int) string {
			return item.Label
		}), nil
	}

	// 断言：不完整代码补全 poc 成员时应能拿到 poc 标准库成员(如 HTTP / Get)。
	pocMemberCases := []struct {
		name string
		code string
		rng  *ypb.Range
	}{
		{
			// 用户示例：if 初始化多重赋值里 poc. 等待成员补全
			name: "if-multi-assign",
			code: `if req, rsp, err := poc.`,
			rng:  &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 21, EndLine: 1, EndColumn: 25},
		},
		{
			name: "plain-multi-assign",
			code: `req, rsp, err = poc.`,
			rng:  &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 17, EndLine: 1, EndColumn: 21},
		},
		{
			name: "walrus-single",
			code: `rsp := poc.`,
			rng:  &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 8, EndLine: 1, EndColumn: 12},
		},
		{
			// 历史 panic 用例：for-range 右值不完整
			name: "for-range",
			code: `for item := range poc.`,
			rng:  &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 19, EndLine: 1, EndColumn: 23},
		},
		{
			name: "for-init",
			code: `for i := poc.`,
			rng:  &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 10, EndLine: 1, EndColumn: 14},
		},
		{
			name: "switch-cond",
			code: `switch poc.`,
			rng:  &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 8, EndLine: 1, EndColumn: 12},
		},
		{
			name: "return",
			code: `return poc.`,
			rng:  &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 8, EndLine: 1, EndColumn: 12},
		},
		{
			name: "nested-call-arg",
			code: `data = poc.HTTP(poc.`,
			rng:  &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 17, EndLine: 1, EndColumn: 21},
		},
		{
			name: "func-body",
			code: "func handle() {\n    rsp, req, err := poc.",
			rng:  &ypb.Range{Code: "poc.", StartLine: 2, StartColumn: 22, EndLine: 2, EndColumn: 26},
		},
	}

	for _, c := range pocMemberCases {
		c := c
		t.Run("poc-member/"+c.name, func(t *testing.T) {
			t.Parallel()
			labels, err := getCompletionLabels(t, c.code, c.rng)
			require.NoErrorf(t, err, "incomplete code should not error: %q", c.code)
			require.NotEmptyf(t, labels, "incomplete code should still get completion: %q", c.code)
			require.Truef(t, lo.Contains(labels, "HTTP") && lo.Contains(labels, "Get"),
				"want poc members (HTTP/Get) for %q, got sample=%v", c.code, lo.Slice(labels, 0, 12))
		})
	}

	// 断言：字符串变量的不完整成员补全给出字符串内置方法。
	t.Run("string-builtin-method", func(t *testing.T) {
		t.Parallel()
		labels, err := getCompletionLabels(t, "name = \"hello\"\nif name.", &ypb.Range{
			Code: "name.", StartLine: 2, StartColumn: 4, EndLine: 2, EndColumn: 9,
		})
		require.NoError(t, err)
		require.Truef(t, lo.Contains(labels, "Contains") || lo.Contains(labels, "Split"),
			"want string builtin methods, got sample=%v", lo.Slice(labels, 0, 12))
	})
}

// 验证：补全/悬浮文档里的 <|EXAMPLE_START|>...<|EXAMPLE_END|> 标记，
// 在发送给前端前已被渲染成代码围栏，前端不应再看到裸标记。
func TestGRPCMUSTPASS_LANGUAGE_Suggestion_ExampleMarkerRendered(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	resp, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   COMPLETION,
		YakScriptType: "yak",
		YakScriptCode: `poc.`,
		Range:         &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 5},
		ModelID:       uuid.NewString(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.SuggestionMessage)

	var checkedHTTP bool
	for _, item := range resp.SuggestionMessage {
		require.NotContainsf(t, item.Description, exampleStartMarkerToken, "label %s desc should not contain raw marker", item.Label)
		require.NotContainsf(t, item.Description, exampleEndMarkerToken, "label %s desc should not contain raw marker", item.Label)
		if item.Label == "HTTP" {
			checkedHTTP = true
			require.Contains(t, item.Description, "```", "HTTP example should be rendered as fenced code block")
			require.Contains(t, item.Description, "示例", "HTTP example should carry a 示例 label")
		}
	}
	require.True(t, checkedHTTP, "should have inspected poc.HTTP completion item")
}

// RenderExampleMarkersForMarkdown 的纯函数单元测试(不依赖 gRPC)。
// 命名前缀 TestGRPCMUSTPASS_LANGUAGE 保证在 essential-tests.yml 的
// "Test gRPC MUSTPASS Language" 分片(run: ^TestGRPCMUSTPASS_LANGUAGE.*$)中被覆盖。
func TestGRPCMUSTPASS_LANGUAGE_RenderExampleMarkersForMarkdown(t *testing.T) {
	t.Run("no marker returns as-is", func(t *testing.T) {
		in := "just some doc\nwith no example"
		require.Equal(t, in, RenderExampleMarkersForMarkdown(in))
	})

	t.Run("single example with title", func(t *testing.T) {
		in := strings.Join([]string{
			"这是函数说明",
			"<|EXAMPLE_START|> 基础用法",
			"```",
			"rsp = poc.Get(\"https://example.com\")~",
			"```",
			"<|EXAMPLE_END|>",
		}, "\n")
		out := RenderExampleMarkersForMarkdown(in)
		require.NotContains(t, out, exampleStartMarkerToken)
		require.NotContains(t, out, exampleEndMarkerToken)
		require.Contains(t, out, "**示例：基础用法**")
		require.Contains(t, out, "```yak")
		require.Contains(t, out, `rsp = poc.Get("https://example.com")~`)
		// 内部作者写的 ``` 围栏应被清洗掉，只保留一层外围围栏
		require.Equal(t, 2, strings.Count(out, "```"), "should keep exactly one open/close fence pair")
	})

	t.Run("multiple examples get numbered when untitled", func(t *testing.T) {
		in := strings.Join([]string{
			"<|EXAMPLE_START|>",
			"a = 1",
			"<|EXAMPLE_END|>",
			"<|EXAMPLE_START|>",
			"b = 2",
			"<|EXAMPLE_END|>",
		}, "\n")
		out := RenderExampleMarkersForMarkdown(in)
		require.Contains(t, out, "**示例 1**")
		require.Contains(t, out, "**示例 2**")
	})

	t.Run("dynamic backtick count wraps code containing backticks", func(t *testing.T) {
		// 示例代码内部含有 4 连反引号，外围围栏必须使用更多反引号以"完美包裹"
		in := strings.Join([]string{
			"<|EXAMPLE_START|> 含反引号",
			"x = \"````\"",
			"<|EXAMPLE_END|>",
		}, "\n")
		out := RenderExampleMarkersForMarkdown(in)
		require.Contains(t, out, "`````yak", "fence must be longer than inner backtick run")
		require.Contains(t, out, "x = \"````\"")
	})

	t.Run("unclosed block is still flushed", func(t *testing.T) {
		in := strings.Join([]string{
			"prose",
			"<|EXAMPLE_START|> 未闭合",
			"y = 2",
		}, "\n")
		out := RenderExampleMarkersForMarkdown(in)
		require.NotContains(t, out, exampleStartMarkerToken)
		require.Contains(t, out, "y = 2")
	})
}

// completionLabels 是给下面若干补全用例复用的小工具：发一次补全请求并返回 label 列表。
func completionLabels(t *testing.T, local ypb.YakClient, scriptType, code string, rng *ypb.Range) ([]string, error) {
	t.Helper()
	resp, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   COMPLETION,
		YakScriptType: scriptType,
		YakScriptCode: code,
		Range:         rng,
		ModelID:       uuid.NewString(),
	})
	if err != nil {
		return nil, err
	}
	return lo.Map(resp.SuggestionMessage, func(i *ypb.SuggestionDescription, _ int) string { return i.Label }), nil
}

// 场景：在 mitm 插件里写 callback（如 mirrorHTTPFlow / hijackSaveHTTPFlow / hijackHTTPResponse）
// 时，callback 的形参类型来自内置 callback 签名(见 static_analyzer/ssa_option/plugin_options.go)。
// 用户在 callback 体内对这些形参做成员补全时，即便 callback 体还没写完，也应能根据形参类型
// 推断出正确成员（req/rsp/body -> []byte 内置方法；flow -> *schema.HTTPFlow 字段）。
// 关键词: mitm callback 形参类型推断, 不完整 callback 体补全
func TestGRPCMUSTPASS_LANGUAGE_SuggestionCompletion_Callback(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("mirrorHTTPFlow-req-bytes", func(t *testing.T) {
		t.Parallel()
		labels, err := completionLabels(t, local, "mitm",
			"mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {\n    req.\n}",
			&ypb.Range{Code: "req.", StartLine: 2, StartColumn: 5, EndLine: 2, EndColumn: 9})
		require.NoError(t, err)
		require.Truef(t, lo.Contains(labels, "Contains") && lo.Contains(labels, "HasPrefix"),
			"want []byte builtin methods for req, got sample=%v", lo.Slice(labels, 0, 15))
	})

	t.Run("mirrorHTTPFlow-body-bytes", func(t *testing.T) {
		t.Parallel()
		labels, err := completionLabels(t, local, "mitm",
			"mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {\n    body.\n}",
			&ypb.Range{Code: "body.", StartLine: 2, StartColumn: 5, EndLine: 2, EndColumn: 10})
		require.NoError(t, err)
		require.Truef(t, lo.Contains(labels, "Contains"),
			"want []byte builtin methods for body, got sample=%v", lo.Slice(labels, 0, 15))
	})

	t.Run("hijackHTTPResponse-rsp-bytes", func(t *testing.T) {
		t.Parallel()
		labels, err := completionLabels(t, local, "mitm",
			"hijackHTTPResponse = func(isHttps, url, rsp, forward, drop) {\n    rsp.\n}",
			&ypb.Range{Code: "rsp.", StartLine: 2, StartColumn: 5, EndLine: 2, EndColumn: 9})
		require.NoError(t, err)
		require.Truef(t, lo.Contains(labels, "Contains") && lo.Contains(labels, "HasPrefix"),
			"want []byte builtin methods for rsp, got sample=%v", lo.Slice(labels, 0, 15))
	})

	t.Run("hijackSaveHTTPFlow-flow-fields", func(t *testing.T) {
		t.Parallel()
		labels, err := completionLabels(t, local, "mitm",
			"hijackSaveHTTPFlow = func(flow, modify, drop) {\n    flow.\n}",
			&ypb.Range{Code: "flow.", StartLine: 2, StartColumn: 5, EndLine: 2, EndColumn: 10})
		require.NoError(t, err)
		require.Truef(t, lo.Contains(labels, "Url") && lo.Contains(labels, "StatusCode"),
			"want *schema.HTTPFlow fields for flow, got sample=%v", lo.Slice(labels, 0, 15))
	})

	// 组合边界：callback 体内再写不完整的 for-range，对 []byte 形参做成员补全。
	// 这个组合曾因 for-range 不完整触发 SSA 空指针 panic。
	t.Run("callback-with-incomplete-forrange", func(t *testing.T) {
		t.Parallel()
		labels, err := completionLabels(t, local, "mitm",
			"mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {\n    for k := range req.\n}",
			&ypb.Range{Code: "req.", StartLine: 2, StartColumn: 20, EndLine: 2, EndColumn: 24})
		require.NoError(t, err)
		require.Truef(t, lo.Contains(labels, "Contains"),
			"want []byte builtin methods in incomplete for-range, got sample=%v", lo.Slice(labels, 0, 15))
	})
}

// 场景：这是生产环境的真实用法——用户经常在“已经写好的完整代码”中间插入/修改一部分，
// 使某一行临时变成不完整状态（如中间插入 `xxx.` 等待补全）。此时前后仍有大量合法代码，
// 补全既不能崩溃，也要针对光标处上下文给出合理结果。这里覆盖各种复杂/边界组合。
// 关键词: 中间编辑, 半成品行, 前后合法代码, 链式/嵌套/未闭合括号/多重错误容错
func TestGRPCMUSTPASS_LANGUAGE_SuggestionCompletion_Boundary(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	// 需要拿到 poc 成员的用例（poc 是 extern 库）。
	pocCases := []struct {
		name string
		code string
		rng  *ypb.Range
	}{
		{"for-in", "for x in poc.", &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 10, EndLine: 1, EndColumn: 14}},
		{"range-two-var", "for k, v = range poc.", &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 18, EndLine: 1, EndColumn: 22}},
		{"chan-recv", "x = <-poc.", &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 7, EndLine: 1, EndColumn: 11}},
		{"unary-not", "x = !poc.", &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 6, EndLine: 1, EndColumn: 10}},
		{"slice-index", "a = [1,2,3]\nb = a[poc.", &ypb.Range{Code: "poc.", StartLine: 2, StartColumn: 7, EndLine: 2, EndColumn: 11}},
		{"if-and-chain", "if str.HasPrefix(\"a\", \"b\") && poc.", &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 31, EndLine: 1, EndColumn: 35}},
		{"switch-case-body", "switch 1 {\ncase 1:\n    x = poc.\n}", &ypb.Range{Code: "poc.", StartLine: 3, StartColumn: 9, EndLine: 3, EndColumn: 13}},
		{"go-lib", "go poc.", &ypb.Range{Code: "poc.", StartLine: 1, StartColumn: 4, EndLine: 1, EndColumn: 8}},
	}
	for _, c := range pocCases {
		c := c
		t.Run("poc/"+c.name, func(t *testing.T) {
			t.Parallel()
			labels, err := completionLabels(t, local, "yak", c.code, c.rng)
			require.NoErrorf(t, err, "code=%q", c.code)
			require.Truef(t, lo.Contains(labels, "HTTP") && lo.Contains(labels, "Get"),
				"want poc members for %q, got sample=%v", c.code, lo.Slice(labels, 0, 12))
		})
	}

	// 中间编辑：合法脚本中某一行是半成品，补全该行仍要拿到该表达式的正确类型。
	t.Run("mid-edit-valid-chain-bytes", func(t *testing.T) {
		t.Parallel()
		// rsp 是 poc.HTTP 的第一个返回值 []byte，中间行 rsp. 应补全 []byte 方法。
		labels, err := completionLabels(t, local, "yak",
			"rsp, req = poc.HTTP(\"GET / HTTP/1.1\\r\\nHost: a\\r\\n\\r\\n\")~\nrsp.\nprintln(req)",
			&ypb.Range{Code: "rsp.", StartLine: 2, StartColumn: 1, EndLine: 2, EndColumn: 5})
		require.NoError(t, err)
		require.Truef(t, lo.Contains(labels, "Contains"),
			"want []byte methods for rsp in mid-edit, got sample=%v", lo.Slice(labels, 0, 15))
	})

	// 中间编辑：嵌套 map，成员链应给出内层 map 的方法与键。
	t.Run("nested-map-member", func(t *testing.T) {
		t.Parallel()
		labels, err := completionLabels(t, local, "yak",
			"m = {\"a\": {\"b\": 1}}\nm[\"a\"].",
			&ypb.Range{Code: "].", StartLine: 2, StartColumn: 6, EndLine: 2, EndColumn: 8})
		require.NoError(t, err)
		require.Truef(t, lo.Contains(labels, "Keys") && lo.Contains(labels, "b"),
			"want map methods + nested key b, got sample=%v", lo.Slice(labels, 0, 15))
	})

	// 鲁棒性：以下用例只要求“不报错且给出非空补全”，用于防止不完整/错误代码把补全打崩。
	robustCases := []struct {
		name string
		code string
		rng  *ypb.Range
	}{
		// 未闭合括号 + 后续还有代码
		{"unclosed-paren", "a = str.Split(\nb = 1", &ypb.Range{Code: "str.", StartLine: 1, StartColumn: 5, EndLine: 1, EndColumn: 9}},
		// 上一行语法错误 + 当前行库补全
		{"prev-line-syntax-error", "a = = =\nb = str.", &ypb.Range{Code: "str.", StartLine: 2, StartColumn: 5, EndLine: 2, EndColumn: 9}},
		// 赋值右值为空 + 后续完整
		{"empty-rhs-then-lib", "x = \ny = str.\nz = 3", &ypb.Range{Code: "str.", StartLine: 2, StartColumn: 5, EndLine: 2, EndColumn: 9}},
		// 空 range（等待非点补全，给出库/关键字/变量）
		{"empty-range", "poc.HTTP()\n", &ypb.Range{Code: "", StartLine: 2, StartColumn: 1, EndLine: 2, EndColumn: 1}},
	}
	for _, c := range robustCases {
		c := c
		t.Run("robust/"+c.name, func(t *testing.T) {
			t.Parallel()
			labels, err := completionLabels(t, local, "yak", c.code, c.rng)
			require.NoErrorf(t, err, "code=%q should not error", c.code)
			require.NotEmptyf(t, labels, "code=%q should still get completion", c.code)
		})
	}

	// 鲁棒性：完全未知变量的成员链、字符串字面量内部——只要求不报错(可以为空)。
	t.Run("robust/unknown-var-chain-no-error", func(t *testing.T) {
		t.Parallel()
		_, err := completionLabels(t, local, "yak", "unknownVar.foo.",
			&ypb.Range{Code: "foo.", StartLine: 1, StartColumn: 12, EndLine: 1, EndColumn: 16})
		require.NoError(t, err)
	})
}
