//go:build hids

package builtin

import (
	"testing"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestDescribeRuleSetDerivesRequiredEventsFromRuntimeRules(t *testing.T) {
	t.Parallel()

	definition, ok := DescribeRuleSet("linux.file.integrity")
	if !ok {
		t.Fatal("expected linux.file.integrity definition")
	}
	if len(definition.RequiredEvents) != 2 ||
		definition.RequiredEvents[0] != model.EventTypeFileChange ||
		definition.RequiredEvents[1] != model.EventTypeAudit {
		t.Fatalf("unexpected required events: %#v", definition.RequiredEvents)
	}
	if len(definition.Rules) != 6 {
		t.Fatalf("unexpected rule count: %d", len(definition.Rules))
	}
	if definition.Rules[2].RuleID != "linux.file.sensitive_permission_drift" {
		t.Fatalf("unexpected audit-backed rule id: %s", definition.Rules[2].RuleID)
	}
	if definition.Rules[2].MatchEventType != model.EventTypeAudit {
		t.Fatalf("unexpected audit-backed rule event: %s", definition.Rules[2].MatchEventType)
	}
}

func TestDescribeRuleSetsReturnsCatalogOrder(t *testing.T) {
	t.Parallel()

	definitions := DescribeRuleSets()
	if len(definitions) != len(builtinRuleSetOrder) {
		t.Fatalf("unexpected definition count: %d", len(definitions))
	}
	for index, expected := range builtinRuleSetOrder {
		if definitions[index].RuleSet != expected {
			t.Fatalf("unexpected rule set at %d: %s", index, definitions[index].RuleSet)
		}
		if len(definitions[index].Rules) == 0 {
			t.Fatalf("expected rules for %s", definitions[index].RuleSet)
		}
	}
}

func TestDescribeRuleSetIncludesExpandedNetworkBaseline(t *testing.T) {
	t.Parallel()

	definition, ok := DescribeRuleSet("linux.network.baseline")
	if !ok {
		t.Fatal("expected linux.network.baseline definition")
	}
	if len(definition.RequiredEvents) != 3 ||
		definition.RequiredEvents[0] != model.EventTypeNetworkConnect ||
		definition.RequiredEvents[1] != model.EventTypeNetworkAccept ||
		definition.RequiredEvents[2] != model.EventTypeNetworkClose {
		t.Fatalf("unexpected required events: %#v", definition.RequiredEvents)
	}
	if len(definition.Rules) != 13 {
		t.Fatalf("unexpected rule count: %d", len(definition.Rules))
	}
	if definition.Rules[1].RuleID != "linux.network.shell_public_egress" {
		t.Fatalf("unexpected shell egress rule id: %s", definition.Rules[1].RuleID)
	}
	if definition.Rules[4].RuleID != "linux.network.interpreter_remote_admin_egress" {
		t.Fatalf("unexpected interpreter rule id: %s", definition.Rules[4].RuleID)
	}
	if definition.Rules[6].RuleID != "linux.network.metadata_service_access" {
		t.Fatalf("unexpected metadata service rule id: %s", definition.Rules[6].RuleID)
	}
	if definition.Rules[7].RuleID != "linux.network.web_process_proxy_tor_egress" {
		t.Fatalf("unexpected web proxy/tor rule id: %s", definition.Rules[7].RuleID)
	}
	if definition.Rules[8].RuleID != "linux.network.public_data_service_egress" {
		t.Fatalf("unexpected data-service egress rule id: %s", definition.Rules[8].RuleID)
	}
	if definition.Rules[9].RuleID != "linux.network.public_kubernetes_api_egress" {
		t.Fatalf("unexpected kubernetes api egress rule id: %s", definition.Rules[9].RuleID)
	}
	if definition.Rules[10].RuleID != "linux.network.unexpected_public_data_service_accept" {
		t.Fatalf("unexpected data-service accept rule id: %s", definition.Rules[10].RuleID)
	}
	if definition.Rules[10].MatchEventType != model.EventTypeNetworkAccept {
		t.Fatalf("unexpected accept rule event: %s", definition.Rules[10].MatchEventType)
	}
	if definition.Rules[11].RuleID != "linux.network.long_lived_public_remote_admin_session" {
		t.Fatalf("unexpected long-lived remote admin rule id: %s", definition.Rules[11].RuleID)
	}
	if definition.Rules[11].MatchEventType != model.EventTypeNetworkClose {
		t.Fatalf("unexpected long-lived remote admin event: %s", definition.Rules[11].MatchEventType)
	}
	if definition.Rules[12].RuleID != "linux.network.long_lived_proxy_tor_session" {
		t.Fatalf("unexpected long-lived proxy rule id: %s", definition.Rules[12].RuleID)
	}
}

func TestDescribeRuleSetClonesMutableFields(t *testing.T) {
	t.Parallel()

	definition, ok := DescribeRuleSet("linux.process.baseline")
	if !ok {
		t.Fatal("expected linux.process.baseline definition")
	}
	definition.Rules[0].Tags[0] = "mutated"
	definition.Examples[0] = "mutated"

	reloaded, ok := DescribeRuleSet("linux.process.baseline")
	if !ok {
		t.Fatal("expected linux.process.baseline definition")
	}
	if reloaded.Rules[0].Tags[0] == "mutated" {
		t.Fatalf("rule tags should be cloned: %#v", reloaded.Rules[0].Tags)
	}
	if reloaded.Examples[0] == "mutated" {
		t.Fatalf("examples should be cloned: %#v", reloaded.Examples)
	}
}

func TestDescribeRuleSetIncludesExpandedProcessBaseline(t *testing.T) {
	t.Parallel()

	definition, ok := DescribeRuleSet("linux.process.baseline")
	if !ok {
		t.Fatal("expected linux.process.baseline definition")
	}
	if len(definition.RequiredEvents) != 1 || definition.RequiredEvents[0] != model.EventTypeProcessExec {
		t.Fatalf("unexpected required events: %#v", definition.RequiredEvents)
	}
	if len(definition.Rules) != 7 {
		t.Fatalf("unexpected rule count: %d", len(definition.Rules))
	}
	if definition.Rules[2].RuleID != "linux.process.download_pipe_shell" {
		t.Fatalf("unexpected download pipe shell rule id: %s", definition.Rules[2].RuleID)
	}
	if definition.Rules[3].RuleID != "linux.process.reverse_shell_command" {
		t.Fatalf("unexpected reverse shell rule id: %s", definition.Rules[3].RuleID)
	}
	if definition.Rules[6].RuleID != "linux.process.account_management_command" {
		t.Fatalf("unexpected account management rule id: %s", definition.Rules[6].RuleID)
	}
}

func TestDescribeRuleSetIncludesExpandedAuditCore(t *testing.T) {
	t.Parallel()

	definition, ok := DescribeRuleSet("linux.audit.core")
	if !ok {
		t.Fatal("expected linux.audit.core definition")
	}
	if len(definition.RequiredEvents) != 1 || definition.RequiredEvents[0] != model.EventTypeAudit {
		t.Fatalf("unexpected required events: %#v", definition.RequiredEvents)
	}
	if len(definition.Rules) != 11 {
		t.Fatalf("unexpected rule count: %d", len(definition.Rules))
	}
	if definition.Rules[1].RuleID != "linux.audit.remote_root_login_success" {
		t.Fatalf("unexpected remote root login rule id: %s", definition.Rules[1].RuleID)
	}
	if definition.Rules[3].RuleID != "linux.audit.download_pipe_shell_command" {
		t.Fatalf("unexpected download pipe shell rule id: %s", definition.Rules[3].RuleID)
	}
	if definition.Rules[4].RuleID != "linux.audit.reverse_shell_command" {
		t.Fatalf("unexpected reverse shell rule id: %s", definition.Rules[4].RuleID)
	}
	if definition.Rules[7].RuleID != "linux.audit.account_management_command" {
		t.Fatalf("unexpected account management rule id: %s", definition.Rules[7].RuleID)
	}
	if definition.Rules[8].RuleID != "linux.audit.sensitive_file_mutation" {
		t.Fatalf("unexpected sensitive file mutation rule id: %s", definition.Rules[8].RuleID)
	}
}

func TestDescribeRuleSetIncludesArtifactDrivenFileRules(t *testing.T) {
	t.Parallel()

	definition, ok := DescribeRuleSet("linux.file.integrity")
	if !ok {
		t.Fatal("expected linux.file.integrity definition")
	}
	if definition.Rules[4].RuleID != "linux.file.writable_tmp_elf_drop" {
		t.Fatalf("unexpected tmp elf drop rule id: %s", definition.Rules[4].RuleID)
	}
	if definition.Rules[5].RuleID != "linux.file.system_elf_change" {
		t.Fatalf("unexpected system elf rule id: %s", definition.Rules[5].RuleID)
	}
	if definition.Rules[5].MatchEventType != model.EventTypeFileChange {
		t.Fatalf("unexpected system elf event type: %s", definition.Rules[5].MatchEventType)
	}
}
