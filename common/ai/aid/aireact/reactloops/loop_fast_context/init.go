package loop_fast_context

import (
	"bytes"
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstructionTpl string

//go:embed prompts/output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveDataTpl string

// allowedActions for the exploration subagent.
var allowedActions = []string{
	"grep_files",
	"grep_files_batch",
	"find_files",
	schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	"submit_fast_context_result",
}

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_FAST_CONTEXT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			maxIter := defaultMaxIterations
			if cfg := r.GetConfig(); cfg != nil && cfg.GetMaxIterationCount() > 0 {
				maxIter = int(cfg.GetMaxIterationCount())
				if maxIter > 15 {
					maxIter = 15
				}
			}

			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowUserInteract(false),
				reactloops.WithEnableSelfReflection(false),
				reactloops.WithDisableLoopPerception(true),
				reactloops.WithDisableTodoSnapshot(true),
				reactloops.WithMaxIterations(maxIter),
				reactloops.WithAITagFieldWithAINodeId(
					"FASTCONTEXT_RESULT", "fastcontext_result_md",
					"fastcontext-explore-result", aicommon.TypeTextMarkdown,
				),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					for _, name := range allowedActions {
						if action.ActionType == name {
							return true
						}
					}
					return false
				}),
				reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
					snap, _ := loop.GetVariable(loopVarEnvSnapshot).(WorkEnvSnapshot)
					return utils.RenderTemplate(persistentInstructionTpl, map[string]any{
						"OSKind":            snap.OSKind,
						"ShellName":         snap.ShellName,
						"WorkDir":           snap.WorkDir,
						"WorkDirLS":         snap.DirListing,
						"Query":             loop.Get(loopVarUserQuery),
						"ReferenceMaterial": loop.Get(loopVarReferenceMaterial),
					})
				}),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					return utils.RenderTemplate(reactiveDataTpl, map[string]any{
						"Nonce":            nonce,
						"Query":            loop.Get(loopVarUserQuery),
						"WorkDir":          loop.Get(loopVarWorkDir),
						"Iteration":        loop.GetCurrentIterationIndex() + 1,
						"MaxIterations":    loop.GetMaxIterations(),
						"SearchRounds":     loop.Get(loopVarSearchRounds),
						"LocationCount":    len(listFileIndex(loop)),
						"FeedbackMessages": feedbacker.String(),
					})
				}),
				submitFastContextResultAction(r),
				grepFilesAction(r),
				findFilesAction(r),
				grepFilesBatchAction(r),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_FAST_CONTEXT, r, preset...)
		},
		reactloops.WithLoopDescription("FastContext: isolated codebase exploration subagent. Parallel read-only search returns structured file paths and line ranges without polluting the caller context."),
		reactloops.WithLoopDescriptionZh("FastContext 代码库探索子智能体：在独立循环中并行只读搜索，返回结构化「文件路径+行号」索引，不污染主智能体上下文。"),
		reactloops.WithVerboseName("FastContext Explorer"),
		reactloops.WithVerboseNameZh("FastContext 探索器"),
		reactloops.WithLoopUsagePrompt("当需要在大型代码库中快速定位相关文件和行号、又不想把 grep 细节灌入主对话时使用。可作为子 loop（@action: fast_context）或由 fastcontext_search 工具委派调用。"),
		reactloops.WithLoopOutputExample(`
* 进入 FastContext 探索：
  {"@action": "fast_context", "human_readable_thought": "需要在代码库中定位与上传处理相关的实现", "query": "file upload handler and validation"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop %s failed: %v", schema.AI_REACT_LOOP_NAME_FAST_CONTEXT, err)
	}
}
