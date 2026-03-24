package loop_smart_qa

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

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

var allowedActions = []string{
	"search_knowledge",
	"web_search",
	"read_file",
	"find_files",
	"grep_text",
	"search_persistent_memory",
	"final_answer",
}

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SMART_QA,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithMaxIterations(5),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					for _, name := range allowedActions {
						if action.ActionType == name {
							return true
						}
					}
					return false
				}),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					renderMap := map[string]any{
						"UserQuery":     loop.Get("user_query"),
						"SearchResults": loop.Get("search_results_summary"),
						"SearchHistory": loop.Get("search_history"),
						"MemoryResults": loop.Get("memory_results"),
						"FileResults":   loop.Get("file_results"),
						"Nonce":         nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				knowledgeSearchAction(r),
				webSearchAction(r),
				readFileAction(r),
				findFilesAction(r),
				grepTextAction(r),
				memorySearchAction(r),
				finalAnswerAction(r),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SMART_QA, r, preset...)
		},
		reactloops.WithLoopDescription("Smart Q&A mode: an intelligent question-answering assistant that helps users learn and understand topics by leveraging knowledge bases, web search, local files, and persistent memory."),
		reactloops.WithLoopDescriptionZh("智能问答模式：结合知识库、网络搜索、本地文件和持久记忆，回答问题并帮助用户理解主题。"),
		reactloops.WithLoopUsagePrompt(`Use this mode when the user needs answers to questions, wants to learn about a topic, or needs to understand something.
The AI assistant will search knowledge bases, the internet, local files, and persistent memory to provide comprehensive, well-structured answers.`),
		reactloops.WithLoopOutputExample(`
* When the user asks a question that can be answered with research:
  {"@action": "smart_qa", "human_readable_thought": "The user wants to understand a topic, I'll search relevant sources and provide a comprehensive answer"}
`),

		reactloops.WithVerboseName("Smart Q&A"),
		reactloops.WithVerboseNameZh("智能问答"),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_SMART_QA, err)
	}
}

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		userQuery := task.GetUserInput()
		loop.Set("user_query", userQuery)
		loop.Set("search_results_summary", "")
		loop.Set("search_history", "")
		loop.Set("memory_results", "")
		loop.Set("file_results", "")

		config := r.GetConfig()
		attachedDatas := task.GetAttachedDatas()

		var knowledgeBaseNames []string
		if len(attachedDatas) > 0 {
			for _, data := range attachedDatas {
				if data == nil {
					continue
				}
				if data.Type == "knowledge_base" || data.Type == "kb" {
					name := data.Value
					if name != "" {
						knowledgeBaseNames = append(knowledgeBaseNames, name)
					}
				}
			}
		}
		if len(knowledgeBaseNames) > 0 {
			loop.Set("knowledge_bases", strings.Join(knowledgeBaseNames, ","))
		}

		_ = config
		r.AddToTimeline("task_initialized", fmt.Sprintf("Smart Q&A task initialized: %s", userQuery))
		operator.Continue()
	}
}
