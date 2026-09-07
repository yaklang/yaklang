package stream_parser

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

type expressionRestoreTestParser struct {
	resultHook func(*base.Node)
}

func (*expressionRestoreTestParser) Parse(*base.BitReader, *base.Node) error { return nil }
func (*expressionRestoreTestParser) Generate(any, *base.Node) error          { return nil }
func (*expressionRestoreTestParser) OnRoot(*base.Node) error                 { return nil }

func (p *expressionRestoreTestParser) Result(node *base.Node) (*base.NodeValue, error) {
	if p.resultHook != nil {
		p.resultHook(node)
	}
	return &base.NodeValue{Origin: node, Name: node.Name, Value: uint8(42)}, nil
}

func newExpressionRestoreTestNode(t *testing.T, recordParser bool, hook func(*base.Node)) *base.Node {
	t.Helper()
	root, err := base.NewNodeTree(yaml.MapSlice{})
	require.NoError(t, err)
	root.Name = "Expression"
	root.Cfg = base.NewEmptyConfig()
	parserName := fmt.Sprintf("expression-restore-test/%s", t.Name())
	base.RegisterParser(parserName, &expressionRestoreTestParser{resultHook: hook})
	if recordParser {
		root.Cfg.SetItem("parser", parserName)
	} else {
		root.Cfg.BaseKV.SetItem("parser", parserName)
	}
	return root
}

type expressionRestoreHistorySnapshot struct {
	present bool
	count   int
}

func snapshotExpressionRestoreHistory(t *testing.T, cfg *base.Config) expressionRestoreHistorySnapshot {
	t.Helper()
	snapshot := expressionRestoreHistorySnapshot{present: cfg.Has(base.CfgOptionFuns)}
	if snapshot.present {
		options, ok := cfg.GetItem(base.CfgOptionFuns).([]base.NodeConfigFun)
		require.True(t, ok)
		snapshot.count = len(options)
	}
	return snapshot
}

func TestExpressionEntryRestoreDoesNotRecordUnchangedConfig(t *testing.T) {
	type entry struct {
		name, key, source string
		call              func(*base.Node) (any, error)
	}
	entries := []entry{
		{
			name: "out", key: "out", source: "return data",
			call: func(node *base.Node) (any, error) { return ExecOut(node) },
		},
		{
			name: "input", key: "input", source: "return data",
			call: func(node *base.Node) (any, error) { return ExecInput(node) },
		},
		{
			name: "parser", key: "out", source: "return 17",
			call: ExecParser,
		},
	}
	setups := []struct {
		name              string
		recordParser      bool
		expressionPresent bool
		setExpression     func(*base.Config, string, string)
		wantReplay        bool
	}{
		{
			name: "recorded expression", recordParser: true, expressionPresent: true,
			setExpression: func(cfg *base.Config, key, source string) { cfg.SetItem(key, source) },
			wantReplay:    true,
		},
		{
			name: "raw expression", recordParser: true, expressionPresent: true,
			setExpression: func(cfg *base.Config, key, source string) { cfg.BaseKV.SetItem(key, source) },
		},
		{
			name: "missing expression", recordParser: true,
			setExpression: func(*base.Config, string, string) {},
		},
		{
			name: "raw config and expression", recordParser: false, expressionPresent: true,
			setExpression: func(cfg *base.Config, key, source string) { cfg.BaseKV.SetItem(key, source) },
		},
		{
			name: "raw config and missing expression", recordParser: false,
			setExpression: func(*base.Config, string, string) {},
		},
	}

	for _, currentEntry := range entries {
		for _, setup := range setups {
			t.Run(currentEntry.name+"/"+setup.name, func(t *testing.T) {
				node := newExpressionRestoreTestNode(t, setup.recordParser, nil)
				setup.setExpression(node.Cfg, currentEntry.key, currentEntry.source)
				before := snapshotExpressionRestoreHistory(t, node.Cfg)

				_, err := currentEntry.call(node)
				if setup.expressionPresent {
					require.NoError(t, err)
				}

				require.Equal(t, before, snapshotExpressionRestoreHistory(t, node.Cfg))
				require.True(t, node.Cfg.Has(currentEntry.key), "the old entry points restore even an initially absent key")
				wantCurrent := ""
				if setup.expressionPresent {
					wantCurrent = currentEntry.source
				}
				require.Equal(t, wantCurrent, node.Cfg.GetString(currentEntry.key))

				merged := base.AppendConfig(base.NewEmptyConfig(), node.Cfg)
				if setup.wantReplay {
					require.Equal(t, currentEntry.source, merged.GetString(currentEntry.key))
				} else {
					require.False(t, merged.Has(currentEntry.key), "a current-only expression must not be promoted into replay history")
				}
			})
		}
	}
}

func TestExecOutExplicitConfigMutationKeepsRestoreLastInReplay(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantError  bool
		wantMarker bool
		rawConfig  bool
	}{
		{
			name: "replace own expression",
			source: `node.Cfg.SetItem("out", "replacement")
return data`,
		},
		{
			name: "replace then delete own expression",
			source: `node.Cfg.SetItem("out", "replacement")
node.Cfg.DeleteItem("out")
return data`,
		},
		{
			name: "set unrelated config",
			source: `node.Cfg.SetItem("expression-marker", 99)
return data`,
			wantMarker: true,
		},
		{
			name: "raw expression records restore after explicit config change",
			source: `node.Cfg.SetItem("expression-marker", 99)
return data`,
			wantMarker: true,
			rawConfig:  true,
		},
		{
			name: "set unrelated config then panic",
			source: `node.Cfg.SetItem("expression-marker", 99)
panic("expected expression failure")`,
			wantError:  true,
			wantMarker: true,
		},
	}

	for _, legacy := range []bool{false, true} {
		for _, test := range tests {
			t.Run(fmt.Sprintf("legacy-%t/%s", legacy, test.name), func(t *testing.T) {
				node := newExpressionRestoreTestNode(t, !test.rawConfig, nil)
				node.Ctx.SetItem("outProgramLegacy", legacy)
				if test.rawConfig {
					node.Cfg.BaseKV.SetItem("out", test.source)
				} else {
					node.Cfg.SetItem("out", test.source)
				}
				before := snapshotExpressionRestoreHistory(t, node.Cfg)

				_, err := ExecOut(node)
				if test.wantError {
					require.ErrorContains(t, err, "expected expression failure")
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, test.source, node.Cfg.GetString("out"))
				require.Equal(t, before.count+2, snapshotExpressionRestoreHistory(t, node.Cfg).count)
				require.Equal(t, test.wantMarker, node.Cfg.Has("expression-marker"))

				merged := base.AppendConfig(base.NewEmptyConfig(), node.Cfg)
				require.Equal(t, test.source, merged.GetString("out"))
				require.Equal(t, test.wantMarker, merged.Has("expression-marker"))
				if test.wantMarker {
					require.Equal(t, 99, merged.GetItem("expression-marker"))
				}
			})
		}
	}
}

func TestExpressionRestoreRecordsOriginalOnConfigReplacement(t *testing.T) {
	var replacement *base.Config
	node := newExpressionRestoreTestNode(t, true, func(node *base.Node) {
		node.Cfg = replacement
	})
	original := "return 17"
	node.Cfg.SetItem("out", original)
	replacement = base.NewEmptyConfig()
	replacement.SetItem("parser", node.Cfg.GetString("parser"))
	replacement.SetItem("replacement-marker", true)
	before := snapshotExpressionRestoreHistory(t, replacement)

	result, err := ExecParser(node)
	require.NoError(t, err)
	require.Equal(t, 17, result)
	require.Same(t, replacement, node.Cfg)
	require.Equal(t, original, node.Cfg.GetString("out"))
	require.Equal(t, before.count+1, snapshotExpressionRestoreHistory(t, node.Cfg).count)

	merged := base.AppendConfig(base.NewEmptyConfig(), node.Cfg)
	require.Equal(t, original, merged.GetString("out"))
	require.True(t, merged.GetBool("replacement-marker"))
}

func TestExpressionRestoreMalformedHistoryKeepsLegacyFailure(t *testing.T) {
	node := newExpressionRestoreTestNode(t, false, nil)
	original := "return 17"
	node.Cfg.BaseKV.SetItem("out", original)
	node.Cfg.BaseKV.SetItem(base.CfgOptionFuns, "malformed replay history")

	_, err := ExecParser(node)
	require.Error(t, err)
	require.Equal(t, original, node.Cfg.GetString("out"))
	require.Equal(t, "malformed replay history", node.Cfg.GetItem(base.CfgOptionFuns))
}
