package yakcliconvert_test

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/stretchr/testify/require"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

func TestConvertCliParameterToJsonSchema(t *testing.T) {
	testJsonSchema := `{
  "type": "object",
  "properties": {
    "lat": {
      "type": "number"
    },
    "lon": {
      "type": "number"
    }
  }
}`

	content := fmt.Sprintf(`
cli.help("example")
a = cli.Int("a", cli.setRequired(true))
b = cli.Bool("b", cli.setHelp("b"), cli.setDefault(true))
c = cli.String("c", cli.setRequired(true))
f = cli.File("f", cli.setHelp("file"))
s = cli.StringSlice("s", cli.setMultipleSelect(true), cli.setSelectOption("name1", "value1"),cli.setSelectOption("name2", "value2"))
j = cli.Json("j",cli.setJsonSchema(%s))
cli.check()
`, "`"+testJsonSchema+"`")

	prog, err := static_analyzer.SSAParse(content, "yak")
	require.NoError(t, err)
	tool := yakcliconvert.ConvertCliParameterToTool("test", prog)
	require.Equal(t, "example", tool.Description)

	require.ElementsMatch(t, tool.InputSchema.Required, []string{"a", "c"})
	checkEx := func(m map[string]any, name string, typ string) map[string]any {
		i, ok := m[name]
		require.True(t, ok)
		v, ok := i.(map[string]any)
		require.True(t, ok)
		require.Equal(t, typ, v["type"])
		return v
	}
	checkOrderedMap := func(om *omap.OrderedMap[string, any], name string, typ string) map[string]any {
		i, ok := om.Get(name)
		require.True(t, ok)
		v, ok := i.(map[string]any)
		require.True(t, ok)
		require.Equal(t, typ, v["type"])
		return v
	}
	check := func(name string, typ string) map[string]any {
		return checkOrderedMap(tool.InputSchema.Properties, name, typ)
	}

	check("a", "integer")
	b := check("b", "boolean")
	require.Equal(t, "b", b["description"])
	require.Equal(t, true, b["default"])
	check("c", "string")
	f := check("f", "string")
	require.Equal(t, "file (filepath)", f["description"])
	s := check("s", "array")
	enum, ok := s["enum"].([]string)
	require.True(t, ok)
	require.ElementsMatch(t, enum, []string{"value1", "value2"})

	j := check("j", "object")
	jProps, ok := j["properties"].(map[string]any)
	require.True(t, ok)
	checkEx(jProps, "lat", "number")
	checkEx(jProps, "lon", "number")

	require.ElementsMatch(t, tool.InputSchema.Required, []string{"a", "c"})

}
