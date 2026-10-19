package reactloops

import (
	"strings"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// timelineCompressionTurnContext carries the same turn material the next
// main-loop prompt would show. The compression request reuses that assembly
// and only replaces the dynamic task with a compression decision.
type timelineCompressionTurnContext struct {
	Nonce             string
	UserInput         string
	FrozenUserContext string
	Todo              string
	TaskInstruction   string
	ReactiveData      string
	Memory            string
	ExtraCapabilities string
	PlanStatus        string
	AutoContext       string
	Prompt            string
}

var contextOptimizationStatusIndex atomic.Uint64

var contextOptimizationStatusNames = []actionStatusName{
	{"正在优化上下文…", "Optimizing the context…"},
	{"正在整理历史记录，保留关键线索…", "Organizing the history and keeping key findings…"},
	{"正在精简上下文，保留重要信息…", "Compacting the context and keeping important information…"},
	{"正在归纳前面的进展…", "Summarizing progress so far…"},
	{"正在梳理上下文，准备继续…", "Organizing the context before continuing…"},
}

// runTimelineCompressionAction performs one compress_timeline action. Context
// generation calls it only after the assembled main-loop prompt is over the
// current timeline limit. Cancellation is returned; any other failure keeps
// the current context.
func (r *ReActLoop) runTimelineCompressionAction(turn timelineCompressionTurnContext) error {
	if r == nil || r.config == nil || timelineFromConfig(r.config) == nil {
		return nil
	}
	if config, ok := r.config.(*aicommon.Config); ok {
		config.SyncSessionEvidenceTimeline()
	}
	ctx := r.config.GetContext()
	if task := r.GetCurrentTask(); task != nil && task.GetContext() != nil {
		ctx = task.GetContext()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.compressionTurn = r.prepareCompressionTurn(turn)
	operator := newLoopActionHandlerOperator(r.GetCurrentTask())
	handleCompressTimeline(r, nil, operator)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func (r *ReActLoop) mainTimelinePromptOptions() aicommon.TimelinePromptOptions {
	if r == nil {
		return aicommon.TimelinePromptOptions{}
	}
	return aicommon.TimelinePromptOptions{
		IncludeLatestModelReplay: true,
		PromoteUserInput:         true,
		UserInputOnly:            r.useSpeedPriorityAI,
	}
}

func (r *ReActLoop) prepareCompressionTurn(turn timelineCompressionTurnContext) timelineCompressionTurnContext {
	if strings.TrimSpace(turn.Nonce) == "" {
		turn.Nonce = "compress-timeline"
	}
	if strings.TrimSpace(turn.TaskInstruction) == "" {
		provider := r.persistentInstructionProvider
		if r.functionCallMode && r.functionCallInstructionProvider != nil {
			provider = r.functionCallInstructionProvider
		}
		if provider != nil {
			if built, err := provider(r, ""); err == nil {
				turn.TaskInstruction = built
			}
		}
	}
	if turn.AutoContext == "" {
		turn.AutoContext = r.renderAsyncExecutionContext(turn.Nonce)
	}
	if turn.PlanStatus == "" && r.planStatusProvider != nil {
		turn.PlanStatus = r.planStatusProvider()
	}
	if turn.Todo == "" {
		turn.Todo = r.currentTodoSnapshot(turn.Nonce)
	}
	return turn
}

func (r *ReActLoop) currentTodoSnapshot(nonce string) string {
	if r == nil || r.config == nil || !r.shouldRenderTodoSnapshot() {
		return ""
	}
	content := r.config.GetVerificationTodoRendered(aicommon.BuildVerificationTodoScope(r.GetCurrentTask()))
	if content == "" {
		return ""
	}
	rendered, err := utils.RenderTemplate(todoListTemplate, map[string]any{"Nonce": nonce, "Todo": content})
	if err != nil {
		return ""
	}
	return rendered
}

const defaultWholePromptTokenLimit int64 = 300 * 1024

// wholePromptTokenLimit is the limit for the assembled main-loop prompt.
// The configured timeline limit defaults to 300k; a too-large response lowers it.
func (r *ReActLoop) wholePromptTokenLimit() int64 {
	if r == nil || r.config == nil {
		return defaultWholePromptTokenLimit
	}
	base := r.config.GetTimelineContentSizeLimit()
	if base <= 0 {
		base = defaultWholePromptTokenLimit
	}
	config, ok := r.config.(*aicommon.Config)
	if !ok || config == nil {
		return base
	}
	return config.EffectiveTimelineTokenLimit(config.AiServerName, config.AiModelName, base)
}

func (r *ReActLoop) noteCompressionStatus() {
	if r == nil {
		return
	}
	index := (contextOptimizationStatusIndex.Add(1) - 1) % uint64(len(contextOptimizationStatusNames))
	label := contextOptimizationStatusNames[index]
	r.UserStatus(label.zh, label.en, aicommon.WithStatusCode("context.optimizing"))
}

// compressMainPrompt folds first, rebuilds the whole main-loop prompt, and
// starts the model only when that prompt is still above half the limit.
func (r *ReActLoop) compressMainPrompt(input *loopPromptBuildInput, limit int64) error {
	if r == nil || input == nil {
		return nil
	}
	ctx := r.config.GetContext()
	if task := r.GetCurrentTask(); task != nil && task.GetContext() != nil {
		ctx = task.GetContext()
	}
	r.noteCompressionStatus()
	defer func() {
		if ctx.Err() == nil {
			r.UserStatus("正在准备下一轮请求…", "Preparing the next request…",
				aicommon.WithStatusCode("response.preparing"))
		}
	}()
	if timeline := timelineFromConfig(r.config); timeline != nil {
		if err := timeline.FoldRedundantForCompression(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Warnf("timeline fold failed; continuing compression: %v", err)
		}
	}
	rebuilt, err := r.generateLoopPromptFromInput(input)
	if err != nil {
		return err
	}
	if limit > 0 && int64(aicommon.MeasureTokens(rebuilt)) <= limit/2 {
		return nil
	}
	return r.runTimelineCompressionAction(timelineCompressionTurnContext{
		Nonce:             input.nonce,
		UserInput:         input.userInput,
		FrozenUserContext: input.frozenUserContext,
		Prompt:            rebuilt,
	})
}
