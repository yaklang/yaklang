//go:build hids && linux

package runtime

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestBaselineDriftDetectorSuppressesFrozenHostUser(t *testing.T) {
	t.Parallel()

	detector := newBaselineDriftDetector(model.BaselinePolicy{
		HostUsers: model.HostUserBaselinePolicy{
			FrozenUsers: []model.FrozenHostUser{{Username: "root", UID: "0"}},
		},
	})
	if detector == nil {
		t.Fatal("expected baseline drift detector")
	}

	alerts := detector.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Timestamp: time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
		Process: &model.Process{
			PID:      100,
			Username: "root",
			UID:      "0",
			Image:    "/usr/bin/id",
		},
	})
	if len(alerts) != 0 {
		t.Fatalf("expected frozen host user to be allowed, got %#v", alerts)
	}
}

func TestBaselineDriftDetectorAlertsForNewHostUserOncePerWindow(t *testing.T) {
	t.Parallel()

	detector := newBaselineDriftDetector(model.BaselinePolicy{
		HostUsers: model.HostUserBaselinePolicy{
			FrozenUsers: []model.FrozenHostUser{{Username: "root", UID: "0"}},
		},
		DriftAlerts: model.DriftAlertPolicy{
			AggregationWindowMinutes: 15,
			MaxAggregationEntries:    16,
		},
	})
	start := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	event := model.Event{
		Type:      model.EventTypeProcessExec,
		Timestamp: start,
		Process: &model.Process{
			PID:      101,
			Username: "deploy",
			UID:      "1001",
			Image:    "/bin/bash",
			Command:  "/bin/bash -lc id",
		},
	}

	alerts := detector.Evaluate(event)
	if len(alerts) != 1 {
		t.Fatalf("expected first drift alert, got %#v", alerts)
	}
	if alerts[0].RuleID != hostUserBaselineDriftRuleID {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Severity != model.DefaultBaselineDriftSeverity {
		t.Fatalf("unexpected severity: %s", alerts[0].Severity)
	}
	if alerts[0].Detail["hit_count"] != 1 {
		t.Fatalf("expected first hit_count=1, got %#v", alerts[0].Detail["hit_count"])
	}

	event.Timestamp = start.Add(time.Second)
	alerts = detector.Evaluate(event)
	if len(alerts) != 1 {
		t.Fatalf("expected immediate aggregation update on second hit, got %#v", alerts)
	}
	if alerts[0].Detail["hit_count"] != 2 {
		t.Fatalf("expected second hit_count=2, got %#v", alerts[0].Detail["hit_count"])
	}

	event.Timestamp = start.Add(2 * time.Second)
	if alerts := detector.Evaluate(event); len(alerts) != 0 {
		t.Fatalf("expected third hit inside throttle window to be suppressed, got %#v", alerts)
	}

	event.Timestamp = start.Add(31 * time.Second)
	if alerts := detector.Evaluate(event); len(alerts) != 1 {
		t.Fatalf("expected throttled aggregation update after interval, got %#v", alerts)
	} else if alerts[0].Detail["hit_count"] != 4 {
		t.Fatalf("expected throttled hit_count=4, got %#v", alerts[0].Detail["hit_count"])
	}

	event.Timestamp = start.Add(16 * time.Minute)
	if alerts := detector.Evaluate(event); len(alerts) != 1 {
		t.Fatalf("expected drift alert after aggregation window, got %#v", alerts)
	}
}

func TestBaselineDriftDetectorMatchesNetworkCIDRAllowlist(t *testing.T) {
	t.Parallel()

	detector := newBaselineDriftDetector(model.BaselinePolicy{
		Network: model.NetworkBaselinePolicy{
			FrozenAllowlist: []model.FrozenNetworkAllowlistEntry{
				{Direction: "outbound", Protocol: "tcp", DestCIDR: "10.10.0.0/16", DestPort: 443},
			},
		},
	})
	allowed := model.Event{
		Type:      model.EventTypeNetworkConnect,
		Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		Process:   &model.Process{PID: 200, Name: "curl", Image: "/usr/bin/curl"},
		Network: &model.Network{
			Direction:   "outbound",
			Protocol:    "tcp",
			DestAddress: "10.10.20.30",
			DestPort:    443,
		},
	}
	if alerts := detector.Evaluate(allowed); len(alerts) != 0 {
		t.Fatalf("expected CIDR allowlist match, got %#v", alerts)
	}

	blocked := allowed
	blocked.Timestamp = allowed.Timestamp.Add(time.Second)
	blocked.Network = &model.Network{
		Direction:   "outbound",
		Protocol:    "tcp",
		DestAddress: "198.51.100.10",
		DestPort:    443,
	}
	alerts := detector.Evaluate(blocked)
	if len(alerts) != 1 {
		t.Fatalf("expected network drift alert, got %#v", alerts)
	}
	if alerts[0].RuleID != networkBaselineDriftRuleID {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	baseline, ok := alerts[0].Detail["baseline"].(map[string]any)
	if !ok || baseline["dest_cidr"] != "198.51.100.10/32" {
		t.Fatalf("unexpected baseline detail: %#v", alerts[0].Detail["baseline"])
	}
}

func TestDriftAggregatorBoundsEntries(t *testing.T) {
	t.Parallel()

	aggregator := newDriftAggregator(15*time.Minute, 2)
	start := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	if result := aggregator.observe("a", start); !result.emit {
		t.Fatal("expected first key to emit")
	}
	if result := aggregator.observe("b", start.Add(time.Second)); !result.emit {
		t.Fatal("expected second key to emit")
	}
	if result := aggregator.observe("c", start.Add(2*time.Second)); !result.emit {
		t.Fatal("expected third key to emit")
	}
	if got := len(aggregator.entries); got != 2 {
		t.Fatalf("expected bounded aggregation map, got %d", got)
	}
	if result := aggregator.observe("a", start.Add(3*time.Second)); !result.emit {
		t.Fatal("expected evicted oldest key to emit again")
	}
}
