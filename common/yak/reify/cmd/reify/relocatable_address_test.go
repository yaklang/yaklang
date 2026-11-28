package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/reify"
)

func assembleAddressFixture(t *testing.T, triple, suffix, assembly string) string {
	t.Helper()
	assembler, err := exec.LookPath("llvm-mc")
	if err != nil {
		t.Skip("llvm-mc is not installed")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "address.s")
	require.NoError(t, os.WriteFile(source, []byte(assembly), 0o644))
	object := filepath.Join(dir, "address"+suffix)
	output, err := exec.Command(assembler, "-triple="+triple, "-filetype=obj", "-o", object, source).CombinedOutput()
	require.NoError(t, err, "%s", output)
	return object
}

func TestCLIStringsAtZeroAddressSymbol(t *testing.T) {
	path := assembleAddressFixture(t, "x86_64-windows-msvc", ".obj", `.data
.globl greeting
greeting:
    .asciz "hello at zero"
`)
	for _, selector := range []string{"0", "greeting"} {
		t.Run(selector, func(t *testing.T) {
			code, stdout, stderr := runCLI("strings", "--json", "--at", selector, path)
			require.Zero(t, code, "%s", stderr)
			var report stringReport
			require.NoError(t, json.Unmarshal([]byte(stdout), &report))
			require.Zero(t, report.Address)
			require.Equal(t, "hello at zero", report.Value)
		})
	}
}

func TestCLILiftsOddAddressFunctionAliases(t *testing.T) {
	path := assembleAddressFixture(t, "x86_64-linux-gnu", ".o", `.text
.byte 0x90
.globl odd_func, odd_alias
.type odd_func,@function
.type odd_alias,@function
odd_func:
odd_alias:
    movl $42, %eax
    ret
.size odd_func, .-odd_func
.size odd_alias, .-odd_alias
`)
	artifact, _, err := reify.Inspect(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, artifact.Close()) })

	for _, name := range []string{"odd_func", "odd_alias"} {
		t.Run(name, func(t *testing.T) {
			entry, ok := artifact.Image.SymbolByName(name)
			require.True(t, ok)
			require.Equal(t, uint64(1), entry.Address&1)

			code, stdout, stderr := runCLI("lift", "--json", "--mode", "all", "--func", name, path)
			require.Zero(t, code, "%s", stderr)
			var report liftReport
			require.NoError(t, json.Unmarshal([]byte(stdout), &report))
			require.NotNil(t, report.IR)
			require.Len(t, report.IR.Functions, 1)
			require.Equal(t, name, report.IR.Functions[0].Name)
			require.Equal(t, entry.Address, report.IR.Functions[0].Address)
			require.Equal(t, entry.Size, report.IR.Functions[0].Size)
			require.Contains(t, report.Assembly, "b8 2a 00 00 00")
			require.Contains(t, report.Assembly, "c3")
		})
	}
}
