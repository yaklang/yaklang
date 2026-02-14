package loopinfra

import (
	"fmt"
	"strconv"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// skillLoadedNamesFromMgr extracts loaded skill names from the SkillsContextManager.
func skillLoadedNamesFromMgr(loop *reactloops.ReActLoop) []string {
	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		return nil
	}
	selected := mgr.GetCurrentSelectedSkills()
	names := make([]string, 0, len(selected))
	for _, s := range selected {
		names = append(names, s.Name)
	}
	return names
}

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

		invoker := loop.GetInvoker()

		// Check if the skill has been blocked due to repeated failures.
		// Do NOT return error here — that would trigger CallAITransaction retry.
		blockedKey := fmt.Sprintf("_skill_blocked_%s", skillName)
		if loop.Get(blockedKey) == "true" {
			loadedNames := skillLoadedNamesFromMgr(loop)
			invoker.AddToTimeline("skill_blocked",
				fmt.Sprintf("Skill '%s' is blocked due to repeated loading failures. "+
					"Use already loaded skills to proceed: %v. "+
					"Do NOT attempt to load '%s' again.",
					skillName, loadedNames, skillName))
			loop.Set("_skill_load_skip", "blocked")
			loop.Set("loading_skill_name", skillName)
			return nil // no error — avoid CallAITransaction retry
		}

		// Check if the skill is already loaded and unfolded — no need to load again.
		// Do NOT return error — set a flag for ActionHandler to silently skip.
		if mgr.IsSkillLoadedAndUnfolded(skillName) {
			viewSummary := mgr.GetSkillViewSummary(skillName)
			alreadyLoadedMsg := fmt.Sprintf(
				"IMPORTANT: Skill '%s' is ALREADY loaded and visible in your context. "+
					"Do NOT load it again. The skill content is already displayed in the "+
					"SKILLS_CONTEXT section of your prompt (look for '<|SKILLS_CONTEXT_' tags). "+
					"Read the View Window content that is already available to you. %s",
				skillName, viewSummary,
			)
			invoker.AddToTimeline("skill_already_loaded", alreadyLoadedMsg)
			loop.Set("_skill_load_skip", "already_loaded")
			loop.Set("loading_skill_name", skillName)
			return nil // no error — avoid CallAITransaction retry
		}

		loop.Set("_skill_load_skip", "")
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

		invoker := loop.GetInvoker()

		// Check skip flag set by ActionVerifier — handle silently without error
		skipReason := loop.Get("_skill_load_skip")
		loop.Set("_skill_load_skip", "") // clear flag immediately

		if skipReason == "already_loaded" {
			viewSummary := mgr.GetSkillViewSummary(skillName)
			op.Feedback(fmt.Sprintf(
				"Skill '%s' is already loaded and active in SKILLS_CONTEXT. "+
					"Do NOT load it again. %s Proceed with your task using the loaded content.",
				skillName, viewSummary))
			op.Continue()
			return
		}
		if skipReason == "blocked" {
			loadedNames := skillLoadedNamesFromMgr(loop)
			op.Feedback(fmt.Sprintf(
				"Skill '%s' has been blocked due to repeated loading failures. "+
					"Proceed with already loaded skills: %v. Do NOT retry loading '%s'.",
				skillName, loadedNames, skillName))
			op.Continue()
			return
		}

		// Attempt to load the skill
		err := mgr.LoadSkill(skillName)
		if err != nil {
			log.Warnf("failed to load skill %q: %v", skillName, err)

			errMsg := fmt.Sprintf("Failed to load skill '%s': %v", skillName, err)
			invoker.AddToTimeline("skill_load_failed", errMsg)

			// Track failure count via loop.Get/Set
			failCountKey := fmt.Sprintf("_skill_fail_%s", skillName)
			prevCount := 0
			if v := loop.Get(failCountKey); v != "" {
				prevCount, _ = strconv.Atoi(v)
			}
			failCount := prevCount + 1
			loop.Set(failCountKey, strconv.Itoa(failCount))

			// Record load history for oscillation detection
			historyKey := "_skill_load_history"
			history := loop.Get(historyKey)
			if history != "" {
				history = history + "," + skillName
			} else {
				history = skillName
			}
			loop.Set(historyKey, history)

			// Check for oscillation: if failCount >= 2, use LiteForge to arbitrate
			if failCount >= 2 {
				log.Warnf("skill '%s' failed %d times, triggering LiteForge conflict resolution", skillName, failCount)

				loadedNames := skillLoadedNamesFromMgr(loop)
				resolved := loop.ResolveIdentifier(skillName)
				loadHistory := loop.Get(historyKey)

				// Get user task context for LiteForge prompt
				taskUserInput := ""
				if task := loop.GetCurrentTask(); task != nil {
					taskUserInput = task.GetUserInput()
				}

				conflictPrompt := fmt.Sprintf(
					"Skill loading conflict detected.\n"+
						"Failed skill: '%s' (attempted %d times, error: %v)\n"+
						"Resolved as: %s (type: %s)\n"+
						"Already loaded skills: %v\n"+
						"Recent load attempts: %s\n"+
						"User task context: %s\n\n"+
						"You must decide what to do next. Options:\n"+
						"- 'proceed_with_loaded': Continue using the already loaded skills\n"+
						"- 'skip': Skip this skill entirely and proceed with the task\n"+
						"- 'use_alternative': Try a different approach for this task\n",
					skillName, failCount, err,
					resolved.Suggestion, string(resolved.IdentityType),
					loadedNames, loadHistory, taskUserInput,
				)

				ctx := op.GetContext()
				decision, liteForgeErr := invoker.InvokeLiteForgeSpeedPriority(ctx, "skill-conflict-resolver",
					conflictPrompt, []aitool.ToolOption{
						aitool.WithStringParam("action",
							aitool.WithParam_Description("One of: proceed_with_loaded, skip, use_alternative"),
							aitool.WithParam_Required(true)),
						aitool.WithStringParam("reason",
							aitool.WithParam_Description("Brief reason for the decision")),
					})

				// Record decision to timeline
				decisionAction := "proceed_with_loaded"
				decisionReason := "LiteForge arbitration completed"
				if liteForgeErr != nil {
					log.Warnf("LiteForge skill-conflict-resolver failed: %v, defaulting to proceed_with_loaded", liteForgeErr)
					decisionReason = fmt.Sprintf("LiteForge failed (%v), defaulting to proceed_with_loaded", liteForgeErr)
				} else if decision != nil {
					if a := decision.GetString("action"); a != "" {
						decisionAction = a
					}
					if r := decision.GetString("reason"); r != "" {
						decisionReason = r
					}
				}

				invoker.AddToTimeline("skill_conflict_resolved",
					fmt.Sprintf("Skill conflict resolved by LiteForge. "+
						"Decision: %s. Reason: %s. "+
						"Blocked further attempts to load '%s'. "+
						"Already loaded skills: %v.",
						decisionAction, decisionReason, skillName, loadedNames))

				// Block the skill from future loading attempts
				blockedKey := fmt.Sprintf("_skill_blocked_%s", skillName)
				loop.Set(blockedKey, "true")

				op.Feedback(fmt.Sprintf(
					"Skill '%s' loading failed %d times and has been blocked. "+
						"LiteForge decision: %s (reason: %s). "+
						"Proceed with already loaded skills: %v. "+
						"Do NOT attempt to load '%s' again.",
					skillName, failCount, decisionAction, decisionReason, loadedNames, skillName))
				op.Continue()
				return
			}

			// First failure: provide resolve identifier guidance
			resolved := loop.ResolveIdentifier(skillName)
			if !resolved.IsUnknown() {
				invoker.AddToTimeline("identifier_resolved", resolved.Suggestion)
				op.Feedback(errMsg + "\n\n" + resolved.Suggestion)
			} else {
				op.Feedback(errMsg + "\n\n" + resolved.Suggestion)
			}

			op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
			op.SetReflectionData("skill_load_error", err.Error())
			op.SetReflectionData("skill_name", skillName)
			op.SetReflectionData("resolved_type", string(resolved.IdentityType))
			op.Continue()
			return
		}

		// Load succeeded
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
