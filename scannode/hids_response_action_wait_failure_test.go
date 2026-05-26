//go:build hids && linux

package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestBuildResponseActionWaitFailureResult(t *testing.T) {
	t.Parallel()

	process := hidsResponseActionProcess{
		PID:                 4321,
		BootID:              "boot-1",
		StartTimeUnixMillis: 1713691199000,
		ProcessName:         "sleep",
	}

	t.Run("maps context cancellation to wait interrupted", func(t *testing.T) {
		t.Parallel()

		result, err := buildResponseActionWaitFailureResult(process, []string{"SIGTERM"}, context.Canceled)
		if !errors.Is(err, ErrHIDSResponseActionWaitInterrupted) {
			t.Fatalf("expected wait interrupted, got %v", err)
		}
		var detail map[string]any
		if unmarshalErr := json.Unmarshal(result.DetailJSON, &detail); unmarshalErr != nil {
			t.Fatalf("unmarshal detail: %v", unmarshalErr)
		}
		if detail["reason"] != "process_wait_interrupted" {
			t.Fatalf("unexpected detail reason: %#v", detail["reason"])
		}
	})

	t.Run("maps generic error to process state query failed", func(t *testing.T) {
		t.Parallel()

		result, err := buildResponseActionWaitFailureResult(
			process,
			[]string{"SIGTERM", "SIGKILL"},
			errors.New("read /proc/4321/stat: permission denied"),
		)
		if !errors.Is(err, ErrHIDSResponseActionProcessStateQueryFailed) {
			t.Fatalf("expected process state query failed, got %v", err)
		}
		var detail map[string]any
		if unmarshalErr := json.Unmarshal(result.DetailJSON, &detail); unmarshalErr != nil {
			t.Fatalf("unmarshal detail: %v", unmarshalErr)
		}
		if detail["reason"] != "process_state_query_failed" {
			t.Fatalf("unexpected detail reason: %#v", detail["reason"])
		}
	})
}
