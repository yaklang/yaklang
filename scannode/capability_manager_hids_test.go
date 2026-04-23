//go:build hids

package scannode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hidsmodel "github.com/yaklang/yaklang/common/hids/model"
)

func TestCapabilityManagerApplyStartsHIDSRuntimeWhenCompiled(t *testing.T) {
	t.Parallel()

	watchDir := t.TempDir()
	baseDir := t.TempDir()
	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: baseDir,
	})
	t.Cleanup(func() {
		_ = manager.Close()
	})

	result, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey: "hids",
		SpecVersion:   "2026-03-28",
		DesiredSpecJSON: []byte(fmt.Sprintf(`{
			"mode": "observe",
			"collectors": {
				"file": {
					"enabled": true,
					"backend": "filewatch",
					"watch_paths": [%q]
				}
			}
		}`, watchDir)),
	})
	if err != nil {
		t.Fatalf("apply capability: %v", err)
	}
	if result.Status != capabilityStatusRunning {
		t.Fatalf("unexpected status: %s", result.Status)
	}

	raw, err := os.ReadFile(filepath.Join(baseDir, "legion", "capabilities", "hids.json"))
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected persisted capability file")
	}
}

func TestCapabilityManagerApplyRejectsInvalidHIDSSpecWhenCompiled(t *testing.T) {
	t.Parallel()

	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})

	_, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey:   "hids",
		SpecVersion:     "2026-03-28",
		DesiredSpecJSON: []byte(`{"collectors":{"file":{"enabled":true,"backend":"polling","watch_paths":["/etc"]}}}`),
	})
	if !errors.Is(err, ErrInvalidHIDSCapabilitySpec) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCapabilityManagerApplyRejectsInvalidHIDSRuleConditionWhenCompiled(t *testing.T) {
	t.Parallel()

	watchDir := t.TempDir()
	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})

	_, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey: "hids",
		SpecVersion:   "2026-03-28",
		DesiredSpecJSON: []byte(fmt.Sprintf(`{
			"mode": "observe",
			"collectors": {
				"file": {
					"enabled": true,
					"backend": "filewatch",
					"watch_paths": [%q]
				}
			},
			"temporary_rules": [
				{
					"rule_id": "bad-rule",
					"enabled": true,
					"match_event_type": "file.change",
					"severity": "high",
					"condition": "event.type =="
				}
			]
		}`, watchDir)),
	})
	if !errors.Is(err, ErrInvalidHIDSCapabilitySpec) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCapabilityManagerForwardsHIDSAlertsWhenReportingEnabled(t *testing.T) {
	t.Parallel()

	watchDir := t.TempDir()
	baseDir := t.TempDir()
	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: baseDir,
	})
	t.Cleanup(func() {
		_ = manager.Close()
	})

	targetFile := filepath.Join(watchDir, "observed.txt")
	condition := fmt.Sprintf("file.path == %q", targetFile)
	result, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey: "hids",
		SpecVersion:   "2026-03-28",
		DesiredSpecJSON: []byte(fmt.Sprintf(`{
			"mode": "observe",
			"collectors": {
				"file": {
					"enabled": true,
					"backend": "filewatch",
					"watch_paths": [%q]
				}
			},
			"temporary_rules": [
				{
					"rule_id": "tmp-observed-file",
					"title": "Observed file write",
					"description": "Detect writes to the watched file during capability tests.",
					"enabled": true,
					"match_event_type": "file.change",
					"severity": "medium",
					"condition": %q,
					"metadata": {
						"template_id": "test.file.write",
						"authoring_surface": "go-test"
					}
				}
			],
			"reporting": {
				"emit_capability_alert": true
			}
		}`, watchDir, condition)),
	})
	if err != nil {
		t.Fatalf("apply capability: %v", err)
	}
	if !strings.Contains(string(result.StatusDetailJSON), `"temporary_rules"`) {
		t.Fatalf(
			"expected temporary rule runtime summary in status detail: %s",
			string(result.StatusDetailJSON),
		)
	}
	if !strings.Contains(string(result.StatusDetailJSON), `"rule_id":"tmp-observed-file"`) {
		t.Fatalf(
			"expected active temporary rule in status detail: %s",
			string(result.StatusDetailJSON),
		)
	}

	if err := os.WriteFile(targetFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	select {
	case alert := <-manager.Alerts():
		if alert.CapabilityKey != "hids" {
			t.Fatalf("unexpected capability key: %s", alert.CapabilityKey)
		}
		if alert.RuleID != "tmp-observed-file" {
			t.Fatalf("unexpected rule id: %s", alert.RuleID)
		}
		if alert.Severity != "medium" {
			t.Fatalf("unexpected severity: %s", alert.Severity)
		}
		if alert.Title != "Observed file write" {
			t.Fatalf("unexpected alert title: %s", alert.Title)
		}

		var detail map[string]any
		if err := json.Unmarshal(alert.DetailJSON, &detail); err != nil {
			t.Fatalf("unmarshal alert detail: %v", err)
		}
		if detail["rule_id"] != "tmp-observed-file" {
			t.Fatalf("unexpected detail rule id: %#v", detail["rule_id"])
		}
		if detail["match_event_type"] != "file.change" {
			t.Fatalf("unexpected match_event_type: %#v", detail["match_event_type"])
		}
		if detail["source"] != "temporary" {
			t.Fatalf("unexpected source: %#v", detail["source"])
		}
		if detail["rule_description"] != "Detect writes to the watched file during capability tests." {
			t.Fatalf("unexpected rule_description: %#v", detail["rule_description"])
		}
		ruleMetadata, ok := detail["rule_metadata"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected rule_metadata payload: %#v", detail["rule_metadata"])
		}
		if ruleMetadata["template_id"] != "test.file.write" {
			t.Fatalf("unexpected template_id: %#v", ruleMetadata["template_id"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for capability alert")
	}
}

func TestCapabilityManagerForwardsBuiltinHIDSAlertsWhenConfigured(t *testing.T) {
	t.Parallel()

	watchDir := t.TempDir()
	baseDir := t.TempDir()
	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: baseDir,
	})
	t.Cleanup(func() {
		_ = manager.Close()
	})

	targetDir := filepath.Join(watchDir, "rootfs", "etc")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	targetFile := filepath.Join(targetDir, "passwd")
	result, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey: "hids",
		SpecVersion:   "2026-03-28",
		DesiredSpecJSON: []byte(fmt.Sprintf(`{
			"mode": "observe",
			"collectors": {
				"file": {
					"enabled": true,
					"backend": "filewatch",
					"watch_paths": [%q]
				}
			},
			"builtin_rule_sets": [
				"linux.file.integrity"
			],
			"reporting": {
				"emit_capability_alert": true
			}
		}`, watchDir)),
	})
	if err != nil {
		t.Fatalf("apply capability: %v", err)
	}
	if result.Status != capabilityStatusRunning {
		t.Fatalf("unexpected normalized capability status: %s", result.Status)
	}
	if !strings.Contains(string(result.StatusDetailJSON), `"inactive_count":2`) {
		t.Fatalf("expected partial builtin rule coverage in status detail: %s", string(result.StatusDetailJSON))
	}

	if err := os.WriteFile(targetFile, []byte("root:x:0:0"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	select {
	case alert := <-manager.Alerts():
		if alert.CapabilityKey != "hids" {
			t.Fatalf("unexpected capability key: %s", alert.CapabilityKey)
		}
		if alert.RuleID != "linux.file.sensitive_path_change" {
			t.Fatalf("unexpected rule id: %s", alert.RuleID)
		}
		if alert.Severity != "high" {
			t.Fatalf("unexpected severity: %s", alert.Severity)
		}

		var detail map[string]any
		if err := json.Unmarshal(alert.DetailJSON, &detail); err != nil {
			t.Fatalf("unmarshal alert detail: %v", err)
		}
		if detail["builtin_rule_set"] != "linux.file.integrity" {
			t.Fatalf("unexpected builtin rule set detail: %#v", detail["builtin_rule_set"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for builtin capability alert")
	}
}

func TestCapabilityManagerRestorePersistedRestartsHIDSRuntime(t *testing.T) {
	t.Parallel()

	watchDir := t.TempDir()
	baseDir := t.TempDir()
	initialManager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: baseDir,
	})
	t.Cleanup(func() {
		_ = initialManager.Close()
	})

	spec := []byte(fmt.Sprintf(`{
		"mode": "observe",
		"collectors": {
			"file": {
				"enabled": true,
				"backend": "filewatch",
				"watch_paths": [%q]
			}
		}
	}`, watchDir))
	if _, err := initialManager.Apply(CapabilityApplyInput{
		CapabilityKey:   "hids",
		SpecVersion:     "2026-04-10",
		DesiredSpecJSON: spec,
	}); err != nil {
		t.Fatalf("seed persisted hids capability: %v", err)
	}
	_ = initialManager.Close()

	restoredManager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: baseDir,
	})
	t.Cleanup(func() {
		_ = restoredManager.Close()
	})

	results, err := restoredManager.RestorePersisted()
	if err != nil {
		t.Fatalf("restore persisted capabilities: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("unexpected restore result count: %d", len(results))
	}
	if results[0].CapabilityKey != "hids" {
		t.Fatalf("unexpected restored capability key: %s", results[0].CapabilityKey)
	}
	if results[0].Status != capabilityStatusRunning {
		t.Fatalf("unexpected restored capability status: %s", results[0].Status)
	}
}

func TestCapabilityManagerApplyReusesUnchangedHIDSRuntime(t *testing.T) {
	t.Parallel()

	watchDir := t.TempDir()
	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})
	t.Cleanup(func() {
		_ = manager.Close()
	})

	spec := []byte(fmt.Sprintf(`{
		"mode": "observe",
		"collectors": {
			"file": {
				"enabled": true,
				"backend": "filewatch",
				"watch_paths": [%q]
			}
		}
	}`, watchDir))

	if _, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey:   "hids",
		SpecVersion:     "2026-04-10",
		DesiredSpecJSON: spec,
	}); err != nil {
		t.Fatalf("initial apply: %v", err)
	}

	result, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey:   "hids",
		SpecVersion:     "2026-04-10",
		DesiredSpecJSON: spec,
	})
	if err != nil {
		t.Fatalf("repeat apply: %v", err)
	}
	if result.Status != capabilityStatusRunning {
		t.Fatalf("unexpected repeated apply status: %s", result.Status)
	}
	if !strings.Contains(result.Message, "desired spec unchanged") {
		t.Fatalf("unexpected repeated apply message: %s", result.Message)
	}
}

func TestCapabilityManagerApplyReusesUnchangedPartiallyCoveredHIDSRuntime(t *testing.T) {
	t.Parallel()

	watchDir := t.TempDir()
	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})
	t.Cleanup(func() {
		_ = manager.Close()
	})

	spec := []byte(fmt.Sprintf(`{
		"mode": "observe",
		"collectors": {
			"file": {
				"enabled": true,
				"backend": "filewatch",
				"watch_paths": [%q]
			}
		},
		"builtin_rule_sets": ["linux.file.integrity"]
	}`, watchDir))

	initial, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey:   "hids",
		SpecVersion:     "2026-04-10",
		DesiredSpecJSON: spec,
	})
	if err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	if initial.Status != capabilityStatusRunning {
		t.Fatalf("unexpected normalized initial status: %s", initial.Status)
	}

	result, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey:   "hids",
		SpecVersion:     "2026-04-10",
		DesiredSpecJSON: spec,
	})
	if err != nil {
		t.Fatalf("repeat apply: %v", err)
	}
	if result.Status != capabilityStatusRunning {
		t.Fatalf("unexpected repeated apply status: %s", result.Status)
	}
	if !strings.Contains(result.Message, "desired spec unchanged") {
		t.Fatalf("unexpected repeated apply message: %s", result.Message)
	}
	if !strings.Contains(string(result.StatusDetailJSON), `"reported":"degraded"`) {
		t.Fatalf("expected normalized degraded detail on repeat apply: %s", string(result.StatusDetailJSON))
	}
}

func TestCapabilityManagerRuntimeStatusesExposeStructuredDetail(t *testing.T) {
	t.Parallel()

	watchDir := t.TempDir()
	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})
	t.Cleanup(func() {
		_ = manager.Close()
	})

	_, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey: "hids",
		SpecVersion:   "2026-04-10",
		DesiredSpecJSON: []byte(fmt.Sprintf(`{
			"mode": "observe",
			"collectors": {
				"file": {
					"enabled": true,
					"backend": "filewatch",
					"watch_paths": [%q]
				}
			},
			"reporting": {
				"emit_capability_status": true
			}
		}`, watchDir)),
	})
	if err != nil {
		t.Fatalf("apply capability: %v", err)
	}

	statuses := manager.RuntimeStatuses()
	if len(statuses) != 1 {
		t.Fatalf("unexpected runtime status count: %d", len(statuses))
	}
	if statuses[0].Status != capabilityStatusRunning {
		t.Fatalf("unexpected runtime status: %s", statuses[0].Status)
	}
	if len(statuses[0].DetailJSON) == 0 {
		t.Fatal("expected structured detail json")
	}

	var detail map[string]any
	if err := json.Unmarshal(statuses[0].DetailJSON, &detail); err != nil {
		t.Fatalf("unmarshal detail json: %v", err)
	}
	collectors, ok := detail["collectors"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected collectors detail: %#v", detail["collectors"])
	}
	filewatch, ok := collectors["file"].(map[string]any)
	if !ok {
		t.Fatalf("expected filewatch collector detail, got %#v", collectors)
	}
	if filewatch["backend"] != "filewatch" {
		t.Fatalf("unexpected collector backend: %#v", filewatch["backend"])
	}
	if filewatch["status"] != capabilityStatusRunning {
		t.Fatalf("unexpected collector status: %#v", filewatch["status"])
	}
}

func TestHIDSObservationForwarderPublishesSnapshotsAndSkipsUnknownTerminalEvents(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{
		observations: make(chan CapabilityRuntimeObservation, 4),
	}
	state := newHIDSSnapshotObservationState()
	config := hidsAlertConfig{
		capabilityKey:            "hids",
		specVersion:              "2026-04-20",
		emitSnapshotObservations: true,
	}
	observedAt := time.Date(2026, 4, 20, 15, 10, 0, 0, time.UTC)

	processEvent := hidsmodel.Event{
		Type:      hidsmodel.EventTypeProcessExec,
		Source:    "inventory.process",
		Timestamp: observedAt,
		Tags:      []string{"inventory"},
		Process: &hidsmodel.Process{
			PID:   123,
			Name:  "nginx",
			Image: "/usr/sbin/nginx",
		},
	}
	observation, key, ok := hooks.convertObservation(processEvent, config, state)
	if !ok {
		t.Fatal("expected process inventory observation to be exported")
	}
	state.pending[key] = observation
	hooks.flushPendingObservations(state)

	select {
	case output := <-hooks.Observations():
		if output.HIDSEventType != hidsmodel.EventTypeProcessExec {
			t.Fatalf("unexpected hids event type: %s", output.HIDSEventType)
		}
		if !json.Valid(output.EventJSON) {
			t.Fatalf("expected json event payload: %s", string(output.EventJSON))
		}
	default:
		t.Fatal("expected flushed observation")
	}

	unknownClose := hidsmodel.Event{
		Type:      hidsmodel.EventTypeNetworkClose,
		Timestamp: observedAt.Add(time.Second),
		Network: &hidsmodel.Network{
			Protocol:      "tcp",
			SourceAddress: "127.0.0.1",
			SourcePort:    50000,
			DestAddress:   "127.0.0.1",
			DestPort:      8080,
		},
	}
	if _, _, ok := hooks.convertObservation(unknownClose, config, state); ok {
		t.Fatal("expected unknown network close to be dropped")
	}
}

func TestHIDSObservationForwarderStillPublishesInventoryWhenSnapshotExportDisabled(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{
		observations: make(chan CapabilityRuntimeObservation, 2),
	}
	state := newHIDSSnapshotObservationState()
	config := hidsAlertConfig{
		capabilityKey:            "hids",
		specVersion:              "2026-04-23",
		emitSnapshotObservations: false,
	}
	observation, key, ok := hooks.convertObservation(
		hidsmodel.Event{
			Type:      hidsmodel.EventTypeHostUsers,
			Source:    "inventory.users",
			Timestamp: time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC),
		},
		config,
		state,
	)
	if !ok {
		t.Fatal("expected inventory replay observation to still be exported")
	}
	state.pending[key] = observation
	hooks.flushPendingObservations(state)

	select {
	case output := <-hooks.Observations():
		if output.HIDSEventType != hidsmodel.EventTypeHostUsers {
			t.Fatalf("unexpected hids event type: %s", output.HIDSEventType)
		}
	default:
		t.Fatal("expected flushed inventory observation")
	}
}

func TestHIDSObservationForwarderDropsInvalidNetworkInventoryWithoutEndpoint(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{
		observations: make(chan CapabilityRuntimeObservation, 1),
	}
	state := newHIDSSnapshotObservationState()
	config := hidsAlertConfig{
		capabilityKey:            "hids",
		specVersion:              "2026-04-23",
		emitSnapshotObservations: true,
	}

	if _, _, ok := hooks.convertObservation(
		hidsmodel.Event{
			Type:      hidsmodel.EventTypeNetworkSocket,
			Source:    "inventory.network",
			Timestamp: time.Date(2026, 4, 23, 13, 0, 0, 0, time.UTC),
			Tags:      []string{"network", "inventory"},
			Process: &hidsmodel.Process{
				PID:                 101,
				BootID:              "boot-1",
				StartTimeUnixMillis: 1713848400000,
			},
			Network: &hidsmodel.Network{
				Protocol: "tcp",
				FD:       11,
			},
		},
		config,
		state,
	); ok {
		t.Fatal("expected invalid network inventory without endpoint to be dropped")
	}
}
