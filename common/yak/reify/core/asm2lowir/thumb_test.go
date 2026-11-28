package asm2lowir_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

func TestAssemblyUsesFunctionMode(t *testing.T) {
	assembler, err := exec.LookPath("llvm-mc")
	if err != nil {
		t.Skip("llvm-mc is not installed")
	}
	const assembly = `.syntax unified
.text
.arm
.globl arm_func
.type arm_func, %function
arm_func:
    mov r0, #42
    bx lr
.size arm_func, .-arm_func

.thumb
.globl thumb_func
.type thumb_func, %function
.thumb_func
thumb_func:
    movs r0, #7
    bx lr
.size thumb_func, .-thumb_func
`
	dir := t.TempDir()
	source := filepath.Join(dir, "mixed.s")
	require.NoError(t, os.WriteFile(source, []byte(assembly), 0o644))
	object := filepath.Join(dir, "mixed.o")
	output, err := exec.Command(assembler, "-triple=armv7-linux-gnueabi", "-filetype=obj", "-o", object, source).CombinedOutput()
	require.NoError(t, err, "%s", output)

	ctx := context.Background()
	artifact, err := loader.Default.OpenFile(ctx, object)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, artifact.Close()) })
	require.Equal(t, "arm", artifact.Target.Mode)

	for _, tc := range []struct {
		name         string
		selector     asm2lowir.Option
		instructions int
	}{
		{"thumb function", asm2lowir.WithFunction("thumb_func"), 2},
		{"mixed section", asm2lowir.WithSection(".text"), 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := asm2lowir.Parse(ctx, artifact, tc.selector, asm2lowir.WithAssembly())
			require.NoError(t, err)
			require.Contains(t, parsed.Assembly, "07 20")
			require.Contains(t, parsed.Assembly, "70 47")

			asm, err := asm2lowir.Disassemble(ctx, artifact, tc.selector)
			require.NoError(t, err)
			require.Equal(t, tc.instructions, asm.Instructions, asm.Text)
			require.Equal(t, parsed.Assembly, asm.Text)
		})
	}
}
