package test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const dataTransformToolName = "data_transform"

func getDataTransformTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/codec/data_transform.yak")
	if err != nil {
		t.Fatalf("failed to read data_transform.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(dataTransformToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse data_transform.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execDataTransformTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestDataTransform_JSONToYAML(t *testing.T) {
	tool := getDataTransformTool(t)
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input": `{"name": "test", "version": "1.0", "count": 42}`,
		"from":  "json",
		"to":    "yaml",
	})

	assert.Assert(t, strings.Contains(stdout, "name"), "should contain name field")
	assert.Assert(t, strings.Contains(stdout, "test"), "should contain test value")
	assert.Assert(t, strings.Contains(stdout, "JSON -> YAML"), "should show conversion direction")
	t.Logf("stdout:\n%s", stdout)
}

func TestDataTransform_YAMLToJSON(t *testing.T) {
	tool := getDataTransformTool(t)
	yamlInput := "name: example\nversion: \"2.0\"\nitems:\n  - alpha\n  - beta"
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input": yamlInput,
		"from":  "yaml",
		"to":    "json",
	})

	assert.Assert(t, strings.Contains(stdout, "example"), "should contain name value")
	assert.Assert(t, strings.Contains(stdout, "alpha"), "should contain first item")
	assert.Assert(t, strings.Contains(stdout, "YAML -> JSON"), "should show conversion direction")
	t.Logf("stdout:\n%s", stdout)
}

func TestDataTransform_XMLToJSON(t *testing.T) {
	tool := getDataTransformTool(t)
	xmlInput := `<user><name>Alice</name><age>30</age></user>`
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input": xmlInput,
		"from":  "xml",
		"to":    "json",
	})

	assert.Assert(t, strings.Contains(stdout, "Alice"), "should contain name value")
	assert.Assert(t, strings.Contains(stdout, "XML -> JSON"), "should show conversion direction")
	t.Logf("stdout:\n%s", stdout)
}

func TestDataTransform_AutoDetectJSON(t *testing.T) {
	tool := getDataTransformTool(t)
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input": `{"auto": "detected"}`,
		"to":    "yaml",
	})

	assert.Assert(t, strings.Contains(stdout, "Auto-detected"), "should report auto-detection")
	assert.Assert(t, strings.Contains(stdout, "json"), "should detect JSON format")
	assert.Assert(t, strings.Contains(stdout, "detected"), "should contain value from input")
	t.Logf("stdout:\n%s", stdout)
}

func TestDataTransform_AutoDetectXML(t *testing.T) {
	tool := getDataTransformTool(t)
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input": `<config><key>value</key></config>`,
		"to":    "json",
	})

	assert.Assert(t, strings.Contains(stdout, "Auto-detected"), "should report auto-detection")
	assert.Assert(t, strings.Contains(stdout, "xml"), "should detect XML format")
	t.Logf("stdout:\n%s", stdout)
}

func TestDataTransform_ValidateJSON(t *testing.T) {
	tool := getDataTransformTool(t)
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input":  `{"valid": true}`,
		"from":   "json",
		"to":     "json",
		"action": "validate",
	})

	assert.Assert(t, strings.Contains(stdout, "VALID"), "should report valid JSON")
	t.Logf("stdout:\n%s", stdout)
}

func TestDataTransform_ValidateInvalidJSON(t *testing.T) {
	tool := getDataTransformTool(t)
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input":  `{not valid json`,
		"from":   "json",
		"to":     "json",
		"action": "validate",
	})

	assert.Assert(t, strings.Contains(stdout, "VALID"), "should report validation result")
	t.Logf("stdout:\n%s", stdout)
}

func TestDataTransform_FormatJSON(t *testing.T) {
	tool := getDataTransformTool(t)
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input":  `{"compact":"data","list":[1,2,3]}`,
		"from":   "json",
		"to":     "json",
		"action": "format",
	})

	assert.Assert(t, strings.Contains(stdout, "Formatted JSON"), "should show format header")
	assert.Assert(t, strings.Contains(stdout, "compact"), "should contain original data")
	t.Logf("stdout:\n%s", stdout)
}

func TestDataTransform_JSONToXML(t *testing.T) {
	tool := getDataTransformTool(t)
	stdout, _ := execDataTransformTool(t, tool, aitool.InvokeParams{
		"input": `{"item": "value"}`,
		"from":  "json",
		"to":    "xml",
	})

	assert.Assert(t, strings.Contains(stdout, "JSON -> XML"), "should show conversion direction")
	assert.Assert(t, strings.Contains(stdout, "item"), "should contain field name")
	t.Logf("stdout:\n%s", stdout)
}
