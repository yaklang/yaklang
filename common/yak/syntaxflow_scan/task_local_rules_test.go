package syntaxflow_scan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestParseTaskLocalSyntaxFlowRulesDoesNotUseProfileDatabase(t *testing.T) {
	oldProfileDB := consts.GetGormProfileDatabase()
	dbPath := filepath.Join(t.TempDir(), "closed-profile.db")
	closedProfileDB, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("create isolated profile DB: %v", err)
	}
	if err := closedProfileDB.Close(); err != nil {
		t.Fatalf("close isolated profile DB: %v", err)
	}
	consts.BindProfileDatabase(closedProfileDB, dbPath)
	t.Cleanup(func() { consts.BindProfileDatabase(oldProfileDB, "") })

	rules, libraries, err := parseTaskLocalSyntaxFlowRules([]*ypb.SyntaxFlowRuleInput{
		{
			RuleName: "snapshot-lib.sf",
			Content: `
desc(lib: "snapshot-only-lib", lang: java)
* as $output;
alert $output;
`,
			Language: "java",
		},
	})
	if err != nil {
		t.Fatalf("parse task-local rules with nil profile DB: %v", err)
	}
	if len(rules) != 1 || rules[0].RuleName != "snapshot-lib.sf" {
		t.Fatalf("unexpected parsed rules: %#v", rules)
	}
	if got := libraries["snapshot-only-lib"]; got != rules[0] {
		t.Fatalf("task-local library was not indexed: %#v", libraries)
	}
}

func TestLoadTaskLocalSyntaxFlowRulesVerifiesFileIdentity(t *testing.T) {
	payload, err := json.Marshal(ssaconfig.TaskLocalRuleInputFile{
		Version: ssaconfig.TaskLocalRuleInputFileVersionV1,
		Rules: []*ypb.SyntaxFlowRuleInput{
			{
				RuleName: "snapshot.sf", Content: `desc(title: "Content title", level: low);`,
				Language: "java", Tags: []string{"content-tag"}, Description: "content description",
			},
		},
		Metadata: map[string]ssaconfig.TaskLocalRuleMetadata{
			"snapshot.sf": {
				AssetID:     "asset-snapshot",
				Title:       "Published title",
				TitleZh:     "已发布标题",
				Severity:    "critical",
				RiskType:    "snapshot-risk",
				ContentHash: "published-content-hash",
				Groups:      []string{"published-group"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal task-local input: %v", err)
	}
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write task-local input: %v", err)
	}
	digest := sha256.Sum256(payload)
	config := &ssaconfig.SyntaxFlowRuleConfig{
		TaskLocal:            true,
		TaskLocalInputFile:   path,
		TaskLocalInputSHA256: hex.EncodeToString(digest[:]),
		TaskLocalInputCount:  1,
	}
	rules, _, err := loadTaskLocalSyntaxFlowRules(config)
	if err != nil {
		t.Fatalf("load task-local input: %v", err)
	}
	if len(rules) != 1 || rules[0].RuleName != "snapshot.sf" {
		t.Fatalf("unexpected task-local rules: %#v", rules)
	}
	if rules[0].RuleId != "asset-snapshot" || rules[0].Title != "Published title" ||
		string(rules[0].Severity) != "critical" || rules[0].RiskType != "snapshot-risk" ||
		rules[0].Hash != "published-content-hash" || rules[0].Language != ssaconfig.General ||
		rules[0].Tag != "" || rules[0].Description != "" || len(rules[0].Groups) != 1 ||
		rules[0].Groups[0].GroupName != "published-group" {
		t.Fatalf("published metadata was not preserved: %#v", rules[0])
	}

	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		t.Fatalf("tamper task-local input: %v", err)
	}
	if _, _, err := loadTaskLocalSyntaxFlowRules(config); err == nil {
		t.Fatal("expected tampered task-local input to fail closed")
	}
}

func TestLoadTaskLocalSyntaxFlowRulesRejectsBroadPermissions(t *testing.T) {
	payload, err := json.Marshal(ssaconfig.TaskLocalRuleInputFile{
		Version: ssaconfig.TaskLocalRuleInputFileVersionV1,
		Rules:   []*ypb.SyntaxFlowRuleInput{{RuleName: "snapshot.sf", Content: `desc(title: "Snapshot");`}},
		Metadata: map[string]ssaconfig.TaskLocalRuleMetadata{
			"snapshot.sf": {AssetID: "asset-snapshot"},
		},
	})
	if err != nil {
		t.Fatalf("marshal task-local input: %v", err)
	}
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write task-local input: %v", err)
	}
	digest := sha256.Sum256(payload)
	_, _, err = loadTaskLocalSyntaxFlowRules(&ssaconfig.SyntaxFlowRuleConfig{
		TaskLocal:            true,
		TaskLocalInputFile:   path,
		TaskLocalInputSHA256: hex.EncodeToString(digest[:]),
		TaskLocalInputCount:  1,
	})
	if err == nil {
		t.Fatal("expected broadly readable task-local input to fail closed")
	}
}

func TestParseTaskLocalSyntaxFlowRulesFailsClosedOnInvalidRule(t *testing.T) {
	_, _, err := parseTaskLocalSyntaxFlowRules([]*ypb.SyntaxFlowRuleInput{
		{RuleName: "broken.sf", Content: "this is not valid syntaxflow ("},
	})
	if err == nil {
		t.Fatal("expected invalid task-local rule to fail closed")
	}
}

func TestTaskLocalRuleUsesContentAlertsWhenBundleOmitsDerivedAlertMetadata(t *testing.T) {
	const ruleName = "snapshot-marker.sf"
	rules, _, err := parseTaskLocalSyntaxFlowRulesWithMetadata(
		[]*ypb.SyntaxFlowRuleInput{{
			RuleName: ruleName,
			Language: "java",
			Content: `desc(title: "Snapshot marker", type: vuln, level: low)
wmllhfUniqueMarker() as $sink;
alert $sink for {
  title: "Snapshot marker",
  level: "low",
  risk: "snapshot-marker",
  name: "snapshot-marker",
}`,
		}},
		map[string]ssaconfig.TaskLocalRuleMetadata{
			ruleName: {
				AssetID:  "asset-snapshot-marker",
				Title:    "Snapshot marker",
				Language: "java",
				Purpose:  "vuln",
				Severity: "low",
			},
		},
	)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Contains(t, rules[0].AlertDesc, "sink",
		"alert metadata derived from canonical rule content must survive when the bundle omits the redundant projection")

	programName := "task-local-snapshot-alert-test-" + uuid.NewString()
	program, err := ssaapi.Parse(`public class WmllhfUniqueFixture {
    private static void wmllhfUniqueMarker() {}
    public static void run() { wmllhfUniqueMarker(); }
}`,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(programName),
	)
	require.NoError(t, err)
	require.NotNil(t, program)
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), programName) })

	result, err := program.SyntaxFlowRule(rules[0], ssaapi.QueryWithMemory())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.GetAlertValue("sink"), 1,
		"canonical rule content must still match the unique source marker")
	require.Equal(t, 1, result.RiskCount(),
		"task-local snapshot execution must materialize risks declared by canonical rule content")
}

func TestRuleInputResultKindPreservesOrdinaryDebugSemantics(t *testing.T) {
	if got := ruleInputResultKind(false); got != schema.SFResultKindDebug {
		t.Fatalf("ordinary inline rule input kind changed: %s", got)
	}
	if got := ruleInputResultKind(true); got != schema.SFResultKindScan {
		t.Fatalf("task-local snapshot rule input kind: got=%s want=%s", got, schema.SFResultKindScan)
	}
}
