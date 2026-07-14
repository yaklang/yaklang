package buildinaitools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// compileSchemaAgainstMetaSchema compiles the given schema against the JSON
// Schema 2020-12 meta-schema (the same draft Claude Code uses to validate
// tool inputSchema). Returns nil if valid, or the compile error.
func compileSchemaAgainstMetaSchema(schema any) error {
	jsonBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}
	var plain any
	if err := json.Unmarshal(jsonBytes, &plain); err != nil {
		return fmt.Errorf("unmarshal schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("schema.json", plain); err != nil {
		return fmt.Errorf("add resource: %w", err)
	}
	if _, err := compiler.Compile("schema.json"); err != nil {
		return err
	}
	return nil
}

// TestAIToolSchemaMetaSchemaCompliance validates every built-in aitool's
// inputSchema against the JSON Schema 2020-12 meta-schema. This catches
// structural violations (e.g. a "type" field that is an object instead of a
// string) that external MCP clients like Claude Code reject with HTTP 400.
//
// The test uses an in-memory database to avoid touching the real profile
// database, following the same pattern as TestGetAllToolsDynamically.
func TestAIToolSchemaMetaSchemaCompliance(t *testing.T) {
	tempDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err, "create temp test database")

	err = tempDB.AutoMigrate(&schema.AIYakTool{}).Error
	require.NoError(t, err, "auto migrate AIYakTool table")

	tools := GetAllToolsDynamically(tempDB)
	require.NotEmpty(t, tools, "no built-in aitools returned")

	type violation struct {
		tool string
		err  string
	}
	var violations []violation

	for _, tool := range tools {
		if tool == nil || tool.Tool == nil {
			continue
		}
		// Marshal/unmarshal to simulate wire format (OrderedMap -> map, etc.)
		jsonBytes, err := json.Marshal(tool.InputSchema)
		require.NoErrorf(t, err, "tool %q: failed to marshal InputSchema", tool.Name)
		var wireSchema any
		if err := json.Unmarshal(jsonBytes, &wireSchema); err != nil {
			violations = append(violations, violation{
				tool: tool.Name,
				err:  fmt.Sprintf("unmarshal InputSchema: %v", err),
			})
			continue
		}

		if compileErr := compileSchemaAgainstMetaSchema(wireSchema); compileErr != nil {
			violations = append(violations, violation{
				tool: tool.Name,
				err:  compileErr.Error(),
			})
		}
	}

	if len(violations) > 0 {
		sort.Slice(violations, func(i, j int) bool {
			return violations[i].tool < violations[j].tool
		})
		var sb strings.Builder
		for _, v := range violations {
			sb.WriteString(fmt.Sprintf("  - tool %q: %s\n", v.tool, v.err))
		}
		t.Errorf("found %d aitool(s) with invalid JSON Schema inputSchema "+
			"(meta-schema: draft 2020-12):\n%s", len(violations), sb.String())
	}
}
