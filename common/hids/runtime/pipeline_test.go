//go:build hids && linux

package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/rule"
)

func TestPipelineRunPublishesRuleAlerts(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-sensitive-write",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "medium",
				Condition:      "event.type == 'file.change' && file.path == '/tmp/secret.txt' && file.operation == 'WRITE'",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Now().UTC(),
		File: &model.File{
			Path:      "/tmp/secret.txt",
			Operation: "WRITE",
		},
	}
	close(events)

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.RuleID != "tmp-sensitive-write" {
			t.Fatalf("unexpected alert rule id: %s", alert.RuleID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for pipeline alert")
	}
}

func TestPipelineRunPublishesTemporaryRuleActionEvidenceRequests(t *testing.T) {
	t.Parallel()

	secretPath := fmt.Sprintf("%s/secret.txt", t.TempDir())
	if err := os.WriteFile(secretPath, []byte("sensitive-bytes"), 0o600); err != nil {
		t.Fatalf("write secret path: %v", err)
	}

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-sensitive-write-action",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "medium",
				Condition:      fmt.Sprintf("file.path == %q && file.operation == 'WRITE'", secretPath),
				Action:         `{"title":"Sensitive file write","detail":{"summary":"sensitive write observed"},"evidence_requests":[{"kind":"file","target":file.path,"reason":"capture artifact"}]}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine).withEvidencePolicy(model.EvidencePolicy{CaptureFileHash: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Now().UTC(),
		File: &model.File{
			Path:      secretPath,
			Operation: "WRITE",
		},
	}
	close(events)

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.Title != "Sensitive file write" {
			t.Fatalf("unexpected alert title: %s", alert.Title)
		}
		if alert.Detail["summary"] != "sensitive write observed" {
			t.Fatalf("unexpected summary: %#v", alert.Detail["summary"])
		}
		evidenceRequests, ok := alert.Detail["evidence_requests"].([]map[string]any)
		if !ok || len(evidenceRequests) != 1 {
			t.Fatalf("unexpected evidence requests: %#v", alert.Detail["evidence_requests"])
		}
		evidenceResults, ok := alert.Detail["evidence_results"].([]map[string]any)
		if !ok || len(evidenceResults) != 1 {
			t.Fatalf("unexpected evidence results: %#v", alert.Detail["evidence_results"])
		}
		artifact, ok := evidenceResults[0]["artifact"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected evidence artifact: %#v", evidenceResults[0]["artifact"])
		}
		if resolvedTarget, _ := evidenceResults[0]["resolved_target"].(string); resolvedTarget != secretPath {
			t.Fatalf("unexpected resolved target: %#v", evidenceResults[0]["resolved_target"])
		}
		if exists, _ := artifact["exists"].(bool); !exists {
			t.Fatalf("expected captured artifact to exist: %#v", artifact)
		}
		hashes, ok := artifact["hashes"].(map[string]any)
		if !ok || hashes["md5"] == "" {
			t.Fatalf("expected file hashes in evidence artifact: %#v", artifact["hashes"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for pipeline alert")
	}
}

func TestPipelineRunExecutesProcessTreeEvidenceRequests(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-tree-action",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Condition:      "process.parent_name == 'systemd' && process.name == 'bash'",
				Action:         `{"title":"Shell under systemd","evidence_requests":[{"kind":"process_tree","target":"process","reason":"capture lineage","metadata":{"pid":process.pid}}]}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine).withEvidencePolicy(model.EvidencePolicy{CaptureProcessTree: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 2)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		Process: &model.Process{
			PID:       10,
			ParentPID: 1,
			Name:      "systemd",
			Image:     "/usr/lib/systemd/systemd",
			Command:   "/usr/lib/systemd/systemd",
		},
	}
	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Date(2026, 4, 14, 12, 0, 1, 0, time.UTC),
		Process: &model.Process{
			PID:       20,
			ParentPID: 10,
			Name:      "bash",
			Image:     "/bin/bash",
			Command:   "/bin/bash -lc id",
		},
	}
	close(events)

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		evidenceResults, ok := alert.Detail["evidence_results"].([]map[string]any)
		if !ok || len(evidenceResults) != 1 {
			t.Fatalf("unexpected evidence results: %#v", alert.Detail["evidence_results"])
		}
		processTree, ok := evidenceResults[0]["process_tree"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected process tree evidence: %#v", evidenceResults[0]["process_tree"])
		}
		lineage, ok := processTree["lineage"].([]map[string]any)
		if !ok || len(lineage) != 2 {
			t.Fatalf("unexpected lineage: %#v", processTree["lineage"])
		}
		rootPID, _ := lineage[0]["pid"].(int)
		if rootPID != 10 {
			t.Fatalf("unexpected root pid: %#v", lineage[0]["pid"])
		}
		processDetail, ok := processTree["process"].(map[string]any)
		if !ok || processDetail["pid"] != 20 {
			t.Fatalf("unexpected process detail: %#v", processTree["process"])
		}
		children, ok := processTree["children"].([]map[string]any)
		if !ok {
			t.Fatalf("unexpected process tree children: %#v", processTree["children"])
		}
		if len(children) != 0 {
			t.Fatalf("expected no children for bash process: %#v", children)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for process tree evidence alert")
	}
}

func TestPipelineRunExecutesProcessMemoryEvidenceRequests(t *testing.T) {
	t.Parallel()

	currentPID := os.Getpid()
	currentExecutable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve current executable: %v", err)
	}

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-memory-action",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Condition:      fmt.Sprintf("process.pid == %d", currentPID),
				Action:         `{"title":"Process memory captured","evidence_requests":[{"kind":"process_memory","target":"process","reason":"capture memory layout","metadata":{"map_limit":4}}]}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine).withEvidencePolicy(model.EvidencePolicy{CaptureProcessMemory: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Now().UTC(),
		Process: &model.Process{
			PID:       currentPID,
			ParentPID: os.Getppid(),
			Name:      filepath.Base(currentExecutable),
			Image:     currentExecutable,
			Command:   currentExecutable,
		},
	}
	close(events)

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		evidenceResults, ok := alert.Detail["evidence_results"].([]map[string]any)
		if !ok || len(evidenceResults) != 1 {
			t.Fatalf("unexpected evidence results: %#v", alert.Detail["evidence_results"])
		}
		memory, ok := evidenceResults[0]["process_memory"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected process memory evidence: %#v", evidenceResults[0]["process_memory"])
		}
		processDetail, ok := memory["process"].(map[string]any)
		if !ok || processDetail["pid"] != currentPID {
			t.Fatalf("unexpected process detail: %#v", memory["process"])
		}
		status, ok := memory["status"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected status payload: %#v", memory["status"])
		}
		if threads, ok := status["threads"].(int64); !ok || threads < 1 {
			t.Fatalf("expected process memory threads summary: %#v", status)
		}
		if vmRSS, ok := status["vm_rss_kb"].(int64); !ok || vmRSS <= 0 {
			t.Fatalf("expected process memory RSS summary: %#v", status)
		}
		maps, ok := memory["maps"].([]map[string]any)
		if !ok || len(maps) == 0 {
			t.Fatalf("expected sampled memory maps: %#v", memory["maps"])
		}
		if sampledCount, ok := memory["sampled_map_count"].(int); !ok || sampledCount < 1 || sampledCount > 4 {
			t.Fatalf("unexpected sampled map count: %#v", memory["sampled_map_count"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for process memory evidence alert")
	}
}

func TestPipelineRunRejectsProcessMemoryEvidenceWhenPolicyDisabled(t *testing.T) {
	t.Parallel()

	currentPID := os.Getpid()
	currentExecutable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve current executable: %v", err)
	}

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-memory-disabled",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "medium",
				Condition:      fmt.Sprintf("process.pid == %d", currentPID),
				Action:         `{"title":"Process memory denied","evidence_requests":[{"kind":"process_memory","target":"process"}]}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Now().UTC(),
		Process: &model.Process{
			PID:       currentPID,
			ParentPID: os.Getppid(),
			Name:      filepath.Base(currentExecutable),
			Image:     currentExecutable,
			Command:   currentExecutable,
		},
	}
	close(events)

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if _, exists := alert.Detail["evidence_results"]; exists {
			t.Fatalf("did not expect evidence results: %#v", alert.Detail["evidence_results"])
		}
		evidenceErrors, ok := alert.Detail["evidence_errors"].([]map[string]any)
		if !ok || len(evidenceErrors) != 1 {
			t.Fatalf("unexpected evidence errors: %#v", alert.Detail["evidence_errors"])
		}
		if !strings.Contains(fmt.Sprint(evidenceErrors[0]["error"]), "capture_process_memory") {
			t.Fatalf("unexpected process memory evidence error: %#v", evidenceErrors[0])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for disabled process memory evidence alert")
	}
}

func TestPipelineRunExecutesSingleFileScanEvidenceRequests(t *testing.T) {
	t.Parallel()

	scanFile := filepath.Join(t.TempDir(), "payload.bin")
	if err := copyCurrentExecutable(scanFile); err != nil {
		t.Fatalf("copy scan file: %v", err)
	}

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-single-file-scan",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "medium",
				Condition:      fmt.Sprintf("process.image == %q", scanFile),
				Action:         `{"title":"single file scan","evidence_requests":[{"kind":"single_file_scan","target":"process.image","reason":"scan executable","metadata":{"scan_match":"list.Contains(scan.matched_rules, 'linux.scan.writable_tmp_elf_artifact')","finding_match":"finding.rule_id == 'linux.scan.writable_tmp_elf_artifact'","matched_only":true}}]}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine).withEvidencePolicy(model.EvidencePolicy{CaptureFileHash: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Now().UTC(),
		Process: &model.Process{
			PID:       901,
			ParentPID: 1,
			Name:      "payload",
			Image:     scanFile,
			Command:   scanFile,
		},
	}
	close(events)

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.Severity != "high" {
			t.Fatalf("expected promoted severity, got %q", alert.Severity)
		}
		if !containsString(alert.Tags, "scan") || !containsString(alert.Tags, "tmp") {
			t.Fatalf("expected promoted tags on alert: %#v", alert.Tags)
		}
		evidenceResults, ok := alert.Detail["evidence_results"].([]map[string]any)
		if !ok || len(evidenceResults) != 1 {
			t.Fatalf("unexpected evidence results: %#v", alert.Detail["evidence_results"])
		}
		if evidenceResults[0]["resolved_target"] != scanFile {
			t.Fatalf("unexpected resolved target: %#v", evidenceResults[0]["resolved_target"])
		}
		scan, ok := evidenceResults[0]["scan"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected scan payload: %#v", evidenceResults[0]["scan"])
		}
		artifact, ok := scan["artifact"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected single-file scan artifact: %#v", scan["artifact"])
		}
		if hashes, ok := artifact["hashes"].(map[string]any); !ok || hashes["sha256"] == "" {
			t.Fatalf("expected hashed scan artifact: %#v", artifact["hashes"])
		}
		matchedRules, ok := scan["matched_rules"].([]string)
		if !ok || len(matchedRules) == 0 {
			t.Fatalf("expected matched rules for single-file scan: %#v", scan["matched_rules"])
		}
		if !containsString(matchedRules, "linux.scan.writable_tmp_elf_artifact") {
			t.Fatalf("expected tmp ELF matched rule: %#v", matchedRules)
		}
		scanMatch, ok := scan["scan_match"].(map[string]any)
		if !ok || scanMatch["matched"] != true {
			t.Fatalf("expected scan_match annotation: %#v", scan["scan_match"])
		}
		findings, ok := scan["findings"].([]map[string]any)
		if !ok || len(findings) == 0 {
			t.Fatalf("expected findings for single-file scan: %#v", scan["findings"])
		}
		if matchedOnly, _ := scan["matched_only"].(bool); !matchedOnly {
			t.Fatalf("expected matched_only scan output: %#v", scan)
		}
		if matchedFindings, ok := scan["matched_findings"].([]map[string]any); !ok || len(matchedFindings) != 1 {
			t.Fatalf("expected matched findings: %#v", scan["matched_findings"])
		}
		promotion, ok := alert.Detail["scan_promotion"].(map[string]any)
		if !ok {
			t.Fatalf("expected scan promotion summary: %#v", alert.Detail["scan_promotion"])
		}
		if promotion["highest_severity"] != "high" {
			t.Fatalf("unexpected promotion severity: %#v", promotion["highest_severity"])
		}
		if findings[0]["rule_id"] != "linux.scan.writable_tmp_elf_artifact" {
			t.Fatalf("unexpected single-file finding: %#v", findings[0])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for single-file scan alert")
	}
}

func TestPipelineRunExecutesDirectoryScanEvidenceRequests(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	firstFile := filepath.Join(rootDir, "a.sh")
	secondFile := filepath.Join(rootDir, "b.bin")
	nestedDir := filepath.Join(rootDir, "nested")
	nestedFile := filepath.Join(nestedDir, "c.txt")
	sshDir := filepath.Join(rootDir, ".ssh")
	authKeys := filepath.Join(sshDir, "authorized_keys")
	if err := copyCurrentExecutable(firstFile); err != nil {
		t.Fatalf("write first file: %v", err)
	}
	if err := os.WriteFile(secondFile, []byte("bin"), 0o644); err != nil {
		t.Fatalf("write second file: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(nestedFile, []byte("nested"), 0o600); err != nil {
		t.Fatalf("write nested file: %v", err)
	}
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		t.Fatalf("mkdir ssh: %v", err)
	}
	if err := os.WriteFile(authKeys, []byte("ssh-ed25519 AAAATEST"), 0o600); err != nil {
		t.Fatalf("write authorized_keys: %v", err)
	}

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-directory-scan",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "medium",
				Condition:      fmt.Sprintf("file.path == %q", firstFile),
				Action:         fmt.Sprintf(`{"title":"directory scan","evidence_requests":[{"kind":"directory_scan","reason":"scan sibling tree","metadata":{"path":%q,"recursive":true,"max_entries":8,"max_depth":2,"entry_match":"artifact.IsELF(entry.artifact)","finding_match":"finding.rule_id == 'linux.scan.authorized_keys_artifact'","matched_only":true}}]}`, rootDir),
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine).withEvidencePolicy(model.EvidencePolicy{CaptureFileHash: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Now().UTC(),
		File: &model.File{
			Path:      firstFile,
			Operation: "WRITE",
		},
	}
	close(events)

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.Severity != "high" {
			t.Fatalf("expected directory scan promotion to raise severity, got %q", alert.Severity)
		}
		if !containsString(alert.Tags, "ssh") {
			t.Fatalf("expected promoted ssh tag on alert: %#v", alert.Tags)
		}
		evidenceResults, ok := alert.Detail["evidence_results"].([]map[string]any)
		if !ok || len(evidenceResults) != 1 {
			t.Fatalf("unexpected evidence results: %#v", alert.Detail["evidence_results"])
		}
		scan, ok := evidenceResults[0]["scan"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected directory scan payload: %#v", evidenceResults[0]["scan"])
		}
		if truncated, _ := scan["truncated"].(bool); truncated {
			t.Fatalf("did not expect truncated directory scan: %#v", scan)
		}
		target, ok := scan["target"].(map[string]any)
		if !ok || target["file_type"] != "directory" {
			t.Fatalf("unexpected directory target artifact: %#v", scan["target"])
		}
		entries, ok := scan["entries"].([]map[string]any)
		if !ok {
			t.Fatalf("unexpected directory entries: %#v", scan["entries"])
		}
		if len(entries) != 1 {
			t.Fatalf("expected entries filtered by matched_only: %#v", entries)
		}
		matchedRules, ok := scan["matched_rules"].([]string)
		if !ok || len(matchedRules) < 2 {
			t.Fatalf("expected aggregated matched rules: %#v", scan["matched_rules"])
		}
		if !containsString(matchedRules, "linux.scan.writable_tmp_elf_artifact") ||
			!containsString(matchedRules, "linux.scan.authorized_keys_artifact") {
			t.Fatalf("unexpected aggregated matched rules: %#v", matchedRules)
		}
		matchedEntries, ok := scan["matched_entries"].([]map[string]any)
		if !ok || len(matchedEntries) != 1 {
			t.Fatalf("expected exactly one matched entry: %#v", scan["matched_entries"])
		}
		if matchedEntries[0]["relative_path"] != "a.sh" {
			t.Fatalf("unexpected matched entry: %#v", matchedEntries[0])
		}
		matchedFindings, ok := scan["matched_findings"].([]map[string]any)
		if !ok || len(matchedFindings) != 1 {
			t.Fatalf("expected exactly one matched finding: %#v", scan["matched_findings"])
		}
		if matchedFindings[0]["rule_id"] != "linux.scan.authorized_keys_artifact" {
			t.Fatalf("unexpected matched finding: %#v", matchedFindings[0])
		}
		findings, ok := scan["findings"].([]map[string]any)
		if !ok || len(findings) != 1 {
			t.Fatalf("expected aggregated findings: %#v", scan["findings"])
		}
		if findings[0]["rule_id"] != "linux.scan.authorized_keys_artifact" {
			t.Fatalf("unexpected filtered findings: %#v", findings)
		}
		promotion, ok := alert.Detail["scan_promotion"].(map[string]any)
		if !ok {
			t.Fatalf("expected scan promotion summary: %#v", alert.Detail["scan_promotion"])
		}
		if promotion["highest_severity"] != "high" {
			t.Fatalf("unexpected promotion severity: %#v", promotion["highest_severity"])
		}
		if categories, ok := promotion["categories"].([]string); !ok || !containsString(categories, "credential_access") {
			t.Fatalf("expected credential_access category: %#v", promotion["categories"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for directory scan alert")
	}
}

func TestPipelineRunExecutesWritableTmpELFTemplateAction(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	payloadPath := filepath.Join(rootDir, "payload")
	if err := copyCurrentExecutable(payloadPath); err != nil {
		t.Fatalf("copy payload: %v", err)
	}

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-writable-tmp-elf-template",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "high",
				Condition:      fmt.Sprintf("file.path == %q && file.operation == 'WRITE'", payloadPath),
				Action:         `{"detail":{"summary":"ELF artifact dropped into writable temporary path"},"evidence_requests":[{"kind":"single_file_scan","target":"file.path","reason":"scan dropped executable","metadata":{"matched_only":true}},{"kind":"directory_scan","target":"file.parent","reason":"scan sibling writable directory","metadata":{"recursive":true,"max_entries":16,"max_depth":2,"entry_match":"artifact.IsELF(entry.artifact)","matched_only":true}}]}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine).withEvidencePolicy(model.EvidencePolicy{CaptureFileHash: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Now().UTC(),
		File: &model.File{
			Path:      payloadPath,
			Operation: "WRITE",
		},
	}
	close(events)

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.Detail["summary"] != "ELF artifact dropped into writable temporary path" {
			t.Fatalf("unexpected action summary: %#v", alert.Detail["summary"])
		}
		evidenceResults, ok := alert.Detail["evidence_results"].([]map[string]any)
		if !ok || len(evidenceResults) != 2 {
			t.Fatalf("unexpected evidence results: %#v", alert.Detail["evidence_results"])
		}

		fileScan := evidenceResults[0]
		if fileScan["kind"] != "single_file_scan" || fileScan["resolved_target"] != payloadPath {
			t.Fatalf("unexpected single file evidence: %#v", fileScan)
		}
		fileScanSummary, ok := fileScan["scan"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected single file scan summary: %#v", fileScan["scan"])
		}
		if matchedRules, ok := fileScanSummary["matched_rules"].([]string); !ok ||
			!containsString(matchedRules, "linux.scan.writable_tmp_elf_artifact") {
			t.Fatalf("expected writable tmp ELF scan match: %#v", fileScanSummary["matched_rules"])
		}

		directoryScan := evidenceResults[1]
		if directoryScan["kind"] != "directory_scan" || directoryScan["resolved_target"] != rootDir {
			t.Fatalf("unexpected directory evidence: %#v", directoryScan)
		}
		directoryScanSummary, ok := directoryScan["scan"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected directory scan summary: %#v", directoryScan["scan"])
		}
		matchedEntries, ok := directoryScanSummary["matched_entries"].([]map[string]any)
		if !ok || len(matchedEntries) != 1 {
			t.Fatalf("expected one matched ELF entry: %#v", directoryScanSummary["matched_entries"])
		}
		if matchedEntries[0]["relative_path"] != "payload" {
			t.Fatalf("unexpected matched entry: %#v", matchedEntries[0])
		}
		if matchedOnly, _ := directoryScanSummary["matched_only"].(bool); !matchedOnly {
			t.Fatalf("expected matched_only directory scan: %#v", directoryScanSummary)
		}

		promotion, ok := alert.Detail["scan_promotion"].(map[string]any)
		if !ok {
			t.Fatalf("expected scan promotion from template action: %#v", alert.Detail["scan_promotion"])
		}
		if categories, ok := promotion["categories"].([]string); !ok || !containsString(categories, "dropper") {
			t.Fatalf("expected dropper scan promotion category: %#v", promotion["categories"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for writable tmp ELF template alert")
	}
}

func TestPromoteAlertFromEvidenceIgnoresEvidenceOnlyScanFindings(t *testing.T) {
	t.Parallel()

	p := newPipeline(nil)
	alert := model.Alert{
		RuleID:   "tmp-nonpromoted-scan",
		Severity: "medium",
		Title:    "scan only",
		Tags:     []string{"temporary"},
		Detail: map[string]any{
			"evidence_results": []map[string]any{
				{
					"kind": "single_file_scan",
					"scan": map[string]any{
						"matched_rules": []string{"linux.scan.system_elf_artifact"},
						"findings": []map[string]any{
							{
								"rule_id":  "linux.scan.system_elf_artifact",
								"severity": "medium",
								"title":    "system ELF artifact found during bounded scan",
								"tags":     []string{"builtin", "scan", "file", "system", "artifact", "elf"},
							},
						},
					},
				},
			},
		},
	}

	promoted := p.promoteAlertFromEvidence(alert)
	if promoted.Severity != "medium" {
		t.Fatalf("unexpected severity change: %q", promoted.Severity)
	}
	if len(promoted.Tags) != 1 || promoted.Tags[0] != "temporary" {
		t.Fatalf("unexpected tag change: %#v", promoted.Tags)
	}
	if _, exists := promoted.Detail["scan_promotion"]; exists {
		t.Fatalf("did not expect scan promotion detail: %#v", promoted.Detail["scan_promotion"])
	}
}

func copyCurrentExecutable(dst string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	content, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return fmt.Errorf("current executable is empty")
	}
	if !bytes.HasPrefix(content, []byte{0x7f, 'E', 'L', 'F'}) {
		return fmt.Errorf("current executable is not an ELF binary")
	}
	return os.WriteFile(dst, content, 0o755)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestPipelineRunEvaluatesTemporaryRulesAcrossEventFamilies(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-family",
				Title:          "Process family alert",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "medium",
				Condition:      "process.parent_name == 'nginx' && str.HasSuffix(process.image, 'sh')",
			},
			{
				RuleID:         "tmp-network-family",
				Title:          "Network family alert",
				Enabled:        true,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "medium",
				Condition:      "network.dest_address == '1.1.1.1' && network.dest_port == 443",
			},
			{
				RuleID:         "tmp-file-family",
				Title:          "File family alert",
				Enabled:        true,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "medium",
				Condition:      "file.path == '/tmp/family.txt' && file.operation == 'WRITE'",
			},
			{
				RuleID:         "tmp-audit-family",
				Title:          "Audit family alert",
				Enabled:        true,
				MatchEventType: model.EventTypeAudit,
				Severity:       "medium",
				Condition:      "audit.family == 'login' && audit.result == 'fail' && audit.remote_ip == '10.0.0.5'",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 4)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
		Tags:      []string{"process", "ebpf"},
		Process: &model.Process{
			PID:        101,
			ParentPID:  1,
			Image:      "/bin/sh",
			Command:    "/bin/sh -c id",
			ParentName: "nginx",
		},
	}
	events <- model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 14, 10, 0, 1, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "outbound"},
		Process: &model.Process{
			PID:        102,
			ParentPID:  1,
			Name:       "curl",
			Username:   "root",
			Image:      "/usr/bin/curl",
			Command:    "curl https://example.com",
			ParentName: "bash",
		},
		Network: &model.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.5",
			SourcePort:      41000,
			DestAddress:     "1.1.1.1",
			DestPort:        443,
			ConnectionState: "ESTABLISHED",
		},
	}
	events <- model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: time.Date(2026, 4, 14, 10, 0, 2, 0, time.UTC),
		Tags:      []string{"file", "filewatch"},
		File: &model.File{
			Path:      "/tmp/family.txt",
			Operation: "WRITE",
		},
	}
	events <- model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Date(2026, 4, 14, 10, 0, 3, 0, time.UTC),
		Tags:      []string{"audit", "login", "fail"},
		Audit: &model.Audit{
			Family:   "login",
			Result:   "fail",
			Username: "root",
			RemoteIP: "10.0.0.5",
		},
	}
	close(events)

	wantTitles := map[string]string{
		"tmp-process-family": "Process family alert",
		"tmp-network-family": "Network family alert",
		"tmp-file-family":    "File family alert",
		"tmp-audit-family":   "Audit family alert",
	}
	deadline := time.After(3 * time.Second)
	for len(wantTitles) > 0 {
		select {
		case alert, ok := <-p.Alerts():
			if !ok {
				t.Fatalf("alert channel closed before all families matched: %#v", wantTitles)
			}
			wantTitle, exists := wantTitles[alert.RuleID]
			if !exists {
				continue
			}
			if alert.Title != wantTitle {
				t.Fatalf("unexpected alert title for %s: %q", alert.RuleID, alert.Title)
			}
			delete(wantTitles, alert.RuleID)
		case <-deadline:
			t.Fatalf("timed out waiting for family alerts: %#v", wantTitles)
		}
	}
}

func TestPipelineRunEnrichesAuditFileDriftAndPublishesBuiltinAlert(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.file.integrity"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 2)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Tags:      []string{"audit", "file"},
		File: &model.File{
			Path:      "/etc/sudoers",
			Operation: "chmod",
			Mode:      "-r--------",
		},
		Audit: &model.Audit{
			Family:    "file",
			Action:    "chmod",
			FileMode:  "-r--------",
			FileUID:   "0",
			FileGID:   "0",
			FileOwner: "root",
			FileGroup: "root",
		},
	}
	events <- model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: time.Date(2026, 4, 10, 12, 0, 5, 0, time.UTC),
		Tags:      []string{"audit", "file"},
		File: &model.File{
			Path:      "/etc/sudoers",
			Operation: "chmod",
			Mode:      "-rw-------",
		},
		Audit: &model.Audit{
			Family:    "file",
			Action:    "chmod",
			FileMode:  "-rw-------",
			FileUID:   "0",
			FileGID:   "0",
			FileOwner: "root",
			FileGroup: "root",
		},
	}
	close(events)

	var secondObservation model.Event
	for seen := 0; seen < 2; seen++ {
		select {
		case observation, ok := <-p.Observations():
			if !ok {
				t.Fatal("expected observation before pipeline close")
			}
			if observation.Timestamp.Equal(time.Date(2026, 4, 10, 12, 0, 5, 0, time.UTC)) {
				secondObservation = observation
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for audit observations")
		}
	}

	if secondObservation.Audit == nil {
		t.Fatal("expected enriched audit payload")
	}
	if secondObservation.Audit.PreviousFileMode != "-r--------" {
		t.Fatalf("unexpected previous file mode: %q", secondObservation.Audit.PreviousFileMode)
	}

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.RuleID != "linux.file.sensitive_permission_drift" {
			t.Fatalf("unexpected alert rule id: %s", alert.RuleID)
		}
		if alert.Detail["summary"] != "mode -r-------- -> -rw-------" {
			t.Fatalf("unexpected alert summary: %#v", alert.Detail["summary"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for builtin file drift alert")
	}
}

func TestPipelineRunPublishesObservations(t *testing.T) {
	t.Parallel()

	p := newPipeline(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	expected := model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf",
		Timestamp: time.Now().UTC(),
		Process: &model.Process{
			PID:        42,
			ParentPID:  1,
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc whoami",
			ParentName: "systemd",
		},
	}
	events <- expected
	close(events)

	select {
	case observation, ok := <-p.Observations():
		if !ok {
			t.Fatal("expected observation before pipeline close")
		}
		if observation.Type != expected.Type {
			t.Fatalf("unexpected observation type: %s", observation.Type)
		}
		if observation.Process == nil || observation.Process.Image != expected.Process.Image {
			t.Fatalf("unexpected process observation: %#v", observation.Process)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for pipeline observation")
	}
}

func TestPipelineRunEnrichesProcessExitFromTrackedLifecycle(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-exit-bash",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExit,
				Severity:       "medium",
				Condition:      "str.HasSuffix(process.image, 'bash') && process.parent_name == 'systemd'",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 2)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC),
		Tags:      []string{"process", "ebpf"},
		Process: &model.Process{
			PID:        42,
			ParentPID:  1,
			Name:       "bash",
			Username:   "root",
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc whoami",
			ParentName: "systemd",
		},
	}
	events <- model.Event{
		Type:      model.EventTypeProcessExit,
		Source:    "ebpf.process",
		Timestamp: time.Date(2026, 4, 10, 10, 0, 2, 0, time.UTC),
		Tags:      []string{"process", "ebpf", "exit"},
		Process: &model.Process{
			PID:  42,
			Name: "bash",
		},
	}
	close(events)

	var exitObservation model.Event
	for seen := 0; seen < 2; seen++ {
		select {
		case observation, ok := <-p.Observations():
			if !ok {
				t.Fatal("expected observation before pipeline close")
			}
			if observation.Type == model.EventTypeProcessExit {
				exitObservation = observation
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for pipeline observations")
		}
	}

	if exitObservation.Process == nil {
		t.Fatal("expected enriched exit observation process payload")
	}
	if exitObservation.Process.Image != "/bin/bash" {
		t.Fatalf("unexpected exit image: %q", exitObservation.Process.Image)
	}
	if exitObservation.Process.Command != "/bin/bash -lc whoami" {
		t.Fatalf("unexpected exit command: %q", exitObservation.Process.Command)
	}
	if exitObservation.Process.ParentName != "systemd" {
		t.Fatalf("unexpected exit parent name: %q", exitObservation.Process.ParentName)
	}
	if exitObservation.Process.Username != "root" {
		t.Fatalf("unexpected exit username: %q", exitObservation.Process.Username)
	}

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.RuleID != "tmp-process-exit-bash" {
			t.Fatalf("unexpected alert rule id: %s", alert.RuleID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for exit alert")
	}
}

func TestPipelineRunEnrichesProcessExecParentNameFromTrackedParent(t *testing.T) {
	t.Parallel()

	p := newPipeline(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 2)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "inventory.process",
		Timestamp: time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC),
		Tags:      []string{"process", "inventory", "baseline"},
		Process: &model.Process{
			PID:       100,
			ParentPID: 1,
			Name:      "bash",
			Username:  "alice",
			Image:     "/bin/bash",
			Command:   "/bin/bash",
		},
	}
	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Date(2026, 4, 13, 11, 0, 1, 0, time.UTC),
		Tags:      []string{"process", "ebpf"},
		Process: &model.Process{
			PID:       101,
			ParentPID: 100,
			Image:     "/usr/bin/curl",
			Command:   "curl https://example.com",
		},
	}
	close(events)

	var childObservation model.Event
	for seen := 0; seen < 2; seen++ {
		select {
		case observation, ok := <-p.Observations():
			if !ok {
				t.Fatal("expected observation before pipeline close")
			}
			if observation.Process != nil && observation.Process.PID == 101 {
				childObservation = observation
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for process observations")
		}
	}

	if childObservation.Process == nil {
		t.Fatal("expected child process observation")
	}
	if childObservation.Process.ParentName != "bash" {
		t.Fatalf("unexpected parent name: %q", childObservation.Process.ParentName)
	}
	if childObservation.Process.Name != "curl" {
		t.Fatalf("expected process name derived from image, got %q", childObservation.Process.Name)
	}
}

func TestPipelineRunEnrichesNetworkProcessFromTrackedExec(t *testing.T) {
	t.Parallel()

	p := newPipeline(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 2)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "ebpf.process",
		Timestamp: time.Date(2026, 4, 13, 11, 5, 0, 0, time.UTC),
		Tags:      []string{"process", "ebpf"},
		Process: &model.Process{
			PID:        42,
			ParentPID:  1,
			Name:       "curl",
			Username:   "alice",
			Image:      "/usr/bin/curl",
			Command:    "curl https://example.com",
			ParentName: "bash",
		},
	}
	events <- model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 13, 11, 5, 1, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "outbound"},
		Process: &model.Process{
			PID:  42,
			Name: "curl",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			SourceAddress: "10.0.0.5",
			SourcePort:    41000,
			DestAddress:   "1.1.1.1",
			DestPort:      443,
		},
		Data: map[string]any{"fd": 7},
	}
	close(events)

	var networkObservation model.Event
	for seen := 0; seen < 2; seen++ {
		select {
		case observation, ok := <-p.Observations():
			if !ok {
				t.Fatal("expected observation before pipeline close")
			}
			if observation.Type == model.EventTypeNetworkConnect {
				networkObservation = observation
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for network observations")
		}
	}

	if networkObservation.Process == nil {
		t.Fatal("expected network process context")
	}
	if networkObservation.Process.Image != "/usr/bin/curl" {
		t.Fatalf("unexpected image: %q", networkObservation.Process.Image)
	}
	if networkObservation.Process.Command != "curl https://example.com" {
		t.Fatalf("unexpected command: %q", networkObservation.Process.Command)
	}
	if networkObservation.Process.Username != "alice" {
		t.Fatalf("unexpected username: %q", networkObservation.Process.Username)
	}
	if networkObservation.Process.ParentName != "bash" {
		t.Fatalf("unexpected parent name: %q", networkObservation.Process.ParentName)
	}
}

func TestPipelineRunEnrichesNetworkCloseFromTrackedLifecycle(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-network-close",
				Enabled:        true,
				MatchEventType: model.EventTypeNetworkClose,
				Severity:       "low",
				Condition:      "network.dest_address == '1.1.1.1' && network.dest_port == 443 && network.connection_state == 'closed' && data.connection_age_seconds == 5 && data.previous_state_age_seconds == 5",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 2)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 11, 0, 0, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "outbound"},
		Process: &model.Process{
			PID:        42,
			ParentPID:  1,
			Name:       "curl",
			Username:   "root",
			Image:      "/usr/bin/curl",
			Command:    "curl https://example.com",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.5",
			SourcePort:      41000,
			DestAddress:     "1.1.1.1",
			DestPort:        443,
			ConnectionState: "ESTABLISHED",
		},
		Data: map[string]any{"fd": 7},
	}
	events <- model.Event{
		Type:      model.EventTypeNetworkClose,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 11, 0, 5, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "close"},
		Process: &model.Process{
			PID: 42,
		},
		Network: &model.Network{
			ConnectionState: "closed",
		},
		Data: map[string]any{"fd": 7},
	}
	close(events)

	var closeObservation model.Event
	for seen := 0; seen < 2; seen++ {
		select {
		case observation, ok := <-p.Observations():
			if !ok {
				t.Fatal("expected observation before pipeline close")
			}
			if observation.Type == model.EventTypeNetworkClose {
				closeObservation = observation
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for pipeline observations")
		}
	}

	if closeObservation.Process == nil || closeObservation.Network == nil {
		t.Fatalf("expected enriched network close observation: %#v", closeObservation)
	}
	if closeObservation.Process.Image != "/usr/bin/curl" {
		t.Fatalf("unexpected process image: %q", closeObservation.Process.Image)
	}
	if closeObservation.Network.DestAddress != "1.1.1.1" || closeObservation.Network.DestPort != 443 {
		t.Fatalf(
			"unexpected destination: %s:%d",
			closeObservation.Network.DestAddress,
			closeObservation.Network.DestPort,
		)
	}
	if closeObservation.Network.ConnectionState != "closed" {
		t.Fatalf("unexpected close state: %q", closeObservation.Network.ConnectionState)
	}
	if got, _ := closeObservation.Data["connection_age_seconds"].(int64); got != 5 {
		t.Fatalf("unexpected connection age: %#v", closeObservation.Data["connection_age_seconds"])
	}
	if got, _ := closeObservation.Data["previous_state_age_seconds"].(int64); got != 5 {
		t.Fatalf("unexpected previous state age: %#v", closeObservation.Data["previous_state_age_seconds"])
	}

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.RuleID != "tmp-network-close" {
			t.Fatalf("unexpected alert rule id: %s", alert.RuleID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for network close alert")
	}
}

func TestPipelineRunPublishesNetworkAcceptObservationAndAlert(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-network-accept",
				Enabled:        true,
				MatchEventType: model.EventTypeNetworkAccept,
				Severity:       "low",
				Condition:      "network.source_port == 22 && network.connection_state == 'ESTABLISHED'",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeNetworkAccept,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 11, 5, 0, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "inbound", "accept"},
		Process: &model.Process{
			PID:        88,
			ParentPID:  1,
			Name:       "sshd",
			Username:   "root",
			Image:      "/usr/sbin/sshd",
			Command:    "/usr/sbin/sshd -D",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.10",
			SourcePort:      22,
			DestAddress:     "192.168.1.7",
			DestPort:        51123,
			ConnectionState: "ESTABLISHED",
		},
		Data: map[string]any{"fd": 9},
	}
	close(events)

	select {
	case observation, ok := <-p.Observations():
		if !ok {
			t.Fatal("expected observation before pipeline close")
		}
		if observation.Type != model.EventTypeNetworkAccept {
			t.Fatalf("unexpected observation type: %s", observation.Type)
		}
		if observation.Network == nil || observation.Network.SourcePort != 22 {
			t.Fatalf("unexpected accept observation: %#v", observation.Network)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for accept observation")
	}

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.RuleID != "tmp-network-accept" {
			t.Fatalf("unexpected alert rule id: %s", alert.RuleID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for accept alert")
	}
}

func TestPipelineRunPublishesNetworkStateObservationAndAlert(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-network-state",
				Enabled:        true,
				MatchEventType: model.EventTypeNetworkState,
				Severity:       "medium",
				Condition:      "network.connection_state == 'ESTABLISHED' && data.previous_connection_state == 'SYN_SENT'",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 2)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 11, 10, 0, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "outbound"},
		Process: &model.Process{
			PID:        42,
			ParentPID:  1,
			Name:       "curl",
			Username:   "root",
			Image:      "/usr/bin/curl",
			Command:    "curl https://example.com",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.5",
			SourcePort:      41000,
			DestAddress:     "1.1.1.1",
			DestPort:        443,
			ConnectionState: "SYN_SENT",
		},
		Data: map[string]any{"fd": 7},
	}
	events <- model.Event{
		Type:      model.EventTypeNetworkState,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 11, 10, 1, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "state"},
		Network: &model.Network{
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
	}
	close(events)

	var stateObservation model.Event
	for seen := 0; seen < 2; seen++ {
		select {
		case observation, ok := <-p.Observations():
			if !ok {
				t.Fatal("expected observation before pipeline close")
			}
			if observation.Type == model.EventTypeNetworkState {
				stateObservation = observation
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for pipeline observations")
		}
	}

	if stateObservation.Process == nil || stateObservation.Process.Image != "/usr/bin/curl" {
		t.Fatalf("expected state observation to inherit process context: %#v", stateObservation.Process)
	}
	if stateObservation.Network == nil || stateObservation.Network.ConnectionState != "ESTABLISHED" {
		t.Fatalf("unexpected state observation network: %#v", stateObservation.Network)
	}
	if got, _ := stateObservation.Data["previous_connection_state"].(string); got != "SYN_SENT" {
		t.Fatalf("unexpected previous state: %#v", stateObservation.Data["previous_connection_state"])
	}
	if got, _ := stateObservation.Data["connection_age_seconds"].(int64); got != 1 {
		t.Fatalf("unexpected connection age: %#v", stateObservation.Data["connection_age_seconds"])
	}
	if got, _ := stateObservation.Data["previous_state_age_seconds"].(int64); got != 1 {
		t.Fatalf("unexpected previous state age: %#v", stateObservation.Data["previous_state_age_seconds"])
	}
	if got, _ := stateObservation.Data["state_age_seconds"].(int64); got != 0 {
		t.Fatalf("unexpected current state age: %#v", stateObservation.Data["state_age_seconds"])
	}

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.RuleID != "tmp-network-state" {
			t.Fatalf("unexpected alert rule id: %s", alert.RuleID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for state alert")
	}
}

func TestPipelineRunEnrichesNetworkObservationContext(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-network-enrich",
				Enabled:        true,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "medium",
				Condition:      "data.direction == 'outbound' && data.dest_scope == 'public' && data.dest_service == 'https' && list.Contains(data.process_roles, 'shell')",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 10, 11, 20, 0, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "outbound"},
		Process: &model.Process{
			PID:        42,
			ParentPID:  1,
			Name:       "bash",
			Username:   "root",
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc curl https://example.com",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.5",
			SourcePort:      41000,
			DestAddress:     "1.1.1.1",
			DestPort:        443,
			ConnectionState: "ESTABLISHED",
		},
		Data: map[string]any{"fd": 7},
	}
	close(events)

	select {
	case observation, ok := <-p.Observations():
		if !ok {
			t.Fatal("expected observation before pipeline close")
		}
		if got, _ := observation.Data["direction"].(string); got != "outbound" {
			t.Fatalf("unexpected direction: %#v", observation.Data["direction"])
		}
		if got, _ := observation.Data["dest_scope"].(string); got != "public" {
			t.Fatalf("unexpected dest_scope: %#v", observation.Data["dest_scope"])
		}
		if got, _ := observation.Data["dest_service"].(string); got != "https" {
			t.Fatalf("unexpected dest_service: %#v", observation.Data["dest_service"])
		}
		processRoles, ok := observation.Data["process_roles"].([]string)
		if !ok {
			t.Fatalf("unexpected process_roles payload: %#v", observation.Data["process_roles"])
		}
		if !containsStringValue(processRoles, "shell") {
			t.Fatalf("expected shell role in process_roles: %#v", processRoles)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for network observation")
	}

	select {
	case alert, ok := <-p.Alerts():
		if !ok {
			t.Fatal("expected alert before pipeline close")
		}
		if alert.RuleID != "tmp-network-enrich" {
			t.Fatalf("unexpected alert rule id: %s", alert.RuleID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for enrich alert")
	}
}

func TestPipelineRunMatchesBuiltinLongLivedPublicRemoteAdminSessionRule(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		BuiltinRuleSets: []string{"linux.network.baseline"},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 2)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "outbound"},
		Process: &model.Process{
			PID:        42,
			ParentPID:  1,
			Name:       "python3",
			Username:   "root",
			Image:      "/usr/bin/python3",
			Command:    "python3 reverse.py",
			ParentName: "systemd",
		},
		Network: &model.Network{
			Protocol:        "tcp",
			SourceAddress:   "10.0.0.5",
			SourcePort:      41000,
			DestAddress:     "203.0.113.10",
			DestPort:        22,
			ConnectionState: "ESTABLISHED",
		},
		Data: map[string]any{"fd": 7},
	}
	events <- model.Event{
		Type:      model.EventTypeNetworkClose,
		Source:    "ebpf.network",
		Timestamp: time.Date(2026, 4, 13, 10, 10, 0, 0, time.UTC),
		Tags:      []string{"network", "ebpf", "close"},
		Process: &model.Process{
			PID: 42,
		},
		Network: &model.Network{
			ConnectionState: "closed",
		},
		Data: map[string]any{"fd": 7},
	}
	close(events)

	found := false
	for alert := range p.Alerts() {
		if alert.RuleID == "linux.network.long_lived_public_remote_admin_session" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected long-lived public remote admin builtin alert")
	}
}

func TestPipelineRunDropsUnknownNetworkCloseWithoutIdentity(t *testing.T) {
	t.Parallel()

	p := newPipeline(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeNetworkClose,
		Source:    "ebpf.network",
		Timestamp: time.Now().UTC(),
		Tags:      []string{"network", "ebpf", "close"},
		Process:   &model.Process{PID: 42},
		Network:   &model.Network{ConnectionState: "closed"},
		Data:      map[string]any{"fd": 99},
	}
	close(events)

	select {
	case observation, ok := <-p.Observations():
		if ok {
			t.Fatalf("expected unknown network close to be dropped, got %#v", observation)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for observation channel to close")
	}
}

func TestPipelineRunSkipsAlertsForInventoryObservations(t *testing.T) {
	t.Parallel()

	engine, err := rule.NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-shell-under-systemd",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Condition:      "process.parent_name == 'systemd' && str.HasSuffix(process.image, 'bash')",
			},
		},
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	p := newPipeline(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan model.Event, 1)
	go p.Run(ctx, events)

	events <- model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    "inventory.process",
		Timestamp: time.Now().UTC(),
		Tags:      []string{"process", "inventory", "baseline"},
		Process: &model.Process{
			PID:        42,
			ParentPID:  1,
			Image:      "/bin/bash",
			Command:    "/bin/bash -lc whoami",
			ParentName: "systemd",
		},
	}
	close(events)

	select {
	case observation, ok := <-p.Observations():
		if !ok {
			t.Fatal("expected observation before pipeline close")
		}
		if observation.Source != "inventory.process" {
			t.Fatalf("unexpected observation source: %s", observation.Source)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for inventory observation")
	}

	select {
	case alert, ok := <-p.Alerts():
		if ok {
			t.Fatalf("expected inventory event to skip alerts, got %#v", alert)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for alert channel to close")
	}
}

type inventoryProviderStub struct {
	processEvents []model.Event
	networkEvents []model.Event
}

func containsStringValue(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func (s inventoryProviderStub) ListProcessEvents(context.Context) ([]model.Event, error) {
	return s.processEvents, nil
}

func (s inventoryProviderStub) ListNetworkEvents(context.Context) ([]model.Event, error) {
	return s.networkEvents, nil
}

func TestEmitInventoryObservationsSeedsProcessAndNetworkEvents(t *testing.T) {
	t.Parallel()

	sink := make(chan model.Event, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go emitInventoryObservations(ctx, model.DesiredSpec{
		Mode: model.ModeObserve,
		Collectors: model.Collectors{
			Process: model.CollectorSpec{
				Enabled: true,
				Backend: model.CollectorBackendEBPF,
			},
			Network: model.CollectorSpec{
				Enabled: true,
				Backend: model.CollectorBackendEBPF,
			},
		},
	}, inventoryProviderStub{
		processEvents: []model.Event{
			{
				Type:      model.EventTypeProcessExec,
				Source:    "inventory.process",
				Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
				Tags:      []string{"process", "inventory", "baseline"},
				Process: &model.Process{
					PID:        101,
					ParentPID:  1,
					Image:      "/usr/bin/sshd",
					Command:    "/usr/sbin/sshd -D",
					ParentName: "systemd",
				},
			},
		},
		networkEvents: []model.Event{
			{
				Type:      model.EventTypeNetworkConnect,
				Source:    "inventory.network",
				Timestamp: time.Date(2026, 4, 10, 12, 0, 1, 0, time.UTC),
				Tags:      []string{"network", "inventory", "baseline"},
				Process: &model.Process{
					PID:        202,
					ParentPID:  1,
					Image:      "/usr/bin/curl",
					Command:    "curl https://example.com",
					ParentName: "systemd",
				},
				Network: &model.Network{
					Protocol:        "tcp",
					SourceAddress:   "10.0.0.5",
					SourcePort:      41000,
					DestAddress:     "1.1.1.1",
					DestPort:        443,
					ConnectionState: "ESTABLISHED",
				},
			},
		},
	}, sink)

	wantTypes := map[string]bool{
		model.EventTypeProcessExec:    false,
		model.EventTypeNetworkConnect: false,
	}
	deadline := time.After(3 * time.Second)
	for remaining := len(wantTypes); remaining > 0; {
		select {
		case observation := <-sink:
			if _, exists := wantTypes[observation.Type]; exists && strings.Contains(observation.Source, "inventory.") {
				if !containsTag(observation.Tags, "inventory") {
					t.Fatalf("expected inventory tag on observation: %#v", observation.Tags)
				}
				if !wantTypes[observation.Type] {
					wantTypes[observation.Type] = true
					remaining--
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for seeded observations: %#v", wantTypes)
		}
	}
}

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
