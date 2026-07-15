package loop_fast_context

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/subagent"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSwapInvokerEmitterForNestedScope(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	parentEmitter := aicommon.NewEmitter("parent", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	cfg.Emitter = parentEmitter
	invoker := mock.NewMockInvoker(context.Background())
	invoker.SetConfig(cfg)

	const categorySubID = "parent-cat-sub-sql_injection-abcd"
	categoryEmitter := subagent.BuildForwardingEmitter(parentEmitter, categorySubID)
	parent := aicommon.NewSubTaskBaseWithOptions(
		aicommon.NewStatefulTaskBase("orchestrator", "audit", context.Background(), parentEmitter, true),
		categorySubID,
		"scan",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseSkipTaskStatusChangeEmit(),
	)
	parent.SetEmitter(categoryEmitter)

	restore := cfg.SwapEmitter(parent.GetEmitter())
	require.Same(t, categoryEmitter, cfg.GetEmitter())
	restore()
	require.Same(t, parentEmitter, cfg.GetEmitter())
}

func TestNestedScopeEmitter_UsesCategorySubAgentTaskId(t *testing.T) {
	var captured []*schema.AiOutputEvent
	rootEmitter := aicommon.NewEmitter("root", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e)
		return e, nil
	})
	rootCfg := aicommon.NewConfig(context.Background(), aicommon.WithEmitter(rootEmitter))

	const categorySubID = "parent-phase2-sub-sql_injection-abcd"
	categoryEmitter := subagent.BuildForwardingEmitter(rootCfg.GetEmitter(), categorySubID)

	categoryTask := aicommon.NewSubTaskBaseWithOptions(
		aicommon.NewStatefulTaskBase("orchestrator", "audit", context.Background(), rootCfg.GetEmitter(), true),
		categorySubID,
		"scan sql injection",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseSkipTaskStatusChangeEmit(),
	)
	categoryTask.SetEmitter(categoryEmitter)

	_, err := categoryTask.GetEmitter().EmitStatus("grep", "running")
	require.NoError(t, err)

	require.Len(t, captured, 1)
	require.Equal(t, categorySubID, captured[0].TaskId,
		"fast_context grep/thought events must use the category sub-agent TaskId so the UI nests them inside the sub-agent card")
}

func TestParseGrepFilesWithMatchesOutput(t *testing.T) {
	stdout := `[file 1] /abs/project/handler/user.go (3 matches)
[file 2] /abs/project/service/auth.go (1 matches)
`
	paths := parseGrepFilesWithMatchesOutput(stdout)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
}

func TestToolOutputString_ExtractsStdout(t *testing.T) {
	exec := &aitool.ToolExecutionResult{
		Stdout: "[file 1] C:\\tmp\\a.go (1 matches)\n",
	}
	out := toolOutputString(exec)
	require.Equal(t, exec.Stdout, out)
	paths := parseGrepFilesWithMatchesOutput(out)
	require.Equal(t, []string{`C:\tmp\a.go`}, paths)

	// JSON blob (old InterfaceToString path) must not be preferred when Data is typed.
	require.NotContains(t, out, `"stdout"`)
}

func TestFilterAuditCandidatePaths(t *testing.T) {
	in := []string{"/app/main.go", "/app/vendor/x.go", "/app/foo_test.go"}
	out := FilterAuditCandidatePaths(in)
	if len(out) != 1 || out[0] != "/app/main.go" {
		t.Fatalf("unexpected: %v", out)
	}
}

func TestPrioritizeAuditCandidatePaths(t *testing.T) {
	in := []string{
		"/dvwa/external/recaptcha/help.php",
		"/dvwa/vulnerabilities/xss_r/source/low.php",
		"/dvwa/vulnerabilities/sqli/index.php",
		"/dvwa/vulnerabilities/exec/source/impossible.php",
	}
	out := PrioritizeAuditCandidatePaths(in, 2)
	require.Len(t, out, 2)
	require.Equal(t, "/dvwa/vulnerabilities/xss_r/source/low.php", out[0])
	require.Equal(t, "/dvwa/vulnerabilities/sqli/index.php", out[1])
}

func TestUniquePathsMergesIndexAndReport(t *testing.T) {
	report := &ExplorationReport{
		Locations: []LocationHit{{Path: "/a.go"}},
	}
	paths := uniquePaths(report, []string{"/a.go", "/b.go"})
	if len(paths) != 2 {
		t.Fatalf("expected 2 unique paths, got %d", len(paths))
	}
}
