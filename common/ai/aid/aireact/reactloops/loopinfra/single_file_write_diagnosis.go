package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const LoopVarLastAIDecisionResponse = "last_ai_decision_response"

// DiagnoseMissingWriteCode explains why write_{suffix} found no extracted code in the AI tag variable.
func (f *SingleFileModificationSuiteFactory) DiagnoseMissingWriteCode(loop *reactloops.ReActLoop) string {
	codeVar := f.GetCodeVariableName()
	tagName := f.aiTagName
	actionName := f.GetActionName("write")

	raw := ""
	if loop != nil {
		raw = loop.Get(LoopVarLastAIDecisionResponse)
	}

	tagOpenPrefix := "<|" + tagName + "_"
	hasTagBlock := strings.Contains(raw, tagOpenPrefix)
	hasMarkdownFence := strings.Contains(raw, "```")

	var detail string
	switch {
	case raw == "":
		detail = "the AI response stream was empty or unavailable for diagnosis"
	case hasMarkdownFence && !hasTagBlock:
		detail = fmt.Sprintf(
			"the response uses markdown code fences (```) which are NOT parsed into %q; after the JSON line you MUST append %s<nonce>|>...|%sEND_<nonce>|>",
			codeVar, tagOpenPrefix, tagOpenPrefix,
		)
	case !hasTagBlock:
		detail = fmt.Sprintf(
			"the response is missing %s<nonce>|>...|%sEND_<nonce>|> after the @action JSON",
			tagOpenPrefix, tagOpenPrefix,
		)
	default:
		detail = fmt.Sprintf(
			"a %s tag block was present but %q stayed empty (wrong nonce, empty block, or tag closed before content streamed)",
			tagName, codeVar,
		)
	}

	return fmt.Sprintf(
		"No code generated in %q action: %s. Do NOT use markdown code blocks.",
		actionName, detail,
	)
}
