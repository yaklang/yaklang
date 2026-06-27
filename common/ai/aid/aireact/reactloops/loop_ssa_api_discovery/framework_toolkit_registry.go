package loop_ssa_api_discovery

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// FrameworkToolkit is a pre-built programmatic pipeline for a known CMS/framework product.
type FrameworkToolkit interface {
	ID() string
	Label() string
	Detect(rt *Runtime) (score float64, evidence []string)
	AcquireCredentials(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime) error
	ExtractAPIs(rt *Runtime) (*CombinedAPICatalog, error)
	VerifyAPIs(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, catalog *CombinedAPICatalog) (*ToolkitVerifyReport, error)
	WriteGateArtifacts(rt *Runtime, catalog *CombinedAPICatalog, report *ToolkitVerifyReport) error
}

var (
	frameworkToolkitMu sync.RWMutex
	frameworkToolkits  = map[string]FrameworkToolkit{}
)

func registerFrameworkToolkit(t FrameworkToolkit) {
	if t == nil || strings.TrimSpace(t.ID()) == "" {
		return
	}
	frameworkToolkitMu.Lock()
	defer frameworkToolkitMu.Unlock()
	frameworkToolkits[normalizeFrameworkToolkitID(t.ID())] = t
}

func GetFrameworkToolkit(id string) FrameworkToolkit {
	frameworkToolkitMu.RLock()
	defer frameworkToolkitMu.RUnlock()
	return frameworkToolkits[normalizeFrameworkToolkitID(id)]
}

func ListFrameworkToolkitIDs() []string {
	frameworkToolkitMu.RLock()
	defer frameworkToolkitMu.RUnlock()
	out := make([]string, 0, len(frameworkToolkits))
	for id := range frameworkToolkits {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

type detectCandidate struct {
	id       string
	score    float64
	evidence []string
}

// DetectFrameworkToolkit picks the highest-scoring registered toolkit above threshold.
func DetectFrameworkToolkit(rt *Runtime) (*FrameworkToolkitSelectionV1, bool) {
	if rt == nil {
		return nil, false
	}
	var candidates []detectCandidate
	for _, id := range ListFrameworkToolkitIDs() {
		t := GetFrameworkToolkit(id)
		if t == nil {
			continue
		}
		score, evidence := t.Detect(rt)
		if score <= 0 {
			continue
		}
		candidates = append(candidates, detectCandidate{id: id, score: score, evidence: evidence})
	}
	if len(candidates) == 0 {
		return nil, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	best := candidates[0]
	if best.score < frameworkToolkitDetectThreshold {
		return nil, false
	}
	return &FrameworkToolkitSelectionV1{
		SchemaVersion: frameworkToolkitSelectionSchemaVersion,
		FrameworkID:   best.id,
		Confidence:    best.score,
		Rationale:     "programmatic detect above threshold",
		Evidence:      best.evidence,
		Source:        "detect_fallback",
	}, true
}

// RunFrameworkToolkit executes the four-step fast path for a known framework.
func RunFrameworkToolkit(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, frameworkID string) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	t := GetFrameworkToolkit(frameworkID)
	if t == nil {
		return utils.Errorf("unknown framework toolkit: %s", frameworkID)
	}
	rt.SelectedFrameworkID = normalizeFrameworkToolkitID(frameworkID)
	rt.FrameworkToolkitMode = FrameworkToolkitModeFast

	steps := []struct {
		name string
		fn   func() error
	}{
		{"acquire_credentials", func() error { return t.AcquireCredentials(ctx, invoker, rt) }},
		{"extract_apis", func() error {
			catalog, err := t.ExtractAPIs(rt)
			if err != nil {
				return err
			}
			if catalog == nil {
				return utils.Error("extract_apis returned nil catalog")
			}
			return nil
		}},
	}
	for _, step := range steps {
		started := step.name
		rt.execStepStart("framework_toolkit."+started, "programmatic")
		if err := step.fn(); err != nil {
			rt.execStepError("framework_toolkit."+started, "programmatic", time.Now(), err, nil)
			return utils.Wrapf(err, "framework_toolkit %s", started)
		}
		rt.execStepEnd("framework_toolkit."+started, "programmatic", time.Now(), nil)
	}

	catalog, err := loadCombinedAPICatalog(rt.WorkDir)
	if err != nil {
		return utils.Wrap(err, "load combined_api_catalog after extract")
	}

	verifyStart := time.Now()
	rt.execStepStart("framework_toolkit.verify_apis", "programmatic")
	report, err := t.VerifyAPIs(ctx, invoker, rt, catalog)
	if err != nil {
		rt.execStepError("framework_toolkit.verify_apis", "programmatic", verifyStart, err, nil)
		return err
	}
	rt.execStepEnd("framework_toolkit.verify_apis", "programmatic", verifyStart, []string{store.CombinedAPICatalogPath(rt.WorkDir)})

	gateStart := time.Now()
	rt.execStepStart("framework_toolkit.write_gate_artifacts", "programmatic")
	if err := t.WriteGateArtifacts(rt, catalog, report); err != nil {
		rt.execStepError("framework_toolkit.write_gate_artifacts", "programmatic", gateStart, err, nil)
		return err
	}
	rt.execStepEnd("framework_toolkit.write_gate_artifacts", "programmatic", gateStart, nil)

	log.Infof("ssa_api_discovery: framework_toolkit %s done verified=%d probed=%d",
		frameworkID, report.Verified, report.Probed)
	return nil
}

// RunGenericProgrammaticExtract runs combined API extraction for other/fallback path.
func RunGenericProgrammaticExtract(rt *Runtime) (*CombinedAPICatalog, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	catalog, err := RunCombinedProgrammaticAPIExtraction(rt)
	if err != nil {
		return catalog, err
	}
	log.Infof("ssa_api_discovery: generic programmatic extract records=%d", catalog.Stats.Total)
	return catalog, nil
}

func init() {
	registerFrameworkToolkit(&PublicCMSToolkit{})
	registerFrameworkToolkit(&MCMSToolkitStub{})
	registerFrameworkToolkit(&OfbizToolkitStub{})
}
