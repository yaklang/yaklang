package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	ssaconfig "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type ruleSyncerStub struct {
	prepared    *PreparedRuleSnapshot
	err         error
	expectation RuleSnapshotExpectation
	callCount   int
}

func (s *ruleSyncerStub) PrepareSnapshot(
	_ context.Context,
	expectation RuleSnapshotExpectation,
) (*PreparedRuleSnapshot, error) {
	s.expectation = expectation
	s.callCount++
	if s.err != nil {
		return nil, s.err
	}
	return s.prepared, nil
}

func TestPrepareRuleSnapshotForExecutionInjectsTaskLocalRules(t *testing.T) {
	prepared := preparedRuleSnapshotFixture(t)
	stub := &ruleSyncerStub{prepared: prepared}
	node := &ScanNode{ruleSyncClient: stub}
	params := map[string]any{
		"config": `{"Mode":127,"SyntaxFlowRule":{"rule_names":["shared"],"rule_filter":{"RuleNames":["shared"]},"rule_input":[{"RuleName":"untrusted"}]}}`,
	}

	got, err := node.prepareRuleSnapshotForExecution(
		context.Background(),
		params,
		map[string]string{"rule_snapshot_id": "rulesnapshot-a"},
		nil,
	)
	if err != nil {
		t.Fatalf("prepare rule snapshot: %v", err)
	}
	if got == nil || got.Receipt.SnapshotID != "rulesnapshot-a" {
		t.Fatalf("unexpected prepared snapshot: %#v", got)
	}
	if info, statErr := os.Stat(got.taskYakitHome); statErr != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("task-local rule runtime is not private: info=%v err=%v", info, statErr)
	}
	taskYakitHome := got.taskYakitHome
	defer got.Cleanup()
	t.Cleanup(func() {
		if _, statErr := os.Stat(taskYakitHome); !os.IsNotExist(statErr) {
			t.Errorf("task-local rule runtime survived snapshot cleanup: %v", statErr)
		}
	})
	if stub.callCount != 1 || stub.expectation.SnapshotID != "rulesnapshot-a" {
		t.Fatalf("unexpected sync call: count=%d expectation=%#v", stub.callCount, stub.expectation)
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(params["config"].(string)), &config); err != nil {
		t.Fatalf("decode injected config: %v", err)
	}
	ruleConfig, ok := config["SyntaxFlowRule"].(map[string]any)
	if !ok {
		t.Fatalf("missing SyntaxFlowRule config: %#v", config)
	}
	if taskLocal, _ := ruleConfig["task_local"].(bool); !taskLocal {
		t.Fatalf("task_local marker missing: %#v", ruleConfig)
	}
	if _, exists := ruleConfig["rule_names"]; exists {
		t.Fatalf("shared rule_names survived injection: %#v", ruleConfig)
	}
	if _, exists := ruleConfig["rule_filter"]; exists {
		t.Fatalf("shared rule_filter survived injection: %#v", ruleConfig)
	}
	if _, exists := ruleConfig["rule_input"]; exists {
		t.Fatalf("snapshot rule content must not be inlined in process arguments: %#v", ruleConfig["rule_input"])
	}
	inputPath, _ := ruleConfig["task_local_input_file"].(string)
	if inputPath == "" || ruleConfig["task_local_input_count"] != float64(2) {
		t.Fatalf("task-local rule input file identity missing: %#v", ruleConfig)
	}
	info, statErr := os.Stat(inputPath)
	if statErr != nil {
		t.Fatalf("stat task-local rule input file: %v", statErr)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("task-local rule input mode=%#o, want 0600", gotMode)
	}
	fileBytes, readErr := os.ReadFile(inputPath)
	if readErr != nil {
		t.Fatalf("read task-local rule input file: %v", readErr)
	}
	var filePayload ssaconfig.TaskLocalRuleInputFile
	if err := json.Unmarshal(fileBytes, &filePayload); err != nil {
		t.Fatalf("decode task-local rule input file: %v", err)
	}
	if filePayload.Version != ssaconfig.TaskLocalRuleInputFileVersionV1 || len(filePayload.Rules) != 2 ||
		filePayload.Rules[0].GetRuleName() != "a-rule" || !strings.Contains(filePayload.Rules[0].GetContent(), "A Rule") {
		t.Fatalf("snapshot rules were not canonicalized into task-local file: %#v", filePayload)
	}
	if len(filePayload.Metadata) != 2 || filePayload.Metadata["a-rule"].AssetID != "asset-a" ||
		filePayload.Metadata["a-rule"].Severity != "critical" ||
		filePayload.Metadata["a-rule"].Title != "Published A" {
		t.Fatalf("published rule metadata was not preserved in task-local file: %#v", filePayload.Metadata)
	}

	var typedConfig ssaconfig.Config
	if err := json.Unmarshal([]byte(params["config"].(string)), &typedConfig); err != nil {
		t.Fatalf("decode injected config through scan contract: %v", err)
	}
	if !typedConfig.IsTaskLocalRuleInput() || len(typedConfig.GetRuleInput()) != 0 ||
		typedConfig.SyntaxFlowRule.TaskLocalInputFile != inputPath {
		t.Fatalf("scan contract did not preserve task-local snapshot input: %#v", typedConfig.SyntaxFlowRule)
	}
}

func TestCreateRuleSnapshotTaskYakitHomeIsPrivateAndRemoved(t *testing.T) {
	t.Parallel()

	dir, cleanup, err := createRuleSnapshotTaskYakitHome()
	if err != nil {
		t.Fatalf("create isolated task rule runtime: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat isolated task rule runtime: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("isolated task rule runtime mode=%#o, want 0700", got)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("isolated task rule runtime survived cleanup: %v", err)
	}
}

func TestPrepareRuleSnapshotForExecutionFailsClosed(t *testing.T) {
	stub := &ruleSyncerStub{err: errors.New("transport unavailable")}
	node := &ScanNode{ruleSyncClient: stub}
	params := map[string]any{"config": `{"Mode":127}`}

	prepared, err := node.prepareRuleSnapshotForExecution(
		context.Background(),
		params,
		map[string]string{"rule_snapshot_id": "rulesnapshot-a"},
		nil,
	)
	if prepared != nil {
		t.Fatalf("unexpected prepared snapshot: %#v", prepared)
	}
	var typed *ruleSnapshotPreparationError
	if !errors.As(err, &typed) || !strings.Contains(err.Error(), "transport unavailable") {
		t.Fatalf("expected fail-closed preparation error, got %v", err)
	}
	if params["config"] != `{"Mode":127}` {
		t.Fatalf("failed preparation mutated execution config: %#v", params)
	}
}

func TestPrepareRuleSnapshotForExecutionRejectsProtobufLegacyMismatch(t *testing.T) {
	node := &ScanNode{ruleSyncClient: &ruleSyncerStub{prepared: preparedRuleSnapshotFixture(t)}}
	_, err := node.prepareRuleSnapshotForExecution(
		context.Background(),
		map[string]any{"config": `{"Mode":127}`, "rule_snapshot_id": "legacy-a"},
		nil,
		&RuleSnapshotExpectation{SnapshotID: "proto-b"},
	)
	if err == nil || !strings.Contains(err.Error(), "protobuf and legacy rule snapshot snapshot_id mismatch") {
		t.Fatalf("expected identity mismatch, got %v", err)
	}
}

func TestPrepareRuleSnapshotForExecutionWithoutSnapshotIsNoop(t *testing.T) {
	stub := &ruleSyncerStub{}
	node := &ScanNode{ruleSyncClient: stub}
	params := map[string]any{"config": `{"Mode":127}`}
	prepared, err := node.prepareRuleSnapshotForExecution(context.Background(), params, nil, nil)
	if err != nil || prepared != nil || stub.callCount != 0 {
		t.Fatalf("unexpected no-snapshot result: prepared=%#v calls=%d err=%v", prepared, stub.callCount, err)
	}
}

func TestPrepareRuleSnapshotForScriptExecutionFailuresDoNotPublishPrepared(t *testing.T) {
	validItems := []RuleSnapshotItem{
		{AssetID: "asset-a", Name: "a-rule", Content: `desc(title: "A");`},
	}
	validDigest, err := calculateRuleSnapshotItemsSHA256(validItems)
	if err != nil {
		t.Fatalf("calculate valid digest: %v", err)
	}
	invalidItems := []RuleSnapshotItem{
		{AssetID: "asset-broken", Name: "broken-rule", Content: "this is not valid syntaxflow ("},
	}
	invalidDigest, err := calculateRuleSnapshotItemsSHA256(invalidItems)
	if err != nil {
		t.Fatalf("calculate invalid-rule digest: %v", err)
	}
	bundleJSON := func(items []RuleSnapshotItem, digest, schemaVersion string) []byte {
		t.Helper()
		raw, marshalErr := json.Marshal(RuleSnapshotBundle{
			RuleSnapshotManifest: RuleSnapshotManifest{
				SnapshotID:    "rulesnapshot-a",
				AssetCount:    len(items),
				ContentSHA256: digest,
				BundleFormat:  ruleSnapshotBundleFormatJSON,
				SchemaVersion: schemaVersion,
			},
			Items: items,
		})
		if marshalErr != nil {
			t.Fatalf("marshal test bundle: %v", marshalErr)
		}
		return raw
	}

	tests := []struct {
		name        string
		status      int
		body        []byte
		expectation RuleSnapshotExpectation
		wantError   string
	}{
		{
			name: "malformed bundle",
			body: []byte(`{"snapshot_id":`),
			expectation: RuleSnapshotExpectation{
				SnapshotID: "rulesnapshot-a", ContentSHA256: validDigest,
				BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
				AssetIDs: []string{"asset-a"},
			},
			wantError: "decode snapshot bundle failed",
		},
		{
			name:   "snapshot 404",
			status: http.StatusNotFound,
			body:   []byte(`{"error":"snapshot not found"}`),
			expectation: RuleSnapshotExpectation{
				SnapshotID: "rulesnapshot-a", ContentSHA256: validDigest,
				BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
				AssetIDs: []string{"asset-a"},
			},
			wantError: "status=404",
		},
		{
			name: "hash mismatch",
			body: bundleJSON(validItems, strings.Repeat("0", 64), ruleSnapshotSchemaVersionV1),
			expectation: RuleSnapshotExpectation{
				SnapshotID: "rulesnapshot-a", ContentSHA256: validDigest,
				BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
				AssetIDs: []string{"asset-a"},
			},
			wantError: "content_sha256 mismatch",
		},
		{
			name: "schema mismatch",
			body: bundleJSON(validItems, validDigest, "ssa_rule_snapshot_bundle.v99"),
			expectation: RuleSnapshotExpectation{
				SnapshotID: "rulesnapshot-a", ContentSHA256: validDigest,
				BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
				AssetIDs: []string{"asset-a"},
			},
			wantError: "unsupported snapshot schema version",
		},
		{
			name: "rule parse failure",
			body: bundleJSON(invalidItems, invalidDigest, ruleSnapshotSchemaVersionV1),
			expectation: RuleSnapshotExpectation{
				SnapshotID: "rulesnapshot-a", ContentSHA256: invalidDigest,
				BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
				AssetIDs: []string{"asset-broken"},
			},
			wantError: "parse syntax flow rule broken-rule failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := test.status
			if status == 0 {
				status = http.StatusOK
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write(test.body)
			}))
			defer server.Close()

			node := &ScanNode{ruleSyncClient: NewRuleSyncClient(&RuleSyncConfig{
				ServerURL:   server.URL,
				SyncEnabled: true,
				CacheDir:    t.TempDir(),
				Client:      server.Client(),
			})}
			const originalConfig = `{"Mode":127}`
			params := map[string]any{"config": originalConfig}
			preparedPublishes := 0
			prepared, prepareErr := node.prepareRuleSnapshotForScriptExecution(
				context.Background(),
				params,
				nil,
				&test.expectation,
				func(context.Context, RuleSnapshotPreparationReceipt) error {
					preparedPublishes++
					return nil
				},
			)
			if prepared != nil {
				t.Fatalf("failure returned prepared snapshot: %#v", prepared)
			}
			var typed *ruleSnapshotPreparationError
			if !errors.As(prepareErr, &typed) || !strings.Contains(prepareErr.Error(), test.wantError) {
				t.Fatalf("unexpected fail-closed error: %v", prepareErr)
			}
			if preparedPublishes != 0 {
				t.Fatalf("failure published %d prepared success receipts", preparedPublishes)
			}
			if params["config"] != originalConfig {
				t.Fatalf("failure mutated execution config: %#v", params)
			}
		})
	}
}

func preparedRuleSnapshotFixture(t *testing.T) *PreparedRuleSnapshot {
	t.Helper()
	items := []RuleSnapshotItem{
		{AssetID: "asset-z", Name: "z-rule", Content: `desc(title: "Z Rule");`},
		{
			AssetID: "asset-a", Name: "a-rule", Title: "Published A", Severity: "critical",
			RiskType: "snapshot-risk", Content: `desc(title: "A Rule", level: low);`,
		},
	}
	digest, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate snapshot digest: %v", err)
	}
	items = canonicalRuleSnapshotItems(items)
	return &PreparedRuleSnapshot{
		Bundle: RuleSnapshotBundle{
			RuleSnapshotManifest: RuleSnapshotManifest{
				SnapshotID:    "rulesnapshot-a",
				AssetCount:    len(items),
				ContentSHA256: digest,
				BundleFormat:  ruleSnapshotBundleFormatJSON,
				SchemaVersion: ruleSnapshotSchemaVersionV1,
			},
			Items: items,
		},
		Receipt: RuleSnapshotPreparationReceipt{
			CapabilityKey: ruleSnapshotExecutionV2,
			State:         ruleSnapshotCacheReadyFile,
			SnapshotID:    "rulesnapshot-a",
			ContentSHA256: digest,
			BundleFormat:  ruleSnapshotBundleFormatJSON,
			SchemaVersion: ruleSnapshotSchemaVersionV1,
			AssetCount:    len(items),
			PreparedAt:    time.Now().UTC(),
		},
	}
}
