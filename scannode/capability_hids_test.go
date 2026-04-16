//go:build hids && linux

package scannode

import (
	"testing"
	"time"

	hidsmodel "github.com/yaklang/yaklang/common/hids/model"
)

func TestHIDSCapabilityHooksConvertObservationSupportsProcessExec(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	hooks.setAlertConfig(hidsAlertConfig{
		capabilityKey: "hids",
		specVersion:   "2026-03-28",
	})

	observation, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      hidsmodel.EventTypeProcessExec,
		Source:    "ebpf",
		Timestamp: time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC),
		Process: &hidsmodel.Process{
			PID:        42,
			ParentPID:  1,
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc whoami",
			ParentName: "systemd",
		},
	})
	if !ok {
		t.Fatal("expected process.exec observation to be exported")
	}
	if observation.CapabilityKey != "hids" {
		t.Fatalf("unexpected capability key: %s", observation.CapabilityKey)
	}
	if observation.EventType != hidsmodel.EventTypeProcessExec {
		t.Fatalf("unexpected event type: %s", observation.EventType)
	}
	if len(observation.EventJSON) == 0 {
		t.Fatal("expected non-empty observation json")
	}
}

func TestHIDSCapabilityHooksConvertObservationSupportsProcessExit(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	hooks.setAlertConfig(hidsAlertConfig{
		capabilityKey: "hids",
		specVersion:   "2026-03-28",
	})

	observation, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      hidsmodel.EventTypeProcessExit,
		Source:    "ebpf.process",
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Process: &hidsmodel.Process{
			PID:        42,
			ParentPID:  1,
			Name:       "bash",
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc whoami",
			ParentName: "systemd",
		},
	})
	if !ok {
		t.Fatal("expected process.exit observation to be exported")
	}
	if observation.EventType != hidsmodel.EventTypeProcessExit {
		t.Fatalf("unexpected event type: %s", observation.EventType)
	}
	if len(observation.EventJSON) == 0 {
		t.Fatal("expected non-empty observation json")
	}
}

func TestHIDSCapabilityHooksConvertObservationSupportsNetworkClose(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	hooks.setAlertConfig(hidsAlertConfig{
		capabilityKey: "hids",
		specVersion:   "2026-04-10",
	})

	observation, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      hidsmodel.EventTypeNetworkClose,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 12, 5, 0, 0, time.UTC),
		Process: &hidsmodel.Process{
			PID:     42,
			Image:   "/usr/bin/curl",
			Command: "curl https://example.com",
		},
		Network: &hidsmodel.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.5",
			SourcePort:      41000,
			DestAddress:     "1.1.1.1",
			DestPort:        443,
			ConnectionState: "closed",
		},
		Data: map[string]any{"fd": 7},
	})
	if !ok {
		t.Fatal("expected network.close observation to be exported")
	}
	if observation.EventType != hidsmodel.EventTypeNetworkClose {
		t.Fatalf("unexpected event type: %s", observation.EventType)
	}
	if len(observation.EventJSON) == 0 {
		t.Fatal("expected non-empty observation json")
	}
}

func TestHIDSCapabilityHooksConvertObservationSupportsNetworkAccept(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	hooks.setAlertConfig(hidsAlertConfig{
		capabilityKey: "hids",
		specVersion:   "2026-04-10",
	})

	observation, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      hidsmodel.EventTypeNetworkAccept,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 12, 3, 0, 0, time.UTC),
		Process: &hidsmodel.Process{
			PID:     42,
			Image:   "/usr/sbin/sshd",
			Command: "/usr/sbin/sshd -D",
		},
		Network: &hidsmodel.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.10",
			SourcePort:      22,
			DestAddress:     "192.168.1.7",
			DestPort:        51123,
			ConnectionState: "ESTABLISHED",
		},
		Data: map[string]any{"fd": 9},
	})
	if !ok {
		t.Fatal("expected network.accept observation to be exported")
	}
	if observation.EventType != hidsmodel.EventTypeNetworkAccept {
		t.Fatalf("unexpected event type: %s", observation.EventType)
	}
	if len(observation.EventJSON) == 0 {
		t.Fatal("expected non-empty observation json")
	}
}

func TestHIDSCapabilityHooksConvertObservationSkipsNetworkStateForPlatform(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	hooks.setAlertConfig(hidsAlertConfig{
		capabilityKey: "hids",
		specVersion:   "2026-04-10",
	})

	observation, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      hidsmodel.EventTypeNetworkState,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 12, 4, 0, 0, time.UTC),
		Process: &hidsmodel.Process{
			PID:     42,
			Image:   "/usr/bin/curl",
			Command: "curl https://example.com",
		},
		Network: &hidsmodel.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.5",
			SourcePort:      41000,
			DestAddress:     "1.1.1.1",
			DestPort:        443,
			ConnectionState: "ESTABLISHED",
		},
		Data: map[string]any{
			"old_connection_state": "SYN_SENT",
			"new_connection_state": "ESTABLISHED",
		},
	})
	if ok {
		t.Fatalf("expected network.state observation to stay node-local, got %#v", observation)
	}
}

func TestHIDSCapabilityHooksConvertObservationSupportsFileChange(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	hooks.setAlertConfig(hidsAlertConfig{
		capabilityKey: "hids",
		specVersion:   "2026-04-10",
	})

	observation, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      hidsmodel.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Now().UTC(),
		File: &hidsmodel.File{
			Path:      "/etc/passwd",
			Operation: "WRITE",
			IsDir:     false,
			Mode:      "-rw-r--r--",
		},
	})
	if !ok {
		t.Fatal("expected file.change observation to be exported")
	}
	if observation.EventType != hidsmodel.EventTypeFileChange {
		t.Fatalf("unexpected event type: %s", observation.EventType)
	}
	if len(observation.EventJSON) == 0 {
		t.Fatal("expected non-empty observation json")
	}
}

func TestHIDSCapabilityHooksConvertObservationSupportsAuditEvent(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	hooks.setAlertConfig(hidsAlertConfig{
		capabilityKey: "hids",
		specVersion:   "2026-04-10",
	})

	observation, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      hidsmodel.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Date(2026, 4, 10, 12, 6, 0, 0, time.UTC),
		Tags:      []string{"audit", "auditd", "login", "fail"},
		Audit: &hidsmodel.Audit{
			Sequence:    7,
			RecordTypes: []string{"USER_LOGIN"},
			Family:      "login",
			Category:    "user-login",
			RecordType:  "USER_LOGIN",
			Result:      "fail",
			Action:      "logged-in",
			Username:    "root",
			LoginUser:   "root",
			RemoteIP:    "10.0.0.5",
		},
		Data: map[string]any{
			"normalized": map[string]any{"result": "fail"},
		},
	})
	if !ok {
		t.Fatal("expected audit.event observation to be exported")
	}
	if observation.EventType != hidsmodel.EventTypeAudit {
		t.Fatalf("unexpected event type: %s", observation.EventType)
	}
	if len(observation.EventJSON) == 0 {
		t.Fatal("expected non-empty observation json")
	}
}

func TestHIDSCapabilityHooksConvertObservationSupportsAuditLoss(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	hooks.setAlertConfig(hidsAlertConfig{
		capabilityKey: "hids",
		specVersion:   "2026-04-10",
	})

	observation, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      hidsmodel.EventTypeAuditLoss,
		Source:    "auditd",
		Timestamp: time.Date(2026, 4, 10, 12, 7, 0, 0, time.UTC),
		Audit: &hidsmodel.Audit{
			Family:     "loss",
			Category:   "audit-daemon",
			RecordType: "EVENTS_LOST",
			Action:     "events-lost",
			Result:     "unknown",
		},
		Data: map[string]any{
			"lost_count": 32,
		},
	})
	if !ok {
		t.Fatal("expected audit.loss observation to be exported")
	}
	if observation.EventType != hidsmodel.EventTypeAuditLoss {
		t.Fatalf("unexpected event type: %s", observation.EventType)
	}
	if len(observation.EventJSON) == 0 {
		t.Fatal("expected non-empty observation json")
	}
}

func TestHIDSCapabilityHooksConvertObservationRejectsUnsupportedTypes(t *testing.T) {
	t.Parallel()

	hooks := &hidsCapabilityHooks{}
	if _, ok := hooks.convertObservation(hidsmodel.Event{
		Type:      "custom.unsupported",
		Timestamp: time.Now().UTC(),
	}); ok {
		t.Fatal("expected unsupported observation type to be rejected")
	}
}
