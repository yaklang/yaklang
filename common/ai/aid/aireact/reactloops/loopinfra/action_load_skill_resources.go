package loopinfra

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_LoadSkillResources = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES,
	Description: `Load a specific file from a skill into the context window. ` +
		`Use format "@skill_name/path/to/file.md" to load a file from a loaded or available skill. ` +
		`If the exact path is not found, fuzzy matching is applied: the system strips the file extension ` +
		`and recursively searches the skill for matching files or directories. ` +
		`The file content will be displayed as a new View Window in the skills context.`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"resource_path",
			aitool.WithParam_Description(
				`The resource path to load, in format "@skill_name/path/to/file". `+
					`Example: "@recon/osint.md" loads osint.md from the recon skill. `+
					`If the exact path doesn't exist, the system will fuzzy-match by filename.`),
			aitool.WithParam_Required(true),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "resource_path", AINodeId: "load_skill_resources_path"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		resourcePath := action.GetString("resource_path")
		if resourcePath == "" {
			return utils.Error("load_skill_resources action requires 'resource_path' parameter")
		}

		mgr := loop.GetSkillsContextManager()
		if mgr == nil {
			return utils.Error("skills context manager is not available")
		}

		skillName, filePath, err := aiskillloader.ParseSkillResourcePath(resourcePath)
		if err != nil {
			return utils.Wrapf(err, "invalid resource_path")
		}
		if filePath == "" {
			return utils.Error("resource_path must include a file path after the skill name (e.g. @skill/file.md)")
		}

		loop.Set("_load_resource_skill", skillName)
		loop.Set("_load_resource_path", filePath)
		loop.Set("_load_resource_raw", resourcePath)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		skillName := loop.Get("_load_resource_skill")
		filePath := loop.Get("_load_resource_path")
		rawPath := loop.Get("_load_resource_raw")

		if skillName == "" || filePath == "" {
			op.Fail("load_skill_resources: missing skill name or file path")
			return
		}

		mgr := loop.GetSkillsContextManager()
		if mgr == nil {
			op.Fail("skills context manager is not available")
			return
		}

		invoker := loop.GetInvoker()

		result, err := mgr.LoadSkillResource(skillName, filePath)
		if err != nil {
			log.Warnf("failed to load skill resource %q: %v", rawPath, err)
			errMsg := fmt.Sprintf("Failed to load resource '%s': %v", rawPath, err)
			invoker.AddToTimeline("skill_resource_load_failed", errMsg)
			op.Feedback(errMsg)
			op.Continue()
			return
		}

		summary := aiskillloader.FormatResourceLoadSummary(result)
		log.Infof("loaded skill resource: %s", summary)

		timelineMsg := fmt.Sprintf(
			"Loaded skill resource '%s'. %s. "+
				"The content is now visible in the SKILLS_CONTEXT section as a View Window. "+
				"Context expanded by %.1fKB.",
			rawPath, summary, float64(result.ContentSize)/1024,
		)
		invoker.AddToTimeline("skill_resource_loaded", timelineMsg)

		feedbackMsg := fmt.Sprintf(
			"Resource '%s' loaded successfully. %s. "+
				"The file content is now displayed in the SKILLS_CONTEXT section of your prompt.",
			rawPath, summary,
		)
		if result.FuzzyMatched {
			feedbackMsg += fmt.Sprintf(
				" Note: exact path not found, fuzzy matched to '%s'.",
				result.MatchedPath,
			)
		}
		op.Feedback(feedbackMsg)
		op.Continue()
	},
}
