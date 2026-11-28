package macho_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

func TestMachODefinedSymbols(t *testing.T) {
	assembler, err := exec.LookPath("llvm-mc")
	if err != nil {
		t.Skip("llvm-mc is not installed")
	}
	const assembly = `.text
.globl _entry
_entry:
    ret

_ltmp29:
    ret

.section __TEXT,__const
.globl _greeting
_greeting:
    .asciz "hello from Mach-O"

.zerofill __DATA,__bss,_buffer,8,3
`
	dir := t.TempDir()
	source := filepath.Join(dir, "symbols.s")
	require.NoError(t, os.WriteFile(source, []byte(assembly), 0o644))
	object := filepath.Join(dir, "symbols.o")
	output, err := exec.Command(assembler, "-triple=arm64-apple-darwin", "-filetype=obj", "-o", object, source).CombinedOutput()
	require.NoError(t, err, "%s", output)

	artifact, err := loader.Default.OpenFile(context.Background(), object)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, artifact.Close()) })
	require.Equal(t, core.FormatMachO, artifact.Format)

	entry, ok := artifact.Image.SymbolByName("entry")
	require.True(t, ok)
	require.Equal(t, symbol.SymbolFunction, entry.Kind)
	require.Zero(t, entry.Address)

	for _, temp := range []string{"ltmp0", "ltmp1", "ltmp2"} {
		_, found := artifact.Image.SymbolByName(temp)
		require.False(t, found, "assembler temporary label %s should be filtered", temp)
	}

	greeting, ok := artifact.Image.SymbolByName("greeting")
	require.True(t, ok, "Mach-O symbol table should include the string's name")
	require.Equal(t, symbol.SymbolObject, greeting.Kind)
	require.False(t, greeting.Imported)
	value, err := artifact.Language.Strings.Decode(artifact, greeting.Address)
	require.NoError(t, err)
	require.Equal(t, "hello from Mach-O", value)

	buffer, ok := artifact.Image.SymbolByName("buffer")
	require.True(t, ok)
	require.Equal(t, symbol.SymbolObject, buffer.Kind)
	section, ok := artifact.Image.SectionAt(buffer.Address)
	require.True(t, ok)
	require.Zero(t, section.FileSize)

	lift, err := asm2lowir.Parse(context.Background(), artifact, asm2lowir.WithAll())
	require.NoError(t, err)
	var names []string
	for _, function := range lift.IR.Functions {
		names = append(names, function.Name)
	}
	require.Equal(t, []string{"entry", "ltmp29"}, names)
}
