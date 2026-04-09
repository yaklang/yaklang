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

func mustExtractAITagBlock(t *testing.T, prompt string, tagName string) aiTagBlock {
	t.Helper()

	startRe := regexp.MustCompile(fmt.Sprintf(`<\|%s_([^\|\n>]+)\|>`, regexp.QuoteMeta(tagName)))
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
