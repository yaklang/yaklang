package base

import (
	"testing"

	"github.com/stretchr/testify/require"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

type parserFactoryTestParser struct {
	id int
}

func (*parserFactoryTestParser) Parse(*BitReader, *Node) error    { return nil }
func (*parserFactoryTestParser) Generate(any, *Node) error        { return nil }
func (*parserFactoryTestParser) OnRoot(*Node) error               { return nil }
func (*parserFactoryTestParser) Result(*Node) (*NodeValue, error) { return nil, nil }

func TestParserFactoryCreatesOneRuntimePerLogicalTree(t *testing.T) {
	const name = "parser-factory-per-logical-tree-test"
	created := 0
	RegisterParserFactory(name, func() Parser {
		created++
		return &parserFactoryTestParser{id: created}
	})

	first := parserFactoryTestTree(t, name)
	firstRuntime := mustGetParser(t, first)
	require.Same(t, firstRuntime, mustGetParser(t, first))

	importedContextTree, err := NewNodeTree(yaml.MapSlice{})
	require.NoError(t, err)
	imported, err := NewNodeTreeWithConfig(first.Cfg, "Imported", yaml.MapSlice{}, importedContextTree.Ctx)
	require.NoError(t, err)
	imported.Cfg.SetItem(CfgParent, first)
	require.Same(t, firstRuntime, mustGetParser(t, imported))

	second := parserFactoryTestTree(t, name)
	secondRuntime := mustGetParser(t, second)
	require.NotSame(t, firstRuntime, secondRuntime)
	require.Equal(t, 2, created)
}

func TestRegisterParserKeepsSharedInstanceBehavior(t *testing.T) {
	const name = "parser-factory-shared-registration-test"
	shared := &parserFactoryTestParser{}
	RegisterParser(name, shared)

	require.Same(t, shared, mustGetParser(t, parserFactoryTestTree(t, name)))
	require.Same(t, shared, mustGetParser(t, parserFactoryTestTree(t, name)))
}

func TestParserFactoryReplacementDoesNotReuseStaleTreeRuntime(t *testing.T) {
	const name = "parser-factory-replacement-test"
	first := &parserFactoryTestParser{id: 1}
	second := &parserFactoryTestParser{id: 2}
	tree := parserFactoryTestTree(t, name)

	RegisterParserFactory(name, func() Parser { return first })
	require.Same(t, first, mustGetParser(t, tree))

	shared := &parserFactoryTestParser{id: 3}
	RegisterParser(name, shared)
	require.Same(t, shared, mustGetParser(t, tree))

	RegisterParserFactory(name, func() Parser { return second })
	require.Same(t, second, mustGetParser(t, tree))
	require.Same(t, second, mustGetParser(t, tree))
}

func parserFactoryTestTree(t *testing.T, parserName string) *Node {
	t.Helper()
	tree, err := NewNodeTree(yaml.MapSlice{})
	require.NoError(t, err)
	tree.Cfg.SetItem("parser", parserName)
	return tree
}

func mustGetParser(t *testing.T, node *Node) Parser {
	t.Helper()
	parser, err := node.getParser()
	require.NoError(t, err)
	return parser
}
