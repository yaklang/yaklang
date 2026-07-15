package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 迭代次数临时扩充相关常量.
//
// 当 ReActLoop 触及迭代上限时,
// 框架通过 review 基建 (EVENT_TYPE_REQUIRE_USER_INTERACTIVE) 向用户发起
// "是否临时扩充迭代次数" 的询问. 为避免无限弹窗 / 被动无限续跑, 这里设置了
// 两道护栏:
//   - maxIterationExtensionCount: 单次 loop 执行期间最多允许扩充的次数 (3).
//   - 三个固定选项: +5 / +10 / 翻倍(=当前 maxIterations, 不低于 iterationExtensionMinDelta).
//
// 关键词: iteration extend, 临时扩充迭代上限, 防自旋护栏
const (
	// maxIterationExtensionCount 限制单次 loop 期间向用户询问扩充的总次数,
	// 超过后不再询问, 直接走原软性中断退出.
	maxIterationExtensionCount = 3
	// iterationExtensionMinDelta 是默认扩充增量的下限, 避免增量过小导致
	// 下一轮立刻又触顶 (频繁打断用户).
	iterationExtensionMinDelta = 10
)

// iterationExtensionCountVar 是 ReActLoop vars 中记录"已临时扩充次数"的 key.
const iterationExtensionCountVar = "__iteration_extension_count__"

// iterationExtensionOptionValues 是交互卡片中三个固定扩充选项的 value.
// 前端 (aireactdeps/handleUserInteractiveClient) 会把所选选项的 PromptTitle
// 作为 suggestion 回传; 这里同时把 value 与中文标题都作为匹配候选, 避免前端
// 回传格式差异导致漏判. 任一命中即视为同意扩充, delta 由 value 决定.
var iterationExtensionOptionValues = map[string]int{
	"+5":          5,
	"+10":         10,
	"翻倍":         0, // 0 表示"取当前 maxIterations", 由调用方换算
	"double":      0,
	"x2":          0,
}

// requestIterationExtension 在 ReActLoop 触及迭代上限时, 复用 review 基建
// (与 aireact/base.go _requireUserInteract / toolcall.go tool-use review 一致)
// 向用户发起 "是否临时扩充迭代次数" 的交互询问.
//
// 返回值:
//   - agreed: 用户是否同意扩充;
//   - delta:  本次扩充的迭代增量 (>=1), 仅在 agreed=true 时有意义;
//   - err:    交互流程中的非致命错误 (用于日志, 不影响主流程决策).
//
// 当已达扩充次数上限、
// 或任务上下文已取消时, 返回 (false, 0, nil), 调用方应回退到原软性中断退出.
//
// 关键词: request iteration extension, review 基建复用, 临时扩充迭代次数
func (r *ReActLoop) requestIterationExtension(
	task aicommon.AIStatefulTask,
	iterationCount int,
	maxIterations int,
) (agreed bool, delta int, err error) {
	if r == nil || utils.IsNil(task) {
		return false, 0, nil
	}
	cfg := r.config
	if utils.IsNil(cfg) {
		return false, 0, nil
	}
	// 防自旋: 累计扩充次数已达上限, 不再询问.
	if r.getIterationExtensionCount() >= maxIterationExtensionCount {
		log.Infof("ReactLoop[%v] iteration extension cap (%d) reached, fallback to soft interrupt",
			r.loopName, maxIterationExtensionCount)
		return false, 0, nil
	}
	// 任务上下文已取消: 不再阻塞等待用户, 直接退出.
	taskCtx := task.GetContext()
	if !utils.IsNil(taskCtx) {
		select {
		case <-taskCtx.Done():
			return false, 0, nil
		default:
		}
	}

	loopName := r.loopName
	if loopName == "" {
		loopName = "general-purpose"
	}

	// 构造交互卡片内容 (复用 EVENT_TYPE_REQUIRE_USER_INTERACTIVE, 前端已有
	// handleUserInteractiveClient 处理该事件类型).
	question := fmt.Sprintf(
		"[%s] 已到达迭代上限 (%d), 任务尚未完成. 请选择临时扩充的迭代轮数以继续:",
		loopName, maxIterations,
	)
	options := []map[string]any{
		{
			"index":              1,
			"prompt_title":       "+5",
			"option_name":        "+5",
			"option_description": fmt.Sprintf("追加 5 轮迭代 (新上限 %d)", maxIterations+5),
		},
		{
			"index":              2,
			"prompt_title":       "+10",
			"option_name":        "+10",
			"option_description": fmt.Sprintf("追加 10 轮迭代 (新上限 %d)", maxIterations+10),
		},
		{
			"index":              3,
			"prompt_title":       "翻倍",
			"option_name":        "翻倍",
			"option_description": fmt.Sprintf("迭代上限翻倍 (新上限 %d)", maxIterations*2),
		},
		{
			"index":              4,
			"prompt_title":       "停止",
			"option_name":        "cancel",
			"option_description": "不再扩充, 按原软性中断结束并总结未完成事项",
		},
	}

	epm := cfg.GetEndpointManager()
	if epm == nil {
		return false, 0, utils.Errorf("endpoint manager is nil")
	}
	ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE)
	ep.SetDefaultSuggestionContinue()

	reqs := map[string]any{
		"id":               ep.GetId(),
		"prompt":           question,
		"options":          options,
		"reason":           "reached max iterations, ask user for temporary extension",
		"iteration_count":  iterationCount,
		"max_iterations":   maxIterations,
		"extension_count":  r.getIterationExtensionCount(),
		"extension_cap":    maxIterationExtensionCount,
	}
	ep.SetReviewMaterials(reqs)
	if submitErr := cfg.SubmitCheckpointRequest(ep.GetCheckpoint(), reqs); submitErr != nil {
		// 检查点入库失败不影响交互流程, 仅记录日志.
		log.Errorf("submit checkpoint request for iteration extension failed: %v", submitErr)
	}

	emitter := r.emitter
	if utils.IsNil(emitter) {
		emitter = cfg.GetEmitter()
	}
	if !utils.IsNil(emitter) {
		emitter.EmitInteractiveJSON(ep.GetId(), schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE, "require-user-interact", reqs)
	}

	// 等待用户决定: 强制 Manual 策略 + skip_ai_review, 确保由真人拍板,
	// 不会被 AI 自动审核 / YOLO 自动放行. ctx 取任务上下文以便取消时解锁.
	waitCtx := taskCtx
	if utils.IsNil(waitCtx) {
		waitCtx = cfg.GetContext()
	}
	waitCtx = utils.SetContextKey(waitCtx, "skip_ai_review", true)
	if realCfg, ok := cfg.(*aicommon.Config); ok {
		realCfg.DoWaitAgreeWithPolicy(waitCtx, aicommon.AgreePolicyManual, ep)
	} else {
		// 非 *Config 实现 (如 mock): 退化为 DoWaitAgree, 仍可被 Feed 释放.
		cfg.DoWaitAgree(waitCtx, ep)
	}

	params := ep.GetParams()
	if !utils.IsNil(emitter) {
		emitter.EmitInteractiveRelease(ep.GetId(), params)
	}
	cfg.CallAfterInteractiveEventReleased(ep.GetId(), params)

	if params == nil {
		// 用户取消 (空响应释放) -> 不扩充.
		log.Infof("ReactLoop[%v] iteration extension request got nil params (user cancelled)", r.loopName)
		return false, 0, nil
	}

	suggestion := strings.TrimSpace(params.GetAnyToString("suggestion"))
	delta, ok := matchIterationExtensionOption(suggestion, maxIterations)
	if !ok {
		if invoker := r.GetInvoker(); invoker != nil {
			invoker.AddToTimeline("iteration_extend", fmt.Sprintf(
				"[%v] user declined iteration extension (suggestion=%q), will soft-interrupt",
				loopName, suggestion))
		}
		return false, 0, nil
	}

	// 记录扩充次数 + timeline 痕迹.
	r.incrementIterationExtensionCount()
	if invoker := r.GetInvoker(); invoker != nil {
		invoker.AddToTimeline("iteration_extend", fmt.Sprintf(
			"[%v] user agreed to extend max iterations by %d (new cap=%d, extension #%d/%d)",
			loopName, delta, maxIterations+delta, r.getIterationExtensionCount(), maxIterationExtensionCount))
	}
	log.Infof("ReactLoop[%v] iteration extended by %d (new max=%d)",
		r.loopName, delta, maxIterations+delta)
	return true, delta, nil
}

// matchIterationExtensionOption 把用户选择的 suggestion 映射为扩充增量.
// 三个固定选项: "+5" -> 5, "+10" -> 10, "翻倍"/"double"/"x2" -> maxIterations.
// 命中返回 (delta, true); 未命中 (含 "停止"/"cancel"/空串) 返回 (0, false).
//
// 关键词: iteration extension option match, 固定三选项
func matchIterationExtensionOption(suggestion string, maxIterations int) (int, bool) {
	if suggestion == "" {
		return 0, false
	}
	lower := strings.ToLower(suggestion)
	for value, delta := range iterationExtensionOptionValues {
		if strings.Contains(lower, strings.ToLower(value)) {
			if delta == 0 {
				// "翻倍" 选项: 增量 = 当前 maxIterations, 但不低于 iterationExtensionMinDelta.
				if maxIterations < iterationExtensionMinDelta {
					return iterationExtensionMinDelta, true
				}
				return maxIterations, true
			}
			return delta, true
		}
	}
	return 0, false
}

// getIterationExtensionCount 返回本次 loop 期间已临时扩充迭代次数的累计值.
func (r *ReActLoop) getIterationExtensionCount() int {
	if r == nil {
		return 0
	}
	return r.GetInt(iterationExtensionCountVar)
}

// incrementIterationExtensionCount 把已扩充次数 +1.
func (r *ReActLoop) incrementIterationExtensionCount() {
	if r == nil {
		return
	}
	r.Set(iterationExtensionCountVar, r.getIterationExtensionCount()+1)
}

