package stream_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestPreparedOperatorConservativeEligibility(t *testing.T) {
	for _, source := range []string{
		`this.ProcessSubNode("Type")`,
		`values = [1, 2, 3]; values[0] = 9; if len(values) != 3 { panic("bad") }`,
		`this.ProcessSubNode("Type"); this.AddInfo("Profile", "fields"); this.AddInfo("Verified", false); this.AddInfo("Limit", 128)`,
	} {
		p, err := loadPreparedOperator(source)
		require.NoError(t, err)
		require.NotNil(t, p, source)
	}
	for _, source := range []string{
		`eval("value = 1")`, `f = eval; f("value = 1")`,
		`f = func() { return 1 }; f()`, `go this.Process()`,
		`getCtx("callback")()`, `getCfg("callback")()`,
		`this.GetCfg("callback")()`, `this.AddInfo("callback", this.Process)`,
		`f = this.AddInfo; f("callback", this.Process)`,
		`this.AddInfo("values", [1,2,3])`,
		`this.AddInfo(key, "value")`,
		`defer this.Process()`,
	} {
		p, err := loadPreparedOperator(source)
		require.NoError(t, err)
		require.Nil(t, p, source)
	}
	// Even a safe custom script keeps the original mutable YakNode API.
	handled, err := execPreparedOperator(nil, `this.Process = this.Result`, nil, []string{ParserMode})
	require.False(t, handled)
	require.NoError(t, err)
}

func TestPreparedChildVisitorShape(t *testing.T) {
	const source = `this.ForEachChild(func(node) { node.Process(); node.AddInfo("seen", true) })`
	artifact, err := loadOperatorProgram(source)
	require.NoError(t, err)
	_, codes, err := yakvm.NewCodesMarshaller().Unmarshal(artifact)
	require.NoError(t, err)
	require.True(t, isPreparedOperator(codes))
	for _, source := range []string{
		`f = func(node) { node.Process() }; this.ForEachChild(f)`,
		`this.ForEachChild(func(node) { node.AddInfo("f", func() { return 1 }) })`,
		`this.ForEachChild(func(node) { node.GetCfg("callback")() })`,
		`this.ForEachChild(func(node) { eval("dynamic = 1") })`,
	} {
		p, err := loadPreparedOperator(source)
		require.NoError(t, err)
		require.Nil(t, p, source)
	}
}
