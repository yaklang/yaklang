package test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
)

var usageJSONExample = regexp.MustCompile("(?s)```json\\s*\\n(.*?)\\n```")

func TestHTTPRequestAndRiskUsagesRenderForBothCallModes(t *testing.T) {
	paths := []string{
		"http/do_http_request.yak",
		"risk/cybersecurity-risk.yak",
		"risk/ssa-risk.yak",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/" + path)
			if err != nil {
				t.Fatal(err)
			}
			name := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".yak")
			tool := yakscripttools.LoadYakScriptToAiTools(name, string(content))
			if tool == nil {
				t.Fatal("tool metadata did not load")
			}
			for _, mode := range []bool{true, false} {
				usage, err := aitool.RenderUsageForMode(tool.Usage, mode)
				if err != nil {
					t.Fatalf("render mode=%t: %v", mode, err)
				}
				if strings.Contains(usage, "[[-") {
					t.Fatalf("unrendered template directive in mode=%t", mode)
				}
				if mode && strings.Contains(usage, "<|TOOL_PARAM_") {
					t.Fatal("AITAG marker leaked into native function-call usage")
				}
				if !mode && path != "risk/ssa-risk.yak" && !strings.Contains(usage, "<|TOOL_PARAM_") {
					t.Fatal("text-call usage lost its multiline parameter example")
				}
				matches := usageJSONExample.FindAllStringSubmatch(usage, -1)
				if len(matches) == 0 {
					t.Fatal("usage has no JSON example")
				}
				for _, match := range matches {
					var value map[string]any
					if err := json.Unmarshal([]byte(match[1]), &value); err != nil {
						t.Fatalf("invalid JSON example mode=%t: %v\n%s", mode, err, match[1])
					}
				}
			}
		})
	}
}
