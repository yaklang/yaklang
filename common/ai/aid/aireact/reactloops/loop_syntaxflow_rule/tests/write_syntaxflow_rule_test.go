package syntaxflowruletests

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommon_testutil "github.com/yaklang/yaklang/common/ai/aid/aicommon/testutil"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// mockedSyntaxFlowWriting mocks AI responses for Write SyntaxFlow ReAct loop.
func mockedSyntaxFlowWriting(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	// Match analyze-requirement-and-search init step
	if utils.MatchAllOfSubString(prompt, "analyze-requirement-and-search", "create_new_file") {
		rsp := i.NewAIResponse()
		if utils.MatchAnyOfSubString(prompt, "search_patterns", "Grep模式", "semantic_questions") {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true,
  "search_patterns": ["rule("],
  "reason": "Simple test rule"
}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true
}`))
		}
		rsp.Close()
		return rsp, nil
	}

	// Match main loop prompt asking for write_rule with GEN_RULE tag
	if utils.MatchAnyOfSubString(prompt, "write_rule", "GEN_RULE", "sf_rule") {
		nonceStr := aicommon_testutil.MustExtractDynamicSectionNonce(t, prompt)
		rsp := i.NewAIResponse()
		// Valid SyntaxFlow rule: rule("test") with desc block
		ruleContent := `rule("test-rule")
desc(
	title: "Test SyntaxFlow Rule"
	type: audit
	level: info
)`
		rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_rule"}

<|GEN_RULE_{{ .nonce }}|>
`+ruleContent+`
<|GEN_RULE_END_{{ .nonce }}|>`, map[string]any{
			"nonce": nonceStr,
		})))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestFocusMode_WriteSyntaxFlowRule(t *testing.T) {
	_ = ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 256)

	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedSyntaxFlowWriting(t, i, r)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:   true,
			FreeInput:     "write a SyntaxFlow rule for testing",
			FocusModeLoop: schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW,
		}
	}()

	du := time.Duration(5)
	if utils.InGithubActions() {
		du = time.Duration(3)
	}
	after := time.After(du * time.Second)

	var gotRuleChange bool
LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE) {
				gotRuleChange = true
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	fmt.Println("--------------------------------------")

	if !gotRuleChange {
		t.Fatal("expected syntaxflow_rule_change after write_rule (disk write is deferred)")
	}
}

type mockStats_forWriteAndModify struct {
	mu        sync.Mutex
	writeDone bool
}

// claimFirstWrite returns true exactly once; later callers must use modify_rule.
func (s *mockStats_forWriteAndModify) claimFirstWrite() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeDone {
		return false
	}
	s.writeDone = true
	return true
}

func mockedSyntaxFlowWritingAndModify(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest, stat *mockStats_forWriteAndModify) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	// Match analyze-requirement-and-search init step
	if utils.MatchAllOfSubString(prompt, "analyze-requirement-and-search", "create_new_file") {
		rsp := i.NewAIResponse()
		if utils.MatchAnyOfSubString(prompt, "search_patterns", "Grep模式", "semantic_questions") {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true,
  "search_patterns": ["rule("],
  "reason": "Simple test rule"
}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true
}`))
		}
		rsp.Close()
		return rsp, nil
	}

	// Match prompts that ask for rule generation (write or modify)
	hasRulePrompt := utils.MatchAnyOfSubString(prompt, "write_rule", "modify_rule", "GEN_RULE", "sf_rule")
	if hasRulePrompt {
		nonceStr := aicommon_testutil.MustExtractDynamicSectionNonce(t, prompt)
		rsp := i.NewAIResponse()
		if stat.claimFirstWrite() {
			// 第一次写：故意返回有语法错误的规则（缺少 desc 的闭合括号），
			// 触发 hasBlockingErrors，循环不退出，进入下一轮以便测试 modify_rule
			ruleContent := `rule("test-rule")
desc(
	title: "Test Rule"
	type: audit
	level: info
` // 缺少 )
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_rule"}

<|GEN_RULE_{{ .nonce }}|>
`+ruleContent+`
<|GEN_RULE_END_{{ .nonce }}|>`, map[string]any{
				"nonce": nonceStr,
			})))
		} else {
			// modify_rule: fix syntax error (also covers concurrent retries after first write)
			modifiedContent := `rule("test-rule")
desc(
	title: "Test Rule Fixed"
	type: audit
	level: info
)`
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "modify_rule", "modify_start_line": 1, "modify_end_line": 6}

<|GEN_RULE_{{ .nonce }}|>
`+modifiedContent+`
<|GEN_RULE_END_{{ .nonce }}|>`, map[string]any{
				"nonce": nonceStr,
			})))
		}
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestFocusMode_WriteSyntaxFlowRuleAndThenModify(t *testing.T) {
	_ = ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 256)

	stat := &mockStats_forWriteAndModify{writeDone: false}
	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedSyntaxFlowWritingAndModify(t, i, r, stat)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:   true,
			FreeInput:     "write a SyntaxFlow rule",
			FocusModeLoop: schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW,
		}
	}()

	du := time.Duration(5)
	if utils.InGithubActions() {
		du = time.Duration(3)
	}
	after := time.After(du * time.Second)

	var writeRuleReceived, modifyRuleReceived bool
	var lastContent string
LOOP:
	for {
		select {
		case e := <-out:
			if e.Type != string(schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE) {
				continue
			}
			payload := parseSyntaxFlowRuleChangeResponse(t, e)
			lastContent = payload.Code.Content
			switch payload.SourceAction {
			case "write_rule":
				writeRuleReceived = true
			case "modify_rule":
				modifyRuleReceived = true
				break LOOP
			}
			// Legacy editor node ids may still appear; prefer source_action from rule change.
			if payload.Op == "replace" && writeRuleReceived {
				modifyRuleReceived = true
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !writeRuleReceived {
		t.Fatal("write_rule syntaxflow_rule_change not received")
	}
	if !modifyRuleReceived {
		t.Fatal("modify_rule syntaxflow_rule_change not received - first write had syntax error, loop should continue and AI should call modify_rule")
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	fmt.Println("--------------------------------------")
	fmt.Println(lastContent)

	if !strings.Contains(lastContent, "rule(") {
		t.Fatal("rule content not delivered in syntaxflow_rule_change")
	}
	if !strings.Contains(lastContent, "desc(") {
		t.Fatal("desc block not found in delivered rule")
	}
	// modify_rule 应修复语法错误，最终内容包含 "Test Rule Fixed"
	if !strings.Contains(lastContent, "Test Rule Fixed") {
		t.Fatal("modify_rule did not apply; expected 'Test Rule Fixed' in final content")
	}
}