package aireact

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"strings"
	"testing"
)

func loopPromptSection(t *testing.T, prompt, name string) string {
	t.Helper()
	startTag := aiprojection.CreateTemplate("<|PROMPT_SECTION_" + name + "|>")
	endTag := aiprojection.CreateTemplate("<|PROMPT_SECTION_END_" + name + "|>")
	start := strings.Index(prompt, startTag)
	require.NotEqual(t, -1, start, "missing %s", name)
	start += len(startTag)
	end := strings.Index(prompt[start:], endTag)
	require.NotEqual(t, -1, end, "missing end of %s", name)
	return prompt[start : start+end]
}
