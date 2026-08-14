package aicache

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func buildReasoningReplayTag(t *testing.T, nonce, reasoning, content string) string {
	t.Helper()
	payload, err := json.Marshal(modelThinkingReplayRecord{ReasoningContent: reasoning, Content: content})
	require.NoError(t, err)
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>",
		timelineModelThinkingReplayTagName, nonce, payload, timelineModelThinkingReplayTagName, nonce)
}

func messageText(t *testing.T, message aispec.ChatDetail) string {
	t.Helper()
	switch content := message.Content.(type) {
	case string:
		return content
	case []*aispec.ChatContent:
		var result strings.Builder
		for _, part := range content {
			if part != nil {
				result.WriteString(part.Text)
			}
		}
		return result.String()
	default:
		t.Fatalf("unexpected message content type %T", message.Content)
		return ""
	}
}

func TestExpandReasoningReplayMessagesStringOrdering(t *testing.T) {
	tag := buildReasoningReplayTag(t, "abc1", "reason-one", `{"@action":"require_tool","tool_require_payload":"read_file"}`)
	result := expandReasoningReplayMessages(&aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			aispec.NewSystemChatDetail("system"),
			aispec.NewUserChatDetail("context-before\n" + tag + "\nobservation-after"),
		},
	})

	require.Len(t, result.Messages, 4)
	require.Equal(t, []string{"system", "user", "assistant", "user"}, []string{
		result.Messages[0].Role, result.Messages[1].Role, result.Messages[2].Role, result.Messages[3].Role,
	})
	require.Contains(t, messageText(t, result.Messages[1]), "context-before")
	require.Equal(t, "reason-one", result.Messages[2].ReasoningContent)
	require.Contains(t, messageText(t, result.Messages[2]), `"@action":"require_tool"`)
	require.Contains(t, messageText(t, result.Messages[3]), "observation-after")
	for _, message := range result.Messages {
		require.NotContains(t, messageText(t, message), timelineModelThinkingReplayTagName)
	}
}

func TestExpandReasoningReplayMessagesChatContentPreservesCacheControl(t *testing.T) {
	tag := buildReasoningReplayTag(t, "cc1", "cache-reason", `{"@action":"finish"}`)
	cacheControl := map[string]any{"type": "ephemeral"}
	result := expandReasoningReplayMessages(&aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			aispec.NewSystemChatDetail("system"),
			{Role: "user", Content: []*aispec.ChatContent{{Type: "text", Text: "before\n" + tag + "\nafter", CacheControl: cacheControl}}},
			aispec.NewUserChatDetail("current-query"),
		},
	})

	require.Len(t, result.Messages, 5)
	require.Equal(t, "assistant", result.Messages[2].Role)
	before := result.Messages[1].Content.([]*aispec.ChatContent)
	after := result.Messages[3].Content.([]*aispec.ChatContent)
	require.Nil(t, before[len(before)-1].CacheControl)
	require.Equal(t, cacheControl, after[len(after)-1].CacheControl)
	require.Equal(t, "current-query", result.Messages[4].Content)
}

func TestExpandReasoningReplayMessagesMalformedAndTerminalFailClosed(t *testing.T) {
	malformed := &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages:   []aispec.ChatDetail{aispec.NewUserChatDetail("before <|TIMELINE_MODEL_THINKING_bad|>{bad-json}<|TIMELINE_MODEL_THINKING_END_bad|> after")},
	}
	require.Same(t, malformed, expandReasoningReplayMessages(malformed))

	terminal := &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages:   []aispec.ChatDetail{aispec.NewUserChatDetail("before\n" + buildReasoningReplayTag(t, "end1", "reason", `{"@action":"finish"}`))},
	}
	require.Same(t, terminal, expandReasoningReplayMessages(terminal))

	emptyAction := &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{aispec.NewUserChatDetail(
			"before\n" + buildReasoningReplayTag(t, "empty1", "reason", "") + "\nafter",
		)},
	}
	require.Same(t, emptyAction, expandReasoningReplayMessages(emptyAction))

	unclosed := &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{aispec.NewUserChatDetail(
			`before <|TIMELINE_MODEL_THINKING_unclosed|>{"reasoning_content":"r","content":"a"}`,
		)},
	}
	require.Same(t, unclosed, expandReasoningReplayMessages(unclosed))
}

func TestExpandReasoningReplayMessagesSupportsAdjacentRecords(t *testing.T) {
	first := buildReasoningReplayTag(t, "multi1", "reason-one", `{"@action":"require_tool"}`)
	second := buildReasoningReplayTag(t, "multi2", "reason-two", `{"@action":"directly_call_tool"}`)
	result := expandReasoningReplayMessages(&aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{aispec.NewUserChatDetail(
			"before\n" + first + second + "\nafter",
		)},
	})

	require.Equal(t, []string{"user", "assistant", "assistant", "user"}, []string{
		result.Messages[0].Role,
		result.Messages[1].Role,
		result.Messages[2].Role,
		result.Messages[3].Role,
	})
	require.Equal(t, "reason-one", result.Messages[1].ReasoningContent)
	require.Equal(t, "reason-two", result.Messages[2].ReasoningContent)
	for _, message := range result.Messages {
		require.NotContains(t, messageText(t, message), timelineModelThinkingReplayTagName)
	}
}

func TestExpandReasoningReplayMessagesNestedMarkerFailsClosed(t *testing.T) {
	outer := buildReasoningReplayTag(t, "outer", "reason", "action-before <|TIMELINE_MODEL_THINKING_inner|> action-after")
	input := &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{aispec.NewUserChatDetail(
			"before\n" + outer + "\nafter",
		)},
	}
	require.Same(t, input, expandReasoningReplayMessages(input))
}

func TestHijackHighStaticProducesStandardReasoningContent(t *testing.T) {
	tag := buildReasoningReplayTag(t, "live1", "previous reasoning", `{"@action":"require_tool","tool_require_payload":"read_file"}`)
	prompt := buildFourSectionPrompt(
		"nonce1",
		"compare two files",
		"tool schema",
		"system policy",
		"timeline-before\n"+tag+"\ntool observation",
		"memory",
	)

	result := hijackHighStatic(prompt)
	require.NotNil(t, result)
	require.True(t, result.IsHijacked)
	require.Equal(t, "user", result.Messages[len(result.Messages)-1].Role)

	assistantCount := 0
	for _, message := range result.Messages {
		require.NotContains(t, messageText(t, message), timelineModelThinkingReplayTagName)
		if message.Role == "assistant" {
			assistantCount++
			require.Equal(t, "previous reasoning", message.ReasoningContent)
			require.Contains(t, messageText(t, message), `"@action":"require_tool"`)
		}
	}
	require.Equal(t, 1, assistantCount)

	request := aispec.NewChatMessage("deepseek-v4-flash", result.Messages)
	request.EnableThinking = true
	raw, err := json.Marshal(request)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"reasoning_content":"previous reasoning"`)
	require.Contains(t, string(raw), `"enable_thinking":true`)
	require.NotContains(t, string(raw), timelineModelThinkingReplayTagName)
	require.NotContains(t, string(raw), `"prefix"`)
}

func TestReasoningReplayRawMessagesReachChatBaseRequest(t *testing.T) {
	tag := buildReasoningReplayTag(t, "request1", "request reasoning", `{"@action":"require_tool","tool_require_payload":"read_file"}`)
	prompt := buildFourSectionPrompt(
		"requestNonce",
		"current query",
		"tool schema",
		"system policy",
		"prior context\n"+tag+"\ntool observation",
		"memory",
	)
	hijacked := hijackHighStatic(prompt)
	require.NotNil(t, hijacked)

	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		requestBody = append([]byte(nil), body...)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	_, err := aispec.ChatBase(
		server.URL,
		"deepseek-v4-flash",
		"ignored-flat-prompt",
		aispec.WithChatBase_RawMessages(hijacked.Messages),
		aispec.WithChatBase_DisableStream(true),
		aispec.WithChatBase_EnableThinkingEx("enable_thinking", true),
		aispec.WithChatBase_StreamHandler(func(reader io.Reader) { _, _ = io.Copy(io.Discard, reader) }),
		aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
	)
	require.NoError(t, err)
	require.NotEmpty(t, requestBody)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(requestBody, &payload))
	require.Equal(t, true, payload["enable_thinking"])
	require.NotContains(t, payload, "prefix")
	messages, ok := payload["messages"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, messages)
	require.Equal(t, "user", messages[len(messages)-1].(map[string]any)["role"])

	foundAssistant := false
	for _, rawMessage := range messages {
		message := rawMessage.(map[string]any)
		encoded, marshalErr := json.Marshal(message["content"])
		require.NoError(t, marshalErr)
		require.NotContains(t, string(encoded), timelineModelThinkingReplayTagName)
		if message["role"] == "assistant" {
			foundAssistant = true
			require.Equal(t, "request reasoning", message["reasoning_content"])
			require.Contains(t, message["content"], `"@action":"require_tool"`)
		}
	}
	require.True(t, foundAssistant)
}
