//go:build hids

package rule

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestEngineEvaluateMatchesTemporaryPathWhitelistRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-path-whitelist",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Condition:      "!path.AnyGlob(process.image, '/usr/bin/*', '/usr/sbin/*', '/opt/company/*')",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Now().UTC(),
		Process: &model.Process{
			PID:     42,
			Image:   "/tmp/payload",
			Command: "/tmp/payload --serve",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "tmp-process-path-whitelist" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateSkipsTemporaryPathWhitelistRuleForAllowedBinary(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-path-whitelist",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Condition:      "!path.AnyGlob(process.image, '/usr/bin/*', '/usr/sbin/*', '/opt/company/*')",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Now().UTC(),
		Process: &model.Process{
			PID:     7,
			Image:   "/usr/bin/ssh",
			Command: "/usr/bin/ssh demo@example.test",
		},
	})
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts, got %d", len(alerts))
	}
}

func TestEngineEvaluateMatchesTemporaryArtifactHashWhitelistRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-artifact-hash-whitelist",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Condition:      "artifact.IsELF(process.artifact) && !artifact.SHA256In(process.artifact, 'trusted-sha256', 'trusted-sha256-2')",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Now().UTC(),
		Process: &model.Process{
			PID:      99,
			Image:    "/usr/local/bin/custom-agent",
			Command:  "/usr/local/bin/custom-agent",
			Artifact: &model.Artifact{Path: "/usr/local/bin/custom-agent", FileType: "elf", Hashes: &model.ArtifactHashes{SHA256: "untrusted-sha256"}},
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "tmp-artifact-hash-whitelist" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesTemporarySystemArtifactDriftRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-system-elf-drift",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "high",
				Condition:      "artifact.IsELF(file.artifact) && path.AnyUnder(file.path, '/usr/bin', '/usr/sbin') && !artifact.SHA256In(file.artifact, 'trusted-binary')",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Now().UTC(),
		File: &model.File{
			Path:      "/usr/bin/curl",
			Operation: "WRITE",
			Artifact:  &model.Artifact{Path: "/usr/bin/curl", FileType: "elf", Hashes: &model.ArtifactHashes{SHA256: "mutated-binary"}},
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "tmp-system-elf-drift" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesTemporaryAuditRemoteLoginRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-audit-remote-login-failed",
				Enabled:        true,
				MatchEventType: model.EventTypeAudit,
				Severity:       "medium",
				Condition:      "auditx.FamilyIs(audit, 'login') && auditx.ResultIs(audit, 'fail') && auditx.HasRemotePeer(audit) && auditx.HasRecordType(audit, 'USER_LOGIN')",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Now().UTC(),
		Audit: &model.Audit{
			Family:      "login",
			Result:      "fail",
			RecordTypes: []string{"USER_LOGIN"},
			RemoteIP:    "10.0.0.5",
			Username:    "root",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "tmp-audit-remote-login-failed" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesTemporaryAuditSensitiveFileRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-audit-sensitive-file-access",
				Enabled:        true,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Condition:      "auditx.FamilyIs(audit, 'file') && auditx.ActionIs(audit, 'read', 'open') && path.AnyUnder(audit.object_primary, '/etc/ssh', '/root/.ssh')",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Now().UTC(),
		Audit: &model.Audit{
			Family:        "file",
			Action:        "read",
			ObjectPrimary: "/etc/ssh/sshd_config",
			Username:      "deploy",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "tmp-audit-sensitive-file-access" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}
