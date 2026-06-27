package loop_ssa_api_discovery

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// phase1AgentBaseOptions returns shared ReAct options for fine-grained Phase1 agents.
func phase1AgentBaseOptions(r aicommon.AIInvokeRuntime, rt *Runtime, playbook string, extra string) []reactloops.ReActLoopOption {
	persistent := strings.TrimSpace(playbook)
	if extra != "" {
		persistent += "\n\n" + strings.TrimSpace(extra)
	}
	persistent += "\n\n" + strings.TrimSpace(ssaDiscoveryFSBuiltinToolParamsHint)
	if strings.Contains(playbook, "do_http_request") || strings.Contains(extra, "HTTP") {
		persistent += "\n\n" + strings.TrimSpace(ssaDiscoveryHTTPBuiltinToolParamsHint)
	}
	return []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowRAG(true),
		reactloops.WithAllowAIForge(false),
		reactloops.WithPersistentInstruction(persistent),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			base := ""
			if rt != nil && rt.Session != nil {
				base = EffectiveTargetBaseURL(rt.Session)
			}
			return fmt.Sprintf(`<|PHASE1_AGENT_%s|>
session: %s
target_base: %s
code_root: %s
target_reachable: %v
feedback:
%s
<|END_%s|>`,
				nonce,
				loop.Get("discovery_session_uuid"),
				base,
				loopGetCodeRoot(rt),
				loopGetTargetReachable(rt),
				feedbacker.String(),
				nonce,
			), nil
		}),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			setRuntime(loop, rt)
			if rt != nil && rt.Session != nil {
				loop.Set("discovery_session_uuid", rt.Session.UUID)
				loop.Set("discovery_sqlite_path", rt.SQLitePath)
				loop.Set("discovery_code_root", rt.Session.CodeRootPath)
			}
			op.NextAction("discovery_get_status")
		}),
		buildDiscoveryGetStatus(),
		buildDiscoveryReadSessionData(),
		buildCodeReadingReadFileAudit(rt),
	}
}

func loopGetCodeRoot(rt *Runtime) string {
	if rt == nil || rt.Session == nil {
		return ""
	}
	return rt.Session.CodeRootPath
}

func loopGetTargetReachable(rt *Runtime) bool {
	return rt != nil && rt.Session != nil && rt.Session.TargetReachable
}

func phase1AgentSearchOptions() []reactloops.ReActLoopOption {
	return phase1SearchExtractActionOptions()
}

func buildBlockedDirectlyAnswer(finalizeAction string) reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType:  "directly_answer",
		Description: "Blocked; use " + finalizeAction + ".",
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			op.Feedback("use " + finalizeAction + " to commit structured output")
			op.Continue()
		},
	})
}

func embeddedArtifactsForAgent(rt *Runtime, paths ...string) string {
	if rt == nil {
		return ""
	}
	var lines []string
	lines = append(lines, "## Embedded upstream artifacts")
	for _, p := range paths {
		excerpt, err := readArtifactExcerpt(p, 6000)
		if err != nil {
			lines = append(lines, fmt.Sprintf("### %s\n(missing)", p))
			continue
		}
		lines = append(lines, fmt.Sprintf("### %s\n```json\n%s\n```", p, excerpt))
	}
	return strings.Join(lines, "\n\n")
}
