//go:build hids && linux

package auditd

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestWrapAuditStartupErrorPermissionDeniedIncludesGuidance(t *testing.T) {
	t.Parallel()

	err := wrapAuditStartupError(syscall.EPERM)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "CAP_AUDIT_READ") {
		t.Fatalf("expected capability guidance, got %q", err.Error())
	}
	if !errors.Is(err, syscall.EPERM) {
		t.Fatal("expected wrapped error to preserve original errno")
	}
}

func TestWrapAuditStartupErrorKernelUnsupportedKeepsMeaning(t *testing.T) {
	t.Parallel()

	source := errors.New("audit not supported by kernel: protocol not supported")
	err := wrapAuditStartupError(source)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "not supported by this kernel") {
		t.Fatalf("expected kernel support guidance, got %q", err.Error())
	}
}

func TestShouldStopReceiveLoopTreatsPermissionErrorsAsFatal(t *testing.T) {
	t.Parallel()

	if !shouldStopReceiveLoop(syscall.EACCES) {
		t.Fatal("expected permission denied to stop receive loop")
	}
	if !shouldStopReceiveLoop(errors.New("use of closed network connection")) {
		t.Fatal("expected closed connection to stop receive loop")
	}
	if shouldStopReceiveLoop(errors.New("temporary retry later")) {
		t.Fatal("did not expect transient error to stop receive loop")
	}
}

func TestBuildAuditRuntimeLossObservationCarriesErrorDetail(t *testing.T) {
	t.Parallel()

	event := buildAuditRuntimeLossObservation("receive-error", errors.New("permission denied"), true)
	if event.Type != "audit.loss" {
		t.Fatalf("unexpected event type: %s", event.Type)
	}
	if event.Audit == nil {
		t.Fatal("expected audit payload")
	}
	if event.Audit.Category != "audit-runtime" {
		t.Fatalf("unexpected category: %s", event.Audit.Category)
	}
	if event.Audit.Action != "receive-error" {
		t.Fatalf("unexpected action: %s", event.Audit.Action)
	}
	if got, ok := event.Data["error"].(string); !ok || got != "permission denied" {
		t.Fatalf("unexpected error detail: %#v", event.Data["error"])
	}
}
