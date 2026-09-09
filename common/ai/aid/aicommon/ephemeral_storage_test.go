package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"os"
	"testing"
	"time"
)

func TestEphemeralStorageNeverCreatesArtifacts(t *testing.T) {
	cfg := &Config{Workdir: t.TempDir()}
	if err := WithEphemeralStorage()(cfg); err != nil {
		t.Fatal(err)
	}
	checkpoint := cfg.CreateAIInteractiveCheckpoint(1)
	if err := cfg.SubmitCheckpointRequest(checkpoint, "request"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SubmitCheckpointResponse(checkpoint, "response"); err != nil {
		t.Fatal(err)
	}
	if cfg.GetDB() != nil || !checkpoint.Finished {
		t.Fatal("ephemeral checkpoint boundary")
	}
	caller := &ToolCaller{config: cfg}
	tool := aitool.NewWithoutCallback("platform.list_projects")
	bundle := caller.newToolCallArtifactBundle(tool, "call-1", "")
	if _, err := bundle.Writer(artifactStdout).Write([]byte("private tool output")); err != nil {
		t.Fatal(err)
	}
	result := &aitool.ToolResult{Data: `{"result_json":{"project":"owned"}}`, Success: true}
	if err := bundle.finalize(caller, tool, "call-1", "", nil, result, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := bundle.discardIfUnfinished(); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(cfg.Workdir)
	if err != nil || len(files) != 0 || bundle.dir != "" {
		t.Fatalf("artifact was persisted: %v %v", files, err)
	}
	if result.Data != `{"result_json":{"project":"owned"}}` {
		t.Fatal("ephemeral result lost platform payload")
	}
}

func TestConfigEndpointManagerUsesConfiguredContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := NewConfig(context.Background(), WithContext(ctx), WithEphemeralStorage(), WithDisableAutoSkills(true), WithAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) { return nil, nil }))
	cancel()
	select {
	case <-cfg.Epm.GetContext().Done():
	case <-time.After(time.Second):
		t.Fatal("endpoint manager retained constructor background context")
	}
	select {
	case <-cfg.Guardian.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("guardian retained constructor background context")
	}
}
