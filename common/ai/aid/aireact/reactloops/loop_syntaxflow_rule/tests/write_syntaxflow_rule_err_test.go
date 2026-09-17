package syntaxflowruletests

import (
	"bytes"
	"fmt"
	"strings"
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

func mockedSyntaxFlowWritingCauseError(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest, stat *mockStats_forWriteAndModify) (*aicommon.AIResponse, error) {
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

	hasRulePrompt := utils.MatchAnyOfSubString(prompt, "write_rule", "modify_rule", "GEN_RULE", "sf_rule")
	if hasRulePrompt {
		nonceStr := aicommon_testutil.MustExtractDynamicSectionNonce(t, prompt)
		rsp := i.NewAIResponse()
		if stat.claimFirstWrite() {
			invalidRule := "rule(\"test\")\ndesc(\n\ttitle: \"Test\"\n\ttype: audit\n\tlevel: info\n"
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_rule"}
<|GEN_RULE_{{ .nonce }}|>
`+invalidRule+`
<|GEN_RULE_END_{{ .nonce }}|>`, map[string]any{"nonce": nonceStr})))
		} else {
			fixedRule := "rule(\"test\")\ndesc(\n\ttitle: \"Test Fixed\"\n\ttype: audit\n\tlevel: info\n)"
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "modify_rule", "modify_start_line": 1, "modify_end_line": 6}
<|GEN_RULE_{{ .nonce }}|>
`+fixedRule+`
<|GEN_RULE_END_{{ .nonce }}|>`, map[string]any{"nonce": nonceStr})))
		}
		rsp.Close()
		return rsp, nil
	}
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestFocusMode_WriteSyntaxFlowRuleCauseErrorAndThenModify(t *testing.T) {
	_ = ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 256)

	var haveError bool
	stat := &mockStats_forWriteAndModify{writeDone: false}
	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if strings.Contains(r.GetPrompt(), "编译错误") || strings.Contains(r.GetPrompt(), "Syntax Error") {
				haveError = true
			}
			return mockedSyntaxFlowWritingCauseError(t, i, r, stat)
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

	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var lastContent string
	var gotModify bool
LOOP:
	for {
		select {
		case e := <-out:
			if e.Type != string(schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE) {
				continue
			}
			payload := parseSyntaxFlowRuleChangeResponse(t, e)
			lastContent = payload.Code.Content
			if payload.SourceAction == "modify_rule" ||
				payload.Op == "replace" ||
				strings.Contains(payload.Code.Content, "Test Fixed") {
				gotModify = true
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

	if !gotModify {
		t.Fatal("expected modify_rule syntaxflow_rule_change after lint-failing write_rule")
	}
	if !strings.Contains(lastContent, "rule(") {
		t.Fatal("rule content not found in syntaxflow_rule_change")
	}
	if !strings.Contains(lastContent, "Test Fixed") {
		t.Fatal("expected repaired rule content 'Test Fixed'")
	}
	_ = haveError
}
