package loop_ssa_api_discovery

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

//go:embed prompts/phase1_coverage_signal_playbook.txt
var phase1CoverageSignalPlaybook string

const (
	loopKeyCoverageDecision  = "coverage_decision"
	loopKeyCoverageReasoning = "coverage_reasoning"
	loopKeyNextReadQueue    = "next_read_queue"
	loopKeyPhaseDone        = "phase_done"
)

// CoverageSignalVerdict is the outcome from CoverageSignalReAct.
type CoverageSignalVerdict string

const (
	VerdictContinue    CoverageSignalVerdict = "continue"
	VerdictFinish      CoverageSignalVerdict = "finish"
	VerdictReprioritize CoverageSignalVerdict = "reprioritize"
)

// CoverageSignalDecision is the structured result of CoverageSignalReAct.
type CoverageSignalDecision struct {
	Verdict     CoverageSignalVerdict
	Reasoning  string
	NextQueue  []string
	SignalJSON string
}

// RunCoverageSignalReAct asks the AI to assess the current coverage and return a decision.
// It runs as a single-shot ReAct loop (no batch reading inside) that receives the signal
// and decides whether to continue, finish, or reprioritize.
func RunCoverageSignalReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) (*CoverageSignalDecision, error) {
	step := "phase2.coverage_signal"
	started := time.Now()
	rt.execStepStart(step, "ai")

	sig, err := ComputeCoverageSignal(rt)
	if err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return nil, fmt.Errorf("compute coverage signal: %w", err)
	}

	signalJSON := CoverageSignalForPrompt(sig)
	signalSummary := SummarizeCoverageSignalForReAct(sig)

	extra := embeddedArtifactsForAgent(rt,
		store.FeatureApiMapPath(rt.WorkDir),
		store.StaticRouteHintsPath(rt.WorkDir),
		store.FeatureWorkProgressPath(rt.WorkDir),
	)
	extra += "\n\n## Coverage Signal Summary\n" + signalSummary + "\n"

	loop, err := buildCoverageSignalLoop(r, rt, extra, signalJSON)
	if err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return nil, err
	}

	subCtx, cancel := context.WithTimeout(ctx, coverageSignalTimeout())
	defer cancel()
	_ = subCtx // used by the loop context

	err = runPhase1ReActLoop(task, "phase1_coverage_signal", loop)
	if err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return nil, fmt.Errorf("coverage signal react: %w", err)
	}

	verdict := CoverageSignalVerdict(strings.TrimSpace(loop.Get(loopKeyCoverageDecision)))
	reasoning := strings.TrimSpace(loop.Get(loopKeyCoverageReasoning))
	nextQueueJSON := strings.TrimSpace(loop.Get(loopKeyNextReadQueue))

	decision := &CoverageSignalDecision{
		Verdict:     verdict,
		Reasoning:  reasoning,
		SignalJSON: signalJSON,
	}

	if nextQueueJSON != "" {
		var queue []string
		if err := json.Unmarshal([]byte(nextQueueJSON), &queue); err == nil {
			decision.NextQueue = queue
		}
	}

	// Persist decision for logging/audit
	_ = PersistCoverageSignal(rt, sig)
	persistCoverageSignalDecision(rt, decision)

	rt.execStepEnd(step, "ai", started, []string{store.CoverageSignalPath(rt.WorkDir)})
	return decision, nil
}

func buildCoverageSignalLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra, signalJSON string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1CoverageSignalPlaybook, extra)
	preset = append(preset,
		buildAssessCoverage(rt, signalJSON),
		reactloops.WithMaxIterations(1),
	)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_COVERAGE_SIGNAL, r, preset...)
}

// buildAssessCoverage registers the assess_coverage action for CoverageSignalReAct.
// This is the single tool the agent can call to report its decision.
func buildAssessCoverage(rt *Runtime, signalJSON string) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"assess_coverage",
		"Assess current coverage and decide whether to continue reading or finish.",
		[]aitool.ToolOption{
			aitool.WithStringParam("signal_json", aitool.WithParam_Required(true)),
			aitool.WithStringParam("next_action", aitool.WithParam_Required(true)),
			aitool.WithStringParam("reasoning", aitool.WithParam_Required(true)),
			aitool.WithStringParam("queue_update", aitool.WithParam_Required(false)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			sigJSON := strings.TrimSpace(action.GetString("signal_json"))
			nextAction := strings.TrimSpace(action.GetString("next_action"))
			reasoning := strings.TrimSpace(action.GetString("reasoning"))
			queueUpdate := strings.TrimSpace(action.GetString("queue_update"))

			validActions := map[string]bool{"continue": true, "finish": true, "reprioritize": true}
			if !validActions[nextAction] {
				op.Feedback("next_action must be one of: continue | finish | reprioritize")
				op.Continue()
				return
			}

			if sigJSON == "" && signalJSON != "" {
				sigJSON = signalJSON
			}

			loop.Set(loopKeyCoverageDecision, nextAction)
			loop.Set(loopKeyCoverageReasoning, reasoning)
			if queueUpdate != "" {
				loop.Set(loopKeyNextReadQueue, queueUpdate)
			}

			switch nextAction {
			case "finish":
				loop.Set(loopKeyPhaseDone, "true")
				op.Feedback(fmt.Sprintf("CoverageSignalReAct voted finish. reasoning: %s", reasoning))
				op.Exit()

			case "continue":
				if queueUpdate != "" {
					op.Feedback(fmt.Sprintf("continue reading. reasoning: %s; queue updated.", reasoning))
				} else {
					op.Feedback(fmt.Sprintf("continue reading. reasoning: %s", reasoning))
				}
				op.Exit()

			case "reprioritize":
				op.Feedback(fmt.Sprintf("reprioritized. reasoning: %s", reasoning))
				op.Exit()
			}
		},
	)
}

// persistCoverageSignalDecision writes the decision to phase artifact for audit.
func persistCoverageSignalDecision(rt *Runtime, decision *CoverageSignalDecision) {
	if rt == nil || decision == nil {
		return
	}
	b, _ := json.MarshalIndent(decision, "", "  ")
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, "coverage_signal_decision", string(b))
	}
}

// coverageSignalTimeout returns the max duration for a single CoverageSignalReAct call.
func coverageSignalTimeout() time.Duration {
	return coverageSignalTimeoutDuration
}

const coverageSignalTimeoutDuration = 60e9 // 60 seconds in nanoseconds

// CoverageSignalReActVerdict returns the verdict string from a decision, for logging.
func CoverageSignalReActVerdict(d *CoverageSignalDecision) string {
	if d == nil {
		return "(nil)"
	}
	return string(d.Verdict)
}
