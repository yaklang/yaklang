package loopinfra

import (
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
			op.Feedback("Failed to load skill '" + skillName + "': " + err.Error())
			op.Continue()
			return
		}

		invoker := loop.GetInvoker()
		invoker.AddToTimeline("skill_loaded", "Loaded skill: "+skillName)

		log.Infof("skill %q loaded into context successfully", skillName)
		op.Feedback("Skill '" + skillName + "' has been loaded into the context. You can now reference its content.")
		op.Continue()
	},
}
