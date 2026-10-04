package liteforge

import (
	"bytes"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

const RecentTimelineTokens = 4 * 1024

type PromptParams struct {
	Nonce, Prompt, StaticInstruction, Params, Schema, PersistentMemory string
	FunctionCallSchema, FunctionName                                   string
	TimelineDump, TimelineFrozenBlock, TimelineOpen                    string
}

var textTemplate = template.Must(template.New("liteforge-text").Parse(aiprojection.CreateTemplate(promptloader.MustLoad("ai/aid/liteforge/liteForgePromptTemplate.txt"))))
var functionTemplate = template.Must(template.New("liteforge-function").Parse(aiprojection.CreateTemplate(promptloader.MustLoad("ai/aid/liteforge/liteForgeFunctionPromptTemplate.txt"))))

func RenderPrompt(p PromptParams, native bool) (string, error) {
	t := textTemplate
	if native {
		t = functionTemplate
	}
	var buf bytes.Buffer
	err := t.Execute(&buf, p)
	return buf.String(), err
}

func RecentTimeline(t *aicommon.Timeline) string {
	if t == nil {
		return ""
	}
	return t.DumpRecentForPrompt(RecentTimelineTokens)
}
