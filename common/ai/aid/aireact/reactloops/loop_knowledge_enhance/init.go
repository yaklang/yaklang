package loop_knowledge_enhance

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
var instruction string

//go:embed prompts/output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(false),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					knowledgeBaseName := loop.Get("knowledge_base_name")
					userQuery := loop.Get("user_query")
					searchResults := loop.Get("search_results")
					searchHistory := loop.Get("search_history")

					feedbacks := feedbacker.String()

					renderMap := map[string]any{
						"KnowledgeBaseName": knowledgeBaseName,
						"UserQuery":         userQuery,
						"SearchResults":     searchResults,
						"SearchHistory":     searchHistory,
						"FeedbackMessages":  feedbacks,
						"Nonce":             nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register actions
				searchKnowledgeAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("知识库增强回答模式：根据用户问题查询指定知识库，利用知识库内容增强回答的准确性和专业性。"),
		reactloops.WithLoopUsagePrompt(`当用户需要基于特定知识库回答问题时使用此流程，如：
- "什么是XXX？"
- "请查询知识库并告诉我关于XXX的信息"
调用本流程，将启用如下专用工具：search_knowledge（搜索知识库）、generate_answer（基于知识生成回答）。
AI会根据用户问题推测相关关键词并从知识库检索，然后基于检索结果生成准确的回答。`),
		reactloops.WithLoopOutputExample(`
* 当用户请求基于知识库回答问题时：
  {"@action": "knowledge_enhance", "human_readable_thought": "需要查询知识库来回答用户关于XXX的问题"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, err)
	}
}

// buildInitTask creates the initial task for the knowledge enhance loop
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		// Get user input from task
		userQuery := task.GetUserInput()

		// Get knowledge base name from loop context (set by caller)
		knowledgeBaseName := loop.Get("knowledge_base_name")
		if knowledgeBaseName == "" {
			// Try to get from config or use default
			knowledgeBaseName = r.GetConfig().GetConfigString("knowledge_base_name")
		}

		// Initialize loop context
		loop.Set("user_query", userQuery)
		loop.Set("knowledge_base_name", knowledgeBaseName)
		loop.Set("search_results", "")
		loop.Set("search_history", "")

		r.AddToTimeline("task_initialized", "Knowledge enhance task initialized: "+userQuery)
		return nil
	}
}
