package loopinfra

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_LoadingSkills = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS,
	Description: `Load a skill into the context window. Use this when you need specialized knowledge or instructions from an available skill. The skill's SKILL.md content and file tree will be displayed in the context.`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"skill_name",
			aitool.WithParam_Description(`The name of the skill to load. Must match one of the available skill names shown in the skills context.`),
			aitool.WithParam_Required(true),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "skill_name", AINodeId: "loading_skills_name"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		skillName := action.GetString("skill_name")
		if skillName == "" {
			return utils.Error("loading_skills action requires 'skill_name' parameter")
		}

		mgr := loop.GetSkillsContextManager()
		if mgr == nil {
			return utils.Error("skills context manager is not available")
		}

		// Check if the skill is already loaded and unfolded - reject redundant loading
		if mgr.IsSkillLoadedAndUnfolded(skillName) {
			invoker := loop.GetInvoker()
			viewSummary := mgr.GetSkillViewSummary(skillName)
			alreadyLoadedMsg := fmt.Sprintf(
				"IMPORTANT: Skill '%s' is ALREADY loaded and visible in your context. "+
					"Do NOT load it again. The skill content is already displayed in the "+
					"SKILLS_CONTEXT section of your prompt (look for '<|SKILLS_CONTEXT_' tags). "+
					"Read the View Window content that is already available to you. %s",
				skillName, viewSummary,
			)
			invoker.AddToTimeline("skill_already_loaded", alreadyLoadedMsg)
			return utils.Errorf("skill '%s' is already loaded and active in your context. "+
				"Check the SKILLS_CONTEXT section in your prompt - the content is already there. "+
				"Do NOT retry loading the same skill.", skillName)
		}

		loop.Set("loading_skill_name", skillName)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		skillName := loop.Get("loading_skill_name")
		if skillName == "" {
			op.Fail("loading_skills action: skill_name is empty")
			return
		}

		mgr := loop.GetSkillsContextManager()
		if mgr == nil {
			op.Fail("skills context manager is not available")
			return
		}

		err := mgr.LoadSkill(skillName)
		if err != nil {
			log.Warnf("failed to load skill %q: %v", skillName, err)

			// Write error to timeline for persistent record (prevents AI from retrying blindly)
			invoker := loop.GetInvoker()
			errMsg := fmt.Sprintf("Failed to load skill '%s': %v", skillName, err)
			invoker.AddToTimeline("skill_load_failed", errMsg)

			// Try to resolve the identifier to suggest the correct action
			resolved := loop.ResolveIdentifier(skillName)
			if !resolved.IsUnknown() {
				// The identifier exists as a different type (tool/forge) - provide clear guidance
				invoker.AddToTimeline("identifier_resolved", resolved.Suggestion)
				op.Feedback(errMsg + "\n\n" + resolved.Suggestion)
			} else {
				// The identifier doesn't exist anywhere
				op.Feedback(errMsg + "\n\n" + resolved.Suggestion)
			}

			op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
			op.SetReflectionData("skill_load_error", err.Error())
			op.SetReflectionData("skill_name", skillName)
			op.SetReflectionData("resolved_type", string(resolved.IdentityType))
			op.Continue()
			return
		}

		invoker := loop.GetInvoker()

		// Build detailed timeline message with view window information
		viewSummary := mgr.GetSkillViewSummary(skillName)
		timelineMsg := fmt.Sprintf(
			"Successfully loaded skill '%s' into context. "+
				"The skill content is now visible in the SKILLS_CONTEXT section of your prompt "+
				"(look for '<|SKILLS_CONTEXT_' tags). %s "+
				"IMPORTANT: Do NOT load this skill again - it is already active. "+
				"Read the View Window content in your prompt and proceed with your task.",
			skillName, viewSummary,
		)
		invoker.AddToTimeline("skill_loaded", timelineMsg)

		log.Infof("skill %q loaded into context successfully", skillName)
		feedbackMsg := fmt.Sprintf(
			"Skill '%s' has been loaded into the context. "+
				"The SKILL.md content and file tree are now displayed in the SKILLS_CONTEXT section of your prompt. "+
				"Read the skill content from your prompt's View Window and proceed with the task. "+
				"Do NOT load this skill again.",
			skillName,
		)
		op.Feedback(feedbackMsg)
		op.Continue()
	},
}
