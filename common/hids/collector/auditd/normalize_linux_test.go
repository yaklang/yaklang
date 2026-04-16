//go:build hids && linux

package auditd

import (
	"testing"

	"github.com/elastic/go-libaudit/v2/aucoalesce"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestBuildAuditEnvelopeCapturesProcessCWDAndFileIdentity(t *testing.T) {
	t.Parallel()

	event := &aucoalesce.Event{
		Session: "7",
		Process: aucoalesce.Process{
			CWD: "/root",
		},
		File: &aucoalesce.File{
			UID:   "0",
			GID:   "0",
			Owner: "root",
			Group: "root",
		},
	}

	audit := buildAuditEnvelope(event, "file", []string{"SYSCALL"})
	if audit.ProcessCWD != "/root" {
		t.Fatalf("unexpected process cwd: %q", audit.ProcessCWD)
	}
	if audit.FileUID != "0" {
		t.Fatalf("unexpected file uid: %q", audit.FileUID)
	}
	if audit.FileGID != "0" {
		t.Fatalf("unexpected file gid: %q", audit.FileGID)
	}
	if audit.FileOwner != "root" {
		t.Fatalf("unexpected file owner: %q", audit.FileOwner)
	}
	if audit.FileGroup != "root" {
		t.Fatalf("unexpected file group: %q", audit.FileGroup)
	}
}

func TestBuildAuditEnvelopeNormalizesActionAndResult(t *testing.T) {
	t.Parallel()

	event := &aucoalesce.Event{
		Data: map[string]string{
			"syscall": "openat",
			"success": "yes",
		},
	}

	audit := buildAuditEnvelope(event, "file", []string{"SYSCALL"})
	if audit.Action != "open" {
		t.Fatalf("unexpected action: %q", audit.Action)
	}
	if audit.Result != "success" {
		t.Fatalf("unexpected result: %q", audit.Result)
	}
}

func TestBuildAuditFileFallsBackToSummaryObjectPrimary(t *testing.T) {
	t.Parallel()

	file := buildAuditFile(&aucoalesce.Event{
		Summary: aucoalesce.Summary{
			Object: aucoalesce.Object{
				Type:    "file",
				Primary: "/etc/shadow",
			},
		},
		Data: map[string]string{
			"syscall": "openat",
		},
	}, "open")
	if file == nil {
		t.Fatal("expected file payload")
	}
	if file.Path != "/etc/shadow" {
		t.Fatalf("unexpected path: %q", file.Path)
	}
	if file.Operation != "open" {
		t.Fatalf("unexpected operation: %q", file.Operation)
	}
}

func TestBuildAuditFileCarriesFileIdentity(t *testing.T) {
	t.Parallel()

	file := buildAuditFile(&aucoalesce.Event{
		File: &aucoalesce.File{
			Path:  "/etc/shadow",
			Mode:  "0640",
			UID:   "0",
			GID:   "42",
			Owner: "root",
			Group: "shadow",
		},
		Data: map[string]string{
			"syscall": "chmod",
		},
	}, "chmod")
	if file == nil {
		t.Fatal("expected file payload")
	}
	if file.Path != "/etc/shadow" {
		t.Fatalf("unexpected path: %q", file.Path)
	}
	if file.Mode != "0640" {
		t.Fatalf("unexpected mode: %q", file.Mode)
	}
	if file.UID != "0" || file.GID != "42" {
		t.Fatalf("unexpected uid/gid: %q/%q", file.UID, file.GID)
	}
	if file.Owner != "root" || file.Group != "shadow" {
		t.Fatalf("unexpected owner/group: %q/%q", file.Owner, file.Group)
	}
}

func TestShouldKeepAuditObservationDropsUnknownFamily(t *testing.T) {
	t.Parallel()

	keep, reason := shouldKeepAuditObservation(model.Event{
		Type: model.EventTypeAudit,
		Audit: &model.Audit{
			Family: "unknown",
		},
	})
	if keep {
		t.Fatal("expected unknown family to be filtered")
	}
	if reason != "family.unknown" {
		t.Fatalf("unexpected filter reason: %q", reason)
	}
}

func TestShouldKeepAuditObservationKeepsUserCommandAudit(t *testing.T) {
	t.Parallel()

	keep, reason := shouldKeepAuditObservation(model.Event{
		Type: model.EventTypeAudit,
		Process: &model.Process{
			Command: "/usr/bin/sudo id",
			Image:   "/usr/bin/sudo",
		},
		Audit: &model.Audit{
			Family:    "command",
			AUID:      "1000",
			LoginUser: "alice",
			SessionID: "7",
			Terminal:  "pts/1",
		},
	})
	if !keep {
		t.Fatal("expected user-attributable command audit to be kept")
	}
	if reason != "" {
		t.Fatalf("unexpected keep reason: %q", reason)
	}
}

func TestShouldKeepAuditObservationDropsServiceCommandNoise(t *testing.T) {
	t.Parallel()

	keep, reason := shouldKeepAuditObservation(model.Event{
		Type: model.EventTypeAudit,
		Process: &model.Process{
			Command: "/usr/sbin/cron -f",
			Image:   "/usr/sbin/cron",
		},
		Audit: &model.Audit{
			Family:   "command",
			AUID:     "4294967295",
			Username: "root",
		},
	})
	if keep {
		t.Fatal("expected unattributed service exec to be filtered")
	}
	if reason != "command.unattributed" {
		t.Fatalf("unexpected filter reason: %q", reason)
	}
}

func TestShouldKeepAuditObservationKeepsSecurityTamperCommandWithoutSession(t *testing.T) {
	t.Parallel()

	keep, reason := shouldKeepAuditObservation(model.Event{
		Type: model.EventTypeAudit,
		Process: &model.Process{
			Command: "systemctl stop auditd",
		},
		Audit: &model.Audit{
			Family: "command",
			AUID:   "4294967295",
		},
	})
	if !keep {
		t.Fatal("expected security tamper command to be kept")
	}
	if reason != "" {
		t.Fatalf("unexpected keep reason: %q", reason)
	}
}

func TestShouldKeepAuditObservationKeepsSensitiveFileAccess(t *testing.T) {
	t.Parallel()

	keep, reason := shouldKeepAuditObservation(model.Event{
		Type: model.EventTypeAudit,
		File: &model.File{
			Path:      "/etc/shadow",
			Operation: "open",
		},
		Audit: &model.Audit{
			Family:    "file",
			Action:    "open",
			AUID:      "1000",
			LoginUser: "alice",
		},
	})
	if !keep {
		t.Fatal("expected sensitive file access to be kept")
	}
	if reason != "" {
		t.Fatalf("unexpected keep reason: %q", reason)
	}
}

func TestShouldKeepAuditObservationDropsNonSensitiveFileAccess(t *testing.T) {
	t.Parallel()

	keep, reason := shouldKeepAuditObservation(model.Event{
		Type: model.EventTypeAudit,
		File: &model.File{
			Path:      "/tmp/demo.txt",
			Operation: "open",
		},
		Audit: &model.Audit{
			Family:    "file",
			Action:    "open",
			AUID:      "1000",
			LoginUser: "alice",
		},
	})
	if keep {
		t.Fatal("expected non-sensitive file access to be filtered")
	}
	if reason != "file.non-sensitive-path" {
		t.Fatalf("unexpected filter reason: %q", reason)
	}
}
