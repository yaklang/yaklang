package loop_ssa_api_discovery

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_tech_arch_playbook.txt
var phase1TechArchPlaybook string

const techArchitectureSchemaVersion = 1

type TechBusinessSurface struct {
	ID       string `json:"id"`
	Evidence string `json:"evidence,omitempty"`
}

type TechModuleLayoutEntry struct {
	Name     string `json:"name"`
	Role     string `json:"role,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

type TechModuleLayout struct {
	Style   string                  `json:"style,omitempty"`
	Modules []TechModuleLayoutEntry `json:"modules,omitempty"`
}

type TechFrontendBackend struct {
	HasSeparateFrontend bool     `json:"has_separate_frontend,omitempty"`
	FrontendRoots       []string `json:"frontend_roots,omitempty"`
	BackendRoots        []string `json:"backend_roots,omitempty"`
	Interaction         string   `json:"interaction,omitempty"`
}

type TechNamedEvidence struct {
	Name     string `json:"name,omitempty"`
	Engine   string `json:"engine,omitempty"`
	Role     string `json:"role,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

type TechFrameworkEntry struct {
	ID         string `json:"id"`
	Label      string `json:"label,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
}

// TechArchitectureRecord is persisted to tech_architecture.json.
type TechArchitectureRecord struct {
	SchemaVersion        int                   `json:"schema_version"`
	GeneratedAt          string                `json:"generated_at"`
	Language             string                `json:"language"`
	LanguagesDetected    []string              `json:"languages_detected,omitempty"`
	Frameworks           []TechFrameworkEntry  `json:"frameworks,omitempty"`
	SystemSummary        string                `json:"system_summary"`
	DeploymentModel      string                `json:"deployment_model,omitempty"`
	FrontendBackend      TechFrontendBackend   `json:"frontend_backend"`
	BusinessSurfaces     []TechBusinessSurface `json:"business_surfaces,omitempty"`
	ModuleLayout         TechModuleLayout      `json:"module_layout,omitempty"`
	Middleware           []TechNamedEvidence   `json:"middleware,omitempty"`
	Databases            []TechNamedEvidence   `json:"databases,omitempty"`
	Caches               []TechNamedEvidence   `json:"caches,omitempty"`
	ExternalIntegrations []TechNamedEvidence   `json:"external_integrations,omitempty"`
	EvidenceRefs         []string              `json:"evidence_refs,omitempty"`
}

func runPhase1TechArchReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	if !rt.Session.CodePathOK {
		log.Infof("ssa_api_discovery: phase1 tech arch skipped (code_path not ok)")
		return nil
	}
	_ = ctx
	loop, err := buildPhase1TechArchLoop(r, rt)
	if err != nil {
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_tech_arch", loop); err != nil {
		log.Warnf("ssa_api_discovery: phase1 tech arch react: %v; programmatic fallback", err)
		return runPhase1TechArchProgrammaticFallback(rt)
	}
	raw := strings.TrimSpace(loop.Get("phase1_tech_arch_committed"))
	if raw == "" {
		return runPhase1TechArchProgrammaticFallback(rt)
	}
	var rec TechArchitectureRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return err
	}
	return persistTechArchitectureRecord(rt, &rec)
}

func buildPhase1TechArchLoop(r aicommon.AIInvokeRuntime, rt *Runtime) (*reactloops.ReActLoop, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowRAG(true),
		reactloops.WithAllowAIForge(false),
		reactloops.WithPersistentInstruction(
			strings.TrimSpace(phase1TechArchPlaybook) + "\n\n" +
				strings.TrimSpace(ssaDiscoveryFSBuiltinToolParamsHint),
		),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			inv, _ := loadJavaBusinessScopeInventory(rt.WorkDir)
			profile, _ := loadProjectProfile(rt.WorkDir)
			scopeLine := summarizeJavaScopeInventory(inv)
			profileLine := ""
			if profile != nil {
				profileLine = fmt.Sprintf("language=%s frameworks=%d files=%d",
					profile.Language, len(profile.Frameworks), len(profile.Files))
			}
			return fmt.Sprintf(`<|PHASE1_TECH_ARCH_%s|>
session: %s
%s
project_profile: %s
feedback:
%s
<|END_%s|>`, nonce, loop.Get("discovery_session_uuid"), scopeLine, profileLine, feedbacker.String(), nonce), nil
		}),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			setRuntime(loop, rt)
			loop.Set("discovery_session_uuid", rt.Session.UUID)
			loop.Set("discovery_sqlite_path", rt.SQLitePath)
			loop.Set("discovery_code_root", rt.Session.CodeRootPath)
			op.NextAction("discovery_get_status")
		}),
		buildDiscoveryGetStatus(),
		buildDiscoveryReadSessionData(),
		buildCodeReadingReadFileAudit(rt),
		buildFinalizePhase1TechArch(rt),
		buildPhase1TechArchDirectlyAnswerOverride(),
	}
	preset = append(preset, phase1SearchExtractActionOptions()...)
	preset = append(preset,
		buildUpsertComponent(),
		buildUpsertConfigArtifact(),
		buildDependencyBatch(),
		// discovery_search_files can be legitimately called many times when exploring
		// project structure; discovery_read_session_data may also trigger consecutive reads.
		// Increase thresholds to 20/20/5 (was 8/8/3) to avoid false spin on normal exploration.
		reactloops.WithSameActionTypeSpinThreshold(20),
		reactloops.WithSameLogicSpinThreshold(20),
		reactloops.WithMaxConsecutiveSpinWarnings(5),
	)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_PHASE1_TECH_ARCH, r, preset...)
}

func buildFinalizePhase1TechArch(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_phase1_tech_arch",
		"Commit tech_architecture JSON and exit tech arch loop.",
		[]aitool.ToolOption{
			aitool.WithStringParam("tech_arch_json", aitool.WithParam_Required(true)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("tech_arch_json"))
			var rec TechArchitectureRecord
			if err := json.Unmarshal([]byte(raw), &rec); err != nil {
				op.Feedback("invalid tech_arch_json: " + err.Error())
				op.Continue()
				return
			}
			if err := validateTechArchitectureRecord(&rec); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			if err := persistTechArchitectureRecord(rt, &rec); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(rec, "", "  ")
			loop.Set("phase1_tech_arch_committed", string(b))
			op.Feedback("tech_architecture written")
			op.Exit()
		},
	)
}

func buildPhase1TechArchDirectlyAnswerOverride() reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType:  "directly_answer",
		Description: "Blocked; use finalize_phase1_tech_arch.",
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			op.Feedback("use finalize_phase1_tech_arch to commit structured tech architecture")
			op.Continue()
		},
	})
}

func validateTechArchitectureRecord(rec *TechArchitectureRecord) error {
	if rec == nil {
		return utils.Error("nil tech architecture")
	}
	if strings.TrimSpace(rec.Language) == "" {
		return utils.Error("language required")
	}
	if strings.TrimSpace(rec.SystemSummary) == "" {
		return utils.Error("system_summary required")
	}
	return nil
}

func persistTechArchitectureRecord(rt *Runtime, rec *TechArchitectureRecord) error {
	if rt == nil || rec == nil {
		return utils.Error("nil record")
	}
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = techArchitectureSchemaVersion
	}
	if rec.GeneratedAt == "" {
		rec.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(store.TechArchitecturePath(rt.WorkDir), b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactTechArchitecture, string(b))
	}
	return nil
}

func runPhase1TechArchProgrammaticFallback(rt *Runtime) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	profile, _ := loadProjectProfile(rt.WorkDir)
	inv, _ := loadJavaBusinessScopeInventory(rt.WorkDir)
	rec := &TechArchitectureRecord{
		SchemaVersion:   techArchitectureSchemaVersion,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Language:        "java",
		SystemSummary:   "programmatic fallback from project_profile and java scope inventory",
		DeploymentModel: "single_module",
	}
	if profile != nil {
		rec.Language = profile.Language
		if rec.Language == "" {
			rec.Language = "java"
		}
		rec.SystemSummary = fmt.Sprintf("Java project with %d files; context_path=%s",
			len(profile.Files), profile.ContextPath)
		for _, f := range profile.Frameworks {
			rec.Frameworks = append(rec.Frameworks, TechFrameworkEntry{
				ID: f.ID, Label: f.Label, Confidence: f.Confidence, Evidence: f.Evidence,
			})
		}
	}
	if inv != nil {
		rec.DeploymentModel = inv.Layout
		for _, mod := range inv.Modules {
			rec.ModuleLayout.Modules = append(rec.ModuleLayout.Modules, TechModuleLayoutEntry{
				Name: mod.ModuleRoot, Role: "module", Evidence: mod.BuildFile,
			})
		}
		rec.ModuleLayout.Style = inv.Layout
	}
	scope, _ := loadBackendScope(rt.WorkDir)
	if scope != nil {
		rec.FrontendBackend.BackendRoots = scope.BackendRoots
		rec.FrontendBackend.FrontendRoots = scope.FrontendRoots
		rec.FrontendBackend.HasSeparateFrontend = len(scope.FrontendRoots) > 0
	}
	return persistTechArchitectureRecord(rt, rec)
}

func loadTechArchitectureRecord(workDir string) (*TechArchitectureRecord, error) {
	b, err := os.ReadFile(store.TechArchitecturePath(workDir))
	if err != nil {
		return nil, err
	}
	var rec TechArchitectureRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func summarizeTechArchitecture(rec *TechArchitectureRecord) string {
	if rec == nil {
		return "tech_arch: missing"
	}
	return fmt.Sprintf("tech_arch: %s %s modules=%d surfaces=%d",
		rec.Language, rec.DeploymentModel, len(rec.ModuleLayout.Modules), len(rec.BusinessSurfaces))
}
