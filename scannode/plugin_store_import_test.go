package scannode

import (
	"testing"

	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidatePluginStoreImportCommand(t *testing.T) {
	newCommand := func() *pluginv1.ImportPluginStoreCommand {
		return &pluginv1.ImportPluginStoreCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-imp-1"},
			TargetNodeId: "node-a",
			ArtifactUrl:  "http://api/v1/plugin-store/imports/x/download",
		}
	}

	if err := validatePluginStoreImportCommand("node-a", newCommand()); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}

	command := newCommand()
	command.Metadata = nil
	if err := validatePluginStoreImportCommand("node-a", command); err == nil {
		t.Fatal("missing metadata should fail")
	}

	command = newCommand()
	command.TargetNodeId = "node-b"
	if err := validatePluginStoreImportCommand("node-a", command); err == nil {
		t.Fatal("target mismatch should fail")
	}

	command = newCommand()
	command.ArtifactUrl = ""
	if err := validatePluginStoreImportCommand("node-a", command); err == nil {
		t.Fatal("missing artifact url should fail")
	}
}

func TestPluginStoreImportStateMachine(t *testing.T) {
	state := &pluginStoreSyncState{}

	// import 复用同步状态机：进度/状态/计数走同一份快照
	progress := state.update("cmd-imp-1", func(p *pluginv1.PluginStoreSyncProgress) {
		p.Status = pluginStoreImportStatusRunning
		p.Progress = 0.5
		p.CurrentPlugin = "downloading plugin database"
	})
	if progress.Status != pluginStoreImportStatusRunning || progress.Progress != 0.5 {
		t.Fatalf("unexpected progress: %+v", progress)
	}

	state.update("cmd-imp-1", func(p *pluginv1.PluginStoreSyncProgress) {
		p.Status = pluginStoreImportStatusSucceeded
		p.Progress = 1
		p.Total = 100
		p.Completed = 100
	})
	snapshot := state.snapshot()
	if snapshot.Status != pluginStoreImportStatusSucceeded || snapshot.Total != 100 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}