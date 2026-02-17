package loop_intent

import (
	"bytes"
	_ "embed"
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

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_INTENT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowUserInteract(false),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(2),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					allowActionNames := []string{
						"search_capabilities",
						"finalize_enrichment",
					}
					for _, actionName := range allowActionNames {
						if action.ActionType == actionName {
							return true
						}
					}
					return false
				}),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					userQuery := loop.Get("user_query")
					searchResults := loop.Get("search_results")
					intentAnalysis := loop.Get("intent_analysis")
					language := loop.Get("language")
					if language == "" {
						language = "zh"
					}

					renderMap := map[string]any{
						"UserQuery":      userQuery,
						"SearchResults":  searchResults,
						"IntentAnalysis": intentAnalysis,
						"Language":       language,
						"Nonce":          nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register custom actions
				searchCapabilitiesAction(r),
				finalizeEnrichmentAction(r),
				// Post-iteration hook: ensures finalization always runs on loop exit
				// (mirrors loop_knowledge_enhance pattern)
				BuildOnPostIterationHook(r),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_INTENT, r, preset...)
		},
		reactloops.WithLoopDescription("Intent recognition and context enrichment mode: analyzes user input to identify intent, search for relevant tools/forges/skills, and produce context enrichment for the main loop."),
		reactloops.WithLoopUsagePrompt("Used internally when user input is medium-to-large scale and requires deep intent decomposition and capability matching before the main loop can proceed effectively."),
		reactloops.WithLoopIsHidden(true),
	)
	if err != nil {
		log.Errorf("register reactloop %s failed: %v", schema.AI_REACT_LOOP_NAME_INTENT, err)
	}
}

// getLanguageFromConfig reads language preference from AICallerConfigIf.
// Default is "zh" (Chinese) if not set or not accessible.
func getLanguageFromConfig(r aicommon.AIInvokeRuntime) string {
	config := r.GetConfig()
	// Try to get language via GetLanguage() if the concrete type supports it
	if langGetter, ok := config.(interface{ GetLanguage() string }); ok {
		if lang := langGetter.GetLanguage(); lang != "" {
			return lang
		}
	}
	// Fallback: check KeyValueConfig
	if lang := config.GetConfigString("language"); lang != "" {
		return lang
	}
	return "zh"
}

// buildInitTask creates the init handler for the intent recognition loop.
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		userQuery := task.GetUserInput()

		// Read language preference: default "zh" (Chinese)
		language := getLanguageFromConfig(r)

		// Store user query and language in loop context for reactive data template
		loop.Set("user_query", userQuery)
		loop.Set("language", language)
		loop.Set("search_results", "")
		loop.Set("intent_analysis", "")
		loop.Set("recommended_tools", "")
		loop.Set("recommended_forges", "")
		loop.Set("context_enrichment", "")

		// Build a summary of available loop metadata for the AI to reference
		allMeta := reactloops.GetAllLoopMetadata()
		var loopSummary strings.Builder
		for _, meta := range allMeta {
			if meta.IsHidden {
				continue
			}
			loopSummary.WriteString("- " + meta.Name)
			if meta.Description != "" {
				loopSummary.WriteString(": " + meta.Description)
			}
			loopSummary.WriteString("\n")
		}
		if loopSummary.Len() > 0 {
			loop.Set("available_focus_modes", loopSummary.String())
		}

		r.AddToTimeline("intent_init", "Intent recognition loop initialized for deep analysis")
		log.Infof("intent recognition loop initialized for query: %s", utils.ShrinkString(userQuery, 200))
		operator.Continue()
	}
}
