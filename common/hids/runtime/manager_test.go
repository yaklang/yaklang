//go:build hids && linux

package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	hidscollector "github.com/yaklang/yaklang/common/hids/collector"
	"github.com/yaklang/yaklang/common/hids/model"
)

func TestManagerApplyReportsPartialBuiltinRuleCoverage(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	t.Cleanup(func() {
		_ = manager.Close()
	})

	result, err := manager.Apply(context.Background(), model.DesiredSpec{
		Mode: model.ModeObserve,
		Collectors: model.Collectors{
			File: model.FileCollectorSpec{
				Enabled:    true,
				Backend:    model.CollectorBackendFileWatch,
				WatchPaths: []string{t.TempDir()},
			},
		},
		BuiltinRuleSets: []string{"linux.file.integrity"},
	})
	if err != nil {
		t.Fatalf("apply hids runtime: %v", err)
	}
	if result.State.Status != "degraded" {
		t.Fatalf("expected degraded runtime status, got %s", result.State.Status)
	}

	rules, ok := result.State.Detail["rules"].(map[string]any)
	if !ok {
		t.Fatalf("expected rule status detail, got %#v", result.State.Detail["rules"])
	}
	if rules["inactive_count"] != 2 {
		t.Fatalf("unexpected inactive rule count: %#v", rules["inactive_count"])
	}
	if rules["active_count"] != 4 {
		t.Fatalf("unexpected active rule count: %#v", rules["active_count"])
	}
}

func TestManagerApplyReportsTemporaryRuleRuntimeSummary(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	t.Cleanup(func() {
		_ = manager.Close()
	})

	result, err := manager.Apply(context.Background(), model.DesiredSpec{
		Mode: model.ModeObserve,
		Collectors: model.Collectors{
			File: model.FileCollectorSpec{
				Enabled:    true,
				Backend:    model.CollectorBackendFileWatch,
				WatchPaths: []string{t.TempDir()},
			},
		},
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp.file.active",
				Title:          "Active file rule",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "high",
				Condition:      "str.HasPrefix(file.path, '/tmp')",
				Metadata: map[string]any{
					"template_id": "system-elf-drift",
					"pack_id":     "file-core",
				},
			},
			{
				RuleID:         "tmp.file.disabled",
				Title:          "Disabled file rule",
				Enabled:        false,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "medium",
				Condition:      "str.HasPrefix(file.path, '/etc')",
			},
		},
	})
	if err != nil {
		t.Fatalf("apply hids runtime: %v", err)
	}

	rules, ok := result.State.Detail["rules"].(map[string]any)
	if !ok {
		t.Fatalf("expected rule status detail, got %#v", result.State.Detail["rules"])
	}
	temporary, ok := rules["temporary_rules"].(map[string]any)
	if !ok {
		t.Fatalf("expected temporary rule runtime detail, got %#v", rules["temporary_rules"])
	}
	if temporary["configured_count"] != 2 {
		t.Fatalf("unexpected configured temporary rule count: %#v", temporary["configured_count"])
	}
	if temporary["active_count"] != 1 {
		t.Fatalf("unexpected active temporary rule count: %#v", temporary["active_count"])
	}
	if temporary["inactive_count"] != 1 {
		t.Fatalf("unexpected inactive temporary rule count: %#v", temporary["inactive_count"])
	}

	activeRules, ok := temporary["active_rules"].([]map[string]any)
	if !ok {
		t.Fatalf("expected active temporary rules, got %#v", temporary["active_rules"])
	}
	if len(activeRules) != 1 || activeRules[0]["rule_id"] != "tmp.file.active" {
		t.Fatalf("unexpected active temporary rules: %#v", activeRules)
	}
	if activeRules[0]["template_id"] != "system-elf-drift" {
		t.Fatalf("unexpected active temporary rule template id: %#v", activeRules[0]["template_id"])
	}

	inactiveRules, ok := temporary["inactive_rules"].([]map[string]any)
	if !ok {
		t.Fatalf("expected inactive temporary rules, got %#v", temporary["inactive_rules"])
	}
	if len(inactiveRules) != 1 || inactiveRules[0]["rule_id"] != "tmp.file.disabled" {
		t.Fatalf("unexpected inactive temporary rules: %#v", inactiveRules)
	}
	if inactiveRules[0]["reason"] != "disabled in desired spec" {
		t.Fatalf("unexpected inactive temporary rule reason: %#v", inactiveRules[0]["reason"])
	}
}

type replayInventoryProviderStub struct {
	processEvents []model.Event
	networkEvents []model.Event
	userEvents    []model.Event
}

func (s replayInventoryProviderStub) ListProcessEvents(context.Context) ([]model.Event, error) {
	return s.processEvents, nil
}

func (s replayInventoryProviderStub) ListNetworkEvents(context.Context) ([]model.Event, error) {
	return s.networkEvents, nil
}

func (s replayInventoryProviderStub) ListHostUserEvents(context.Context) ([]model.Event, error) {
	return s.userEvents, nil
}

func TestManagerReplayInventoryReemitsCurrentSnapshots(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	instance := &Instance{
		spec: model.DesiredSpec{
			Collectors: model.Collectors{
				Process: model.CollectorSpec{Enabled: true},
				Network: model.CollectorSpec{Enabled: true},
			},
		},
		events: make(chan model.Event, 4),
		baselineProvider: replayInventoryProviderStub{
			processEvents: []model.Event{{Type: model.EventTypeProcessState, Source: "inventory.process"}},
			networkEvents: []model.Event{{Type: model.EventTypeNetworkSocket, Source: "inventory.network"}},
			userEvents:    []model.Event{{Type: model.EventTypeHostUsers, Source: "inventory.users"}},
		},
	}
	manager.instance = instance

	if err := manager.ReplayInventory(context.Background()); err != nil {
		t.Fatalf("replay inventory: %v", err)
	}

	first := <-instance.events
	second := <-instance.events
	third := <-instance.events
	if first.Source != "inventory.process" || second.Source != "inventory.network" || third.Source != "inventory.users" {
		t.Fatalf("unexpected replay sources: %s / %s / %s", first.Source, second.Source, third.Source)
	}
}

func TestInstanceStartRefreshesInventoryPeriodically(t *testing.T) {
	t.Parallel()

	instance := &Instance{
		spec: model.DesiredSpec{
			Collectors: model.Collectors{
				Process: model.CollectorSpec{Enabled: true},
				Network: model.CollectorSpec{Enabled: true},
			},
		},
		collectors: []collectorBinding{
			{
				kind:    "process",
				backend: model.CollectorBackendEBPF,
				collector: &collectorStub{
					name: "ebpf.process",
					snapshot: hidscollector.HealthSnapshot{
						Name:    "process",
						Backend: "ebpf",
						Status:  "running",
						Message: "ebpf process collector is running",
					},
				},
			},
		},
		events: make(chan model.Event, 8),
		baselineProvider: replayInventoryProviderStub{
			processEvents: []model.Event{{Type: model.EventTypeProcessState, Source: "inventory.process"}},
			networkEvents: []model.Event{{Type: model.EventTypeNetworkSocket, Source: "inventory.network"}},
			userEvents:    []model.Event{{Type: model.EventTypeHostUsers, Source: "inventory.users"}},
		},
		inventoryRefreshInterval: 10 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := instance.start(ctx); err != nil {
		t.Fatalf("start instance: %v", err)
	}
	defer func() { _ = instance.close() }()

	sources := make([]string, 0, 6)
	deadline := time.After(150 * time.Millisecond)
	for len(sources) < 6 {
		select {
		case event := <-instance.events:
			sources = append(sources, event.Source)
		case <-deadline:
			t.Fatalf("timed out waiting for refreshed inventory events, got %v", sources)
		}
	}

	if sources[0] != "inventory.process" || sources[1] != "inventory.network" || sources[2] != "inventory.users" {
		t.Fatalf("unexpected initial inventory sources: %v", sources[:3])
	}
	if sources[3] != "inventory.process" || sources[4] != "inventory.network" || sources[5] != "inventory.users" {
		t.Fatalf("expected periodic inventory replay, got %v", sources)
	}
}

type collectorStub struct {
	name     string
	startErr error
	snapshot hidscollector.HealthSnapshot
}

func (s *collectorStub) Name() string {
	return s.name
}

func (s *collectorStub) Start(context.Context, chan<- model.Event) error {
	return s.startErr
}

func (s *collectorStub) Close() error {
	return nil
}

func (s *collectorStub) HealthSnapshot() hidscollector.HealthSnapshot {
	snapshot := s.snapshot
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = time.Now().UTC()
	}
	return snapshot
}

func TestInstanceStartAllowsCollectorLevelDegradation(t *testing.T) {
	t.Parallel()

	instance := &Instance{
		spec: model.DesiredSpec{Mode: model.ModeObserve},
		collectors: []collectorBinding{
			{
				kind:    "file",
				backend: model.CollectorBackendFileWatch,
				collector: &collectorStub{
					name: "filewatch",
					snapshot: hidscollector.HealthSnapshot{
						Name:    "file",
						Backend: "filewatch",
						Status:  "running",
						Message: "filewatch collector is running",
					},
				},
			},
			{
				kind:    "audit",
				backend: model.CollectorBackendAuditd,
				collector: &collectorStub{
					name:     "auditd",
					startErr: errors.New("operation not permitted"),
				},
			},
		},
		events: make(chan model.Event, 4),
	}

	if err := instance.start(context.Background()); err != nil {
		t.Fatalf("start instance with partial collector failure: %v", err)
	}
	defer func() { _ = instance.close() }()

	state := instance.runtimeState()
	if state.Status != "degraded" {
		t.Fatalf("expected degraded runtime state, got %s", state.Status)
	}
	if len(state.ActiveCollectors) != 1 || state.ActiveCollectors[0] != "file:filewatch" {
		t.Fatalf("unexpected active collectors: %#v", state.ActiveCollectors)
	}
	if !strings.Contains(state.Message, "degraded collectors: audit:auditd") {
		t.Fatalf("expected degraded collector summary in message, got %q", state.Message)
	}

	collectors, ok := state.Detail["collectors"].(map[string]any)
	if !ok {
		t.Fatalf("expected collectors detail, got %#v", state.Detail["collectors"])
	}
	audit, ok := collectors["audit"].(map[string]any)
	if !ok {
		t.Fatalf("expected audit collector detail, got %#v", collectors["audit"])
	}
	if audit["status"] != "degraded" {
		t.Fatalf("expected degraded audit collector status, got %#v", audit["status"])
	}
	file, ok := collectors["file"].(map[string]any)
	if !ok {
		t.Fatalf("expected file collector detail, got %#v", collectors["file"])
	}
	if file["status"] != "running" {
		t.Fatalf("expected running file collector status, got %#v", file["status"])
	}
}

func TestInstanceStartFailsWhenNoCollectorStarts(t *testing.T) {
	t.Parallel()

	instance := &Instance{
		spec: model.DesiredSpec{Mode: model.ModeObserve},
		collectors: []collectorBinding{
			{
				kind:    "audit",
				backend: model.CollectorBackendAuditd,
				collector: &collectorStub{
					name:     "auditd",
					startErr: errors.New("operation not permitted"),
				},
			},
		},
		events: make(chan model.Event, 1),
	}

	err := instance.start(context.Background())
	if err == nil {
		t.Fatal("expected startup failure when no collectors start")
	}
	if !strings.Contains(err.Error(), "no hids collector started successfully") {
		t.Fatalf("unexpected error: %v", err)
	}
}
