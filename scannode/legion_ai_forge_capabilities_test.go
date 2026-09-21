package scannode

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestImmutableForgeDoesNotDiscoverHostCapabilities(t *testing.T) {
	release := testLegionContextForgeRelease(t)
	opts, _, err := legionForgeCapabilityOptions(context.Background(), release, aiSessionBinding{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := aicommon.NewConfig(context.Background(), opts...)
	if !cfg.DisableToolUse || !cfg.DisableIntentRecognition || cfg.GetSkillLoader() != nil {
		t.Fatal("advisory release must not discover host skills or recommend undeclared capabilities")
	}
	if _, err := cfg.GetAiToolManager().GetToolByName("web_search"); err == nil {
		t.Fatal("advisory release unexpectedly exposes a host tool")
	}
	for _, action := range []string{"require_tool", "directly_call_tool", "tool_batch", "read_file", "ask_for_clarification", "request_plan"} {
		if cfg.IsReActActionAllowed("default", action) {
			t.Fatalf("advisory release allows %s", action)
		}
	}
	for _, action := range []string{"directly_answer", "finish"} {
		if !cfg.IsReActActionAllowed("default", action) {
			t.Fatalf("advisory release blocks %s", action)
		}
	}
}
