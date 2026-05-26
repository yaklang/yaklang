package scannode

import (
	"fmt"
	"testing"

	capabilityv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/capability/v1"
	hidsv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/hids/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateHIDSResponseActionCommand(t *testing.T) {
	t.Parallel()

	command := &hidsv1.ExecuteHIDSResponseActionCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: "cmd-1",
		},
		TargetNodeId: "node-1",
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: "hids",
			SpecVersion:   "2026-04-15T06:48:51.016Z",
		},
		Action: hidsResponseActionProcessTerminate,
		Process: &hidsv1.HIDSProcessRef{
			Pid:             1234,
			BootId:          "boot-1",
			StartTimeUnixMs: 1713691199000,
		},
	}

	if err := validateHIDSResponseActionCommand("node-1", command); err != nil {
		t.Fatalf("expected valid command, got %v", err)
	}

	command.TargetNodeId = "node-2"
	if err := validateHIDSResponseActionCommand("node-1", command); err == nil {
		t.Fatal("expected mismatch error")
	}
	command.TargetNodeId = "node-1"

	command.Process.BootId = ""
	if err := validateHIDSResponseActionCommand("node-1", command); err == nil {
		t.Fatal("expected boot id error")
	}
	command.Process.BootId = "boot-1"

	command.Action = "file.delete"
	if err := validateHIDSResponseActionCommand("node-1", command); err == nil {
		t.Fatal("expected unsupported action error")
	}
}

func TestMapHIDSResponseActionError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{
			name:     "unsupported",
			err:      ErrHIDSResponseActionUnsupported,
			wantCode: "action_not_supported",
		},
		{
			name:     "process not found",
			err:      ErrHIDSResponseActionProcessNotFound,
			wantCode: "process_not_found",
		},
		{
			name:     "identity mismatch",
			err:      ErrHIDSResponseActionIdentityMismatch,
			wantCode: "process_identity_mismatch",
		},
		{
			name:     "process state query failed",
			err:      ErrHIDSResponseActionProcessStateQueryFailed,
			wantCode: "process_state_query_failed",
		},
		{
			name:     "protected process",
			err:      ErrHIDSResponseActionProtectedProcess,
			wantCode: "protected_process",
		},
		{
			name:     "still running",
			err:      ErrHIDSResponseActionStillRunning,
			wantCode: "process_still_running",
		},
		{
			name:     "signal failed",
			err:      ErrHIDSResponseActionSignalFailed,
			wantCode: "process_signal_failed",
		},
		{
			name:     "wait interrupted",
			err:      ErrHIDSResponseActionWaitInterrupted,
			wantCode: "process_wait_interrupted",
		},
		{
			name:     "wrapped wait interrupted",
			err:      fmt.Errorf("%w: runtime shutting down", ErrHIDSResponseActionWaitInterrupted),
			wantCode: "process_wait_interrupted",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, message := mapHIDSResponseActionError(testCase.err)
			if code != testCase.wantCode {
				t.Fatalf("unexpected code: %s", code)
			}
			if message == "" {
				t.Fatal("expected non-empty message")
			}
		})
	}
}
