package aireact

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type aiTagBlock struct {
	Nonce      string
	StartIndex int
	EndIndex   int
	Body       string
}

func extractPromptNonce(prompt string, tagNames ...string) string {
	for _, tagName := range tagNames {
		if nonce := extractPipeTagNonce(prompt, tagName); nonce != "" {
			return nonce
		}
		if nonce := extractAngleTagNonce(prompt, tagName); nonce != "" {
			return nonce
		}
	}
	return ""
}

func mustExtractPromptNonce(t *testing.T, prompt string, tagNames ...string) string {
	t.Helper()

	nonce := extractPromptNonce(prompt, tagNames...)
	require.NotEmpty(t, nonce, "failed to find nonce for tags %v in prompt:\n%s", tagNames, prompt)
	return nonce
}

func extractPipeTagNonce(prompt string, tagName string) string {
	startRe := regexp.MustCompile(fmt.Sprintf(`<\|%s_([^\s\|\n>]+)(?:\s*\|>|\s*>)`, regexp.QuoteMeta(tagName)))
	matches := startRe.FindStringSubmatch(prompt)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func extractAngleTagNonce(prompt string, tagName string) string {
	startRe := regexp.MustCompile(fmt.Sprintf(`<%s_([^\s>\n]+)>`, regexp.QuoteMeta(tagName)))
	matches := startRe.FindStringSubmatch(prompt)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func mustExtractAITagBlock(t *testing.T, prompt string, tagName string) aiTagBlock {
	t.Helper()

	startRe := regexp.MustCompile(fmt.Sprintf(`<\|%s_([^\s\|\n>]+)\|>`, regexp.QuoteMeta(tagName)))
	loc := startRe.FindStringSubmatchIndex(prompt)
	require.Len(t, loc, 4, "failed to find %s start block in prompt:\n%s", tagName, prompt)

	nonce := prompt[loc[2]:loc[3]]
	startContent := loc[1]
	endMarker := fmt.Sprintf("<|%s_END_%s|>", tagName, nonce)
	endOffset := strings.Index(prompt[startContent:], endMarker)
	require.NotEqual(t, -1, endOffset, "failed to find %s end block in prompt:\n%s", tagName, prompt)

	return aiTagBlock{
		Nonce:      nonce,
		StartIndex: loc[0],
		EndIndex:   startContent + endOffset + len(endMarker),
		Body:       strings.TrimSpace(prompt[startContent : startContent+endOffset]),
	}
}

func TestExtractPromptNonce(t *testing.T) {
	t.Run("extracts standard ai tag nonce", func(t *testing.T) {
		prompt := "<|FINAL_ANSWER_ab12|>\nbody\n<|FINAL_ANSWER_END_ab12|>"
		require.Equal(t, "ab12", extractPromptNonce(prompt, "FINAL_ANSWER"))
	})

	t.Run("extracts relaxed cache tool nonce", func(t *testing.T) {
		prompt := "<|CACHE_TOOL_CALL_xy99>\nbody"
		require.Equal(t, "xy99", extractPromptNonce(prompt, "CACHE_TOOL_CALL"))
	})

	t.Run("extracts legacy angle tag nonce", func(t *testing.T) {
		prompt := "<background_n123>\nbody"
		require.Equal(t, "n123", extractPromptNonce(prompt, "background"))
	})
}
