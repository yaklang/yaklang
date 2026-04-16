package scannode

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestInspectExportedRuleArchiveCountsRulesGroupsAndRiskTypes(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	metaWriter, err := writer.Create("meta.json")
	if err != nil {
		t.Fatalf("create meta.json: %v", err)
	}
	if _, err := metaWriter.Write([]byte(`{"relationship":[{"rule_id":"rule-1","group_names":["OWASP A1","SQL"]},{"rule_id":"rule-2","group_names":["Auth"]}]}`)); err != nil {
		t.Fatalf("write meta.json: %v", err)
	}

	rule1Writer, err := writer.Create("rules/rule-1.json")
	if err != nil {
		t.Fatalf("create rule-1.json: %v", err)
	}
	if _, err := rule1Writer.Write([]byte(`{"RiskType":"sql-injection"}`)); err != nil {
		t.Fatalf("write rule-1.json: %v", err)
	}

	rule2Writer, err := writer.Create("rules/rule-2.json")
	if err != nil {
		t.Fatalf("create rule-2.json: %v", err)
	}
	if _, err := rule2Writer.Write([]byte(`{"RiskType":"weak-password"}`)); err != nil {
		t.Fatalf("write rule-2.json: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	inventory, err := inspectExportedRuleArchive(buffer.Bytes())
	if err != nil {
		t.Fatalf("inspect archive: %v", err)
	}
	if inventory.RuleCount != 2 {
		t.Fatalf("expected 2 rules, got %d", inventory.RuleCount)
	}
	if inventory.GroupCount != 3 {
		t.Fatalf("expected 3 groups, got %d", inventory.GroupCount)
	}
	if inventory.RiskTypeCount != 2 {
		t.Fatalf("expected 2 risk types, got %d", inventory.RiskTypeCount)
	}
}
