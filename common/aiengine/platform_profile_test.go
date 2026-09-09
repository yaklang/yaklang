package aiengine

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestPlatformProfileCannotBeWidened(t *testing.T) {
	tool := aitool.NewWithoutCallback("platform.list_projects")
	cfg := NewAIEngineConfig(WithPlatformOnlyProfile(nil, tool), WithStateless(false), WithDisableAIForge(false), WithExtOptions(aicommon.WithReActActionPolicy(func(string, string) bool { return true }), aicommon.WithBuiltinTools()))
	c := aicommon.NewConfig(context.Background(), buildReActOptions(context.Background(), cfg, make(chan *schema.AiOutputEvent, 100))...)
	if c.GetDB() != nil || c.PersistentSessionId != "" || c.EnablePlanAndExec || !c.DisallowMCPServers {
		t.Fatal("profile persistence/capability boundary widened")
	}
	for _, action := range []string{"yaklang_code", "bash", "write_file", "loading_skills", "require_ai_blueprint", "request_plan_and_execution", "tool_compose", "load_capability"} {
		if c.IsReActActionAllowed("default", action) {
			t.Fatalf("allowed action %s", action)
		}
	}
	for _, action := range []string{"directly_answer", "finish", "require_tool", "directly_call_tool"} {
		if !c.IsReActActionAllowed("default", action) {
			t.Fatalf("blocked platform action %s", action)
		}
	}
	if got, err := c.AiToolManager.GetToolByName("platform.list_projects"); err != nil || got != tool {
		t.Fatalf("platform tool unavailable: %v", err)
	}
	for _, name := range []string{"bash", "read_file", "write_file", "yak", "search_tools"} {
		if got, _ := c.AiToolManager.GetToolByName(name); got != nil {
			t.Fatalf("unexpected tool %s", name)
		}
	}
}

func TestPlatformProfileDoesNotInheritGlobalPreset(t *testing.T) {
	previous := yakit.GetCachedAIGlobalConfig()
	t.Cleanup(func() { yakit.SetCachedAIGlobalConfigForTest(previous) })
	yakit.SetCachedAIGlobalConfigForTest(&ypb.AIGlobalConfig{AIPresetPrompt: "other-user-private-preset"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	callback := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		calls++
		if strings.Contains(req.GetPrompt(), "other-user-private-preset") || strings.Contains(req.GetPrompt(), "AI_PRESET") {
			t.Error("global preset entered delegated prompt")
		}
		rsp := c.NewAIResponse()
		rsp.EmitOutputStream(strings.NewReader("ok"))
		rsp.Close()
		return rsp, nil
	}
	cfg := NewAIEngineConfig(WithPlatformOnlyProfile(callback))
	c := aicommon.NewConfig(context.Background(), buildReActOptions(ctx, cfg, make(chan *schema.AiOutputEvent, 100))...)
	for _, cb := range []aicommon.AICallbackType{c.GetOriginalAICallback(), c.GetQualityPriorityAICallback(), c.GetSpeedPriorityAICallback(), c.GetVisionPriorityAICallback()} {
		request := aicommon.NewAIRequest("platform prompt")
		request.SetDetachCheckpoint(true)
		if _, err := cb(c, request); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 4 {
		t.Fatalf("expected all four tiers, got %d", calls)
	}
}
