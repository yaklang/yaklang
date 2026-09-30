package yaklangcodetests

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommon_testutil "github.com/yaklang/yaklang/common/ai/aid/aicommon/testutil"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type mockStats_forYakdoc struct {
	yakdocDone  bool
	codeWritten bool
}

func mockedYaklangYakdocFlow(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest, stat *mockStats_forYakdoc) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	if utils.MatchAllOfSubString(prompt, "analyze-requirement-and-search", "create_new_file") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true
}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"yakdoc_function_details"`, `"require_tool"`, `"write_code"`, `"@action"`) {
		nonceStr := aicommon_testutil.MustExtractDynamicSectionNonce(t, prompt)
		rsp := i.NewAIResponse()

		if !stat.yakdocDone {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "yakdoc_function_details",
  "library": "str",
  "function": ["Split"],
  "human_readable_thought": "Query str.Split API before writing code"
}`))
			stat.yakdocDone = true
			rsp.Close()
			return rsp, nil
		}

		if !stat.codeWritten {
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_code"}

<|GEN_CODE_{{ .nonce }}|>
parts = str.Split("a,b", ",")
println(parts)
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
				"nonce": nonceStr,
			})))
			stat.codeWritten = true
			rsp.Close()
			return rsp, nil
		}

		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"@action"`, `"create_new_file"`, `"check-filepath"`, `"existed_filepath"`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "check-filepath", "create_new_file": true}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestFocusMode_YakdocThenWriteCode(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	stat := &mockStats_forYakdoc{}

	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedYaklangYakdocFlow(t, i, r, stat)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:   true,
			FreeInput:     "use str.Split to split a string",
			FocusModeLoop: schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
		}
	}()

	waitResult := waitForYaklangDeferredEditorSync(out, focusModeWriteYaklangTestTimeout())
	close(in)
	ins.Wait()

	require.True(t, stat.yakdocDone, "yakdoc_function_details should be invoked")
	require.True(t, stat.codeWritten, "write_code should be invoked after yakdoc")
	require.NotEmpty(t, waitResult.codeChangeEvents, "deferred yaklang_code_change should be emitted after write_code")

	var yakdocTimelineSeen bool
	tl := ins.DumpTimeline()
	require.Contains(t, tl, "yakdoc_function_details")
	require.Contains(t, tl, "str.Split")
	if strings.Contains(tl, "yakdoc_function_details_result") ||
		strings.Contains(tl, "yakdoc_function_details") {
		yakdocTimelineSeen = true
	}
	require.True(t, yakdocTimelineSeen, "yakdoc timeline node should be present")
}

func TestWriteYaklangLoopPromptContainsYakdocActions(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 128)
	prompts := make(chan string, 32)
	stat := &mockStats_forYakdoc{}
	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompts <- r.GetPrompt()
			return mockedYaklangYakdocFlow(t, i, r, stat)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) { out <- e.ToGRPC() }),
	)
	require.NoError(t, err)
	in <- &ypb.AIInputEvent{IsFreeInput: true, FreeInput: "quick prompt check", FocusModeLoop: schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG}
	result := waitForYaklangDeferredEditorSync(out, focusModeWriteYaklangTestTimeout())
	close(in)
	ins.Wait()
	require.False(t, result.taskFailed)
	require.True(t, result.taskCompleted, "the prompt-check task must complete")
	close(prompts)
	var mainPrompt string
	for prompt := range prompts {
		if strings.Contains(prompt, `"write_code"`) && strings.Contains(prompt, `"yakdoc_search"`) {
			mainPrompt = prompt
			break
		}
	}
	require.NotEmpty(t, mainPrompt, "the actual code-writing prompt must be visited")
	for _, action := range []string{"yakdoc_search", "yakdoc_get_all_library_names", "yakdoc_library_details", "yakdoc_function_details", "yakdoc_variable_details"} {
		require.Contains(t, mainPrompt, action)
	}
}
