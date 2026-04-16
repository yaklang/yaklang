//go:build hids

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunAcceptsCompilableDesiredSpec(t *testing.T) {
	input := strings.NewReader(`{
		"mode": "observe",
		"collectors": {
			"process": {"enabled": true, "backend": "ebpf"}
		},
		"builtin_rule_sets": ["linux.process.baseline"],
		"temporary_rules": [
			{
				"rule_id": "tmp.contract.process",
				"enabled": true,
				"match_event_type": "process.exec",
				"severity": "medium",
				"condition": "str.Contains(process.command, 'sh')"
			}
		]
	}`)
	var output bytes.Buffer
	if err := run(nil, input, &output); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(output.String(), `"temporary_rule_count": 1`) {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestRunRejectsInvalidTemporaryRuleCondition(t *testing.T) {
	input := strings.NewReader(`{
		"mode": "observe",
		"collectors": {
			"process": {"enabled": true, "backend": "ebpf"}
		},
		"temporary_rules": [
			{
				"rule_id": "tmp.contract.bad",
				"enabled": true,
				"match_event_type": "process.exec",
				"condition": "process."
			}
		]
	}`)
	var output bytes.Buffer
	if err := run(nil, input, &output); err == nil {
		t.Fatal("expected invalid temporary rule condition")
	}
}
