package scannode

import (
	"testing"

	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

func TestPluginStoreSyncStateUpdate(t *testing.T) {
	state := &pluginStoreSyncState{}

	progress := state.update("cmd-1", func(p *pluginv1.PluginStoreSyncProgress) {
		p.Status = pluginStoreSyncStatusRunning
		p.Total = 100
		p.Completed = 10
	})
	if progress.CommandId != "cmd-1" || progress.Status != pluginStoreSyncStatusRunning {
		t.Fatalf("unexpected progress: %+v", progress)
	}
	if progress.Total != 100 || progress.Completed != 10 {
		t.Fatalf("unexpected counters: %+v", progress)
	}
	if progress.UpdatedAt == nil || progress.UpdatedAt.AsTime().IsZero() {
		t.Fatal("updated_at must be set")
	}

	// 同一命令连续 update 累加字段
	state.update("cmd-1", func(p *pluginv1.PluginStoreSyncProgress) {
		p.Completed = 50
		p.Status = pluginStoreSyncStatusSucceeded
	})
	snapshot := state.snapshot()
	if snapshot.Completed != 50 || snapshot.Status != pluginStoreSyncStatusSucceeded {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	// 新命令应重置状态
	progress = state.update("cmd-2", func(p *pluginv1.PluginStoreSyncProgress) {
		p.Status = pluginStoreSyncStatusRunning
	})
	if progress.CommandId != "cmd-2" || progress.Completed != 0 {
		t.Fatalf("new command must reset state: %+v", progress)
	}
}

func TestPluginStoreSyncStateEmptySnapshot(t *testing.T) {
	state := &pluginStoreSyncState{}
	if snapshot := state.snapshot(); snapshot != nil {
		t.Fatalf("empty state snapshot = %+v, want nil", snapshot)
	}
}

func TestValidatePluginStoreSyncCommand(t *testing.T) {
	newCommand := func() *pluginv1.SyncPluginStoreCommand {
		return &pluginv1.SyncPluginStoreCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-sync-1"},
			TargetNodeId: "node-a",
		}
	}

	if err := validatePluginStoreSyncCommand("node-a", newCommand()); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}

	command := newCommand()
	command.Metadata = nil
	if err := validatePluginStoreSyncCommand("node-a", command); err == nil {
		t.Fatal("missing metadata should fail")
	}

	command = newCommand()
	command.TargetNodeId = "node-b"
	if err := validatePluginStoreSyncCommand("node-a", command); err == nil {
		t.Fatal("target node mismatch should fail")
	}
}

func TestValidatePluginStoreSyncStatusQuery(t *testing.T) {
	newCommand := func() *pluginv1.QueryPluginStoreSyncStatusCommand {
		return &pluginv1.QueryPluginStoreSyncStatusCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-query-1"},
			TargetNodeId: "node-a",
			CommandId:    "cmd-sync-1",
		}
	}

	if err := validatePluginStoreSyncStatusQuery("node-a", newCommand()); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}

	command := newCommand()
	command.TargetNodeId = ""
	if err := validatePluginStoreSyncStatusQuery("node-a", command); err == nil {
		t.Fatal("missing target node should fail")
	}
}
