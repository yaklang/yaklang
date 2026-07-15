package mcp

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

// TestMCPSchemaOpenAICompatDiagnostic inspects every builtin legacy MCP tool
// schema for patterns that are known to break OpenAI-style function calling /
// tool providers (e.g. "invalid_request_error" / "Upstream request failed").
//
// It does NOT start a network server; it reads the registered tool definitions
// directly so it stays fast and deterministic.
func TestMCPSchemaOpenAICompatDiagnostic(t *testing.T) {
	diag := newOpenAICompatDiagnostic()

	for _, tw := range GlobalBuiltinTools() {
		if tw == nil || tw.tool == nil {
			continue
		}
		diag.checkTool(tw.tool)
	}

	diag.printReport(t)

	// This is a diagnostic test: it prints the report but does not fail.
	// Once we decide which compatibility rules are hard requirements we can
	// convert the report into assertions.
}

// openAICompatDiagnostic collects schema compatibility issues.
type openAICompatDiagnostic struct {
	checked            int
	rootNotObject      []string
	missingType        []string
	disallowedKeywords []string
	emptyEnum          []string
	mixedEnumTypes     []string
	deeplyNested       []string
	emptyDescription   []string
	totalSchemaBytes   int
}

func newOpenAICompatDiagnostic() *openAICompatDiagnostic {
	return &openAICompatDiagnostic{}
}

func (d *openAICompatDiagnostic) checkTool(tool *mcp.Tool) {
	d.checked++

	raw, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return
	}
	d.totalSchemaBytes += len(raw)

	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return
	}

	if typ, _ := schema["type"].(string); typ != "object" {
		d.rootNotObject = append(d.rootNotObject, tool.Name)
	}

	d.checkNode(tool.Name, "", schema, 0)
}

func (d *openAICompatDiagnostic) checkNode(toolName, path string, node map[string]interface{}, depth int) {
	const maxDepth = 4 // OpenAI recommends shallow nesting
	if depth > maxDepth {
		d.deeplyNested = append(d.deeplyNested, fmt.Sprintf("%s.%s", toolName, path))
		return
	}

	if node == nil {
		return
	}

	if _, hasType := node["type"]; !hasType {
		if _, hasOneOf := node["oneOf"]; !hasOneOf {
			if _, hasAnyOf := node["anyOf"]; !hasAnyOf {
				if _, hasAllOf := node["allOf"]; !hasAllOf {
					d.missingType = append(d.missingType, fmt.Sprintf("%s.%s", toolName, path))
				}
			}
		}
	}

	for _, kw := range []string{"oneOf", "anyOf", "allOf"} {
		if _, ok := node[kw]; ok {
			d.disallowedKeywords = append(d.disallowedKeywords, fmt.Sprintf("%s.%s: %s", toolName, path, kw))
		}
	}

	if enum, ok := node["enum"].([]interface{}); ok {
		if len(enum) == 0 {
			d.emptyEnum = append(d.emptyEnum, fmt.Sprintf("%s.%s", toolName, path))
		} else {
			firstType := fmt.Sprintf("%T", enum[0])
			for _, v := range enum[1:] {
				if fmt.Sprintf("%T", v) != firstType {
					d.mixedEnumTypes = append(d.mixedEnumTypes, fmt.Sprintf("%s.%s", toolName, path))
					break
				}
			}
		}
	}

	if desc, _ := node["description"].(string); desc == "" && path != "" {
		typ, _ := node["type"].(string)
		if typ == "object" || typ == "array" {
			d.emptyDescription = append(d.emptyDescription, fmt.Sprintf("%s.%s", toolName, path))
		}
	}

	if props, ok := node["properties"].(map[string]interface{}); ok {
		for k, v := range props {
			child, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			d.checkNode(toolName, childPath, child, depth+1)
		}
	}

	if items, ok := node["items"].(map[string]interface{}); ok {
		d.checkNode(toolName, path+".items", items, depth+1)
	}
}

func (d *openAICompatDiagnostic) printReport(t *testing.T) {
	fmt.Println("\n========== MCP Schema OpenAI-Compat Diagnostic ==========")
	fmt.Printf("Tools checked: %d\n", d.checked)
	fmt.Printf("Total inputSchema bytes: %d (%.1f KB)\n", d.totalSchemaBytes, float64(d.totalSchemaBytes)/1024)

	printSection := func(title string, items []string) {
		fmt.Printf("\n%s: %d\n", title, len(items))
		for _, it := range items {
			fmt.Printf("  - %s\n", it)
		}
	}

	printSection("Root inputSchema.type != object", d.rootNotObject)
	printSection("Properties missing type", d.missingType)
	printSection("Disallowed keywords (oneOf/anyOf/allOf)", d.disallowedKeywords)
	printSection("Empty enum", d.emptyEnum)
	printSection("Mixed-type enum", d.mixedEnumTypes)
	printSection("Nested deeper than 4 levels", d.deeplyNested)
	printSection("Object/array properties with empty description", d.emptyDescription)
	fmt.Println("========================================================")
}
