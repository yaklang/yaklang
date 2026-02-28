package syntaxflowruletests

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedSyntaxFlowWritingCauseError(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, stat *mockStats_forWriteAndModify) (*aicommon.AIResponse, error) {
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
		re := regexp.MustCompile(`<\|GEN_RULE_([^|]+)\|>`)
		matches := re.FindStringSubmatch(prompt)
		nonceStr := ""
		if len(matches) > 1 {
			nonceStr = matches[1]
		}
		rsp := i.NewAIResponse()
		if !stat.writeDone {
			invalidRule := "rule(\"test\")\ndesc(\n\ttitle: \"Test\"\n\ttype: audit\n"
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_rule"}
<|GEN_RULE_{{ .nonce }}|>
`+invalidRule+`
<|GEN_RULE_END_{{ .nonce }}|>`, map[string]any{"nonce": nonceStr})))
			stat.writeDone = true
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
	out := make(chan *ypb.AIOutputEvent, 10)

	var haveError bool
	stat := &mockStats_forWriteAndModify{writeDone: false}
	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if strings.Contains(r.GetPrompt(), "编译错误") {
				haveError = true
			}
			return mockedSyntaxFlowWritingCauseError(i, r, stat)
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

	var filenames []string
LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filenames = append(filenames, utils.InterfaceToString(jsonpath.FindFirst(content, "$.path")))
			}
			if e.Type == string(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR) && e.GetNodeId() == "modify_code" {
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	var filename string
	for _, name := range filenames {
		if strings.Contains(name, "gen_code_") || strings.HasSuffix(name, ".sf") {
			filename = name
			break
		}
	}
	if filename == "" {
		t.Fatal("gen_code_/.sf filename not found")
	}
	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	fmt.Println("--------------------------------------")

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "rule(") {
		t.Fatal("rule content not found in file")
	}
	_ = haveError
}
