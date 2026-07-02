package loop_fast_context

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestFastContextIsolatedSubTaskDoesNotCancelParent(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-scan-sql", "audit", context.Background(), nil)
	sub := newFastContextIsolatedSubTask(parent)
	require.NotNil(t, sub)
	require.NotEqual(t, parent.GetId(), sub.GetId())

	sub.SetStatus(aicommon.AITaskState_Completed)

	select {
	case <-parent.GetContext().Done():
		t.Fatal("parent task context must stay alive when isolated fast_context sub-task completes")
	default:
	}
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
