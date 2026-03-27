package genmetadata

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestCompleteYakScriptAIFields_FillsMissingFields(t *testing.T) {
	aicommon.RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*aicommon.ForgeResult, error) {
		action := aicommon.NewSimpleAction("call-tool", aitool.InvokeParams{
			aicommon.ActionMagicKey: "call-tool",
			"ai_desc":               "检测 HTTP 请求走私风险的插件",
			"ai_keywords":           []string{"http request smuggling", "HTTP走私", "cl.te", "http request smuggling"},
			"ai_usage":              "适用于检测目标站点是否存在 CL.TE/TE.CL 请求走私问题，重点提供目标 URL 或请求上下文，并关注异常响应差异。",
		})
		return &aicommon.ForgeResult{Action: action, Name: "mock"}, nil
	})

	script := &schema.YakScript{
		ScriptName:  "HTTP请求走私",
		Type:        "mitm",
		Help:        "检测 HTTP 请求走私",
		Params:      `"[{\"Field\":\"target\",\"Required\":true}]"`,
		Content:     "println(\"hello\")",
		EnableForAI: true,
	}

	if err := CompleteYakScriptAIFields(context.Background(), script); err != nil {
		t.Fatalf("CompleteYakScriptAIFields failed: %v", err)
	}

	if !script.EnableForAI {
		t.Fatalf("expected EnableForAI to be true")
	}
	if script.AIDesc != "检测 HTTP 请求走私风险的插件" {
		t.Fatalf("unexpected AIDesc: %q", script.AIDesc)
	}
	if script.AIKeywords != "http request smuggling,HTTP走私,cl.te" {
		t.Fatalf("unexpected AIKeywords: %q", script.AIKeywords)
	}
	if script.AIUsage == "" {
		t.Fatal("expected AIUsage to be populated")
	}
}

func TestCompleteYakScriptAIFields_UsesEmbeddedMetadataBeforeAI(t *testing.T) {
	callCount := 0
	aicommon.RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*aicommon.ForgeResult, error) {
		callCount++
		action := aicommon.NewSimpleAction("call-tool", aitool.InvokeParams{
			aicommon.ActionMagicKey: "call-tool",
			"ai_desc":               "should not be used",
			"ai_keywords":           []string{"unused"},
			"ai_usage":              "unused",
		})
		return &aicommon.ForgeResult{Action: action, Name: "mock"}, nil
	})

	script := &schema.YakScript{
		ScriptName:  "meta-plugin",
		Type:        "yak",
		EnableForAI: true,
		Content: `__DESC__ = "从脚本元数据读取描述"
__KEYWORDS__ = "yak,metadata"
__USAGE__ = "从脚本内嵌元数据读取使用说明"
__ENABLE_FOR_AI__ = true
println("ok")`,
	}

	if err := CompleteYakScriptAIFields(context.Background(), script); err != nil {
		t.Fatalf("CompleteYakScriptAIFields failed: %v", err)
	}

	if callCount != 0 {
		t.Fatalf("expected AI callback not to be invoked, got %d", callCount)
	}
	if !script.EnableForAI {
		t.Fatal("expected EnableForAI from embedded metadata")
	}
	if script.AIDesc != "从脚本元数据读取描述" {
		t.Fatalf("unexpected AIDesc: %q", script.AIDesc)
	}
	if script.AIKeywords != "yak,metadata" {
		t.Fatalf("unexpected AIKeywords: %q", script.AIKeywords)
	}
	if script.AIUsage != "从脚本内嵌元数据读取使用说明" {
		t.Fatalf("unexpected AIUsage: %q", script.AIUsage)
	}
}

func TestCompleteYakScriptAIFields_DisabledSkipsGeneration(t *testing.T) {
	callCount := 0
	aicommon.RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*aicommon.ForgeResult, error) {
		callCount++
		action := aicommon.NewSimpleAction("call-tool", aitool.InvokeParams{
			aicommon.ActionMagicKey: "call-tool",
			"ai_desc":               "unused",
			"ai_keywords":           []string{"unused"},
			"ai_usage":              "unused",
		})
		return &aicommon.ForgeResult{Action: action, Name: "mock"}, nil
	})

	script := &schema.YakScript{
		ScriptName: "disabled-plugin",
		Type:       "yak",
		Content:    `__DESC__ = "should stay empty"`,
	}

	if err := CompleteYakScriptAIFields(context.Background(), script); err != nil {
		t.Fatalf("CompleteYakScriptAIFields failed: %v", err)
	}
	if callCount != 0 {
		t.Fatalf("expected AI callback not to be invoked, got %d", callCount)
	}
	if script.AIDesc != "" || script.AIKeywords != "" || script.AIUsage != "" {
		t.Fatalf("expected AI fields to stay empty when EnableForAI is false: %#v", script)
	}
}
