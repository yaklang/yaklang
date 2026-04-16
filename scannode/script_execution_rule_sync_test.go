package scannode

import (
	"context"
	"testing"
)

type ruleSyncerStub struct {
	hasLocal   bool
	snapshotID string
	callCount  int
}

func (s *ruleSyncerStub) HasLocalSnapshot(string) bool {
	return s.hasLocal
}

func (s *ruleSyncerStub) SyncSnapshot(_ context.Context, snapshotID string) (int, error) {
	s.snapshotID = snapshotID
	s.callCount++
	return 2, nil
}

func TestSyncRulesIfNeededUsesRuleSnapshotLabel(t *testing.T) {
	node := &ScanNode{ruleSyncClient: &ruleSyncerStub{}}

	node.syncRulesIfNeeded(context.Background(), map[string]any{
		"config": "{}",
	}, map[string]string{
		"rule_snapshot_id": "rulesnapshot-a",
	})

	stub := node.ruleSyncClient.(*ruleSyncerStub)
	if stub.callCount != 1 {
		t.Fatalf("expected sync call count 1, got %d", stub.callCount)
	}
	if stub.snapshotID != "rulesnapshot-a" {
		t.Fatalf("unexpected synced snapshot id: %q", stub.snapshotID)
	}
}

func TestSyncRulesIfNeededIgnoresInlineRuleInputAndUsesSnapshot(t *testing.T) {
	stub := &ruleSyncerStub{}
	node := &ScanNode{ruleSyncClient: stub}

	node.syncRulesIfNeeded(context.Background(), map[string]any{
		"config": `{"SyntaxFlowRule":{"rule_input":[{"RuleName":"sql-injection","Content":"desc(title:'x');"}]}}`,
	}, map[string]string{
		"rule_snapshot_id": "rulesnapshot-a",
	})

	if stub.callCount != 1 {
		t.Fatalf("expected sync call even when inline rules exist, got %d", stub.callCount)
	}
	if stub.snapshotID != "rulesnapshot-a" {
		t.Fatalf("unexpected synced snapshot id: %q", stub.snapshotID)
	}
}

func TestSyncRulesIfNeededSkipsWhenSnapshotAlreadyCached(t *testing.T) {
	stub := &ruleSyncerStub{hasLocal: true}
	node := &ScanNode{ruleSyncClient: stub}

	node.syncRulesIfNeeded(context.Background(), map[string]any{
		"config": "{}",
	}, map[string]string{
		"rule_snapshot_id": "rulesnapshot-a",
	})

	if stub.callCount != 0 {
		t.Fatalf("expected no sync call for cached snapshot, got %d", stub.callCount)
	}
}
