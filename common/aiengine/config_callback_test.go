package aiengine

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func newClosedTestCallback(name string, seen map[string]string) aicommon.AICallbackType {
	return func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		seen[req.GetPrompt()] = name
		rsp := aicommon.NewAIResponse(i)
		rsp.Close()
		return rsp, nil
	}
}

func TestAIEngineConfigTieredCallbackOptions(t *testing.T) {
	seen := make(map[string]string)
	quality := newClosedTestCallback("quality", seen)
	speed := newClosedTestCallback("speed", seen)

	config := NewAIEngineConfig(
		WithQualityPriorityAICallback(quality),
		WithSpeedPriorityAICallback(speed),
	)

	if config.QualityPriorityAICallback == nil {
		t.Fatal("expected quality priority callback to be set")
	}
	if config.SpeedPriorityAICallback == nil {
		t.Fatal("expected speed priority callback to be set")
	}
}

func TestAIEngineConfigTieredAIConfigOptions(t *testing.T) {
	var usageCalled bool
	config := NewAIEngineConfig(
		WithQualityPriorityAIConfig("openai",
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("quality-model"),
			aispec.WithUsageCallback(func(*aispec.ChatUsage) {
				usageCalled = true
			}),
		),
		WithSpeedPriorityAIConfig("openai",
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("speed-model"),
		),
	)

	if config.QualityPriorityAICallback == nil {
		t.Fatal("expected quality priority callback from ai config to be set")
	}
	if config.SpeedPriorityAICallback == nil {
		t.Fatal("expected speed priority callback from ai config to be set")
	}
	if config.UserUsageCallback == nil {
		t.Fatal("expected usage callback from ai config options to be set")
	}
	config.UserUsageCallback(&aispec.ChatUsage{})
	if !usageCalled {
		t.Fatal("expected configured usage callback to be callable")
	}
}

func TestBuildReActOptionsAppendsTieredCallbackOverrides(t *testing.T) {
	originalTiered := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(nil)
	t.Cleanup(func() {
		consts.SetTieredAIConfig(originalTiered)
	})

	seen := make(map[string]string)
	defaultCallback := newClosedTestCallback("default", seen)
	qualityCallback := newClosedTestCallback("quality", seen)
	speedCallback := newClosedTestCallback("speed", seen)

	engineConfig := NewAIEngineConfig(
		WithAICallback(defaultCallback),
		WithQualityPriorityAICallback(qualityCallback),
		WithSpeedPriorityAICallback(speedCallback),
		WithDisableMCPServers(true),
		WithSessionID("tiered-callback-test"),
	)

	opts := buildReActOptions(context.Background(), engineConfig, make(chan *schema.AiOutputEvent, 1))
	opts = append(opts,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithEnableSelfReflection(false),
	)
	config := aicommon.NewConfig(context.Background(), opts...)

	if config.GetOriginalAICallback() == nil {
		t.Fatal("expected original callback to be set")
	}
	if config.GetQualityPriorityRawAICallback() == nil {
		t.Fatal("expected raw quality priority callback to be set")
	}
	if config.GetSpeedPriorityRawAICallback() == nil {
		t.Fatal("expected raw speed priority callback to be set")
	}

	if _, err := config.GetOriginalAICallback()(config, aicommon.NewAIRequest("original")); err != nil {
		t.Fatalf("invoke original callback: %v", err)
	}
	if _, err := config.GetQualityPriorityRawAICallback()(config, aicommon.NewAIRequest("quality")); err != nil {
		t.Fatalf("invoke raw quality callback: %v", err)
	}
	if _, err := config.GetSpeedPriorityRawAICallback()(config, aicommon.NewAIRequest("speed")); err != nil {
		t.Fatalf("invoke raw speed callback: %v", err)
	}

	if seen["original"] != "default" {
		t.Fatalf("expected original callback to use default, got %q", seen["original"])
	}
	if seen["quality"] != "quality" {
		t.Fatalf("expected quality callback override, got %q", seen["quality"])
	}
	if seen["speed"] != "speed" {
		t.Fatalf("expected speed callback override, got %q", seen["speed"])
	}
}
