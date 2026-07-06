package sfvm

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/utils/diagnostics"
)

func TestSFVMWithDiagnostics(t *testing.T) {
	oldLevel := diagnostics.GetLevel()
	diagnostics.SetLevel(diagnostics.LevelLow)
	t.Cleanup(func() {
		diagnostics.SetLevel(oldLevel)
	})

	// Create a diagnostics recorder
	recorder := diagnostics.NewRecorder()

	// Create config with diagnostics enabled
	config := NewConfig(
		WithDiagnostics(true, recorder),
		WithContext(context.Background()),
	)

	// Create a simple rule
	rule := `
desc(
	title: "Test Rule"
)

$test as $output
alert $output
`

	// Compile the rule
	frame, err := CompileRule(rule)
	if err != nil {
		t.Fatalf("Failed to compile rule: %v", err)
	}

	// Set the config with diagnostics
	frame.config = config

	// Execute the rule with some input
	emptyInput := NewEmptyValues()
	_, err = frame.Feed(emptyInput)
	if err != nil {
		t.Fatalf("Failed to execute rule: %v", err)
	}

	// Check if diagnostics were recorded
	entries := recorder.Snapshot()
	if len(entries) == 0 {
		t.Error("Expected diagnostics entries to be recorded, but got none")
	}

	// Print diagnostics entries for verification
	t.Logf("Recorded %d diagnostics entries:", len(entries))
	for i, entry := range entries {
		t.Logf("Entry %d: %s - %v (Count: %d)", i+1, entry.Name, entry.Total, entry.Count)
	}
}

// TestSFVM_NoDiagnosticsZeroTrackAlloc guards the Fix 4 extraction: with
// diagnostics OFF (the default code-scan level), running a rule's opcode loop
// must NOT allocate the per-opcode "sfvm.op:..." name string, the execOpcode
// closure, or the diagnostics.TrackLow variadic slice. Those used to be built
// eagerly every opcode even when profiling was off — a top churn driver
// (~117M opcodes on large projects) attributed through execRule/execSyntaxFlowOp.
// The fix routes the off path through execOneOpcode (plain method call) and
// only builds name+closure+recorder.Track when a recorder is present.
func TestSFVM_NoDiagnosticsZeroTrackAlloc(t *testing.T) {
	// Diagnostics OFF (default).
	oldLevel := diagnostics.GetLevel()
	diagnostics.SetLevel(diagnostics.LevelOff)
	t.Cleanup(func() { diagnostics.SetLevel(oldLevel) })

	rule := `
desc(title: "Test Rule")
$test as $output
alert $output
`
	frame, err := CompileRule(rule)
	if err != nil {
		t.Fatalf("CompileRule: %v", err)
	}
	frame.config = NewConfig(WithContext(context.Background()))
	emptyInput := NewEmptyValues()

	// Warm up once (let any first-call lazies settle), then measure allocations
	// of a full Feed (the opcode loop). We assert it's bounded — the OLD code
	// allocated a closure + 2-3 name strings + a TrackLow slice PER OPCODE; the
	// new code allocates none of those on the off path. We don't assert exactly
	// 0 (Feed may alloc for stack/result), just that the per-opcode track
	// machinery is gone: compare against a baseline that forces the recorder on
	// and check the off-path allocs are strictly lower.
	if _, err := frame.Feed(emptyInput); err != nil {
		t.Fatalf("warm Feed: %v", err)
	}

	allocsOff := testing.AllocsPerRun(1, func() {
		_, _ = frame.Feed(emptyInput)
	})

	// Force the recorder ON path (allocates name+closure+Track per opcode).
	recorder := diagnostics.NewRecorder()
	frame.config = NewConfig(WithDiagnostics(true, recorder), WithContext(context.Background()))
	allocsOn := testing.AllocsPerRun(1, func() {
		_, _ = frame.Feed(emptyInput)
	})

	t.Logf("Feed allocs: diagnostics-off=%v diagnostics-on=%v", allocsOff, allocsOn)
	if allocsOff >= allocsOn {
		t.Fatalf("diagnostics-off path (%v allocs) should allocate LESS than diagnostics-on (%v); "+
			"if equal/higher the per-opcode name/closure/TrackLow is still built on the off path", allocsOff, allocsOn)
	}
}
