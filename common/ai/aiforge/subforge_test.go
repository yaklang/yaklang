package aiforge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestSubForgeDeclarationsAndArguments(t *testing.T) {
	child := NewForgeBlueprint("child", WithOriginYaklangCliCode(`
		cli.String("query", cli.setRequired(true))
		cli.Int("limit", cli.setDefault(3))
		cli.StringSlice("targets", cli.setDefault("one,two"))
		cli.String("mode", cli.setSelectOption("Quick", "quick"), cli.setSelectOption("Thorough", "thorough"))
	`))
	parent := NewForgeBlueprint("parent", WithSubForge("inspect", "Inspect a reference", child))
	tools, err := parent.prepareSubForges(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "sub_forge_inspect", tools[0].Schema.Name)
	query, args, err := subForgeArguments(tools[0].Schema, aitool.InvokeParams{"query": "ref", "mode": "quick"})
	require.NoError(t, err)
	require.Equal(t, "ref", query)
	require.ElementsMatch(t, []Parameter{{"query", "ref"}, {"limit", "3"}, {"mode", "quick"}, {"targets", `["one","two"]`}}, args)
	for _, params := range []aitool.InvokeParams{
		{"limit": 5}, {"query": "ref", "limit": "5"}, {"query": "ref", "mode": "undefined"}, {"query": "ref", "other": 1},
	} {
		_, _, err := subForgeArguments(tools[0].Schema, params)
		require.Error(t, err, "%v", params)
	}

	defaultTool, err := subForgeSchema("sub_forge_default", "Default", "")
	require.NoError(t, err)
	_, _, err = subForgeArguments(defaultTool, aitool.InvokeParams{})
	require.Error(t, err)
	_, _, err = subForgeArguments(defaultTool, aitool.InvokeParams{"query": "ref"})
	require.NoError(t, err)
	_, err = subForgeSchema("sub_forge_invalid", "Invalid", `cli.String("query", cli.setJsonSchema("{"))`)
	require.Error(t, err)
	for _, options := range [][]Option{
		{WithSubForge("invalid alias", "Description", child)},
		{WithSubForge("missing", "Description", nil)},
		{WithSubForge("same", "Description", child), WithSubForge("same", "Description", child)},
		{WithSubForge("self", "Description", NewForgeBlueprint("parent"))},
	} {
		_, err := NewForgeBlueprint("parent", options...).prepareSubForges(context.Background())
		require.Error(t, err)
	}
}
