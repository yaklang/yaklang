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
}
