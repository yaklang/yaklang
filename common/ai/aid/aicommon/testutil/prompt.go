// Package testutil contains prompt assertions shared by AI package tests.
package testutil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

const (
	promptSectionTagName = "PROMPT_SECTION"
	dynamicSectionName   = "dynamic"
)

// AITagBlock describes a parsed AITag block.
type AITagBlock struct {
	Nonce      string
	StartIndex int
	EndIndex   int
	Body       string
}

type TestingT interface {
	Helper()
	Fatalf(format string, args ...interface{})
}

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

func ExtractPromptSectionNonce(prompt string, sectionName string) string {
	nonce := ExtractPromptNonce(prompt, promptSectionTagName+"_"+sectionName)
	// Prompt section envelopes carry a process signature after the section's
	// own nonce. Simulated model responses must use only the latter for AITags.
	return strings.TrimSuffix(nonce, "_"+aiprojection.Nonce())
}

func ExtractDynamicSectionNonce(prompt string) string {
	// Projection signs the section marker with a process nonce. AI response
	// tags use only the turn nonce preceding that signature.
	return strings.TrimSuffix(ExtractPromptSectionNonce(prompt, dynamicSectionName), "_"+aiprojection.Nonce())
}

func IsLegalNonce(data string) bool {
	return data != "" && strings.ToLower(data) != "end"
}

func ExtractPipeTagNonce(prompt string, tagName string) string {
	startRe := regexp.MustCompile(fmt.Sprintf(`<\|%s_([^\s\|\n>]+)(?:\s*\|>|\s*>)`, regexp.QuoteMeta(tagName)))
	matches := startRe.FindAllStringSubmatch(prompt, -1)
	for _, match := range matches {
		if len(match) > 1 && IsLegalNonce(match[1]) {
			return match[1]
		}
	}
	return ""
}

func ExtractAngleTagNonce(prompt string, tagName string) string {
	startRe := regexp.MustCompile(fmt.Sprintf(`<%s_([^\s>\n]+)>`, regexp.QuoteMeta(tagName)))
	matches := startRe.FindAllStringSubmatch(prompt, -1)
	for _, match := range matches {
		if len(match) > 1 && IsLegalNonce(match[1]) {
			return match[1]
		}
	}
	return ""
}

func MustExtractPromptNonce(t TestingT, prompt string, tagNames ...string) string {
	t.Helper()
	nonce := ExtractPromptNonce(prompt, tagNames...)
	if nonce == "" {
		t.Fatalf("failed to find nonce for tags %v in prompt:\n%s", tagNames, prompt)
	}
	return nonce
}

func MustExtractPromptSectionNonce(t TestingT, prompt string, sectionName string) string {
	t.Helper()
	nonce := ExtractPromptSectionNonce(prompt, sectionName)
	if nonce == "" {
		t.Fatalf("failed to find prompt section nonce for %q in prompt:\n%s", sectionName, prompt)
	}
	return nonce
}

func MustExtractDynamicSectionNonce(t TestingT, prompt string) string {
	t.Helper()
	nonce := ExtractDynamicSectionNonce(prompt)
	if nonce == "" {
		t.Fatalf("failed to find dynamic prompt section nonce in prompt:\n%s", prompt)
	}
	return nonce
}

func MustExtractAITagBlock(t TestingT, prompt string, tagName string) AITagBlock {
	t.Helper()
	startRe := regexp.MustCompile(fmt.Sprintf(`<\|%s_([^\s\|\n>]+)\|>`, regexp.QuoteMeta(tagName)))
	loc := startRe.FindStringSubmatchIndex(prompt)
	if len(loc) != 4 {
		t.Fatalf("failed to find %s start block in prompt:\n%s", tagName, prompt)
	}

	nonce := prompt[loc[2]:loc[3]]
	startContent := loc[1]
	endMarker := fmt.Sprintf("<|%s_END_%s|>", tagName, nonce)
	endOffset := strings.Index(prompt[startContent:], endMarker)
	if endOffset == -1 {
		t.Fatalf("failed to find %s end block in prompt:\n%s", tagName, prompt)
	}

	return AITagBlock{
		Nonce:      nonce,
		StartIndex: loc[0],
		EndIndex:   startContent + endOffset + len(endMarker),
		Body:       strings.TrimSpace(prompt[startContent : startContent+endOffset]),
	}
}

// ExtractPlanCurrentTask reads the selected node only, avoiding false matches
// in the immutable definition which intentionally includes sibling goals.
func ExtractPlanCurrentTask(prompt string) string {
	start := strings.LastIndex(prompt, "# Plan Runtime State")
	if start < 0 {
		return ""
	}
	rest := prompt[start:]
	nonce := ExtractPipeTagNonce(rest, "USER_INTERACT")
	if nonce == "" {
		return ""
	}
	open := "<|USER_INTERACT_" + nonce + "|>"
	close := "<|USER_INTERACT_END_" + nonce + "|>"
	begin := strings.Index(rest, open) + len(open)
	end := strings.Index(rest[begin:], close)
	if end < 0 {
		return ""
	}
	var state struct {
		TaskID string `json:"current_task_id"`
		Name   string `json:"current_task_name"`
		Goal   string `json:"current_task_goal"`
	}
	if json.Unmarshal([]byte(rest[begin:begin+end]), &state) != nil {
		return ""
	}
	return state.TaskID + "\n任务名称: " + state.Name + "\n任务目标: " + state.Goal
}
