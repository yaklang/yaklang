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

	t.Run("rejects imported zero-address symbol", func(t *testing.T) {
		elfPath := assembleAddressFixture(t, "x86_64-linux-gnu", ".o", `.rodata
.globl greeting
greeting:
    .asciz "hello elf zero"

.data
.quad imported_greeting
`)
		artifact, _, err := reify.Inspect(context.Background(), elfPath)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, artifact.Close()) })
		greeting, ok := artifact.Image.SymbolByName("greeting")
		require.True(t, ok)
		imported, ok := artifact.Image.SymbolByName("imported_greeting")
		require.True(t, ok)
		require.True(t, imported.Imported)
		require.Zero(t, imported.Address)

		code, stdout, stderr := runCLI("strings", "--json", "--at", "greeting", elfPath)
		require.Zero(t, code, "%s", stderr)
		var report stringReport
		require.NoError(t, json.Unmarshal([]byte(stdout), &report))
		require.Equal(t, greeting.Address, report.Address)
		require.Equal(t, "hello elf zero", report.Value)

		code, _, stderr = runCLI("strings", "--at", "imported_greeting", elfPath)
		require.Equal(t, 2, code)
		require.Contains(t, stderr, "unknown address")
	})
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

			code, asmStdout, asmStderr := runCLI("lift", "--json", "--mode", "asm", "--func", name, path)
			require.Zero(t, code, "%s", asmStderr)
			var asmReport liftReport
			require.NoError(t, json.Unmarshal([]byte(asmStdout), &asmReport))
			require.Equal(t, entry.Address, asmReport.Region.Address)
			require.Equal(t, entry.Size, asmReport.Region.Size)
			require.Equal(t, 2, asmReport.Instructions)
			require.Equal(t, report.Assembly, asmReport.Assembly)
		})
	}
}

func TestCLILiftsMixedARMThumb(t *testing.T) {
	path := assembleAddressFixture(t, "armv7-linux-gnueabi", ".o", `.syntax unified
.text
.arm
.globl arm_func
.type arm_func, %function
arm_func:
    mov r0, #42
    bx lr
.size arm_func, .-arm_func

.thumb
.globl thumb_func, thumb_alias
.type thumb_func, %function
.type thumb_alias, %function
.thumb_func
thumb_func:
thumb_alias:
    movs r0, #7
    bx lr
.size thumb_func, .-thumb_func
.size thumb_alias, .-thumb_alias
`)

	artifact, _, err := reify.Inspect(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, artifact.Close()) })

	for _, name := range []string{"thumb_func", "thumb_alias"} {
		t.Run(name, func(t *testing.T) {
			entry, ok := artifact.Image.SymbolByName(name)
			require.True(t, ok)
			require.Equal(t, uint64(1), entry.Address&1)
			wantAddress := entry.Address &^ 1

			code, allStdout, allStderr := runCLI("lift", "--json", "--mode", "all", "--func", name, path)
			require.Zero(t, code, "%s", allStderr)
			var allReport liftReport
			require.NoError(t, json.Unmarshal([]byte(allStdout), &allReport))
			require.NotNil(t, allReport.IR)
			require.Len(t, allReport.IR.Functions, 1)
			require.Equal(t, name, allReport.IR.Functions[0].Name)
			require.Equal(t, wantAddress, allReport.Region.Address)
			require.Equal(t, uint64(4), allReport.Region.Size)
			require.Contains(t, allReport.Assembly, "07 20")
			require.Contains(t, allReport.Assembly, "70 47")

			code, asmStdout, asmStderr := runCLI("lift", "--json", "--mode", "asm", "--func", name, path)
			require.Zero(t, code, "%s", asmStderr)
			var asmReport liftReport
			require.NoError(t, json.Unmarshal([]byte(asmStdout), &asmReport))
			require.Equal(t, wantAddress, asmReport.Region.Address)
			require.Equal(t, uint64(4), asmReport.Region.Size)
			require.Equal(t, 2, asmReport.Instructions)
			require.Equal(t, allReport.Assembly, asmReport.Assembly)
		})
	}
}

func TestCLIStringsMachO(t *testing.T) {
	path := assembleAddressFixture(t, "arm64-apple-darwin", ".o", `.text
.globl _entry
_entry:
    ret

.section __TEXT,__const
.globl _greeting
_greeting:
    .asciz "hello from Mach-O"
`)
	code, stdout, stderr := runCLI("strings", "--json", "--at", "greeting", path)
	require.Zero(t, code, "%s", stderr)
	var report stringReport
	require.NoError(t, json.Unmarshal([]byte(stdout), &report))
	require.Equal(t, "hello from Mach-O", report.Value)
}
