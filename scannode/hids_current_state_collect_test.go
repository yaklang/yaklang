package scannode

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/nats-io/nats.go"
	capabilityv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/capability/v1"
	hidsv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/hids/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateHIDSCurrentStateCollectCommand(t *testing.T) {
	t.Parallel()

	command := validHIDSCurrentStateCollectCommand("processes")
	if err := validateHIDSCurrentStateCollectCommand("node-1", command); err != nil {
		t.Fatalf("expected valid command, got %v", err)
	}

	command.TargetNodeId = "node-2"
	if err := validateHIDSCurrentStateCollectCommand("node-1", command); err == nil {
		t.Fatal("expected target mismatch error")
	}
	command.TargetNodeId = "node-1"

	command.CollectType = "files"
	if err := validateHIDSCurrentStateCollectCommand("node-1", command); err == nil {
		t.Fatal("expected unsupported collect type error")
	}

	command.CollectType = "connections"
	if err := validateHIDSCurrentStateCollectCommand("node-1", command); err != nil {
		t.Fatalf("expected connections collect type to be valid, got %v", err)
	}
}

func TestHandleHIDSCurrentStateCollectRequestsCapabilityCollection(t *testing.T) {
	t.Parallel()

	hooks := &recordingHIDSCurrentStateHooks{}
	manager := &CapabilityManager{
		nodeID:    "node-1",
		hidsHooks: hooks,
	}
	bridge := &legionJobBridge{
		agent: &ScanNode{
			capabilityManager: manager,
		},
	}

	raw, err := proto.Marshal(validHIDSCurrentStateCollectCommand("connections"))
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}

	if err := bridge.handleHIDSCurrentStateCollect(context.Background(), raw); err != nil {
		t.Fatalf("handle collect command: %v", err)
	}
	if hooks.collectType != "connections" {
		t.Fatalf("expected connections collection, got %q", hooks.collectType)
	}
}

func TestHandleMessageDispatchesHIDSCurrentStateCollect(t *testing.T) {
	t.Parallel()

	hooks := &recordingHIDSCurrentStateHooks{}
	bridge := &legionJobBridge{
		agent: &ScanNode{
			capabilityManager: &CapabilityManager{
				nodeID:    "node-1",
				hidsHooks: hooks,
			},
		},
	}

	raw, err := proto.Marshal(validHIDSCurrentStateCollectCommand("processes"))
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}

	if err := bridge.handleMessage(context.Background(), &nats.Msg{
		Subject: "legion.command.node.node-1.hids.current_state.collect",
		Data:    raw,
	}); err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if hooks.collectType != "processes" {
		t.Fatalf("expected processes collection, got %q", hooks.collectType)
	}
}

func validHIDSCurrentStateCollectCommand(collectType string) *hidsv1.CollectHIDSCurrentStateCommand {
	return &hidsv1.CollectHIDSCurrentStateCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: "cmd-1",
		},
		TargetNodeId: "node-1",
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: "hids",
			SpecVersion:   "2026-05-12",
		},
		CollectType: collectType,
	}
}

type recordingHIDSCurrentStateHooks struct {
	collectType string
}

func (h *recordingHIDSCurrentStateHooks) Apply(
	_ *CapabilityManager,
	_ capabilityHIDSApplyInput,
) (CapabilityApplyResult, error) {
	return CapabilityApplyResult{}, nil
}

func (h *recordingHIDSCurrentStateHooks) DryRun(
	_ *CapabilityManager,
	_ capabilityHIDSApplyInput,
) (CapabilityDryRunResult, error) {
	return CapabilityDryRunResult{}, nil
}

func (h *recordingHIDSCurrentStateHooks) CollectCurrentState(_ context.Context, collectType string) error {
	h.collectType = collectType
	return nil
}

func (h *recordingHIDSCurrentStateHooks) CollectFileEvidence(
	context.Context,
	hidsFileEvidenceCollectInput,
) (map[string]any, error) {
	return nil, nil
}

func (h *recordingHIDSCurrentStateHooks) Alerts() <-chan CapabilityRuntimeAlert { return nil }

func (h *recordingHIDSCurrentStateHooks) Observations() <-chan CapabilityRuntimeObservation {
	return nil
}

func (h *recordingHIDSCurrentStateHooks) CurrentStatus() (CapabilityRuntimeStatus, bool) {
	return CapabilityRuntimeStatus{}, false
}

func (h *recordingHIDSCurrentStateHooks) OnSessionReady(context.Context) error { return nil }

func (h *recordingHIDSCurrentStateHooks) Close() error { return nil }

var _ capabilityHIDSHooks = (*recordingHIDSCurrentStateHooks)(nil)
