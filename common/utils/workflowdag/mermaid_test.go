package workflowdag

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== ID Sanitization Tests ====================

func TestMUSTPASS_SanitizeMermaidID_SimpleAlphanumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc", "abc"},
		{"ABC", "ABC"},
		{"abc123", "abc123"},
		{"ABC123", "ABC123"},
		{"a1b2c3", "a1b2c3"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeMermaidID(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.True(t, ValidateMermaidID(result), "sanitized ID should be valid: %s", result)
		})
	}
}

func TestMUSTPASS_SanitizeMermaidID_StartsWithDigit(t *testing.T) {
	// IDs starting with digits should be prefixed with 'n'
	tests := []struct {
		input string
	}{
		{"123"},
		{"1abc"},
		{"0"},
		{"9test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeMermaidID(tt.input)
			assert.True(t, strings.HasPrefix(result, "n"), "ID starting with digit should have 'n' prefix: %s -> %s", tt.input, result)
			assert.True(t, ValidateMermaidID(result), "sanitized ID should be valid: %s", result)
		})
	}
}

func TestMUSTPASS_SanitizeMermaidID_SpecialCharacters(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"hello-world"},
		{"hello.world"},
		{"hello:world"},
		{"hello/world"},
		{"hello\\world"},
		{"hello@world"},
		{"hello#world"},
		{"hello$world"},
		{"hello%world"},
		{"hello^world"},
		{"hello&world"},
		{"hello*world"},
		{"hello(world)"},
		{"hello[world]"},
		{"hello{world}"},
		{"hello|world"},
		{"hello<world>"},
		{"hello\"world"},
		{"hello'world"},
		{"hello`world"},
		{"hello~world"},
		{"hello!world"},
		{"hello?world"},
		{"hello=world"},
		{"hello+world"},
		{"hello world"},    // space
		{"hello\tworld"},   // tab
		{"hello\nworld"},   // newline
		{"hello\rworld"},   // carriage return
		{"hello\r\nworld"}, // CRLF
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeMermaidID(tt.input)
			assert.True(t, ValidateMermaidID(result), "sanitized ID should be valid: input=%q, result=%s", tt.input, result)
		})
	}
}

func TestMUSTPASS_SanitizeMermaidID_Underscore(t *testing.T) {
	// Underscores should be escaped by doubling
	tests := []struct {
		input    string
		expected string
	}{
		{"hello_world", "hello__world"},
		{"_start", "__start"},
		{"end_", "end__"},
		{"a_b_c", "a__b__c"},
		{"___", "______"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeMermaidID(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.True(t, ValidateMermaidID(result), "sanitized ID should be valid: %s", result)
		})
	}
}

func TestMUSTPASS_SanitizeMermaidID_Unicode(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"你好世界"},
		{"こんにちは"},
		{"안녕하세요"},
		{"مرحبا"},
		{"שלום"},
		{"Привет"},
		{"🎉🎊"}, // Emojis
		{"hello你好world"},
		{"αβγδ"}, // Greek
		{"日本語テスト"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeMermaidID(tt.input)
			assert.True(t, ValidateMermaidID(result), "sanitized ID should be valid: input=%q, result=%s", tt.input, result)
		})
	}
}

func TestMUSTPASS_SanitizeMermaidID_EmptyString(t *testing.T) {
	result := sanitizeMermaidID("")
	assert.Equal(t, "_empty_", result)
	assert.True(t, ValidateMermaidID(result))
}

func TestMUSTPASS_SanitizeMermaidID_VeryLongString(t *testing.T) {
	// Test that very long strings are truncated
	longString := strings.Repeat("a", 1000)
	result := sanitizeMermaidID(longString)
	// Should be truncated and end with "_etc"
	assert.True(t, len(result) < 200, "result should be truncated: len=%d", len(result))
	assert.True(t, ValidateMermaidID(result), "sanitized ID should be valid: %s", result)
}

func TestMUSTPASS_SanitizeMermaidID_MermaidReservedWords(t *testing.T) {
	// Test Mermaid reserved words
	reservedWords := []string{
		"end",
		"subgraph",
		"direction",
		"click",
		"style",
		"classDef",
		"class",
		"linkStyle",
		"graph",
		"flowchart",
	}

	for _, word := range reservedWords {
		t.Run(word, func(t *testing.T) {
			result := sanitizeMermaidID(word)
			// The result should be the same since these are valid identifiers
			// Mermaid handles context-sensitive reserved words
			assert.Equal(t, word, result)
			assert.True(t, ValidateMermaidID(result))
		})
	}
}

// ==================== Label Escaping Tests ====================

func TestMUSTPASS_EscapeMermaidLabel_SimpleText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", `"hello"`},
		{"Hello World", `"Hello World"`},
		{"abc123", `"abc123"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMUSTPASS_EscapeMermaidLabel_Empty(t *testing.T) {
	result := escapeMermaidLabel("")
	assert.Equal(t, `""`, result)
}

func TestMUSTPASS_EscapeMermaidLabel_DoubleQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, `"#quot;hello#quot;"`},
		{`say "hi"`, `"say #quot;hi#quot;"`},
		{`"""`, `"#quot;#quot;#quot;"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMUSTPASS_EscapeMermaidLabel_HTMLCharacters(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<script>", `"#lt;script#gt;"`},
		{"a < b", `"a #lt; b"`},
		{"a > b", `"a #gt; b"`},
		{"a & b", `"a #amp; b"`},
		{"<a & b>", `"#lt;a #amp; b#gt;"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMUSTPASS_EscapeMermaidLabel_Newlines(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello\nworld", `"hello<br/>world"`},
		{"a\nb\nc", `"a<br/>b<br/>c"`},
		{"hello\r\nworld", `"hello<br/>world"`}, // CRLF
		{"hello\rworld", `"helloworld"`},        // CR only is skipped
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMUSTPASS_EscapeMermaidLabel_Brackets(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"[test]", `"#91;test#93;"`},
		{"(test)", `"#40;test#41;"`},
		{"{test}", `"#123;test#125;"`},
		{"[a](b){c}", `"#91;a#93;#40;b#41;#123;c#125;"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMUSTPASS_EscapeMermaidLabel_Pipes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a|b", `"a#124;b"`},
		{"|start|end|", `"#124;start#124;end#124;"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMUSTPASS_EscapeMermaidLabel_Backslash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`a\b`, `"a#92;b"`},
		{`\\`, `"#92;#92;"`},
		{`C:\Users\test`, `"C:#92;Users#92;test"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMUSTPASS_EscapeMermaidLabel_Unicode(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"你好世界"},
		{"こんにちは"},
		{"🎉 Party 🎊"},
		{"α + β = γ"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			// Should start and end with quotes
			assert.True(t, strings.HasPrefix(result, `"`))
			assert.True(t, strings.HasSuffix(result, `"`))
		})
	}
}

func TestMUSTPASS_EscapeMermaidLabel_ComplexMix(t *testing.T) {
	// Test a complex mix of special characters
	input := `Hello "World" <test> & [array] | {object} \path\n新`
	result := escapeMermaidLabel(input)

	// Verify the result starts and ends with quotes
	assert.True(t, strings.HasPrefix(result, `"`))
	assert.True(t, strings.HasSuffix(result, `"`))

	// Verify special characters are escaped
	assert.Contains(t, result, "#quot;")
	assert.Contains(t, result, "#lt;")
	assert.Contains(t, result, "#gt;")
	assert.Contains(t, result, "#amp;")
	assert.Contains(t, result, "#91;")
	assert.Contains(t, result, "#93;")
	assert.Contains(t, result, "#124;")
	assert.Contains(t, result, "#123;")
	assert.Contains(t, result, "#125;")
	assert.Contains(t, result, "#92;")
}

// ==================== Flowchart Generation Tests ====================

func TestMUSTPASS_GenerateMermaidFlowChart_EmptyDAG(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	_, err := dag.GenerateMermaidFlowChart()
	assert.ErrorIs(t, err, ErrEmptyDAG)
}

func TestMUSTPASS_GenerateMermaidFlowChart_SingleNode(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, `A["A"]`)
}

func TestMUSTPASS_GenerateMermaidFlowChart_SimpleChain(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A -> B -> C (C depends on B, B depends on A)
	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("C", "B")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, `A["A"]`)
	assert.Contains(t, result, `B["B"]`)
	assert.Contains(t, result, `C["C"]`)
	assert.Contains(t, result, "A --> B")
	assert.Contains(t, result, "B --> C")
}

func TestMUSTPASS_GenerateMermaidFlowChart_DiamondPattern(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	//     D
	//    / \
	//   B   C
	//    \ /
	//     A
	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("C", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("D", "B", "C")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "A --> B")
	assert.Contains(t, result, "A --> C")
	assert.Contains(t, result, "B --> D")
	assert.Contains(t, result, "C --> D")
}

func TestMUSTPASS_GenerateMermaidFlowChart_Cycle(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A -> B -> C -> A (cycle)
	require.NoError(t, dag.AddNode(NewTestNode("A", "C")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("C", "B")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "C --> A")
	assert.Contains(t, result, "A --> B")
	assert.Contains(t, result, "B --> C")
}

func TestMUSTPASS_GenerateMermaidFlowChart_SpecialCharacterNodeIDs(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Nodes with special characters in IDs
	require.NoError(t, dag.AddNode(NewTestNode("node-1")))
	require.NoError(t, dag.AddNode(NewTestNode("node.2", "node-1")))
	require.NoError(t, dag.AddNode(NewTestNode("node:3", "node.2")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// The flowchart should be generated without syntax errors
	assert.Contains(t, result, "flowchart TB")
	// Edges should exist
	assert.Contains(t, result, "-->")
}

func TestMUSTPASS_GenerateMermaidFlowChart_UnicodeNodeIDs(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("开始")))
	require.NoError(t, dag.AddNode(NewTestNode("处理", "开始")))
	require.NoError(t, dag.AddNode(NewTestNode("结束", "处理")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	// Labels should contain the original Chinese text
	assert.Contains(t, result, "开始")
	assert.Contains(t, result, "处理")
	assert.Contains(t, result, "结束")
	// Should have edges
	assert.Contains(t, result, "-->")
}

func TestMUSTPASS_GenerateMermaidFlowChart_NumericNodeIDs(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("1")))
	require.NoError(t, dag.AddNode(NewTestNode("2", "1")))
	require.NoError(t, dag.AddNode(NewTestNode("3", "2")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// IDs starting with digits should be prefixed
	assert.Contains(t, result, "n1")
	assert.Contains(t, result, "n2")
	assert.Contains(t, result, "n3")
}

func TestMUSTPASS_GenerateMermaidFlowChart_WithDirection(t *testing.T) {
	directions := []MermaidFlowChartDirection{
		MermaidDirectionTB,
		MermaidDirectionTD,
		MermaidDirectionBT,
		MermaidDirectionLR,
		MermaidDirectionRL,
	}

	for _, dir := range directions {
		t.Run(string(dir), func(t *testing.T) {
			ctx := context.Background()
			dag := New[*TestNode](ctx)

			require.NoError(t, dag.AddNode(NewTestNode("A")))
			require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
			require.NoError(t, dag.Build())

			opts := &MermaidFlowChartOptions{Direction: dir}
			result, err := dag.GenerateMermaidFlowChartWithOptions(opts)
			require.NoError(t, err)

			assert.Contains(t, result, "flowchart "+string(dir))
		})
	}
}

func TestMUSTPASS_GenerateMermaidFlowChart_WithTitle(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.Build())

	opts := &MermaidFlowChartOptions{
		Title: "My Workflow DAG",
	}
	result, err := dag.GenerateMermaidFlowChartWithOptions(opts)
	require.NoError(t, err)

	assert.Contains(t, result, "%% My Workflow DAG")
}

func TestMUSTPASS_GenerateMermaidFlowChart_WithCustomLabelFunc(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.Build())

	opts := &MermaidFlowChartOptions{
		NodeLabelFunc: func(node DAGNode) string {
			return "Node: " + node.GetID()
		},
	}
	result, err := dag.GenerateMermaidFlowChartWithOptions(opts)
	require.NoError(t, err)

	assert.Contains(t, result, `"Node: A"`)
	assert.Contains(t, result, `"Node: B"`)
}

func TestMUSTPASS_GenerateMermaidFlowChart_NoDuplicateEdges(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Node with duplicate dependencies
	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A", "A", "A")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Count occurrences of the edge
	edgeCount := strings.Count(result, "A --> B")
	assert.Equal(t, 1, edgeCount, "should have exactly one edge from A to B")
}

func TestMUSTPASS_GenerateMermaidFlowChart_DisconnectedComponents(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Component 1: A -> B
	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))

	// Component 2: X -> Y (disconnected)
	require.NoError(t, dag.AddNode(NewTestNode("X")))
	require.NoError(t, dag.AddNode(NewTestNode("Y", "X")))

	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "A --> B")
	assert.Contains(t, result, "X --> Y")
}

func TestMUSTPASS_GenerateMermaidFlowChart_MissingDependency(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// B depends on A, but A doesn't exist
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should still generate valid flowchart
	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, `B["B"]`)
	// Should not have an edge to non-existent node
	assert.NotContains(t, result, "A -->")
}

func TestMUSTPASS_GenerateMermaidFlowChart_QuotesInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode(`"quoted"`)))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should escape the quotes in the label
	assert.Contains(t, result, "#quot;")
}

func TestMUSTPASS_GenerateMermaidFlowChart_BracketsInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("[array]")))
	require.NoError(t, dag.AddNode(NewTestNode("(parens)")))
	require.NoError(t, dag.AddNode(NewTestNode("{object}")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should escape brackets in labels
	assert.Contains(t, result, "#91;")  // [
	assert.Contains(t, result, "#93;")  // ]
	assert.Contains(t, result, "#40;")  // (
	assert.Contains(t, result, "#41;")  // )
	assert.Contains(t, result, "#123;") // {
	assert.Contains(t, result, "#125;") // }
}

func TestMUSTPASS_GenerateMermaidFlowChart_NewlinesInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("line1\nline2")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should convert newlines to <br/>
	assert.Contains(t, result, "<br/>")
}

func TestMUSTPASS_GenerateMermaidFlowChart_HTMLInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("<script>alert('xss')</script>")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should escape HTML characters
	assert.Contains(t, result, "#lt;")
	assert.Contains(t, result, "#gt;")
	assert.NotContains(t, result, "<script>")
}

func TestMUSTPASS_GenerateMermaidFlowChart_PipeInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Pipes are used in Mermaid for edge labels, so they need escaping
	require.NoError(t, dag.AddNode(NewTestNode("A|B|C")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "#124;")
}

func TestMUSTPASS_GenerateMermaidFlowChart_BackslashInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode(`C:\Users\test`)))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "#92;")
}

func TestMUSTPASS_GenerateMermaidFlowChart_ComplexSpecialChars(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A node with many special characters
	complexID := `Process "data" <from> [source] | {config}`
	require.NoError(t, dag.AddNode(NewTestNode(complexID)))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should be valid Mermaid syntax (no raw special chars in labels)
	assert.Contains(t, result, "flowchart TB")
	// The label should be properly quoted
	assert.Contains(t, result, `["`)
	assert.Contains(t, result, `"]`)
}

func TestMUSTPASS_GenerateMermaidFlowChart_LargeDAG(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Create a large DAG with 100 nodes in a chain
	for i := 0; i < 100; i++ {
		nodeID := "N" + string(rune('0'+(i%10))) + string(rune('A'+(i/10)))
		var deps []string
		if i > 0 {
			prevID := "N" + string(rune('0'+((i-1)%10))) + string(rune('A'+((i-1)/10)))
			deps = []string{prevID}
		}
		require.NoError(t, dag.AddNode(NewTestNode(nodeID, deps...)))
	}
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	// Should have many edges
	assert.True(t, strings.Count(result, "-->") >= 99)
}

func TestMUSTPASS_GenerateMermaidFlowChart_DeterministicOutput(t *testing.T) {
	// Run multiple times to ensure output is deterministic
	var outputs []string

	for i := 0; i < 5; i++ {
		ctx := context.Background()
		dag := New[*TestNode](ctx)

		require.NoError(t, dag.AddNode(NewTestNode("A")))
		require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
		require.NoError(t, dag.AddNode(NewTestNode("C", "A")))
		require.NoError(t, dag.AddNode(NewTestNode("D", "B", "C")))
		require.NoError(t, dag.Build())

		result, err := dag.GenerateMermaidFlowChart()
		require.NoError(t, err)
		outputs = append(outputs, result)
	}

	// All outputs should be identical
	for i := 1; i < len(outputs); i++ {
		assert.Equal(t, outputs[0], outputs[i], "output should be deterministic")
	}
}

// ==================== Style Generation Tests ====================

func TestMUSTPASS_GenerateMermaidFlowChartWithStyles_BasicTest(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	nodeA := NewTestNode("A")
	nodeB := NewTestNode("B", "A")
	nodeC := NewTestNode("C", "B")

	require.NoError(t, dag.AddNode(nodeA))
	require.NoError(t, dag.AddNode(nodeB))
	require.NoError(t, dag.AddNode(nodeC))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChartWithStyles()
	require.NoError(t, err)

	// Should contain style definitions
	assert.Contains(t, result, "classDef pending")
	assert.Contains(t, result, "classDef processing")
	assert.Contains(t, result, "classDef completed")
	assert.Contains(t, result, "classDef failed")
	assert.Contains(t, result, "classDef skipped")
}

func TestMUSTPASS_GenerateMermaidFlowChartWithStyles_EmptyDAG(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	_, err := dag.GenerateMermaidFlowChartWithStyles()
	assert.ErrorIs(t, err, ErrEmptyDAG)
}

// ==================== Mermaid Reserved Word Tests ====================

func TestMUSTPASS_GenerateMermaidFlowChart_EndKeyword(t *testing.T) {
	// "end" is a reserved word in Mermaid
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("start")))
	require.NoError(t, dag.AddNode(NewTestNode("end", "start")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should still generate valid flowchart
	assert.Contains(t, result, "flowchart TB")
}

func TestMUSTPASS_GenerateMermaidFlowChart_SubgraphKeyword(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("subgraph")))
	require.NoError(t, dag.AddNode(NewTestNode("graph", "subgraph")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
}

// ==================== Edge Case Stress Tests ====================

func TestMUSTPASS_GenerateMermaidFlowChart_OnlySpecialChars(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("!@#$%^&*()")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
}

func TestMUSTPASS_GenerateMermaidFlowChart_OnlyWhitespace(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("   ")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
}

func TestMUSTPASS_GenerateMermaidFlowChart_TabsAndNewlines(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("\t\n\r")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
}

func TestMUSTPASS_GenerateMermaidFlowChart_EmptyStringNodeID(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, "_empty_")
}

func TestMUSTPASS_GenerateMermaidFlowChart_ArrowLikeNodeID(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Node IDs that look like Mermaid arrows
	require.NoError(t, dag.AddNode(NewTestNode("-->")))
	require.NoError(t, dag.AddNode(NewTestNode("---")))
	require.NoError(t, dag.AddNode(NewTestNode("==>")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should escape these properly
	assert.Contains(t, result, "flowchart TB")
}

func TestMUSTPASS_GenerateMermaidFlowChart_SQLInjectionLike(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// SQL injection-like strings
	require.NoError(t, dag.AddNode(NewTestNode("'; DROP TABLE users;--")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
}

func TestMUSTPASS_GenerateMermaidFlowChart_MarkdownInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Markdown-like content
	require.NoError(t, dag.AddNode(NewTestNode("**bold** _italic_ `code`")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
}

func TestMUSTPASS_GenerateMermaidFlowChart_URLInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("https://example.com/path?query=value&other=123")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	// Ampersand should be escaped
	assert.Contains(t, result, "#amp;")
}

func TestMUSTPASS_GenerateMermaidFlowChart_JSONInLabel(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode(`{"key": "value", "arr": [1,2,3]}`)))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	// Brackets and quotes should be escaped
	assert.Contains(t, result, "#123;") // {
	assert.Contains(t, result, "#125;") // }
	assert.Contains(t, result, "#91;")  // [
	assert.Contains(t, result, "#93;")  // ]
	assert.Contains(t, result, "#quot;")
}

func TestMUSTPASS_GenerateMermaidFlowChart_NullBytes(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// String with null bytes
	require.NoError(t, dag.AddNode(NewTestNode("before\x00after")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
}

func TestMUSTPASS_GenerateMermaidFlowChart_MermaidDirectives(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Node ID that looks like Mermaid directives
	require.NoError(t, dag.AddNode(NewTestNode("%%{init: {}}%%")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
}

// ==================== Integration Tests ====================

func TestMUSTPASS_GenerateMermaidFlowChart_RealWorldWorkflow(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Simulate a real CI/CD pipeline
	require.NoError(t, dag.AddNode(NewTestNode("checkout-code")))
	require.NoError(t, dag.AddNode(NewTestNode("install-deps", "checkout-code")))
	require.NoError(t, dag.AddNode(NewTestNode("lint", "install-deps")))
	require.NoError(t, dag.AddNode(NewTestNode("unit-tests", "install-deps")))
	require.NoError(t, dag.AddNode(NewTestNode("build", "lint", "unit-tests")))
	require.NoError(t, dag.AddNode(NewTestNode("integration-tests", "build")))
	require.NoError(t, dag.AddNode(NewTestNode("deploy-staging", "integration-tests")))
	require.NoError(t, dag.AddNode(NewTestNode("e2e-tests", "deploy-staging")))
	require.NoError(t, dag.AddNode(NewTestNode("deploy-production", "e2e-tests")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Verify structure
	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, "checkout")
	assert.Contains(t, result, "deploy")
	assert.Contains(t, result, "-->")
}

func TestMUSTPASS_GenerateMermaidFlowChart_AIToolCallDAG(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Simulate an AI tool call DAG with realistic names
	require.NoError(t, dag.AddNode(NewTestNode("tc_001_search_files")))
	require.NoError(t, dag.AddNode(NewTestNode("tc_002_read_content", "tc_001_search_files")))
	require.NoError(t, dag.AddNode(NewTestNode("tc_003_analyze", "tc_002_read_content")))
	require.NoError(t, dag.AddNode(NewTestNode("tc_004_generate_code", "tc_003_analyze")))
	require.NoError(t, dag.AddNode(NewTestNode("tc_005_write_file", "tc_004_generate_code")))
	require.NoError(t, dag.Build())

	opts := &MermaidFlowChartOptions{
		Direction: MermaidDirectionLR,
		Title:     "AI Tool Call Workflow",
	}

	result, err := dag.GenerateMermaidFlowChartWithOptions(opts)
	require.NoError(t, err)

	assert.Contains(t, result, "%% AI Tool Call Workflow")
	assert.Contains(t, result, "flowchart LR")
}

// TestMUSTPASS_GenerateMermaidFlowChart_OutputExample demonstrates the output format
func TestMUSTPASS_GenerateMermaidFlowChart_OutputExample(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Diamond pattern DAG
	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("C", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("D", "B", "C")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Expected output format:
	// flowchart TB
	//     A["A"]
	//     B["B"]
	//     A --> B
	//     C["C"]
	//     A --> C
	//     D["D"]
	//     B --> D
	//     C --> D

	// Verify structure
	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, `A["A"]`)
	assert.Contains(t, result, `B["B"]`)
	assert.Contains(t, result, `C["C"]`)
	assert.Contains(t, result, `D["D"]`)
	assert.Contains(t, result, "A --> B")
	assert.Contains(t, result, "A --> C")
	assert.Contains(t, result, "B --> D")
	assert.Contains(t, result, "C --> D")

	// Log the output for documentation purposes
	t.Logf("Generated Mermaid Flowchart:\n%s", result)
}

// ==================== Multiline Label Tests ====================

func TestMUSTPASS_EscapeMermaidLabel_MultilineVariants(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:     "Unix newline",
			input:    "line1\nline2\nline3",
			contains: []string{"<br/>", "line1", "line2", "line3"},
		},
		{
			name:     "Windows CRLF",
			input:    "line1\r\nline2\r\nline3",
			contains: []string{"<br/>", "line1", "line2", "line3"},
		},
		{
			name:     "Mixed newlines",
			input:    "line1\nline2\r\nline3\rline4",
			contains: []string{"<br/>", "line1", "line2", "line3", "line4"},
		},
		{
			name:     "Multiple consecutive newlines",
			input:    "line1\n\n\nline2",
			contains: []string{"<br/><br/><br/>"},
		},
		{
			name:     "Newline at start",
			input:    "\nstart with newline",
			contains: []string{"<br/>"},
		},
		{
			name:     "Newline at end",
			input:    "end with newline\n",
			contains: []string{"<br/>"},
		},
		{
			name:     "Only newlines",
			input:    "\n\n\n",
			contains: []string{"<br/><br/><br/>"},
		},
		{
			name:     "Newline with special chars",
			input:    "line1 <test>\nline2 \"quoted\"\nline3 [array]",
			contains: []string{"<br/>", "#lt;", "#gt;", "#quot;", "#91;", "#93;"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			for _, c := range tt.contains {
				assert.Contains(t, result, c, "should contain: %s", c)
			}
			// Should not contain raw newlines
			assert.NotContains(t, result, "\n")
			assert.NotContains(t, result, "\r")
		})
	}
}

// ==================== Chinese Punctuation Tests ====================

func TestMUSTPASS_EscapeMermaidLabel_ChinesePunctuation(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Chinese comma", "测试，数据"},
		{"Chinese period", "测试。结束"},
		{"Chinese colon", "标题：内容"},
		{"Chinese semicolon", "项目；任务"},
		{"Chinese exclamation", "成功！"},
		{"Chinese question", "什么？"},
		{"Chinese quotes", "\u201c引用内容\u201d"},
		{"Chinese single quotes", "\u2018单引号\u2019"},
		{"Chinese parentheses", "（括号内容）"},
		{"Chinese brackets", "【方括号】"},
		{"Chinese angle brackets", "《书名》"},
		{"Chinese ellipsis", "省略……"},
		{"Chinese dash", "破折号——"},
		{"Chinese middle dot", "项目·子项"},
		{"All Chinese punctuation", "测试，数据。问题？回答！\u201c引用\u201d（说明）【标注】《参考》……——·"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			// Should be wrapped in quotes
			assert.True(t, strings.HasPrefix(result, `"`))
			assert.True(t, strings.HasSuffix(result, `"`))
			// Should not break Mermaid syntax - no raw problematic chars
			assert.NotContains(t, result, "\n")
		})
	}
}

func TestMUSTPASS_GenerateMermaidFlowChart_ChinesePunctuationNodes(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Nodes with various Chinese punctuation
	require.NoError(t, dag.AddNode(NewTestNode("开始，初始化")))
	require.NoError(t, dag.AddNode(NewTestNode("处理（数据）", "开始，初始化")))
	require.NoError(t, dag.AddNode(NewTestNode("检查\u201c状态\u201d", "处理（数据）")))
	require.NoError(t, dag.AddNode(NewTestNode("结束。完成！", "检查\u201c状态\u201d")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, "-->")
	// Labels should contain the Chinese text
	assert.Contains(t, result, "开始")
	assert.Contains(t, result, "处理")
	assert.Contains(t, result, "检查")
	assert.Contains(t, result, "结束")
	t.Logf("Chinese punctuation flowchart:\n%s", result)
}

// ==================== Mixed Chinese-English Tests ====================

func TestMUSTPASS_EscapeMermaidLabel_MixedChineseEnglish(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Basic mix", "Hello世界"},
		{"Mix with space", "Hello 世界 World"},
		{"Mix with numbers", "测试Test123数据"},
		{"Mix with English punctuation", "测试, test. 数据!"},
		{"Mix with Chinese punctuation", "Test，测试。Data"},
		{"Mix with both punctuation", "测试, Test。数据! 完成？"},
		{"Technical term mix", "HTTP请求 Response响应"},
		{"Code style mix", "func 函数名() { return 返回值 }"},
		{"Error message style", "Error: 错误信息 - failed to 处理数据"},
		{"Path style mix", "/path/路径/file文件.txt"},
		{"JSON style mix", `{"name": "名称", "value": 123}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.True(t, strings.HasPrefix(result, `"`))
			assert.True(t, strings.HasSuffix(result, `"`))
		})
	}
}

func TestMUSTPASS_GenerateMermaidFlowChart_MixedLanguageNodes(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("Start开始")))
	require.NoError(t, dag.AddNode(NewTestNode("Process处理Data数据", "Start开始")))
	require.NoError(t, dag.AddNode(NewTestNode("Validate验证", "Process处理Data数据")))
	require.NoError(t, dag.AddNode(NewTestNode("End结束", "Validate验证")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, "-->")
	t.Logf("Mixed language flowchart:\n%s", result)
}

// ==================== Special Symbol Combination Tests ====================

func TestMUSTPASS_EscapeMermaidLabel_SymbolCombinations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:     "Arrow symbols",
			input:    "A -> B => C --> D",
			contains: []string{"#gt;"},
		},
		{
			name:     "Comparison operators",
			input:    "a < b && c > d || e == f",
			contains: []string{"#lt;", "#gt;", "#amp;"},
		},
		{
			name:     "Math symbols",
			input:    "x + y - z * w / v % n",
			contains: []string{"x", "y", "z"},
		},
		{
			name:     "Currency symbols",
			input:    "$100 €50 ¥200 £30",
			contains: []string{"$100", "€50", "¥200", "£30"},
		},
		{
			name:     "Regex pattern",
			input:    `^[a-z]+\d{3}$`,
			contains: []string{"#91;", "#93;", "#92;"},
		},
		{
			name:     "Shell command",
			input:    `echo "hello" | grep -E "test" > output.txt`,
			contains: []string{"#quot;", "#124;", "#gt;"},
		},
		{
			name:     "HTML tags",
			input:    `<div class="test"><span>content</span></div>`,
			contains: []string{"#lt;", "#gt;", "#quot;"},
		},
		{
			name:     "Template syntax",
			input:    "{{.Name}} ${var} #{expr}",
			contains: []string{"#123;", "#125;"},
		},
		{
			name:     "Markdown links",
			input:    "[link text](http://example.com)",
			contains: []string{"#91;", "#93;", "#40;", "#41;"},
		},
		{
			name:     "SQL query",
			input:    `SELECT * FROM users WHERE name = 'test' AND age > 18;`,
			contains: []string{"#gt;"},
		},
		{
			name:     "YAML style",
			input:    "key: value\n  nested: data",
			contains: []string{"<br/>"},
		},
		{
			name:     "Environment variable",
			input:    "${HOME}/path/$USER",
			contains: []string{"#123;", "#125;"},
		},
		{
			name:     "Windows path",
			input:    `C:\Users\test\Documents\file.txt`,
			contains: []string{"#92;"},
		},
		{
			name:     "URL with params",
			input:    "https://api.example.com/v1/users?id=123&name=test",
			contains: []string{"#amp;"},
		},
		{
			name:     "Escape sequences",
			input:    `\n\t\r\\\"`,
			contains: []string{"#92;"},
		},
		{
			name:     "Binary operators",
			input:    "a & b | c ^ d ~ e",
			contains: []string{"#amp;", "#124;"},
		},
		{
			name:     "Triple backticks",
			input:    "```code block```",
			contains: []string{"code"},
		},
		{
			name:     "Control characters description",
			input:    "Tab:\t Newline:\n Return:\r",
			contains: []string{"<br/>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.True(t, strings.HasPrefix(result, `"`), "should start with quote")
			assert.True(t, strings.HasSuffix(result, `"`), "should end with quote")
			for _, c := range tt.contains {
				assert.Contains(t, result, c, "should contain: %s", c)
			}
		})
	}
}

// ==================== Multiline with Special Characters Tests ====================

func TestMUSTPASS_EscapeMermaidLabel_MultilineWithSpecialChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Multiline code block",
			input: "function test() {\n  return true;\n}",
		},
		{
			name:  "Multiline JSON",
			input: "{\n  \"name\": \"test\",\n  \"value\": 123\n}",
		},
		{
			name:  "Multiline with Chinese",
			input: "第一行\n第二行\n第三行",
		},
		{
			name:  "Multiline mixed language",
			input: "Step 1: 初始化\nStep 2: 处理\nStep 3: 完成",
		},
		{
			name:  "Multiline with all bracket types",
			input: "Array: [1,2,3]\nObject: {a:1}\nCall: func()",
		},
		{
			name:  "Multiline with quotes",
			input: "Line 1: \"quoted\"\nLine 2: 'single'\nLine 3: `backtick`",
		},
		{
			name:  "Multiline shell script",
			input: "#!/bin/bash\necho \"hello\"\nexit 0",
		},
		{
			name:  "Multiline SQL",
			input: "SELECT *\nFROM users\nWHERE id > 0",
		},
		{
			name:  "Multiline with pipes",
			input: "cat file.txt |\n  grep pattern |\n  sort",
		},
		{
			name:  "Multiline log entry",
			input: "[2024-01-01 10:00:00] INFO: Started\n[2024-01-01 10:00:01] ERROR: Failed\n[2024-01-01 10:00:02] INFO: Retry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			// Should not contain raw newlines
			assert.NotContains(t, result, "\n", "should not contain raw newlines")
			assert.NotContains(t, result, "\r", "should not contain raw carriage returns")
			// Should contain <br/> for line breaks
			assert.Contains(t, result, "<br/>", "should contain <br/> for line breaks")
			// Should be properly quoted
			assert.True(t, strings.HasPrefix(result, `"`))
			assert.True(t, strings.HasSuffix(result, `"`))
		})
	}
}

func TestMUSTPASS_GenerateMermaidFlowChart_MultilineLabels(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("Step 1\n初始化")))
	require.NoError(t, dag.AddNode(NewTestNode("Step 2\n处理数据\nProcess Data", "Step 1\n初始化")))
	require.NoError(t, dag.AddNode(NewTestNode("Step 3\n完成", "Step 2\n处理数据\nProcess Data")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	// Should not contain raw newlines in the output (except for flowchart structure)
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		// Each line should not contain embedded newlines within node definitions
		if strings.Contains(line, "[\"") {
			assert.Contains(t, line, "<br/>", "multiline labels should use <br/>")
		}
	}
	t.Logf("Multiline labels flowchart:\n%s", result)
}

// ==================== Mermaid Syntax Edge Cases ====================

func TestMUSTPASS_EscapeMermaidLabel_MermaidSyntaxBreakers(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Arrow in text", "A --> B"},
		{"Dotted arrow", "A -.-> B"},
		{"Thick arrow", "A ==> B"},
		{"Link text syntax", "A --|text|--> B"},
		{"Subgraph keyword", "subgraph cluster"},
		{"End keyword", "end"},
		{"Direction keyword", "direction LR"},
		{"Style definition", "style A fill:#f9f"},
		{"Class definition", "classDef red fill:#f00"},
		{"Click handler", "click A callback"},
		{"Comment syntax", "%% this is a comment"},
		{"Node shape syntax", "A[text] B(text) C{text} D((text))"},
		{"Edge label", "|edge label|"},
		{"Semicolon", "A;B;C"},
		{"Double hyphen", "A--B"},
		{"Triple hyphen", "A---B"},
		{"Pipe chain", "A|B|C|D"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			// Should be safely escaped
			assert.True(t, strings.HasPrefix(result, `"`))
			assert.True(t, strings.HasSuffix(result, `"`))
			// Should not contain raw problematic characters
			innerContent := result[1 : len(result)-1]
			assert.NotContains(t, innerContent, `"`, "should not contain unescaped quotes in content")
		})
	}
}

func TestMUSTPASS_GenerateMermaidFlowChart_MermaidSyntaxInLabels(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Nodes with labels that look like Mermaid syntax
	require.NoError(t, dag.AddNode(NewTestNode("A --> B")))
	require.NoError(t, dag.AddNode(NewTestNode("subgraph test", "A --> B")))
	require.NoError(t, dag.AddNode(NewTestNode("end", "subgraph test")))
	require.NoError(t, dag.AddNode(NewTestNode("style A fill:#f9f", "end")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, "-->")
	t.Logf("Mermaid syntax in labels flowchart:\n%s", result)
}

// ==================== Full Width Characters Tests ====================

func TestMUSTPASS_EscapeMermaidLabel_FullWidthCharacters(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Full width letters", "ＡＢＣＤＥＦＧ"},
		{"Full width numbers", "０１２３４５６７８９"},
		{"Full width symbols", "！＠＃＄％＾＆＊"},
		{"Full width parentheses", "（）［］｛｝"},
		{"Full width quotes", "＂＇"},
		{"Full width operators", "＋－＝／＼"},
		{"Mix full and half width", "Hello（ＴＥＳＴ）World"},
		{"Japanese full width", "テスト　データ"}, // with full-width space
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.True(t, strings.HasPrefix(result, `"`))
			assert.True(t, strings.HasSuffix(result, `"`))
		})
	}
}

// ==================== Emoji Tests ====================

func TestMUSTPASS_EscapeMermaidLabel_Emojis(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Simple emoji", "✅ Success"},
		{"Multiple emojis", "🚀 Start ➡️ Process ✅ Done"},
		{"Emoji with text", "Hello 👋 World 🌍"},
		{"Status emojis", "⏳ Pending | ✅ Done | ❌ Failed"},
		{"Flag emojis", "🇺🇸 🇨🇳 🇯🇵 🇬🇧"},
		{"Complex emojis", "👨‍👩‍👧‍👦 Family"},
		{"Emoji in brackets", "[✅] Task completed"},
		{"Emoji with Chinese", "成功 ✅ 完成"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.True(t, strings.HasPrefix(result, `"`))
			assert.True(t, strings.HasSuffix(result, `"`))
		})
	}
}

func TestMUSTPASS_GenerateMermaidFlowChart_EmojiLabels(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("🚀 Start")))
	require.NoError(t, dag.AddNode(NewTestNode("⚙️ Process", "🚀 Start")))
	require.NoError(t, dag.AddNode(NewTestNode("✅ Complete", "⚙️ Process")))
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	assert.Contains(t, result, "-->")
	assert.Contains(t, result, "🚀")
	assert.Contains(t, result, "⚙️")
	assert.Contains(t, result, "✅")
	t.Logf("Emoji labels flowchart:\n%s", result)
}

// ==================== Stress Tests for Labels ====================

func TestMUSTPASS_EscapeMermaidLabel_StressTest(t *testing.T) {
	// Very long label
	t.Run("Very long label", func(t *testing.T) {
		longLabel := strings.Repeat("测试Test数据Data", 100)
		result := escapeMermaidLabel(longLabel)
		assert.True(t, strings.HasPrefix(result, `"`))
		assert.True(t, strings.HasSuffix(result, `"`))
	})

	// Many special characters
	t.Run("Many special characters", func(t *testing.T) {
		specialChars := `<>{}[]()|\/"'` + "`" + `!@#$%^&*-+=;:,.?~`
		result := escapeMermaidLabel(specialChars)
		assert.True(t, strings.HasPrefix(result, `"`))
		assert.True(t, strings.HasSuffix(result, `"`))
	})

	// Many newlines
	t.Run("Many newlines", func(t *testing.T) {
		manyNewlines := strings.Repeat("line\n", 50)
		result := escapeMermaidLabel(manyNewlines)
		assert.NotContains(t, result, "\n")
		assert.Contains(t, result, "<br/>")
	})

	// Unicode stress
	t.Run("Unicode stress", func(t *testing.T) {
		unicodeStress := "中文日本語한국어العربيةעבריתΕλληνικάРусский"
		result := escapeMermaidLabel(unicodeStress)
		assert.True(t, strings.HasPrefix(result, `"`))
		assert.True(t, strings.HasSuffix(result, `"`))
	})

	// Mixed everything
	t.Run("Mixed everything", func(t *testing.T) {
		mixed := "Start开始\n<tag>\"quoted\"</tag>\n[array]{object}(func)\n测试|pipe|test\nEnd结束"
		result := escapeMermaidLabel(mixed)
		assert.True(t, strings.HasPrefix(result, `"`))
		assert.True(t, strings.HasSuffix(result, `"`))
		assert.NotContains(t, result, "\n")
		assert.Contains(t, result, "<br/>")
	})
}

func TestMUSTPASS_GenerateMermaidFlowChart_ComplexRealWorld(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Simulate a real-world complex workflow with various label types
	nodes := []struct {
		id   string
		deps []string
	}{
		{id: "1. 开始 (Start)", deps: nil},
		{id: "2. 读取配置\nRead Config", deps: []string{"1. 开始 (Start)"}},
		{id: "3. 验证参数 <validate>", deps: []string{"2. 读取配置\nRead Config"}},
		{id: "4. 处理数据 {process}", deps: []string{"3. 验证参数 <validate>"}},
		{id: "5. 保存结果 [save]", deps: []string{"4. 处理数据 {process}"}},
		{id: "6. 发送通知 | notify |", deps: []string{"5. 保存结果 [save]"}},
		{id: "7. 完成 ✅", deps: []string{"6. 发送通知 | notify |"}},
	}

	for _, n := range nodes {
		require.NoError(t, dag.AddNode(NewTestNode(n.id, n.deps...)))
	}
	require.NoError(t, dag.Build())

	result, err := dag.GenerateMermaidFlowChart()
	require.NoError(t, err)

	assert.Contains(t, result, "flowchart TB")
	// Should have 6 edges (7 nodes in a chain)
	edgeCount := strings.Count(result, "-->")
	assert.Equal(t, 6, edgeCount, "should have 6 edges")

	t.Logf("Complex real-world flowchart:\n%s", result)
}
