package loop_coding

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	LoopName                = schema.AI_REACT_LOOP_NAME_CODING
	CodingWorkflowSkillName = "coding-workflow"
)

var instruction = promptloader.MustLoad("ai/aid/aireact/reactloops/loop_coding/prompts/persistent_instruction.txt")

var outputExample = promptloader.MustLoad("ai/aid/aireact/reactloops/loop_coding/prompts/output_example.txt")

// codingDefaultToolNames controls only the initial disclosure order. Every
// enabled tool, profile plugin, MCP tool, and dynamically discovered tool stays
// callable through the ordinary require_tool/directly_call_tool paths.
var codingDefaultToolNames = []string{
	"search_capabilities",
	"ls",
	"tree",
	"find_file",
	"grep",
	"read_file",
	"read_file_lines",
	"head_file",
	"tail_file",
	"query_file_meta",
	"write_file",
	"modify_file",
	"copy_file",
	"remove_file",
	"diff_file",
	"query_document",
	"web_search",
	"use_browser",
	"bash",
	"powershell",
	"cmd",
	"check_command_availability",
	"ostype",
	"git-current-branch",
	"git-head",
	"git-branch",
	"git-blame",
	"git-clone",
	"git-dump-fs-commit",
	"ssa-project-info",
	"ssa-list-files",
	"ssa-read-file",
	"ssa-grep",
	"check-yaklang-syntax",
	"check-syntaxflow-syntax",
}

var codingScenarioToolNames = []string{
	"ssa-project-info",
	"ssa-list-files",
	"ssa-read-file",
	"ssa-grep",
	"check-yaklang-syntax",
	"check-syntaxflow-syntax",
}

var codingDefaultSkillNames = []string{
	CodingWorkflowSkillName,
	"yaklang-system",
	"fuzztag",
	"code-review",
}

// DefaultToolNames returns the coding branch's default disclosure list.
func DefaultToolNames() []string {
	return append([]string(nil), codingDefaultToolNames...)
}

// DefaultSkillNames returns the coding branch's default skill directory.
func DefaultSkillNames() []string {
	return append([]string(nil), codingDefaultSkillNames...)
}

// ToolManagerOptions composes the coding branch's default tool set from the
// shared builtin catalog. MCP tools are appended by the normal ReAct loader.
func ToolManagerOptions() []buildinaitools.ToolManagerOption {
	return []buildinaitools.ToolManagerOption{
		buildinaitools.WithEnabledTools(DefaultToolNames()),
		buildinaitools.WithDefaultToolDisclosure(buildinaitools.ToolDisclosurePolicy{
			Names:     DefaultToolNames(),
			Prefixes:  []string{"mcp_"},
			Exclusive: true,
		}),
	}
}

func init() {
	err := reactloops.RegisterLoopFactory(
		LoopName,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithScenarioToolWhitelist(codingScenarioToolNames),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithOutputExample(outputExample),
				reactloops.WithInitTask(buildInitTask(r)),
			}
			if realConfig, ok := r.GetConfig().(*aicommon.Config); ok {
				if loader := realConfig.GetSkillLoader(); loader != nil {
					preset = append(preset, reactloops.WithSkillLoader(
						loader,
						aiskillloader.WithDefaultSkillInventory(DefaultSkillNames()...),
					))
				}
			}
			preset = append(preset, opts...)
			return loop_default.NewLoop(LoopName, r, preset...)
		},
		reactloops.WithLoopDescription("Coding-only software engineering mode for repository inspection, implementation, testing, review, and debugging."),
		reactloops.WithLoopDescriptionZh("编程专用模式：聚焦仓库阅读、代码实现、测试、审查与缺陷修复，不执行安全扫描或其它非编程任务。"),
		reactloops.WithLoopUsagePrompt("Use only for software engineering work such as implementing code, fixing bugs, refactoring, writing tests, reviewing code, running builds, and diagnosing development issues."),
		reactloops.WithLoopOutputExample(`
* When the user asks to implement or fix code:
  {"@action": "coding", "human_readable_thought": "Inspect the repository, make the smallest complete change, then verify it"}
`),
		reactloops.WithVerboseName("Coding Agent"),
		reactloops.WithVerboseNameZh("编程 Agent"),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", LoopName, err)
	}
}

// buildInitTask deliberately skips the general loop's intent router. A coding
// distribution is a fixed profile: it must not silently switch into security,
// traffic-analysis, or other focus modes after the user has chosen this entry.
func buildInitTask(r aicommon.AIInvokeRuntime) func(
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	operator *reactloops.InitTaskOperator,
) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		reactloops.RunAttachedExtraResourcesInit(r, loop, aicommon.NonEmptyAttachedResources(task.GetAttachedDatas()))
		if mgr := loop.GetSkillsContextManager(); mgr != nil {
			if _, err := mgr.LoadForcedSkill(CodingWorkflowSkillName); err != nil &&
				!strings.Contains(err.Error(), "not found") {
				log.Warnf("load coding workflow skill: %v", err)
			}
		}
		operator.Continue()
	}
}
