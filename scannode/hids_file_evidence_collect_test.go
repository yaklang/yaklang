package scannode

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	capabilityv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/capability/v1"
	hidsv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/hids/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateHIDSFileEvidenceCollectCommand(t *testing.T) {
	t.Parallel()

	command := validHIDSFileEvidenceCollectCommand("directory_scan", "/tmp")
	if err := validateHIDSFileEvidenceCollectCommand("node-1", command); err != nil {
		t.Fatalf("expected valid file evidence collect command: %v", err)
	}

	command = validHIDSFileEvidenceCollectCommand("directory_scan", "")
	if err := validateHIDSFileEvidenceCollectCommand("node-1", command); err == nil {
		t.Fatal("expected missing path to be rejected")
	}

	command = validHIDSFileEvidenceCollectCommand("file", "/tmp/a")
	if err := validateHIDSFileEvidenceCollectCommand("node-1", command); err == nil {
		t.Fatal("expected unsupported kind to be rejected")
	}

	command = validHIDSFileEvidenceCollectCommand("single_file_scan", "/tmp/a")
	if err := validateHIDSFileEvidenceCollectCommand("node-2", command); err == nil {
		t.Fatal("expected node mismatch to be rejected")
	}
}

func TestHandleHIDSFileEvidenceCollectPublishesFileAlert(t *testing.T) {
	t.Parallel()

	hooks := &recordingHIDSFileEvidenceHooks{
		result: map[string]any{
			"kind":            "directory_scan",
			"resolved_target": "/tmp",
			"scan": map[string]any{
				"mode":          "directory",
				"scanned_count": 1,
			},
		},
	}
	publisher := &recordingHIDSFileEvidencePublisher{}
	bridge := &legionJobBridge{
		agent: &ScanNode{
			capabilityManager: &CapabilityManager{
				nodeID:    "node-1",
				hidsHooks: hooks,
			},
		},
		capabilityPublisher: publisher,
	}
	raw, err := proto.Marshal(validHIDSFileEvidenceCollectCommand("directory_scan", "/tmp"))
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}

	if err := bridge.handleHIDSFileEvidenceCollect(context.Background(), raw); err != nil {
		t.Fatalf("handleHIDSFileEvidenceCollect returned error: %v", err)
	}

	if hooks.kind != "directory_scan" || hooks.path != "/tmp" {
		t.Fatalf("unexpected scan request: kind=%s path=%s", hooks.kind, hooks.path)
	}
	if len(publisher.alerts) != 1 {
		t.Fatalf("expected one published alert, got %d", len(publisher.alerts))
	}
	alert := publisher.alerts[0]
	if alert.RuleID != "manual.file_evidence_scan" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	var detail map[string]any
	if err := json.Unmarshal(alert.DetailJSON, &detail); err != nil {
		t.Fatalf("decode alert detail: %v", err)
	}
	if detail["match_event_type"] != "file.change" {
		t.Fatalf("unexpected match_event_type: %v", detail["match_event_type"])
	}
	results, _ := detail["evidence_results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected one evidence result, got %#v", detail["evidence_results"])
	}
}

func validHIDSFileEvidenceCollectCommand(kind string, path string) *hidsv1.CollectHIDSFileEvidenceCommand {
	return &hidsv1.CollectHIDSFileEvidenceCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId:   "command-file-1",
			CommandType: legionCommandHIDSFileEvidenceCollect,
			TraceId:     "trace-file-1",
			IssuedAt:    timestamppb.New(time.Date(2026, 5, 13, 10, 30, 0, 0, time.UTC)),
			ExpireAt:    timestamppb.New(time.Date(2026, 5, 13, 10, 35, 0, 0, time.UTC)),
		},
		TargetNodeId: "node-1",
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: "hids",
			SpecVersion:   "spec-v1",
		},
		Kind:       kind,
		Path:       path,
		Recursive:  true,
		MaxDepth:   2,
		MaxEntries: 24,
	}
}

type recordingHIDSFileEvidenceHooks struct {
	sessionReadyHooksStub
	kind       string
	path       string
	recursive  bool
	maxDepth   int
	maxEntries int
	result     map[string]any
	err        error
}

func (h *recordingHIDSFileEvidenceHooks) CollectFileEvidence(
	_ context.Context,
	input hidsFileEvidenceCollectInput,
) (map[string]any, error) {
	h.kind = input.Kind
	h.path = input.Path
	h.recursive = input.Recursive
	h.maxDepth = input.MaxDepth
	h.maxEntries = input.MaxEntries
	return h.result, h.err
}

type recordingHIDSFileEvidencePublisher struct {
	sessionReadyReporterStub
	alerts []CapabilityRuntimeAlert
}

func (p *recordingHIDSFileEvidencePublisher) PublishAlert(
	_ context.Context,
	alert CapabilityRuntimeAlert,
) error {
	p.alerts = append(p.alerts, alert)
	return nil
}
