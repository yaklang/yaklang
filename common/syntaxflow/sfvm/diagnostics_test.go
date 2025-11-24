package sfvm

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/utils/diagnostics"
)

func TestSFVMWithDiagnostics(t *testing.T) {
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
