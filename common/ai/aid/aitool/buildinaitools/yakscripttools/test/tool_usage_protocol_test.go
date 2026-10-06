package test

import (
	"encoding/json"
	"io/fs"
	"path"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
)

// Exercise every embedded builtin, rather than a handpicked list. The example
// envelopes differ, but business values (including AITAG bodies) must match and
// validate against the real production parameter schema in both modes.
func TestBusinessToolUsagesKeepActionAndParameterProtocolsSeparate(t *testing.T) {
	embedded := yakscripttools.GetEmbedFS()
	count := 0
	err := fs.WalkDir(embedded, "yakscriptforai", func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path.Ext(filename) != ".yak" {
			return nil
		}
		count++
		t.Run(strings.TrimPrefix(filename, "yakscriptforai/"), func(t *testing.T) {
			content, err := embedded.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			name := strings.TrimSuffix(path.Base(filename), ".yak")
			metadata := yakscripttools.LoadYakScriptToAiTools(name, string(content))
			if metadata == nil {
				t.Fatal("tool metadata did not load")
			}
			if !strings.Contains(metadata.Usage, "[[- if .FunctionCallMode -]]") {
				t.Fatal("tool examples have no explicit protocol branch")
			}
			tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
			if len(tools) != 1 {
				t.Fatal("tool did not convert")
			}
			var toolSchema aitool.InvokeParams
			if err := json.Unmarshal([]byte(tools[0].ToJSONSchemaString()), &toolSchema); err != nil {
				t.Fatal(err)
			}
			properties := toolSchema.GetObject("properties").GetObject("params").GetObject("properties")
			var nativeExamples []map[string]any
			for _, native := range []bool{true, false} {
				usage, err := aitool.RenderUsageForMode(metadata.Usage, native)
				if err != nil {
					t.Fatal(err)
				}
				// Embedded sources use platform checkout newlines. AITAG examples
				// must compare with JSON string values independently of CRLF/LF.
				usage = strings.ReplaceAll(usage, "\r\n", "\n")
				if strings.Contains(usage, "[[-") || (native && (strings.Contains(usage, "\"@action\"") || strings.Contains(usage, "\"directly_call_tool_name\"") || strings.Contains(usage, "\"directly_call_tool_params\""))) {
					t.Fatalf("mode=%t: tool usage leaked a template or text action", native)
				}
				if native && strings.Contains(usage, "<|TOOL_PARAM_") {
					t.Fatal("native example leaked external AITAG fields")
				}
				matches := usageJSONExample.FindAllStringSubmatchIndex(usage, -1)
				if len(matches) == 0 {
					t.Fatal("tool has no JSON call example")
				}
				var examples []map[string]any
				for i, match := range matches {
					var call map[string]any
					if err := json.Unmarshal([]byte(usage[match[2]:match[3]]), &call); err != nil {
						t.Fatal(err)
					}
					if !native && call["directly_call_tool_name"] != name {
						t.Fatalf("example calls %v instead of %s", call["directly_call_tool_name"], name)
					}
					if !native && call["@action"] != "directly_call_tool" {
						t.Fatal("text example lacks the directly_call_tool action")
					}
					params := call
					if !native {
						var ok bool
						params, ok = call["directly_call_tool_params"].(map[string]any)
						if !ok {
							t.Fatal("text example lacks a business parameter object")
						}
						end := len(usage)
						if i+1 < len(matches) {
							end = matches[i+1][0]
						}
						mergeUsageExampleTags(t, params, usage[match[1]:end])
					}
					// Execution accepts extra business fields for compatibility.
					// Documentation must still spell known parameter names.
					for key := range params {
						if _, known := properties[key]; !known {
							t.Fatalf("example uses unknown parameter %q", key)
						}
					}
					if valid, problems := tools[0].ValidateParams(params); !valid {
						t.Fatalf("mode=%t: example violates production schema: %v", native, problems)
					}
					// HTTP JSON uses explicit CRLF; displayed AITAG packet lines
					// use source newlines. Compare the same HTTP message content.
					for _, key := range []string{"packet", "request", "response"} {
						if raw, ok := params[key].(string); ok {
							params[key] = strings.TrimRight(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
						}
					}
					examples = append(examples, params)
				}
				if native {
					nativeExamples = examples
				} else if !reflect.DeepEqual(nativeExamples, examples) {
					t.Fatalf("business values differ between native and text examples:\nnative=%v\ntext=%v", nativeExamples, examples)
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no builtin tools were checked")
	}
	t.Logf("validated both protocols for %d builtin tools", count)
}

var usageExampleTag = regexp.MustCompile("<\\|TOOL_PARAM_([^\\n|]+)_\\{NONCE\\}\\|>\\n")

func mergeUsageExampleTags(t *testing.T, params map[string]any, suffix string) {
	t.Helper()
	for _, match := range usageExampleTag.FindAllStringSubmatchIndex(suffix, -1) {
		name := suffix[match[2]:match[3]]
		if strings.HasSuffix(name, "_END") {
			continue
		}
		end := strings.Index(suffix[match[1]:], "\n<|TOOL_PARAM_"+name+"_END_{NONCE}|>")
		if end < 0 {
			t.Fatalf("example has an unclosed AITAG for %s", name)
		}
		params[name] = suffix[match[1] : match[1]+end]
	}
}
