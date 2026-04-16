//go:build hids

package rule

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestEngineEvaluateMatchesTemporaryProcessArtifactRule(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(model.DesiredSpec{
		TemporaryRules: []model.TemporaryRule{
			{
				RuleID:         "tmp-process-artifact",
				Enabled:        true,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Condition:      "process.artifact.file_type == 'elf' && process.artifact.hashes.sha256 != '' && process.artifact.elf.machine != ''",
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
			PID:      42,
			Image:    "/usr/bin/bash",
			Command:  "/usr/bin/bash -lc whoami",
			Artifact: &model.Artifact{FileType: "elf", Hashes: &model.ArtifactHashes{SHA256: "abc"}, ELF: &model.ELFArtifact{Machine: "EM_X86_64"}},
		},
	})
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %d", len(alerts))
	}
	if alerts[0].RuleID != "tmp-process-artifact" {
		t.Fatalf("unexpected rule id: %s", alerts[0].RuleID)
	}
}
