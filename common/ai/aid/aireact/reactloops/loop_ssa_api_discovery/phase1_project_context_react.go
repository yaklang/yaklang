package loop_ssa_api_discovery

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

//go:embed prompts/phase1_project_context_playbook.txt
var phase1ProjectContextPlaybook string

func runPhase1ProjectContextReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil
	}
	_ = ctx
	extra := embeddedArtifactsForAgent(rt,
		store.ProjectProfilePath(rt.WorkDir),
		store.JavaBusinessScopeInventoryPath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
		store.ComponentPackageMapPath(rt.WorkDir),
		store.RoutingProfilePath(rt.WorkDir),
	)
	loop, err := buildPhase1ProjectContextLoop(r, rt, extra)
	if err != nil {
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_project_context", loop); err != nil {
		log.Warnf("ssa_api_discovery: project context react: %v; programmatic fallback", err)
		return bootstrapProjectContextSummary(rt)
	}
	raw := strings.TrimSpace(loop.Get("project_context_summary_committed"))
	if raw == "" {
		return bootstrapProjectContextSummary(rt)
	}
	var summary ProjectContextSummaryV1
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return err
	}
	return persistProjectContextSummary(rt, &summary)
}

func buildPhase1ProjectContextLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1ProjectContextPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset, buildFinalizeProjectContextSummary(rt), buildBlockedDirectlyAnswer("finalize_project_context_summary"))
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_PROJECT_CONTEXT, r, preset...)
}

func buildFinalizeProjectContextSummary(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_project_context_summary",
		"Commit project_context_summary.json and exit.",
		[]aitool.ToolOption{
			aitool.WithStringParam("summary_json", aitool.WithParam_Required(true)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("summary_json"))
			var summary ProjectContextSummaryV1
			if err := parseAgentJSONObject(raw, &summary); err != nil {
				op.Feedback("invalid summary_json: " + err.Error())
				op.Continue()
				return
			}
			if strings.TrimSpace(summary.Summary) == "" {
				op.Feedback("summary (Chinese project description) is required")
				op.Continue()
				return
			}
			if err := persistProjectContextSummary(rt, &summary); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(summary, "", "  ")
			loop.Set("project_context_summary_committed", string(b))
			op.Feedback("project_context_summary persisted")
			op.Exit()
		},
	)
}

func bootstrapProjectContextSummary(rt *Runtime) error {
	inv, _ := loadJavaBusinessScopeInventory(rt.WorkDir)
	summary := &ProjectContextSummaryV1{
		SchemaVersion:   artifactV2SchemaVersion,
		ProjectType:     "java_multi_module",
		Summary:         "Java 多模块项目（bootstrap：由 java_business_scope 推断）",
		PrimaryLanguage: strings.TrimSpace(rt.Session.Language),
		FirstPartyBoundary: ProjectCodeBoundary{
			Description: "含 src/main/java 的业务模块与模板资源",
			PathPatterns: []string{
				"**/src/main/java/**",
				"**/src/main/resources/**",
			},
			PackageRoots: []string{"com.publiccms"},
		},
		ThirdPartyBoundary: ProjectCodeBoundary{
			Description: "webapp 插件、语言包、构建工具目录",
			PathPatterns: []string{
				"**/.mvn/**",
				"**/gradle/wrapper/**",
				"**/webapp/resource/plugins/**",
				"**/locale/**",
				"**/target/**",
				"**/build/**",
				"**/node_modules/**",
			},
			PackagePrefixes: []string{
				"org.apache.", "com.google.", "org.springframework.",
			},
		},
		EvidenceRefs: []string{"bootstrap:java_business_scope_inventory"},
	}
	if inv != nil {
		summary.PrimaryLanguage = inv.Language
		if inv.Layout != "" {
			summary.ProjectType = inv.Layout
		}
		seenModules := map[string]struct{}{}
		for _, mod := range inv.Modules {
			root := strings.TrimSpace(mod.ModuleRoot)
			if root == "" {
				continue
			}
			if root == ".mvn" || root == "gradle" {
				continue
			}
			if _, ok := seenModules[root]; ok {
				continue
			}
			seenModules[root] = struct{}{}
			summary.FirstPartyBoundary.ModuleRoots = append(summary.FirstPartyBoundary.ModuleRoots, root)
			summary.BusinessModules = append(summary.BusinessModules, ProjectBusinessModule{
				ModuleRoot:  root,
				Role:        "业务模块 " + root,
				JavaPathHint: root + "/src/main/java",
			})
		}
	}
	if len(summary.FirstPartyBoundary.ModuleRoots) == 0 {
		summary.FirstPartyBoundary.ModuleRoots = []string{"."}
	}
	return persistProjectContextSummary(rt, summary)
}
