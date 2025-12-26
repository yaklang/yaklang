package loop_explore_filesystem

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

//go:embed prompts/reflection_output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_EXPLORE_FILESYSTEM,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			config := r.GetConfig()

			// Create preset options for explore_filesystem loop
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(config.GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(config.GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					// Get exploration findings from loop state
					findings := loop.Get("exploration_findings")
					targetPath := loop.Get("target_path")
					explorationGoal := loop.Get("exploration_goal")

					feedbacks := feedbacker.String()
					feedbacks = strings.TrimSpace(feedbacks)

					renderMap := map[string]any{
						"TargetPath":       targetPath,
						"ExplorationGoal":  explorationGoal,
						"Findings":         findings,
						"Nonce":            nonce,
						"FeedbackMessages": feedbacks,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register core grep action for filesystem exploration
				grepFilesystemAction(r),
				// Register conclude action for summarizing findings
				concludeExplorationAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_EXPLORE_FILESYSTEM, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("Enter focused mode for exploring filesystem and codebase with grep-based pattern matching. Use to analyze code structure, find implementations, and understand code relationships."),
		reactloops.WithLoopUsagePrompt("Use when user requests to explore, search, or analyze codebase. Provides specialized grep tool for pattern matching, code discovery, and structural analysis. Best for finding function implementations, tracking code flow, and understanding project structure."),
		reactloops.WithLoopOutputExample(`
* When user requests to explore codebase:
  {"@action": "explore_filesystem", "human_readable_thought": "I need to explore the codebase to find relevant code patterns and understand the structure"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_EXPLORE_FILESYSTEM)
	}
}
