package aicommon

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	promptSectionTagName = "PROMPT_SECTION"
	dynamicSectionName   = "dynamic"
)

// AITagBlock describes a parsed AITag block. It is primarily used by tests.
type AITagBlock struct {
	Nonce      string
	StartIndex int
	EndIndex   int
	Body       string
}

// ExtractPromptNonce returns the first legal nonce found for any of the tags.
func ExtractPromptNonce(prompt string, tagNames ...string) string {
	for _, tagName := range tagNames {
		if nonce := ExtractPipeTagNonce(prompt, tagName); nonce != "" {
			return nonce
		}
		if nonce := ExtractAngleTagNonce(prompt, tagName); nonce != "" {
			return nonce
		}
	}
	return ""
}

// MustExtractPromptNonce fails the test if no legal nonce is found.
func MustExtractPromptNonce(t testing.TB, prompt string, tagNames ...string) string {
	t.Helper()

	nonce := ExtractPromptNonce(prompt, tagNames...)
	require.NotEmpty(t, nonce, "failed to find nonce for tags %v in prompt:\n%s", tagNames, prompt)
	return nonce
}

func promptSectionTag(sectionName string) string {
	return promptSectionTagName + "_" + sectionName
}

// ExtractPromptSectionNonce extracts the nonce from a prompt section wrapper.
func ExtractPromptSectionNonce(prompt string, sectionName string) string {
	return ExtractPromptNonce(prompt, promptSectionTag(sectionName))
}

// MustExtractPromptSectionNonce fails the test if the prompt section nonce is missing.
func MustExtractPromptSectionNonce(t testing.TB, prompt string, sectionName string) string {
	t.Helper()

	nonce := ExtractPromptSectionNonce(prompt, sectionName)
	require.NotEmpty(t, nonce, "failed to find prompt section nonce for %q in prompt:\n%s", sectionName, prompt)
	return nonce
}

// ExtractDynamicSectionNonce extracts the loop dynamic section nonce.
func ExtractDynamicSectionNonce(prompt string) string {
	return ExtractPromptSectionNonce(prompt, dynamicSectionName)
}

// MustExtractDynamicSectionNonce fails the test if the loop dynamic section nonce is missing.
func MustExtractDynamicSectionNonce(t testing.TB, prompt string) string {
	t.Helper()
	return MustExtractPromptSectionNonce(t, prompt, dynamicSectionName)
}

// IsLegalNonce reports whether nonce should be treated as a real start tag nonce.
func IsLegalNonce(data string) bool {
	return data != "" && strings.ToLower(data) != "end"
}

// ExtractPipeTagNonce extracts a nonce from tags like <|TAG_nonce|> or <|TAG_nonce>.
func ExtractPipeTagNonce(prompt string, tagName string) string {
	startRe := regexp.MustCompile(fmt.Sprintf(`<\|%s_([^\s\|\n>]+)(?:\s*\|>|\s*>)`, regexp.QuoteMeta(tagName)))
	matches := startRe.FindAllStringSubmatch(prompt, -1)
	for _, match := range matches {
		if len(match) <= 1 {
			continue
		}
		if IsLegalNonce(match[1]) {
			return match[1]
		}
	}
	return ""
}

// ExtractAngleTagNonce extracts a nonce from legacy tags like <TAG_nonce>.
func ExtractAngleTagNonce(prompt string, tagName string) string {
	startRe := regexp.MustCompile(fmt.Sprintf(`<%s_([^\s>\n]+)>`, regexp.QuoteMeta(tagName)))
	matches := startRe.FindAllStringSubmatch(prompt, -1)
	for _, match := range matches {
		if len(match) <= 1 {
			continue
		}
		if IsLegalNonce(match[1]) {
			return match[1]
		}
	}
	return ""
}

// MustExtractAITagBlock fails the test if the named block cannot be found.
func MustExtractAITagBlock(t testing.TB, prompt string, tagName string) AITagBlock {
	t.Helper()

	startRe := regexp.MustCompile(fmt.Sprintf(`<\|%s_([^\s\|\n>]+)\|>`, regexp.QuoteMeta(tagName)))
	loc := startRe.FindStringSubmatchIndex(prompt)
	require.Len(t, loc, 4, "failed to find %s start block in prompt:\n%s", tagName, prompt)

	nonce := prompt[loc[2]:loc[3]]
	startContent := loc[1]
	endMarker := fmt.Sprintf("<|%s_END_%s|>", tagName, nonce)
	endOffset := strings.Index(prompt[startContent:], endMarker)
	require.NotEqual(t, -1, endOffset, "failed to find %s end block in prompt:\n%s", tagName, prompt)

	return AITagBlock{
		Nonce:      nonce,
		StartIndex: loc[0],
		EndIndex:   startContent + endOffset + len(endMarker),
		Body:       strings.TrimSpace(prompt[startContent : startContent+endOffset]),
	}
}
