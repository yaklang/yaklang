package bash2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseBashAST(t *testing.T) {
	builder := CreateBuilder()
	raw, err := builder.ParseAST(`x=$(curl -s http://example.com); echo "$x" | bash`, nil)
	require.NoError(t, err)

	file, ok := raw.(*syntax.File)
	require.True(t, ok)
	require.Len(t, file.Stmts, 2)

	first, ok := file.Stmts[0].Cmd.(*syntax.CallExpr)
	require.True(t, ok)
	require.Len(t, first.Assigns, 1)
	require.Equal(t, "x", first.Assigns[0].Name.Value)
	require.NotNil(t, first.Assigns[0].Value)
	require.Len(t, first.Assigns[0].Value.Parts, 1)
	_, ok = first.Assigns[0].Value.Parts[0].(*syntax.CmdSubst)
	require.True(t, ok)

	second, ok := file.Stmts[1].Cmd.(*syntax.BinaryCmd)
	require.True(t, ok)
	require.Equal(t, syntax.Pipe, second.Op)

	echo, ok := second.X.Cmd.(*syntax.CallExpr)
	require.True(t, ok)
	require.Len(t, echo.Args, 2)
	require.Equal(t, "echo", echo.Args[0].Lit())
	require.Len(t, echo.Args[1].Parts, 1)
	quoted, ok := echo.Args[1].Parts[0].(*syntax.DblQuoted)
	require.True(t, ok)
	require.Len(t, quoted.Parts, 1)
	_, ok = quoted.Parts[0].(*syntax.ParamExp)
	require.True(t, ok)

	bashStmt, ok := second.Y.Cmd.(*syntax.CallExpr)
	require.True(t, ok)
	require.Equal(t, "bash", bashStmt.Args[0].Lit())
}
