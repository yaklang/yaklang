package yakgrpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestGRPCMUSTPASS_LANGUAGE_SuggestionCacheConcurrentPublication(t *testing.T) {
	const workers = 64
	var cache completionSuggestionCache
	var builds atomic.Int32
	start := make(chan struct{})
	buildStarted := make(chan struct{})
	releaseBuild := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(workers)
	var entering sync.WaitGroup
	entering.Add(workers)

	expected := make([]*ypb.SuggestionDescription, 8)
	for i := range expected {
		expected[i] = &ypb.SuggestionDescription{
			Label:             fmt.Sprintf("method%d", i),
			Description:       fmt.Sprintf("method %d documentation", i),
			InsertText:        fmt.Sprintf("method%d(${1:value})", i),
			JustAppend:        i%2 == 0,
			DefinitionVerbose: fmt.Sprintf("method%d(value string) string", i),
			Kind:              CompletionKindMethod,
			Command:           fmt.Sprintf("command%d", i),
		}
		expected[i].ProtoReflect().SetUnknown([]byte{0x80, 0x06, byte(i)})
	}
	build := func() []*ypb.SuggestionDescription {
		if builds.Add(1) == 1 {
			close(buildStarted)
		}
		<-releaseBuild
		items := make([]*ypb.SuggestionDescription, len(expected))
		for i, item := range expected {
			items[i] = proto.Clone(item).(*ypb.SuggestionDescription)
		}
		return items
	}
	type result struct {
		items   []*ypb.SuggestionDescription
		byLabel []*ypb.SuggestionDescription
	}
	results := make(chan result, workers)
	for i := 0; i < workers; i++ {
		go func() {
			ready.Done()
			<-start
			entering.Done()
			items := cache.get(build)
			indexed := make([]*ypb.SuggestionDescription, len(expected))
			for i, item := range expected {
				indexed[i] = cache.byLabel[item.Label]
			}
			results <- result{items: items, byLabel: indexed}
		}()
	}
	ready.Wait()
	close(start)
	entering.Wait()
	<-buildStarted
	// Hold the builder while the callers enter the cold cache. Publishing a
	// partial slice or map must never let a caller finish before this barrier.
	select {
	case <-results:
		close(releaseBuild)
		t.Fatal("a caller observed the cache before its builder finished")
	default:
	}
	close(releaseBuild)
	for i := 0; i < workers; i++ {
		result := <-results
		require.Len(t, result.items, len(expected), "caller %d saw a partial cache", i)
		for index, want := range expected {
			require.NotNil(t, result.items[index])
			require.True(t, proto.Equal(want, result.items[index]), "caller %d item %d lost a field", i, index)
			require.Same(t, result.items[index], result.byLabel[index], "the index and slice must publish together")
		}
	}
	require.EqualValues(t, 1, builds.Load())
	require.Len(t, cache.byLabel, len(expected))
	items := cache.get(func() []*ypb.SuggestionDescription {
		t.Fatal("a warm cache must not rebuild its suggestions")
		return nil
	})
	require.Len(t, items, len(expected))

	t.Run("empty cache is initialized once", func(t *testing.T) {
		var empty completionSuggestionCache
		var calls int
		buildEmpty := func() []*ypb.SuggestionDescription {
			calls++
			return nil
		}
		require.Empty(t, empty.get(buildEmpty))
		require.Empty(t, empty.get(buildEmpty))
		require.Empty(t, empty.byLabel)
		require.Equal(t, 1, calls)
	})
	t.Run("nil entries never reach a request", func(t *testing.T) {
		var sparse completionSuggestionCache
		items := sparse.get(func() []*ypb.SuggestionDescription {
			return []*ypb.SuggestionDescription{nil, expected[0], nil}
		})
		require.Len(t, sparse.byLabel, 1)
		require.Same(t, expected[0], sparse.byLabel[expected[0].Label])
		owned := cloneCompletionSuggestions(items)
		require.Len(t, owned, 1)
		require.NotSame(t, expected[0], owned[0])
		require.True(t, proto.Equal(expected[0], owned[0]))
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionCacheCloneIsolation(t *testing.T) {
	original := &ypb.SuggestionDescription{
		Label:             "Contains",
		Description:       "string containment",
		InsertText:        "Contains(${1:substr})$0",
		JustAppend:        true,
		DefinitionVerbose: "Contains(substr string) bool",
		Kind:              CompletionKindMethod,
		Command:           "editor.action.triggerParameterHints",
	}
	original.ProtoReflect().SetUnknown([]byte{0x80, 0x06, 0x07})
	other := &ypb.SuggestionDescription{Label: "string", InsertText: "string", Kind: CompletionKindClass}
	source := []*ypb.SuggestionDescription{nil, original, nil, other, nil}
	first := cloneCompletionSuggestions(source)
	second := cloneCompletionSuggestions(source)
	require.Len(t, first, 2)
	require.Len(t, second, 2)
	for i, want := range []*ypb.SuggestionDescription{original, other} {
		require.NotNil(t, first[i])
		require.NotSame(t, want, first[i])
		require.NotSame(t, first[i], second[i])
		require.True(t, proto.Equal(want, first[i]), "cloning must preserve every protobuf field")
		require.True(t, proto.Equal(want, second[i]))
	}

	first[0].Label = "changed label"
	first[0].Description = "changed description"
	first[0].InsertText = "Contains"
	first[0].JustAppend = false
	first[0].DefinitionVerbose = "changed definition"
	first[0].Kind = CompletionKindFunction
	first[0].Command = "changed command"
	first[0].ProtoReflect().GetUnknown()[2] = 0x09
	require.True(t, proto.Equal(original, second[0]), "one request must not change another request or the cache")
	require.Equal(t, "Contains(${1:substr})$0", original.InsertText)
	require.Equal(t, []byte{0x80, 0x06, 0x07}, []byte(original.ProtoReflect().GetUnknown()))
	first[1] = &ypb.SuggestionDescription{Label: "replaced slice entry"}
	require.Same(t, other, source[3], "cloning must allocate its own slice")
	require.True(t, proto.Equal(other, second[1]))
	require.Nil(t, source[0])
	require.Nil(t, source[2])
	require.Nil(t, source[4])
	require.Len(t, source, 5, "nil filtering must not compact the source in place")
	require.Empty(t, cloneCompletionSuggestions(nil))
	require.Empty(t, cloneCompletionSuggestions([]*ypb.SuggestionDescription{nil, nil}))
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionCacheRequestContextIsolation(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)
	findContains := func(items []*ypb.SuggestionDescription) *ypb.SuggestionDescription {
		t.Helper()
		var found *ypb.SuggestionDescription
		for _, item := range items {
			require.NotNil(t, item)
			if item.Label == "Contains" {
				require.Nil(t, found, "builtin suggestions must not contain duplicate labels")
				found = item
			}
		}
		require.NotNil(t, found, "the string builtin Contains must be present")
		require.Equal(t, CompletionKindMethod, found.Kind)
		return found
	}
	getContains := func(code string, rng *ypb.Range) *ypb.SuggestionDescription {
		t.Helper()
		response, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
			InspectType:   COMPLETION,
			YakScriptType: "yak",
			YakScriptCode: code,
			Range:         rng,
			ModelID:       uuid.NewString(),
		})
		require.NoError(t, err)
		return findContains(response.SuggestionMessage)
	}
	memberCode := "a = \"abc\"\na."
	memberRange := &ypb.Range{Code: "a.", StartLine: 2, StartColumn: 1, EndLine: 2, EndColumn: 3}
	before := getContains(memberCode, memberRange)
	require.Equal(t, "Contains(${1:substr})$0", before.InsertText)
	contextCode := "a = \"abc\"\na.Contains(\"a\")"
	contextProgram, err := static_analyzer.SSAParse(contextCode, "yak", ssaapi.WithEnableCache(false))
	require.NoError(t, err)
	contextRange := GrpcRangeToSSARange(contextCode, &ypb.Range{
		Code: "a.Contains", StartLine: 2, StartColumn: 1, EndLine: 2, EndColumn: 11,
	})
	contextEditor := contextRange.GetEditor()
	require.Equal(t, "(", contextEditor.GetTextFromOffset(contextRange.GetEndOffset(), contextRange.GetEndOffset()+1))
	stringValue := contextProgram.Ref("a").Get(0)
	require.NotNil(t, stringValue)
	// Parsed files are saved by URL. The existing context helper requires an
	// active editor; exercise that supported path with a private SSA program.
	contextProgram.Program.PushEditor(contextEditor)
	defer contextProgram.Program.PopEditor(false)
	beforeParen := findContains(OnCompletion(contextProgram, "a.Contains", true, false, contextRange, "yak", stringValue))
	require.Equal(t, "Contains", beforeParen.InsertText, "an existing parenthesis requires only the method name")
	after := getContains(memberCode, memberRange)
	require.Equal(t, "Contains(${1:substr})$0", after.InsertText, "a previous request must not strip the cached parameter placeholders")
	require.True(t, proto.Equal(before, after), "the ordinary member suggestion must retain all fields")
}
