package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// userUsageCallbackCtxKeyT 是 ctx-based user usage callback 透传机制的 key.
// 该机制用于把 ai.usageCallback(...) 注册的 callback 通过 ctx 透传给
// `aicommon.InvokeLiteForge` 等顶级 API 创建的子 Config, 修复 enhancesearch
// (HyDE / SplitQuery / GeneralizeQuery / ExtractKeywords / build_questions)
// 等子调用走 MustGetSpeedPriorityAIModelCallback 时 user usage callback 丢失的 BUG.
//
// 关键词: userUsageCallbackCtxKey, ctx-based user usage callback, P3-T5
type userUsageCallbackCtxKeyT struct{}

var userUsageCallbackCtxKey = userUsageCallbackCtxKeyT{}

// WithUserUsageCallbackContext 把 user usage callback 注入 ctx, 子 Config 创建后
// 通过 cfg.GetContext() / GetUserUsageCallbackFromContext 即可拿回 callback,
// 进一步透传到 aispec.WithUsageCallback 让 LLM 末帧 token usage 触达用户脚本.
//
// 关键词: WithUserUsageCallbackContext, ctx 透传 user usage callback
func WithUserUsageCallbackContext(ctx context.Context, cb func(*aispec.ChatUsage)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if cb == nil {
		return ctx
	}
	return context.WithValue(ctx, userUsageCallbackCtxKey, cb)
}

// GetUserUsageCallbackFromContext 从 ctx 中取出 user usage callback. 若 ctx 为 nil
// 或未注入, 返回 nil.
//
// 关键词: GetUserUsageCallbackFromContext, ctx 透传 user usage callback
func GetUserUsageCallbackFromContext(ctx context.Context) func(*aispec.ChatUsage) {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(userUsageCallbackCtxKey)
	if v == nil {
		return nil
	}
	cb, ok := v.(func(*aispec.ChatUsage))
	if !ok {
		return nil
	}
	return cb
}
