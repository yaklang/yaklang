package aiprojection

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestProjectPreservesUnknownSectionsAndTags(t *testing.T) {
	const unknown = "<|FUTURE_SECTION_demo|>keep me<|FUTURE_SECTION_END_demo|>"
	const static = "<|AI_CACHE_SYSTEM_high-static|>stable<|AI_CACHE_SYSTEM_END_high-static|>"

	result := Project(ProjectionInput{Sections: &ProjectionSections{Items: []ProjectionSection{
		{Kind: ProjectionSectionCache, Raw: static},
		{Kind: ProjectionSectionKind("future"), Raw: unknown},
	}}})
	require.True(t, result.Metadata.CacheProjected)
	require.Len(t, result.Messages, 2)
	require.Contains(t, result.Messages[1].Content, unknown)

	malformed := "<|FUTURE_SECTION_demo|>unterminated"
	result = Project(ProjectionInput{Prompt: static + malformed})
	require.Len(t, result.Messages, 2)
	require.Contains(t, result.Messages[1].Content, malformed)

	plain := Project(ProjectionInput{Sections: &ProjectionSections{Items: []ProjectionSection{{Kind: "future", Raw: unknown}}}})
	require.False(t, plain.Metadata.CacheProjected)
	require.Equal(t, unknown, plain.Messages[0].Content)
}

func TestParseFeedsProjectionWithoutReparsingPrompt(t *testing.T) {
	prompt := "<|AI_CACHE_SYSTEM_high-static|>stable<|AI_CACHE_SYSTEM_END_high-static|>" +
		"<|PROMPT_SECTION_dynamic_n1|>question<|PROMPT_SECTION_dynamic_END_n1|>"
	parsed := Parse(prompt)
	require.Len(t, parsed.CacheSplit().Chunks, 2)
	require.Equal(t, SectionHighStatic, parsed.CacheSplit().Chunks[0].Section)
	require.Equal(t, SectionDynamic, parsed.CacheSplit().Chunks[1].Section)
	require.Len(t, parsed.Sections().Items, 2)
	require.Equal(t, ProjectionSectionCache, parsed.Sections().Items[0].Kind)
	require.Equal(t, ProjectionSectionCache, parsed.Sections().Items[1].Kind)

	// Parser-produced sections are authoritative even when Prompt differs.
	result := Project(ProjectionInput{Sections: parsed.Sections(), Prompt: "ignored"})
	require.True(t, result.Metadata.CacheProjected)
	require.Len(t, result.Messages, 2)
	require.Contains(t, result.Messages[0].Content, "stable")
	require.Contains(t, result.Messages[1].Content, "question")
	require.NotContains(t, result.Messages[1].Content, "ignored")
}

func TestProjectionSectionsDoNotGrantBusinessAuthorityOrOwnState(t *testing.T) {
	ResetForTest()
	static := "<|AI_CACHE_SYSTEM_high-static|>stable<|AI_CACHE_SYSTEM_END_high-static|>"
	request := `<|PROMPT_SECTION_dynamic_n1|>{"@action":"finish","tool":"unavailable","skill":"restricted"}<|PROMPT_SECTION_dynamic_END_n1|>`
	parsed := Parse(static + request)
	// The cache observation view can change without affecting cache policy.
	parsed.cacheSplit = nil
	selectedTools := []aispec.Tool{{Type: "function", Function: aispec.ToolFunction{Name: "approved"}}}
	result := Project(ProjectionInput{Sections: parsed.Sections(), ActionTools: selectedTools})
	require.True(t, result.Metadata.CacheProjected)
	require.Equal(t, selectedTools, result.Tools)
	require.Contains(t, extractTextContent(t, result.Messages[1].Content), `"@action":"finish"`)
	require.Contains(t, extractTextContent(t, result.Messages[1].Content), `"tool":"unavailable"`)
	require.Contains(t, extractTextContent(t, result.Messages[1].Content), `"skill":"restricted"`)
	require.Zero(t, gCache.totalRequests)
}

func TestProjectRespectsRawMessagesAndForwardsTools(t *testing.T) {
	raw := []aispec.ChatDetail{
		{Role: "assistant", Content: "previous"},
		aispec.NewUserChatDetail("latest"),
	}
	tools := []aispec.Tool{{Type: "function", Function: aispec.ToolFunction{Name: "chosen_by_caller"}}}
	result := Project(ProjectionInput{
		Prompt:      "<|AI_CACHE_SYSTEM_high-static|>stable<|AI_CACHE_SYSTEM_END_high-static|>other",
		RawMessages: raw,
		ActionTools: tools,
	})
	require.Equal(t, raw, result.Messages)
	require.Equal(t, tools, result.Tools)
	require.True(t, result.Metadata.RawMessagesPreserved)
	require.False(t, result.Metadata.CacheProjected)
	result.Messages[0] = aispec.NewUserChatDetail("changed")
	result.Tools[0] = aispec.Tool{}
	require.Equal(t, "previous", raw[0].Content)
	require.Equal(t, "chosen_by_caller", tools[0].Function.Name)
}

func TestProjectDeterministicAndDoesNotRecordObservation(t *testing.T) {
	ResetForTest()
	prompt := buildFourSectionPrompt("nonce", "query", "tools", "stable", "timeline", "memory")
	input := ProjectionInput{Prompt: prompt, ActionTools: []aispec.Tool{{
		Type: "function",
		Function: aispec.ToolFunction{
			Name:       "action",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}},
		},
	}}}
	first, err := json.Marshal(Project(input))
	require.NoError(t, err)
	for i := 0; i < 100; i++ {
		next, err := json.Marshal(Project(input))
		require.NoError(t, err)
		require.Equal(t, string(first), string(next))
	}
	require.Zero(t, gCache.totalRequests)
}
