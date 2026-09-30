package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func getExecFuzztagTool(t *testing.T) *aitool.Tool {
	t.Helper()
	content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/codec/exec_fuzztag.yak")
	require.NoError(t, err)
	for _, issue := range yak.StaticAnalyze(string(content)) {
		require.NotEqual(t, result.Error, issue.Severity, issue.String())
	}
	metadata := yakscripttools.LoadYakScriptToAiTools("exec_fuzztag", string(content))
	require.NotNil(t, metadata)
	require.Equal(t, "FuzzTag 模板生成", metadata.VerboseNameZh)
	var inputSchema map[string]any
	require.NoError(t, json.Unmarshal([]byte(metadata.Params), &inputSchema))
	properties := inputSchema["properties"].(map[string]any)
	require.Contains(t, properties, "variables")
	require.NotContains(t, properties, "params", "variables must not collide with the action's params envelope")
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
	require.Len(t, tools, 1)
	return tools[0]
}

func runExecFuzztag(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	raw, err := tool.Callback(context.Background(), params, nil, &stdout, &stderr)
	require.NoError(t, err, stderr.String())
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	var receipt map[string]any
	require.NoError(t, json.Unmarshal(encoded, &receipt))
	require.Contains(t, receipt, "success")
	return receipt, stdout.String()
}

func TestExecFuzztagStdout(t *testing.T) {
	tool := getExecFuzztagTool(t)
	receipt, stdout := runExecFuzztag(t, tool, aitool.InvokeParams{
		"template": "通知：{{list(小王|小李)}}，任务{{int(1-2)}}已完成。",
	})
	require.Equal(t, true, receipt["success"])
	require.Equal(t, float64(4), receipt["count"])
	require.Equal(t, float64(len([]byte(stdout))), receipt["bytes"], "Chinese output must report UTF-8 bytes rather than character count")
	require.Equal(t, false, receipt["truncated"])
	require.ElementsMatch(t, []string{"通知：小王，任务1已完成。", "通知：小李，任务1已完成。", "通知：小王，任务2已完成。", "通知：小李，任务2已完成。"}, strings.Split(strings.TrimSuffix(stdout, "\n"), "\n"))

	receipt, stdout = runExecFuzztag(t, tool, aitool.InvokeParams{"template": "{{repeat(abc|3)}}"})
	require.Equal(t, float64(3), receipt["count"])
	require.Equal(t, "abc\nabc\nabc\n", stdout, "the delivery tool must preserve duplicates")
}

func TestExecFuzztagJSONVariables(t *testing.T) {
	receipt, stdout := runExecFuzztag(t, getExecFuzztagTool(t), aitool.InvokeParams{
		"template":  "{{params(name)}}\n他说：\"100%\" \\ {{int(1-2)}}",
		"variables": map[string]any{"name": []string{"小王", "小李"}},
		"format":    "json",
	})
	require.Equal(t, true, receipt["success"])
	var rows []string
	require.Equal(t, float64(len([]byte(stdout))), receipt["bytes"])
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows), "stdout should contain only the JSON array")
	require.ElementsMatch(t, []string{"小王\n他说：\"100%\" \\ 1", "小王\n他说：\"100%\" \\ 2", "小李\n他说：\"100%\" \\ 1", "小李\n他说：\"100%\" \\ 2"}, rows)
}

func TestExecFuzztagFile(t *testing.T) {
	tool := getExecFuzztagTool(t)
	target := filepath.Join(t.TempDir(), "nested", "items.txt")
	params := aitool.InvokeParams{"template": "item{{int(1-20)}}", "output-file": target}
	receipt, stdout := runExecFuzztag(t, tool, params)
	require.Equal(t, true, receipt["success"])
	require.Equal(t, target, receipt["output_file"])
	require.Equal(t, false, receipt["stdout"])
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, float64(len(content)), receipt["bytes"])
	require.Len(t, strings.Split(strings.TrimSuffix(string(content), "\n"), "\n"), 20)
	require.Contains(t, stdout, target)
	require.NotContains(t, stdout, "item20", "file-only mode must not echo the full artifact")

	params["template"] = "changed"
	receipt, _ = runExecFuzztag(t, tool, params)
	require.Equal(t, false, receipt["success"])
	preserved, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, content, preserved)

	params["force"] = true
	params["stdout"] = true
	receipt, stdout = runExecFuzztag(t, tool, params)
	require.Equal(t, true, receipt["success"])
	require.Contains(t, stdout, "changed\n")
	overwritten, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "changed\n", string(overwritten))
}

func TestExecFuzztagUnicodeFileReceipt(t *testing.T) {
	target := filepath.Join(t.TempDir(), "unicode.txt")
	template := strings.Repeat("中", 250)
	receipt, _ := runExecFuzztag(t, getExecFuzztagTool(t), aitool.InvokeParams{"template": template, "output-file": target})
	require.Equal(t, true, receipt["success"])
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, template+"\n", string(content))
	require.Equal(t, float64(len(content)), receipt["bytes"])
	preview := receipt["preview"].([]any)
	require.Equal(t, strings.Repeat("中", 200)+"...", preview[0], "preview must remain valid UTF-8 while limiting characters")
}

func TestExecFuzztagLimitAndErrors(t *testing.T) {
	tool := getExecFuzztagTool(t)
	receipt, stdout := runExecFuzztag(t, tool, aitool.InvokeParams{"template": "{{int(1-1000000000)}}", "limit": 2})
	require.Equal(t, true, receipt["success"])
	require.Equal(t, true, receipt["truncated"])
	require.Equal(t, float64(2), receipt["count"])
	require.Equal(t, "1\n2\n", stdout)
	receipt, _ = runExecFuzztag(t, tool, aitool.InvokeParams{"template": "{{int(1-2)}}", "limit": 2})
	require.Equal(t, false, receipt["truncated"])

	for _, invalid := range []aitool.InvokeParams{
		{"template": "hello", "limit": 0},
		{"template": "hello", "limit": 100001},
		{"template": "hello", "format": "invalid"},
		{"template": "{{regen([)}}"},
	} {
		target := filepath.Join(t.TempDir(), "must-not-exist.txt")
		invalid["output-file"] = target
		receipt, _ = runExecFuzztag(t, tool, invalid)
		require.Equal(t, false, receipt["success"], invalid)
		require.NotEmpty(t, receipt["error"])
		_, err := os.Stat(target)
		require.True(t, os.IsNotExist(err), "invalid rendering must not create an output file")
	}
}

func TestExecFuzztagFileTags(t *testing.T) {
	dictionary := filepath.Join(t.TempDir(), "words.txt")
	require.NoError(t, os.WriteFile(dictionary, []byte("alpha\nbeta\n"), 0o600))
	receipt, stdout := runExecFuzztag(t, getExecFuzztagTool(t), aitool.InvokeParams{
		"template": "word={{file:line(" + dictionary + ")}}", "enable-file-tags": true,
	})
	require.Equal(t, true, receipt["success"])
	require.Equal(t, "word=alpha\nword=beta\n", stdout)
}

func TestExecFuzztagCanceled(t *testing.T) {
	target := filepath.Join(t.TempDir(), "canceled.txt")
	tool := getExecFuzztagTool(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	_, err := tool.Callback(ctx, aitool.InvokeParams{"template": "{{int(1-1000000000)}}", "output-file": target}, nil, &stdout, &stderr)
	require.Error(t, err)
	_, statErr := os.Stat(target)
	require.True(t, os.IsNotExist(statErr))
}

// Keep the script runtime alive while its rendering context expires, so the
// test exercises the semantic receipt and file protection inside the script.
func TestExecFuzztagDeadlineReceipt(t *testing.T) {
	content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/codec/exec_fuzztag.yak")
	require.NoError(t, err)
	start := strings.Index(string(content), "// Declare the semantic result")
	require.NotEqual(t, -1, start)
	target := filepath.Join(t.TempDir(), "must-not-exist.txt")
	prefix := fmt.Sprintf(`template = "{{int(1-1000000000)}}"
variables = {}
outputFile = %q
printStdout = false
format = "lines"
limit = 100000
force = false
enableFileTags = false
`, target)
	body := strings.Replace(string(content)[start:], `injectedCtx = getParam("CTX")`, `injectedCtx = context.Seconds(0.01)`, 1)
	engine, err := yak.NewScriptEngine(1).ExecuteExWithContext(context.Background(), prefix+body, nil)
	require.NoError(t, err)
	raw, ok := engine.GetVar("RESULT")
	require.True(t, ok)
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	var receipt map[string]any
	require.NoError(t, json.Unmarshal(encoded, &receipt))
	require.Equal(t, false, receipt["success"])
	require.Contains(t, receipt["error"], "render canceled")
	_, err = os.Stat(target)
	require.True(t, os.IsNotExist(err), "a deadline must not deliver a partial file as success")
}

func TestExecFuzztagCompositionAndTagSemantics(t *testing.T) {
	tool := getExecFuzztagTool(t)
	cases := []struct {
		template string
		expected []string
	}{
		{"{{array::row(A|B)}}={{int::row(1-2)}}", []string{"A=1", "B=2"}},
		{"{{base64({{array(hello|world)}})}}", []string{"aGVsbG8=", "d29ybGQ="}},
		{"{{={{int(1-2)}}=}}", []string{"{{int(1-2)}}"}},
		{"{{int(1-5|3|2)}}", []string{"001", "003", "005"}},
		{"{{repeat:range(abc|3)}}", []string{"abc", "abcabc", "abcabcabc"}},
		{"{{null(3)}}", []string{"\x00", "\x00", "\x00"}},
		{"{{crlf(3)}}", []string{"\r\n", "\r\n", "\r\n"}},
		{`{{nth({{unquote(a\nb\nc)}}|1)}}`, []string{"b"}},
		{`{{first({{unquote(a\nb)}})}}`, []string{"a"}},
		{`{{last({{unquote(a\nb)}})}}`, []string{"b"}},
		{"{{padding:zero(abc|5)}}", []string{"abc00"}},
		{"{{padding:zero(abc|-5)}}", []string{"00abc"}},
		{"{{padding:null(abc|5)}}", []string{"\x00\x00abc"}},
		{"{{padding:null(abc|-5)}}", []string{"\x00\x00abc"}},
		{`{{jsonpath({{={"key":"value"}=}}|$.key)}}`, []string{"value"}},
		{"{{base64(hello)}}", []string{"aGVsbG8="}},
		{"{{base64dec(aGVsbG8=)}}", []string{"hello"}},
		{"{{hex(abc)}}", []string{"616263"}},
		{"{{hexdec(616263)}}", []string{"abc"}},
		{"{{base64({{hex(abc)}})}}", []string{"NjE2MjYz"}},
		{"{{hexdec({{base64dec(NjE2MjYz)}})}}", []string{"abc"}},
		{`{{urlescape({{base64({{={"a":1}=}})}})}}`, []string{"eyJhIjoxfQ%3D%3D"}},
	}
	for _, c := range cases {
		t.Run(c.template, func(t *testing.T) {
			receipt, stdout := runExecFuzztag(t, tool, aitool.InvokeParams{"template": c.template, "format": "json"})
			require.Equal(t, true, receipt["success"])
			var rows []string
			require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
			require.Equal(t, c.expected, rows)
		})
	}
}

func TestExecFuzztagHTTPPacket(t *testing.T) {
	target := filepath.Join(t.TempDir(), "packets.json")
	template := "GET /query?id={{base64({{={\"a\":1}=}})}} HTTP/1.1\r\nHost: {{params(host)}}\r\n\r\n"
	receipt, stdout := runExecFuzztag(t, getExecFuzztagTool(t), aitool.InvokeParams{
		"template":    template,
		"variables":   map[string]any{"host": []string{"example.com", "example.org"}},
		"output-file": target, "stdout": true, "format": "json",
	})
	require.Equal(t, true, receipt["success"])
	require.Equal(t, float64(2), receipt["count"])
	require.Equal(t, false, receipt["truncated"])
	var packets []string
	require.NoError(t, json.Unmarshal([]byte(stdout), &packets))
	require.Equal(t, []string{
		"GET /query?id=eyJhIjoxfQ== HTTP/1.1\r\nHost: example.com\r\n\r\n",
		"GET /query?id=eyJhIjoxfQ== HTTP/1.1\r\nHost: example.org\r\n\r\n",
	}, packets)
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, stdout, string(content))
	require.Equal(t, float64(len(content)), receipt["bytes"])
}
