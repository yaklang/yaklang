package scannode

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeCapabilityEventStatusMapsStoppedToStored(t *testing.T) {
	t.Parallel()

	status, detail := normalizeCapabilityEventStatus(
		"stopped",
		[]byte(`{"collectors":{"process":{"status":"stopped"}}}`),
	)
	if status != capabilityStatusStored {
		t.Fatalf("expected stopped to normalize to stored, got %s", status)
	}
	if !json.Valid(detail) {
		t.Fatalf("expected normalized detail to stay valid json: %s", string(detail))
	}
	if !strings.Contains(string(detail), `"reported":"stopped"`) {
		t.Fatalf("expected normalized detail to preserve reported status: %s", string(detail))
	}
}

func TestNormalizeCapabilityEventStatusMapsDegradedToRunning(t *testing.T) {
	t.Parallel()

	status, detail := normalizeCapabilityEventStatus(
		"degraded",
		[]byte(`{"collectors":{"audit":{"status":"degraded"}}}`),
	)
	if status != capabilityStatusRunning {
		t.Fatalf("expected degraded to normalize to running, got %s", status)
	}
	if !json.Valid(detail) {
		t.Fatalf("expected normalized detail to stay valid json: %s", string(detail))
	}
	if !strings.Contains(string(detail), `"reported":"degraded"`) {
		t.Fatalf("expected normalized detail to preserve reported status: %s", string(detail))
	}
}
