package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_ChangeSkillViewOffset = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_CHANGE_SKILL_VIEW_OFFSET,
	Description: `Change the view offset for a skill file that has been truncated. Use this to scroll through large skill files (like SKILL.md) that exceed the view window limit. The offset is a 1-based line number.`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"skill_name",
			aitool.WithParam_Description(`The name of the skill whose file view you want to scroll.`),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"file_path",
			aitool.WithParam_Description(`The file path within the skill to scroll. Defaults to SKILL.md if not specified.`),
		),
		aitool.WithNumberParam(
			"offset",
			aitool.WithParam_Description(`The 1-based line number to start viewing from. Must be a positive integer.`),
			aitool.WithParam_Required(true),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "skill_name", AINodeId: "change_skill_view_offset_name"},
		{FieldName: "offset", AINodeId: "change_skill_view_offset_value"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		skillName := action.GetString("skill_name")
		if skillName == "" {
			return utils.Error("change_skill_view_offset action requires 'skill_name' parameter")
		}

		mgr := loop.GetSkillsContextManager()
		if mgr == nil {
			return utils.Error("skills context manager is not available")
		}

		offsetRaw := action.GetString("offset")
		if utils.InterfaceToInt(offsetRaw) < 1 {
			return utils.Error("change_skill_view_offset action requires 'offset' to be a positive integer")
		}

		loop.Set("skill_view_offset_name", skillName)
		loop.Set("skill_view_offset_file", action.GetString("file_path"))
		loop.Set("skill_view_offset_value", offsetRaw)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		skillName := loop.Get("skill_view_offset_name")
		filePath := loop.Get("skill_view_offset_file")
		offsetStr := loop.Get("skill_view_offset_value")
		offset := utils.InterfaceToInt(offsetStr)

		if skillName == "" {
			op.Fail("change_skill_view_offset action: skill_name is empty")
			return
		}

		mgr := loop.GetSkillsContextManager()
		if mgr == nil {
			op.Fail("skills context manager is not available")
			return
		}

		err := mgr.ChangeViewOffset(skillName, filePath, offset)
		if err != nil {
			log.Warnf("failed to change view offset for skill %q: %v", skillName, err)
			op.Feedback("Failed to change view offset: " + err.Error())
			op.Continue()
			return
		}

		displayPath := filePath
		if displayPath == "" {
			displayPath = "SKILL.md"
		}

		invoker := loop.GetInvoker()
		invoker.AddToTimeline("skill_view_offset_changed",
			"Changed view offset for "+skillName+"/"+displayPath+" to line "+offsetStr)

		log.Infof("changed view offset for skill %q file %q to %d", skillName, displayPath, offset)
		op.Feedback("View offset for '" + skillName + "/" + displayPath + "' changed to line " + offsetStr + ". Updated content is now visible in the skills context.")
		op.Continue()
	},
}
