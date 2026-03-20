package loopinfra

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type yakExecutableProvider interface {
	GetYakExecutablePath() string
}

var loopAction_LoadSkillResources = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES,
	Description: `Load a specific file or search content across skill files. Two modes (mutually exclusive): ` +
		`(1) resource_path: load a file using "@skill_name/path/to/file.md". Fuzzy matching is applied if exact path not found. ` +
		`Set resource_type to "document" (default) for content or "script" for executable file path resolution. ` +
		`(2) pattern: grep/search across all files in skills using a regex or string pattern. ` +
		`Returns matching lines with surrounding context. Use optional skill_name to limit grep scope.`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"resource_path",
			aitool.WithParam_Description(
				`The resource path to load, in format "@skill_name/path/to/file". `+
					`Example: "@recon/osint.md" loads osint.md from the recon skill. `+
					`Mutually exclusive with "pattern" — provide one or the other.`),
		),
		aitool.WithStringParam(
			"resource_type",
			aitool.WithParam_Description(
				`Type of resource being loaded (only used with resource_path). `+
					`"document" (default): loads file content into context. `+
					`"script": resolves absolute file path for shell commands.`),
		),
		aitool.WithStringParam(
			"pattern",
			aitool.WithParam_Description(
				`A regex or string pattern to search for across skill files. `+
					`Returns matching lines with context (like grep -C 3). `+
					`Mutually exclusive with "resource_path" — provide one or the other.`),
		),
		aitool.WithStringParam(
			"skill_name",
			aitool.WithParam_Description(
				`Optional. Limit grep scope to a specific skill. `+
					`If omitted, all available skills are searched. Only used with "pattern".`),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "resource_path", AINodeId: "load_skill_resources_path"},
		{FieldName: "resource_type", AINodeId: "load_skill_resources_type"},
		{FieldName: "pattern", AINodeId: "load_skill_resources_pattern"},
		{FieldName: "skill_name", AINodeId: "load_skill_resources_skill_name"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		loop.Set("_load_resource_skip", "")
		loop.Set("_load_resource_skip_message", "")
		loop.Set("_load_resource_mode", "")
		loop.Set("_load_resource_skill", "")
		loop.Set("_load_resource_path", "")
		loop.Set("_load_resource_raw", "")
		loop.Set("_load_resource_type", "")
		loop.Set("_grep_pattern", "")
		loop.Set("_grep_skill_name", "")

		setValidationSkip := func(msg string) error {
			loop.Set("_load_resource_skip", "validation_failed")
			loop.Set("_load_resource_skip_message", msg)
			return nil
		}

		mgr := loop.GetSkillsContextManager()
		if mgr == nil {
			return utils.Error("skills context manager is not available")
		}

		resourcePath := action.GetString("resource_path")
		pattern := action.GetString("pattern")

		if resourcePath == "" && pattern == "" {
			return setValidationSkip(
				"load_skill_resources requires either 'resource_path' or 'pattern'. " +
					"Example: use resource_path='@skill-name/file.md' to load a file, " +
					"or use pattern='keyword' to search across skill files.",
			)
		}
		if resourcePath != "" && pattern != "" {
			return setValidationSkip(
				"'resource_path' and 'pattern' are mutually exclusive. " +
					"Provide only one of them in a single load_skill_resources action.",
			)
		}

		if pattern != "" {
			if _, err := regexp.Compile(pattern); err != nil {
				return setValidationSkip(
					fmt.Sprintf("invalid grep pattern %q: %v. Provide a valid regex or plain string pattern.", pattern, err),
				)
			}
			skillName := action.GetString("skill_name")
			loop.Set("_load_resource_mode", "grep")
			loop.Set("_grep_pattern", pattern)
			loop.Set("_grep_skill_name", skillName)
			return nil
		}

		skillName, filePath, err := aiskillloader.ParseSkillResourcePath(resourcePath)
		if err != nil {
			return setValidationSkip(
				fmt.Sprintf("invalid resource_path: %v. Use the format '@skill-name/path/to/file'.", err),
			)
		}
		if filePath == "" {
			return setValidationSkip(
				"resource_path must include a file path after the skill name. " +
					"Example: @skill/file.md or @skill/scripts/run.yak",
			)
		}

		resourceType := action.GetString("resource_type")
		if resourceType == "" {
			resourceType = "document"
		}
		if resourceType != "document" && resourceType != "script" {
			return setValidationSkip(
				fmt.Sprintf("resource_type must be 'document' or 'script', got %q.", resourceType),
			)
		}

		loop.Set("_load_resource_mode", "load")
		loop.Set("_load_resource_skill", skillName)
		loop.Set("_load_resource_path", filePath)
		loop.Set("_load_resource_raw", resourcePath)
		loop.Set("_load_resource_type", resourceType)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		if loop.Get("_load_resource_skip") == "validation_failed" {
			msg := loop.Get("_load_resource_skip_message")
			if msg == "" {
				msg = "load_skill_resources parameters are invalid. Provide either resource_path or pattern."
			}
			loop.Set("_load_resource_skip", "")
			loop.Set("_load_resource_skip_message", "")
			loop.GetInvoker().AddToTimeline("skill_resource_validation_failed", msg)
			op.Feedback(msg)
			op.Continue()
			return
		}

		mgr := loop.GetSkillsContextManager()
		if mgr == nil {
			op.Fail("skills context manager is not available")
			return
		}

		invoker := loop.GetInvoker()
		mode := loop.Get("_load_resource_mode")

		if mode == "grep" {
			handleGrepResource(mgr, invoker, loop, op)
			return
		}

		skillName := loop.Get("_load_resource_skill")
		filePath := loop.Get("_load_resource_path")
		rawPath := loop.Get("_load_resource_raw")
		resourceType := loop.Get("_load_resource_type")

		if skillName == "" || filePath == "" {
			op.Fail("load_skill_resources: missing skill name or file path")
			return
		}

		if resourceType == "script" {
			handleScriptResource(loop, mgr, invoker, skillName, filePath, rawPath, op)
		} else {
			handleDocumentResource(mgr, invoker, rawPath, skillName, filePath, op)
		}
	},
}

func handleDocumentResource(
	mgr *aiskillloader.SkillsContextManager,
	invoker aicommon.AIInvokeRuntime,
	rawPath, skillName, filePath string,
	op *reactloops.LoopActionHandlerOperator,
) {
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
}

func handleScriptResource(
	loop *reactloops.ReActLoop,
	mgr *aiskillloader.SkillsContextManager,
	invoker aicommon.AIInvokeRuntime,
	skillName, filePath, rawPath string,
	op *reactloops.LoopActionHandlerOperator,
) {
	result, err := mgr.LoadSkillResourceAsScript(
		skillName, filePath, invoker.EmitFileArtifactWithExt,
	)
	if err != nil {
		log.Warnf("failed to load script resource %q: %v", rawPath, err)
		errMsg := fmt.Sprintf("Failed to load script resource '%s': %v", rawPath, err)
		invoker.AddToTimeline("skill_script_resource_load_failed", errMsg)
		op.Feedback(errMsg)
		op.Continue()
		return
	}

	summary := aiskillloader.FormatResourceLoadSummary(result)
	log.Infof("loaded script resource: %s", summary)
	scriptExt := strings.ToLower(filepath.Ext(result.FilePath))
	commandHint := buildScriptCommandHint(invoker, result.AbsolutePath, scriptExt)

	timelineMsg := fmt.Sprintf(
		"Script resource '%s' resolved to absolute path '%s'. %s.",
		rawPath, result.AbsolutePath, summary,
	)
	if result.MaterializedToArtifacts {
		timelineMsg += " Script materialized from virtual filesystem to artifacts directory."
	}
	if commandHint != "" {
		timelineMsg += " " + commandHint
	} else {
		timelineMsg += " Use this path directly in shell commands."
	}
	invoker.AddToTimeline("skill_script_resource_loaded", timelineMsg)
	invoker.AddToTimeline("use_script", buildScriptUsageGuidance(invoker, result.AbsolutePath, scriptExt))

	feedbackMsg := fmt.Sprintf(
		"Script resource '%s' loaded successfully. %s. "+
			"Absolute path: %s — use this path directly in shell commands.",
		rawPath, summary, result.AbsolutePath,
	)
	if result.FuzzyMatched {
		feedbackMsg += fmt.Sprintf(
			" Note: exact path not found, fuzzy matched to '%s'.",
			result.MatchedPath,
		)
	}
	if result.MaterializedToArtifacts {
		feedbackMsg += " The script was materialized from a virtual filesystem to the artifacts directory."
	}
	if commandHint != "" {
		feedbackMsg += " " + commandHint
	}
	feedbackMsg += " The path reference is now visible in the SKILLS_CONTEXT section."
	op.Feedback(feedbackMsg)
	op.Continue()
}

func getYakExecutablePath(invoker aicommon.AIInvokeRuntime) string {
	if provider, ok := invoker.(yakExecutableProvider); ok {
		return strings.TrimSpace(provider.GetYakExecutablePath())
	}
	return ""
}

func buildScriptCommandHint(invoker aicommon.AIInvokeRuntime, absPath string, scriptExt string) string {
	switch scriptExt {
	case ".yak":
		yakPath := getYakExecutablePath(invoker)
		if yakPath != "" {
			return fmt.Sprintf("Recommended command: %s %s", yakPath, absPath)
		}
		return fmt.Sprintf("Recommended command: yak %s", absPath)
	case ".py", ".python":
		return fmt.Sprintf("Recommended command: python %s", absPath)
	default:
		return ""
	}
}

func buildScriptUsageGuidance(invoker aicommon.AIInvokeRuntime, absPath string, scriptExt string) string {
	switch scriptExt {
	case ".yak":
		yakPath := getYakExecutablePath(invoker)
		if yakPath != "" {
			return "Use the Yak executable absolute path from the RUNTIME_ENVIRONMENT section to execute the script directly in shell commands.\n" +
				fmt.Sprintf("Recommended command: %s %s", yakPath, absPath)
		}
		return "Use the yak command to execute the script directly in shell commands.\n" +
			fmt.Sprintf("Recommended command: yak %s", absPath)
	case ".py", ".python":
		return "Use the python interpreter to execute the script directly in shell commands.\n" +
			fmt.Sprintf("Recommended command: python %s", absPath)
	default:
		return "Use the absolute path of the script to execute it directly in shell commands."
	}
}

func handleGrepResource(
	mgr *aiskillloader.SkillsContextManager,
	invoker aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	op *reactloops.LoopActionHandlerOperator,
) {
	pattern := loop.Get("_grep_pattern")
	skillName := loop.Get("_grep_skill_name")

	result, err := mgr.GrepSkillResources(pattern, skillName)
	if err != nil {
		log.Warnf("grep skill resources failed: %v", err)
		errMsg := fmt.Sprintf("Grep failed for pattern %q: %v", pattern, err)
		invoker.AddToTimeline("skill_grep_failed", errMsg)
		op.Feedback(errMsg)
		op.Continue()
		return
	}

	summary := aiskillloader.FormatGrepSummary(result)
	log.Infof("skill grep completed: %s", summary)

	invoker.AddToTimeline("skill_grep_completed", summary)

	if result.TotalMatches == 0 {
		feedbackMsg := fmt.Sprintf(
			"No matches found for pattern %q. %s. "+
				"Try a different pattern or check available skill files.",
			pattern, summary,
		)
		op.Feedback(feedbackMsg)
		op.Continue()
		return
	}

	feedbackMsg := fmt.Sprintf(
		"Grep completed. %s. "+
			"Results are now visible in the SKILLS_CONTEXT section as a View Window.",
		summary,
	)
	if result.IsTruncated {
		feedbackMsg += " Results were truncated — refine your pattern or specify a skill_name for more targeted search."
	}
	op.Feedback(feedbackMsg)
	op.Continue()
}
