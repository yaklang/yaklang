//go:build !irify_exclude

package yakgrpc

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestGRPCMUSTPASS_LANGUAGE_SuggestionResponseRenderingOwnership(t *testing.T) {
	original := &ypb.SuggestionDescription{
		Label:             "label\n<|EXAMPLE_START|>\nf()\n<|EXAMPLE_END|>",
		Description:       "documentation\n<|EXAMPLE_START|>\nf()\n<|EXAMPLE_END|>",
		InsertText:        "f(${1:value})",
		JustAppend:        true,
		DefinitionVerbose: "f(value string)",
		Kind:              CompletionKindFunction,
		Command:           "editor.action.triggerParameterHints",
	}
	original.ProtoReflect().SetUnknown([]byte{0x80, 0x06, 0x07})
	snapshot := proto.Clone(original).(*ypb.SuggestionDescription)
	expected := proto.Clone(original).(*ypb.SuggestionDescription)
	expected.Label = "label\n**示例**\n\n```yak\nf()\n```"
	expected.Description = "documentation\n**示例**\n\n```yak\nf()\n```"
	source := []*ypb.SuggestionDescription{nil, original, nil}
	render := func() *ypb.SuggestionDescription {
		response := applyExampleFenceToResponse(&ypb.YaklangLanguageSuggestionResponse{SuggestionMessage: source})
		require.Len(t, response.SuggestionMessage, 1)
		return response.SuggestionMessage[0]
	}
	first, second := render(), render()
	require.NotSame(t, original, first)
	require.NotSame(t, first, second)
	require.True(t, proto.Equal(expected, first), "rendering must preserve every completion field and protobuf unknown field")
	require.True(t, proto.Equal(expected, second))
	require.True(t, proto.Equal(snapshot, original), "rendering must not change the cached source")
	require.Len(t, source, 3, "nil filtering must not compact the source slice")
	require.Nil(t, source[0])
	require.Same(t, original, source[1])
	require.Nil(t, source[2])

	first.Description = "one request's description"
	first.InsertText = "f"
	first.ProtoReflect().GetUnknown()[2] = 0x09
	require.True(t, proto.Equal(expected, second), "a response must not share mutable data with another request")
	require.True(t, proto.Equal(snapshot, original))
	require.Nil(t, applyExampleFenceToResponse(nil))

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	errors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			for n := 0; n < 8; n++ {
				response := applyExampleFenceToResponse(&ypb.YaklangLanguageSuggestionResponse{SuggestionMessage: source})
				if len(response.SuggestionMessage) != 1 {
					errors <- fmt.Errorf("concurrent rendering produced %d suggestions", len(response.SuggestionMessage))
					return
				}
				actual := response.SuggestionMessage[0]
				if actual == original || !proto.Equal(expected, actual) {
					errors <- fmt.Errorf("concurrent rendering shared a source object or lost completion fields")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.True(t, proto.Equal(snapshot, original))
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionResponseCachedRPCOwnership(t *testing.T) {
	server := &Server{}
	fuzztagSuggestions, _ := _getAllFuzztagSuggestionInfo()
	cases := []struct {
		name   string
		cached []*ypb.SuggestionDescription
		call   func() (*ypb.YaklangLanguageSuggestionResponse, error)
	}{
		{
			name:   "syntaxflow native calls",
			cached: getNativeCallSuggestion(),
			call: func() (*ypb.YaklangLanguageSuggestionResponse, error) {
				return server.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
					YakScriptType: "syntaxflow", InspectType: COMPLETION, Range: &ypb.Range{Code: "<"},
				})
			},
		},
		{
			name:   "fuzztag language completion",
			cached: fuzztagSuggestions,
			call: func() (*ypb.YaklangLanguageSuggestionResponse, error) {
				return server.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
					YakScriptType: "fuzztag", InspectType: COMPLETION, Range: &ypb.Range{Code: "{{"},
				})
			},
		},
		{
			name:   "fuzztag dedicated completion",
			cached: fuzztagSuggestions,
			call: func() (*ypb.YaklangLanguageSuggestionResponse, error) {
				return server.FuzzTagSuggestion(context.Background(), &ypb.FuzzTagSuggestionRequest{
					InspectType: COMPLETION, FuzztagCode: "{{",
				})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NotEmpty(t, tc.cached)
			snapshots := make([]*ypb.SuggestionDescription, len(tc.cached))
			expected := make([]*ypb.SuggestionDescription, len(tc.cached))
			for i, item := range tc.cached {
				require.NotNil(t, item)
				snapshots[i] = proto.Clone(item).(*ypb.SuggestionDescription)
				expected[i] = proto.Clone(item).(*ypb.SuggestionDescription)
				expected[i].Label = RenderExampleMarkersForMarkdown(item.Label)
				expected[i].Description = RenderExampleMarkersForMarkdown(item.Description)
			}
			first, err := tc.call()
			require.NoError(t, err)
			second, err := tc.call()
			require.NoError(t, err)
			require.Len(t, first.SuggestionMessage, len(expected))
			require.Len(t, second.SuggestionMessage, len(expected))
			for i, item := range tc.cached {
				require.NotSame(t, item, first.SuggestionMessage[i])
				require.NotSame(t, first.SuggestionMessage[i], second.SuggestionMessage[i])
				require.True(t, proto.Equal(expected[i], first.SuggestionMessage[i]))
				require.True(t, proto.Equal(expected[i], second.SuggestionMessage[i]))
			}
			first.SuggestionMessage[0].Label = "one request's label"
			first.SuggestionMessage[0].Description = "one request's documentation"
			first.SuggestionMessage[0].InsertText = "one request's snippet"
			require.True(t, proto.Equal(expected[0], second.SuggestionMessage[0]))

			const workers = 32
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(workers)
			errors := make(chan error, workers)
			for n := 0; n < workers; n++ {
				go func() {
					defer wg.Done()
					<-start
					for repeat := 0; repeat < 4; repeat++ {
						response, err := tc.call()
						if err != nil {
							errors <- err
							return
						}
						if response == nil || len(response.SuggestionMessage) != len(expected) {
							errors <- fmt.Errorf("concurrent RPC returned an incomplete suggestion list")
							return
						}
						for i, actual := range response.SuggestionMessage {
							if actual == tc.cached[i] || !proto.Equal(expected[i], actual) {
								errors <- fmt.Errorf("concurrent RPC suggestion %d shared its cache or lost fields", i)
								return
							}
						}
					}
				}()
			}
			close(start)
			wg.Wait()
			close(errors)
			for err := range errors {
				require.NoError(t, err)
			}
			for i, original := range tc.cached {
				require.True(t, proto.Equal(snapshots[i], original), "RPC requests changed cached suggestion %d", i)
			}
		})
	}
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionBeforeParenContext(t *testing.T) {
	prog, err := ssaapi.Parse("(1)")
	require.NoError(t, err)
	editor := memedit.NewMemEditor("(1)")
	prog.Program.PushEditor(editor)
	defer prog.Program.PopEditor(false)
	rng := editor.GetRangeOffset(0, 0)
	noEditor := ssaapi.NewTmpProgram("language-completion-no-editor")
	_, hasEditor := noEditor.Program.GetEditor("")
	require.False(t, hasEditor)
	cases := []struct {
		name string
		prog *ssaapi.Program
		rng  *memedit.Range
		trim bool
	}{
		{name: "missing program", rng: rng},
		{name: "missing SSA program", prog: &ssaapi.Program{}, rng: rng},
		{name: "missing editor", prog: noEditor, rng: rng},
		{name: "missing range must not mean offset zero", prog: prog},
		{name: "existing parenthesis at offset zero", prog: prog, rng: rng, trim: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			function := &ypb.SuggestionDescription{Label: "f", InsertText: "f(${1:value})", Kind: CompletionKindFunction}
			method := &ypb.SuggestionDescription{Label: "m", InsertText: "m(${1:value})", Kind: CompletionKindMethod}
			keyword := &ypb.SuggestionDescription{Label: "if", InsertText: "if (${1:condition}) {}", Kind: CompletionKindKeyword}
			actual := fixCompletionBeforeParen([]*ypb.SuggestionDescription{nil, function, method, keyword}, tc.prog, tc.rng, nil)
			require.Len(t, actual, 4)
			require.Nil(t, actual[0])
			if tc.trim {
				require.Equal(t, "f", actual[1].InsertText)
				require.Equal(t, "m", actual[2].InsertText)
			} else {
				require.Equal(t, "f(${1:value})", actual[1].InsertText)
				require.Equal(t, "m(${1:value})", actual[2].InsertText)
			}
			require.Equal(t, "if (${1:condition}) {}", actual[3].InsertText, "only function and method snippets may lose their call arguments")
		})
	}
}
