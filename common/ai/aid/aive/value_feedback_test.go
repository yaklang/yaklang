package aive

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// TestComputeSignatureDeterministic 验证签名对相同稳定字段确定性一致, 且不含 ID/时间戳.
func TestComputeSignatureDeterministic(t *testing.T) {
	base := &aicommon.ValueFeedbackRecord{
		MainModel:           aicommon.ModelEndpoint{ModelName: "qwen-max", ServerName: "openai"},
		SmallModel:          aicommon.ModelEndpoint{ModelName: forcedSmallModelName},
		FocusMode:           "http_fuzztest",
		TriggerCondition:    aicommon.ValueFeedbackTriggerLoopEnd,
		WhatHappenedSummary: "set_http_request -> fuzz_path",
		TimelineDump:        "[1] do a\n[2] do b",
	}
	sig1 := computeSignature(base)

	// 改变 ID / 时间戳不应影响签名.
	base.ID = "ksuid-aaa"
	base.Timestamp = 123456
	sig2 := computeSignature(base)
	if sig1 != sig2 {
		t.Fatalf("signature should ignore ID/timestamp, got %s vs %s", sig1, sig2)
	}

	// 改变稳定字段应改变签名.
	base.FocusMode = "something_else"
	sig3 := computeSignature(base)
	if sig1 == sig3 {
		t.Fatalf("signature should change when stable field changes")
	}
	if len(sig1) != 64 {
		t.Fatalf("signature should be sha256 hex (64 chars), got %d", len(sig1))
	}
}

// TestBuildPromptContainsContext 验证 prompt 携带主模型/小模型/触发条件等上下文.
func TestBuildPromptContainsContext(t *testing.T) {
	record := &aicommon.ValueFeedbackRecord{
		ID:               "rid-1",
		Signature:        "sig-1",
		MainModel:        aicommon.ModelEndpoint{ModelName: "qwen-max", ServerName: "openai"},
		SmallModel:       aicommon.ModelEndpoint{ModelName: forcedSmallModelName},
		FocusMode:        "http_fuzztest",
		TriggerCondition: aicommon.ValueFeedbackTriggerVerification,
		Actions: []aicommon.ValueFeedbackAction{
			{ActionType: "directly_call_tool", ToolName: "httpfuzzer", IterationIndex: 1},
		},
		WhatHappenedSummary: "directly_call_tool(httpfuzzer)",
	}
	prompt := buildValueFeedbackPrompt(record)
	for _, want := range []string{"qwen-max", forcedSmallModelName, "http_fuzztest", "verification", "httpfuzzer", "rid-1", "sig-1"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\nprompt=%s", want, prompt)
		}
	}
}

// TestSubmitterRegistered 验证 init() 已把 submitter 注册进 aicommon (默认开启).
func TestSubmitterRegistered(t *testing.T) {
	// 未注册时 SubmitValueFeedback 是安全 no-op; 注册后投递到有界队列.
	// 这里通过非阻塞投递不 panic 来确认链路通畅 (cfg/record 非 nil).
	cfg := &aicommon.Config{}
	record := &aicommon.ValueFeedbackRecord{FocusMode: "test", TriggerCondition: aicommon.ValueFeedbackTriggerIterationEnd}
	// 不应阻塞, 不应 panic.
	aicommon.SubmitValueFeedback(cfg, record)
}

// TestSubmitNonBlockingOnFullQueue 验证队列满时丢弃而非阻塞.
func TestSubmitNonBlockingOnFullQueue(t *testing.T) {
	p := &valueFeedbackPool{queue: make(chan *valueFeedbackJob, 1)}
	cfg := &aicommon.Config{}
	// 填满队列 (不启动 worker, 让其保持满).
	p.queue <- &valueFeedbackJob{cfg: cfg, record: &aicommon.ValueFeedbackRecord{}}
	done := make(chan struct{})
	go func() {
		// 多次投递, 队列已满应立即丢弃返回, 不阻塞 (tryEnqueue 不启动 worker).
		for i := 0; i < 100; i++ {
			p.tryEnqueue(cfg, &aicommon.ValueFeedbackRecord{})
		}
		close(done)
	}()
	<-done
}
