//go:build hids

package model

import "testing"

func TestDesiredSpecValidateRejectsUnknownTemporaryRuleEventType(t *testing.T) {
	t.Parallel()

	spec := DesiredSpec{
		Mode: ModeObserve,
		Collectors: Collectors{
			File: FileCollectorSpec{
				Enabled:    true,
				Backend:    CollectorBackendFileWatch,
				WatchPaths: []string{"/tmp"},
			},
		},
		TemporaryRules: []TemporaryRule{
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
	if got := err.Error(); got != `temporary_rules[0].match_event_type: unsupported event type "file.changed"` {
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
				WatchPaths: []string{"/tmp"},
			},
		},
		TemporaryRules: []TemporaryRule{
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
	if got := err.Error(); got != `temporary_rules[0].match_event_type: event type "audit.event" is not producible by the enabled collectors` {
		t.Fatalf("unexpected validation error: %s", got)
	}
}

func TestDesiredSpecValidateSkipsDisabledTemporaryRule(t *testing.T) {
	t.Parallel()

	spec := DesiredSpec{
		Mode: ModeObserve,
		Collectors: Collectors{
			Process: CollectorSpec{
				Enabled: true,
				Backend: CollectorBackendEBPF,
			},
		},
		TemporaryRules: []TemporaryRule{
			{
				RuleID:         "tmp-disabled-audit",
				Enabled:        false,
				MatchEventType: EventTypeAudit,
				Condition:      "event.type ==",
			},
		},
	}

	if err := spec.Validate(); err != nil {
		t.Fatalf("disabled temporary rule should not block desired spec validation: %v", err)
	}
}

func TestDesiredSpecValidateAcceptsCoveredTemporaryRuleEventType(t *testing.T) {
	t.Parallel()

	spec := DesiredSpec{
		Mode: ModeObserve,
		Collectors: Collectors{
			Audit: CollectorSpec{
				Enabled: true,
				Backend: CollectorBackendAuditd,
			},
		},
		TemporaryRules: []TemporaryRule{
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

func TestReportingPolicyDefaultsSnapshotObservationsOn(t *testing.T) {
	t.Parallel()

	if !(ReportingPolicy{}).ShouldEmitSnapshotObservations() {
		t.Fatal("expected snapshot observation export to default on")
	}

	disabled := false
	if (ReportingPolicy{EmitSnapshotObservations: &disabled}).ShouldEmitSnapshotObservations() {
		t.Fatal("expected explicit false to disable snapshot observation export")
	}
}
