//go:build hids

package model

import (
	"fmt"
	"testing"
	"time"
)

func TestDesiredSpecValidateRejectsUnknownCustomRuleEventType(t *testing.T) {
	t.Parallel()

	spec := DesiredSpec{
		Mode: ModeObserve,
		Collectors: Collectors{
			File: FileCollectorSpec{
				Enabled:    true,
				Backend:    CollectorBackendFileWatch,
				WatchPaths: []string{"/tmp/hids-spec-test"},
			},
		},
		CustomRules: []CustomRule{
			{
				RuleID:         "tmp-invalid-event",
				Enabled:        true,
				MatchEventType: "file.changed",
				Condition:      "true",
			},
		},
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := err.Error(); got != `custom_rules[0].match_event_type: unsupported event type "file.changed"` {
		t.Fatalf("unexpected validation error: %s", got)
	}
}

func TestDesiredSpecValidateRejectsCollectorCoverageMismatch(t *testing.T) {
	t.Parallel()

	spec := DesiredSpec{
		Mode: ModeObserve,
		Collectors: Collectors{
			File: FileCollectorSpec{
				Enabled:    true,
				Backend:    CollectorBackendFileWatch,
				WatchPaths: []string{"/tmp/hids-spec-test"},
			},
		},
		CustomRules: []CustomRule{
			{
				RuleID:         "tmp-audit-without-auditd",
				Enabled:        true,
				MatchEventType: EventTypeAudit,
				Condition:      "true",
			},
		},
	}

	err := spec.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := err.Error(); got != `custom_rules[0].match_event_type: event type "audit.event" is not producible by the enabled collectors` {
		t.Fatalf("unexpected validation error: %s", got)
	}
}

func TestDesiredSpecValidateRejectsBroadFileWatchRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "workspace root", path: "/home/go0p/code"},
		{name: "wsl windows drive root", path: "/mnt/c"},
		{name: "wsl windows workspace", path: "/mnt/c/Users/go0p/source"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseDesiredSpec([]byte(fmt.Sprintf(`{
				"mode": "observe",
				"collectors": {
					"file": {
						"enabled": true,
						"backend": "filewatch",
						"watch_paths": [%q]
					}
				}
			}`, tt.path)))
			if err == nil {
				t.Fatal("expected validation error")
			}
			expected := fmt.Sprintf("collectors.file.watch_paths: %s is too broad for recursive filewatch", tt.path)
			if got := err.Error(); got != expected {
				t.Fatalf("unexpected validation error: %s", got)
			}
		})
	}
}

func TestDesiredSpecValidateSkipsDisabledCustomRule(t *testing.T) {
	t.Parallel()

	spec := DesiredSpec{
		Mode: ModeObserve,
		Collectors: Collectors{
			Process: CollectorSpec{
				Enabled: true,
				Backend: CollectorBackendEBPF,
			},
		},
		CustomRules: []CustomRule{
			{
				RuleID:         "tmp-disabled-audit",
				Enabled:        false,
				MatchEventType: EventTypeAudit,
				Condition:      "event.type ==",
			},
		},
	}

	if err := spec.Validate(); err != nil {
		t.Fatalf("disabled custom rule should not block desired spec validation: %v", err)
	}
}

func TestDesiredSpecValidateAcceptsCoveredCustomRuleEventType(t *testing.T) {
	t.Parallel()

	spec := DesiredSpec{
		Mode: ModeObserve,
		Collectors: Collectors{
			Audit: CollectorSpec{
				Enabled: true,
				Backend: CollectorBackendAuditd,
			},
		},
		CustomRules: []CustomRule{
			{
				RuleID:         "tmp-audit",
				Enabled:        true,
				MatchEventType: EventTypeAudit,
				Condition:      "true",
			},
		},
	}

	if err := spec.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestReportingPolicyDefaultsSnapshotObservationsOff(t *testing.T) {
	t.Parallel()

	if (ReportingPolicy{}).ShouldEmitSnapshotObservations() {
		t.Fatal("expected snapshot observation export to default off")
	}

	disabled := false
	if (ReportingPolicy{EmitSnapshotObservations: &disabled}).ShouldEmitSnapshotObservations() {
		t.Fatal("expected explicit false to disable snapshot observation export")
	}

	enabled := true
	if !(ReportingPolicy{EmitSnapshotObservations: &enabled}).ShouldEmitSnapshotObservations() {
		t.Fatal("expected explicit true to enable snapshot observation export")
	}
}

func TestDesiredSpecParsesResponsePolicy(t *testing.T) {
	t.Parallel()

	spec, err := ParseDesiredSpec([]byte(`{
		"mode": "observe",
		"collectors": {
			"process": {"enabled": true, "backend": "ebpf"}
		},
		"response_policy": {
			"mode": "observe",
			"allowed_actions": ["process.terminate", "process.terminate"],
			"expires_at": "2026-04-23T20:00:00+08:00"
		}
	}`))
	if err != nil {
		t.Fatalf("ParseDesiredSpec returned error: %v", err)
	}
	if spec.ResponsePolicy.Mode != ModeObserve {
		t.Fatalf("unexpected response policy mode: %s", spec.ResponsePolicy.Mode)
	}
	if len(spec.ResponsePolicy.AllowedActions) != 1 ||
		spec.ResponsePolicy.AllowedActions[0] != ResponseActionProcessTerminate {
		t.Fatalf("unexpected allowed response actions: %#v", spec.ResponsePolicy.AllowedActions)
	}
	if spec.ResponsePolicy.ExpiresAt != "2026-04-23T12:00:00Z" {
		t.Fatalf("unexpected response policy expiry: %s", spec.ResponsePolicy.ExpiresAt)
	}
	if !spec.ResponsePolicy.AllowsAction(
		ResponseActionProcessTerminate,
		time.Date(2026, 4, 23, 11, 59, 59, 0, time.UTC),
	) {
		t.Fatal("expected response policy to allow process terminate before expiry")
	}
}

func TestDesiredSpecRejectsUnsupportedResponsePolicyMode(t *testing.T) {
	t.Parallel()

	_, err := ParseDesiredSpec([]byte(`{
		"mode": "observe",
		"collectors": {
			"process": {"enabled": true, "backend": "ebpf"}
		},
		"response_policy": {
			"mode": "prevent",
			"allowed_actions": ["process.terminate"]
		}
	}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := err.Error(); got != "response_policy.mode: only observe mode is supported" {
		t.Fatalf("unexpected validation error: %s", got)
	}
}

func TestDesiredSpecRejectsUnsupportedResponsePolicyAction(t *testing.T) {
	t.Parallel()

	_, err := ParseDesiredSpec([]byte(`{
		"mode": "observe",
		"collectors": {
			"process": {"enabled": true, "backend": "ebpf"}
		},
		"response_policy": {
			"mode": "observe",
			"allowed_actions": ["process.suspend"]
		}
	}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := err.Error(); got != `response_policy.allowed_actions[0]: unsupported action "process.suspend"` {
		t.Fatalf("unexpected validation error: %s", got)
	}
}

func TestResponsePolicyAllowsActionRejectsExpiredPolicy(t *testing.T) {
	t.Parallel()

	policy := ResponsePolicy{
		Mode:           ModeObserve,
		AllowedActions: []string{ResponseActionProcessTerminate},
		ExpiresAt:      "2026-04-23T12:00:00Z",
	}

	if policy.AllowsAction(
		ResponseActionProcessTerminate,
		time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC),
	) {
		t.Fatal("expected response policy to deny action at expiry boundary")
	}
}

func TestDesiredSpecParsesBaselineAndContextPolicy(t *testing.T) {
	t.Parallel()

	spec, err := ParseDesiredSpec([]byte(`{
		"collectors": {
			"process": {"enabled": true, "backend": "ebpf"},
			"network": {"enabled": true, "backend": "ebpf"}
		},
		"context_policy": {
			"short_term_window_minutes": 5
		},
		"baseline_policy": {
			"host_users": {
				"frozen_users": [
					{"username": "root", "uid": "0", "groups": ["root"], "privileged": true}
				]
			},
			"network": {
				"frozen_allowlist": [
					{"direction": "OUTBOUND", "protocol": "TCP", "dest_cidr": "10.0.0.1", "dest_port": 443}
				]
			},
			"drift_alerts": {
				"aggregation_window_minutes": 15,
				"max_aggregation_entries": 32
			}
		}
	}`))
	if err != nil {
		t.Fatalf("ParseDesiredSpec returned error: %v", err)
	}
	if spec.ContextPolicy.ShortTermWindowMinutes != 5 {
		t.Fatalf("unexpected short term window: %d", spec.ContextPolicy.ShortTermWindowMinutes)
	}
	if got := len(spec.BaselinePolicy.HostUsers.FrozenUsers); got != 1 {
		t.Fatalf("unexpected frozen user count: %d", got)
	}
	if got := spec.BaselinePolicy.Network.FrozenAllowlist[0].DestCIDR; got != "10.0.0.1/32" {
		t.Fatalf("unexpected normalized dest cidr: %s", got)
	}
	if got := spec.BaselinePolicy.Network.FrozenAllowlist[0].Direction; got != "outbound" {
		t.Fatalf("unexpected direction: %s", got)
	}
	if got := spec.BaselinePolicy.DriftAlerts.Severity; got != DefaultBaselineDriftSeverity {
		t.Fatalf("unexpected drift severity: %s", got)
	}
}

func TestDesiredSpecRejectsInvalidBaselinePolicy(t *testing.T) {
	t.Parallel()

	_, err := ParseDesiredSpec([]byte(`{
		"collectors": {
			"network": {"enabled": true, "backend": "ebpf"}
		},
		"baseline_policy": {
			"network": {
				"frozen_allowlist": [
					{"direction": "outbound", "protocol": "tcp", "dest_cidr": "not a cidr", "dest_port": 443}
				]
			}
		}
	}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := err.Error(); got != "baseline_policy.network.frozen_allowlist[0].dest_cidr: must be a valid CIDR prefix" {
		t.Fatalf("unexpected validation error: %s", got)
	}
}
