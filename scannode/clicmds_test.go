package scannode

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunDistYakFileUsesExecutionContext(t *testing.T) {
	script, err := os.CreateTemp(t.TempDir(), "distyak-context-*.yak")
	if err != nil {
		t.Fatalf("create script: %v", err)
	}
	if _, err := script.WriteString(`time.sleep(0.5)`); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := script.Close(); err != nil {
		t.Fatalf("close script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = runDistYakFile(ctx, script.Name(), "test-runtime")
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("distyak context cancellation took too long: %v", elapsed)
	}
}
