package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

// 用户在编辑器里动态输入时，代码常处于不完整/半成品状态。语言服务会用
// ssaapi.WithIgnoreSyntaxError(true) 兜底解析这些代码来做补全/悬浮。
// 历史上一些不完整输入(如 `for item := range poc.`、`defer m.`)会在 SSA
// 构建阶段触发空指针 panic，导致整个解析崩溃、补全彻底失效。
//
// 本用例把这些不完整片段固化下来，确保 SSA 前端对它们“只报错、不 panic”。
// 关键词: 不完整代码, IgnoreSyntaxError, 防 panic 回归, for-range, defer, go
func TestIncompleteCode_NoPanicOnSSABuild(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{"for-range-member-dot", "for item := range poc."},
		{"for-range-two-var", "for k, v = range poc."},
		{"for-in-member-dot", "for x in poc."},
		{"for-init-member-dot", "for i := poc."},
		{"defer-user-var-dot", "m = {\"a\": 1}\ndefer m."},
		{"defer-lib-dot", "defer poc."},
		{"go-lib-dot", "go poc."},
		{"chan-recv-dot", "x = <-poc."},
		{"unary-dot", "x = !poc."},
		{"slice-index-dot", "a = [1,2,3]\nb = a[poc."},
		{"if-init-multi-assign-dot", "if req, rsp, err := poc."},
		{"nested-call-arg-dot", "data = poc.HTTP(poc."},
		{"switch-cond-dot", "switch poc."},
		{"return-dot", "return poc."},
		{"func-body-dot", "func handle() {\n    rsp, req, err := poc."},
		{"if-chain-dot", "if str.HasPrefix(\"a\", \"b\") && poc."},
		{"multi-error-then-dot", "a = = =\nb = str."},
		{"unclosed-paren-dot", "a = str.Split("},
	}

	parse := func(code string) (err error) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("SSA build panicked on incomplete code:\n%s\npanic: %v", code, r)
			}
		}()
		opts := append(static_analyzer.GetPluginSSAOpt("yak"), ssaapi.WithIgnoreSyntaxError(true))
		_, err = ssaapi.Parse(code, opts...)
		return err
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			// 允许返回错误(不完整本就有语法/语义错误)，但绝不允许 panic。
			_ = parse(c.code)
			require.True(t, true)
		})
	}
}
