package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// DeepIntentResult holds the output from a deep intent recognition sub-loop.
type DeepIntentResult struct {
	IntentAnalysis    string
	RecommendedTools  string
	RecommendedForges string
	ContextEnrichment string

	MatchedToolNames  string // comma-separated, e.g. "tool1,tool2"
	MatchedForgeNames string // comma-separated, e.g. "forge1,forge2"
	MatchedSkillNames string // comma-separated, e.g. "skill1,skill2"
}

// ExecuteDeepIntentRecognition invokes the loop_intent sub-loop for deep
// intent analysis. It creates a sub-task, runs the intent loop, and extracts
// structured results. Returns nil on any failure (non-fatal).
func ExecuteDeepIntentRecognition(r aicommon.AIInvokeRuntime, loop *ReActLoop, task aicommon.AIStatefulTask) *DeepIntentResult {
	userInput := task.GetUserInput()

	intentTask := aicommon.NewStatefulTaskBase(
		task.GetId()+"_intent",
		userInput,
		r.GetConfig().GetContext(),
		r.GetConfig().GetEmitter(),
	)

	originOptions := r.GetConfig().OriginOptions()
	var opts []any
	for _, option := range originOptions {
		opts = append(opts, option)
	}

	var intentLoop *ReActLoop
	opts = append(opts, WithOnLoopInstanceCreated(func(l *ReActLoop) {
		intentLoop = l
	}))

	ok, err := r.ExecuteLoopTaskIF(schema.AI_REACT_LOOP_NAME_INTENT, intentTask, opts...)
	if err != nil {
		log.Warnf("deep intent recognition failed: %v", err)
		return nil
	}
	if !ok {
		log.Warnf("deep intent recognition returned not ok")
		return nil
	}
	if intentLoop == nil {
		log.Warnf("deep intent recognition: intent loop reference is nil")
		return nil
	}

	result := &DeepIntentResult{
		IntentAnalysis:    intentLoop.Get("intent_analysis"),
		RecommendedTools:  intentLoop.Get("recommended_tools"),
		RecommendedForges: intentLoop.Get("recommended_forges"),
		ContextEnrichment: intentLoop.Get("context_enrichment"),
		MatchedToolNames:  intentLoop.Get("matched_tool_names"),
		MatchedForgeNames: intentLoop.Get("matched_forge_names"),
		MatchedSkillNames: intentLoop.Get("matched_skill_names"),
	}

	log.Infof("deep intent recognition completed: analysis=%d bytes, tools=%d bytes, forges=%d bytes, enrichment=%d bytes",
		len(result.IntentAnalysis), len(result.RecommendedTools),
		len(result.RecommendedForges), len(result.ContextEnrichment))

	return result
}

// ApplyDeepIntentResult injects deep intent recognition results into the loop
// context and populates ExtraCapabilitiesManager with resolved tools, forges,
// skills, and focus modes.
func ApplyDeepIntentResult(r aicommon.AIInvokeRuntime, loop *ReActLoop, result *DeepIntentResult) {
	if result == nil {
		return
	}

	loop.Set("intent_hint", "deep_analysis")
	loop.Set("intent_scale", "medium_or_large")

	if result.IntentAnalysis != "" {
		loop.Set("intent_analysis", result.IntentAnalysis)
		r.AddToTimeline("intent_analysis", result.IntentAnalysis)
	}
	if result.RecommendedTools != "" {
		loop.Set("intent_recommended_tools", result.RecommendedTools)
		r.AddToTimeline("intent_recommended_tools", result.RecommendedTools)
	}
	if result.RecommendedForges != "" {
		loop.Set("intent_recommended_forges", result.RecommendedForges)
		r.AddToTimeline("intent_recommended_forges", result.RecommendedForges)
	}
	if result.ContextEnrichment != "" {
		loop.Set("intent_context_enrichment", result.ContextEnrichment)
		r.AddToTimeline("intent_context_enrichment", result.ContextEnrichment)
	}

	PopulateExtraCapabilitiesFromDeepIntent(r, loop, result)

	log.Infof("deep intent results applied to loop context")
}

// PopulateExtraCapabilitiesFromDeepIntent resolves matched names to actual
// objects and adds them to the loop's ExtraCapabilitiesManager.
func PopulateExtraCapabilitiesFromDeepIntent(r aicommon.AIInvokeRuntime, loop *ReActLoop, result *DeepIntentResult) {
	ecm := loop.GetExtraCapabilities()
	if ecm == nil {
		return
	}

	cfg := r.GetConfig()

	if result.MatchedToolNames != "" {
		toolNames := splitAndTrimNames(result.MatchedToolNames)
		toolMgr := cfg.GetAiToolManager()
		if toolMgr != nil {
			for _, name := range toolNames {
				tool, err := toolMgr.GetToolByName(name)
				if err != nil {
					log.Debugf("extra capabilities: skip tool %q: %v", name, err)
					continue
				}
				ecm.AddTools(tool)
			}
		}
	}

	if result.MatchedForgeNames != "" {
		forgeNames := splitAndTrimNames(result.MatchedForgeNames)
		type forgeManagerProvider interface {
			GetAIForgeManager() aicommon.AIForgeFactory
		}
		if provider, ok := cfg.(forgeManagerProvider); ok {
			forgeMgr := provider.GetAIForgeManager()
			if forgeMgr != nil {
				for _, name := range forgeNames {
					forge, err := forgeMgr.GetAIForge(name)
					if err != nil {
						log.Debugf("extra capabilities: skip forge %q: %v", name, err)
						continue
					}
					ecm.AddForges(ExtraForgeInfo{
						Name:        forge.ForgeName,
						VerboseName: forge.ForgeVerboseName,
						Description: forge.Description,
					})
				}
			}
		}
	}

	if result.MatchedSkillNames != "" {
		skillNames := splitAndTrimNames(result.MatchedSkillNames)
		type skillLoaderProvider interface {
			GetSkillLoader() aiskillloader.SkillLoader
		}
		if provider, ok := cfg.(skillLoaderProvider); ok {
			skillLoader := provider.GetSkillLoader()
			if skillLoader != nil && skillLoader.HasSkills() {
				allMetas := skillLoader.AllSkillMetas()
				nameSet := make(map[string]bool, len(skillNames))
				for _, n := range skillNames {
					nameSet[strings.ToLower(n)] = true
				}
				for _, meta := range allMetas {
					if nameSet[strings.ToLower(meta.Name)] {
						ecm.AddSkills(ExtraSkillInfo{
							Name:        meta.Name,
							Description: meta.Description,
						})
					}
				}
			}
		}
	}

	if ecm.HasCapabilities() {
		log.Infof("extra capabilities populated from deep intent: %d tools, %d forges, %d skills",
			ecm.ToolCount(), len(ecm.ListForges()), len(ecm.ListSkills()))
	}
}

func splitAndTrimNames(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
