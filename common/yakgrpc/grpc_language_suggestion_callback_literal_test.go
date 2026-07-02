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

// 场景：库函数的形参类型是「函数(回调)」时，用户在实参位置输入 f / func / ( 等，
// 补全应直接给出可 tab 展开的回调函数字面量骨架(func 声明式 / 箭头式)，
// 且形参个数与回调签名一致。例如 poc.saveHandler(f) 期望 func(*lowhttp.LowhttpResponse)。
// 编辑器通常会自动补全右括号，故这里以闭合括号 poc.saveHandler(f) 作为真实文档形态。
// 关键词: 回调函数自动补全, poc.saveHandler, func 字面量, 箭头函数
func TestGRPCMUSTPASS_LANGUAGE_SuggestionCompletion_CallbackLiteral(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	getSuggestions := func(t *testing.T, scriptType, code string, rng *ypb.Range) []*ypb.SuggestionDescription {
		t.Helper()
		resp, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
			InspectType:   COMPLETION,
			YakScriptType: scriptType,
			YakScriptCode: code,
			Range:         rng,
			ModelID:       uuid.NewString(),
		})
		require.NoError(t, err)
		return resp.SuggestionMessage
	}

	// findCallbackLiteral 从补全项里找出 func 声明式与箭头式回调字面量。
	findCallbackLiteral := func(items []*ypb.SuggestionDescription) (funcItem, arrowItem *ypb.SuggestionDescription) {
		for _, it := range items {
			if it.Kind != CompletionKindSnippet {
				continue
			}
			if strings.HasPrefix(it.InsertText, "func(") {
				funcItem = it
			} else if strings.HasPrefix(it.InsertText, "(") && strings.Contains(it.InsertText, "=>") {
				arrowItem = it
			}
		}
		return
	}

	t.Run("poc.saveHandler-partial-ident", func(t *testing.T) {
		t.Parallel()
		// poc.saveHandler(f) 补全 f，期望 func(*lowhttp.LowhttpResponse)
		items := getSuggestions(t, "yak", `poc.saveHandler(f)`,
			&ypb.Range{Code: "f", StartLine: 1, StartColumn: 17, EndLine: 1, EndColumn: 18})
		funcItem, arrowItem := findCallbackLiteral(items)
		require.NotNilf(t, funcItem, "want func literal suggestion, got labels=%v",
			lo.Map(items, func(i *ypb.SuggestionDescription, _ int) string { return i.Label }))
		require.NotNil(t, arrowItem, "want arrow literal suggestion")

		// 回调只有一个形参：snippet 里应恰好一个占位符 ${1:...}，且能被 tab 展开
		require.Contains(t, funcItem.InsertText, "${1:")
		require.NotContains(t, funcItem.InsertText, "${2:")
		require.Contains(t, funcItem.InsertText, "$0")
		// 友好形参名：*lowhttp.LowhttpResponse -> rsp
		require.Containsf(t, funcItem.InsertText, "${1:rsp}",
			"want friendly param name rsp, got %q", funcItem.InsertText)
		require.True(t, strings.HasSuffix(strings.TrimSpace(funcItem.InsertText), "}"))
		// 箭头式同样只有一个形参
		require.Contains(t, arrowItem.InsertText, "${1:")
		require.Contains(t, arrowItem.InsertText, "=>")
	})

	t.Run("poc.saveHandler-empty-arg", func(t *testing.T) {
		t.Parallel()
		// 光标紧贴左括号、实参尚为空：poc.saveHandler() 也应给出回调字面量
		items := getSuggestions(t, "yak", `poc.saveHandler()`,
			&ypb.Range{Code: "", StartLine: 1, StartColumn: 17, EndLine: 1, EndColumn: 17})
		funcItem, arrowItem := findCallbackLiteral(items)
		require.NotNilf(t, funcItem, "want func literal for empty arg, got labels=%v",
			lo.Map(items, func(i *ypb.SuggestionDescription, _ int) string { return i.Label }))
		require.NotNil(t, arrowItem)
	})

	t.Run("poc.afterSaveHandler-httpflow", func(t *testing.T) {
		t.Parallel()
		// 期望 func(*schema.HTTPFlow)
		items := getSuggestions(t, "yak", `poc.afterSaveHandler(f)`,
			&ypb.Range{Code: "f", StartLine: 1, StartColumn: 22, EndLine: 1, EndColumn: 23})
		funcItem, _ := findCallbackLiteral(items)
		require.NotNilf(t, funcItem, "want func literal for afterSaveHandler, got labels=%v",
			lo.Map(items, func(i *ypb.SuggestionDescription, _ int) string { return i.Label }))
		require.Contains(t, funcItem.InsertText, "${1:")
		// 友好形参名：*schema.HTTPFlow -> flow
		require.Containsf(t, funcItem.InsertText, "${1:flow}",
			"want friendly param name flow, got %q", funcItem.InsertText)
	})

	t.Run("tcp.serverCallback-single-nonvariadic", func(t *testing.T) {
		t.Parallel()
		// tcp.serverCallback(f) 期望 func(*tcpConnection)，非变长单形参
		items := getSuggestions(t, "yak", `tcp.Serve("127.0.0.1", 8080, tcp.serverCallback(f))`,
			&ypb.Range{Code: "f", StartLine: 1, StartColumn: 49, EndLine: 1, EndColumn: 50})
		funcItem, arrowItem := findCallbackLiteral(items)
		require.NotNilf(t, funcItem, "want func literal for tcp.serverCallback, got labels=%v",
			lo.Map(items, func(i *ypb.SuggestionDescription, _ int) string { return i.Label }))
		require.NotNil(t, arrowItem)
		require.Contains(t, funcItem.InsertText, "${1:")
		require.NotContains(t, funcItem.InsertText, "${2:")
	})

	t.Run("httpserver.handler-two-params", func(t *testing.T) {
		t.Parallel()
		// httpserver 的 handler 期望 func(rsp, req)，两个形参
		items := getSuggestions(t, "yak", `httpserver.Serve("127.0.0.1", 8080, httpserver.handler(f))`,
			&ypb.Range{Code: "f", StartLine: 1, StartColumn: 56, EndLine: 1, EndColumn: 57})
		funcItem, _ := findCallbackLiteral(items)
		require.NotNilf(t, funcItem, "want func literal for httpserver.handler, got labels=%v",
			lo.Map(items, func(i *ypb.SuggestionDescription, _ int) string { return i.Label }))
		// 两个形参：应有 ${1:...} 与 ${2:...}
		require.Contains(t, funcItem.InsertText, "${1:")
		require.Contains(t, funcItem.InsertText, "${2:")
	})

	t.Run("callback-with-return-value", func(t *testing.T) {
		t.Parallel()
		// http.redirect 的回调返回 bool，函数体应预置 return 骨架
		items := getSuggestions(t, "yak", `http.Get("http://a", http.redirect(f))`,
			&ypb.Range{Code: "f", StartLine: 1, StartColumn: 36, EndLine: 1, EndColumn: 37})
		funcItem, _ := findCallbackLiteral(items)
		require.NotNilf(t, funcItem, "want func literal for http.redirect, got labels=%v",
			lo.Map(items, func(i *ypb.SuggestionDescription, _ int) string { return i.Label }))
		require.Containsf(t, funcItem.InsertText, "return $0",
			"callback returning value should carry return skeleton, got %q", funcItem.InsertText)
	})

	t.Run("negative-string-param-no-callback", func(t *testing.T) {
		t.Parallel()
		// 第一个实参期望 string，不应给出回调字面量
		items := getSuggestions(t, "yak", `poc.Get(u)`,
			&ypb.Range{Code: "u", StartLine: 1, StartColumn: 9, EndLine: 1, EndColumn: 10})
		funcItem, arrowItem := findCallbackLiteral(items)
		require.Nil(t, funcItem, "string param should not offer func literal")
		require.Nil(t, arrowItem, "string param should not offer arrow literal")
	})
}
