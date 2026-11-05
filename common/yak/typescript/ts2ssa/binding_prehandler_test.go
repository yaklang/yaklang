package ts2ssa

import (
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestBindingPatternPrehandlerDeclaresExportsWithoutEvaluating(t *testing.T) {
	code := `
export const { [key()]: renamed = fallback(), nested: { leaf }, ...rest } = source();
export const [first, , { value: last }, ...tail] = arraySource();
`
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	fb := prog.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
	editor := prog.CreateEditor([]byte(code), "exports.ts")
	fb.SetEditor(editor)
	frontend := CreateBuilder()
	frontend.InitHandler(fb)
	defer frontend.Clearup()
	prog.SetPreHandler(true)
	tree, err := Frontend(code)
	require.NoError(t, err)
	require.NoError(t, frontend.BuildFromAST(tree, fb))
	require.Empty(t, prog.GetErrors())
	lib, _ := prog.GetLibrary(path.Join(editor.GetGlobalFolderPath(), editor.GetFilename()))
	require.NotNil(t, lib)
	wantNames := []string{"renamed", "leaf", "rest", "first", "last", "tail"}
	require.ElementsMatch(t, wantNames, lib.ExportValue.Keys())
	for _, name := range wantNames {
		value := lib.GetExportValue(name)
		require.NotNil(t, value)
		require.Equal(t, ssa.SSAOpcodeUndefined, value.GetOpcode(), name)
	}
	for _, function := range lib.Funcs.Values() {
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			require.True(t, ok)
			for _, id := range block.Insts {
				inst, ok := prog.GetInstructionById(id)
				require.True(t, ok)
				require.NotEqual(t, ssa.SSAOpcodeCall, inst.GetOpcode(), "initializers, keys and defaults must be deferred")
			}
		}
	}
}

func TestBindingPatternKeepsFileEditorAcrossPhases(t *testing.T) {
	code := `export const { value: selected } = { value: 42 };`
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	fb := prog.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
	editor := prog.CreateEditor([]byte(code), "values.ts")
	fb.SetEditor(editor)
	frontend := CreateBuilder()
	frontend.InitHandler(fb)
	defer frontend.Clearup()
	prog.SetPreHandler(true)
	// Deterministically model another compile unit owning the application stack.
	prog.PushEditor(prog.CreateEditor([]byte(`const unrelated = 0;`), "unrelated.ts"))
	tree, err := Frontend(code)
	require.NoError(t, err)
	require.NoError(t, frontend.BuildFromAST(tree, fb))
	lib, _ := prog.GetLibrary(path.Join(editor.GetGlobalFolderPath(), editor.GetFilename()))
	require.NotNil(t, lib)
	require.Same(t, editor, lib.GetCurrentEditor())
	require.Same(t, editor, lib.GetExportValue("selected").GetRange().GetEditor())
	prog.SetPreHandler(false)
	require.NoError(t, frontend.BuildFromAST(tree, fb))
	require.Empty(t, prog.GetErrors())
	value := lib.GetExportValue("selected")
	require.Equal(t, ssa.SSAOpcodeConstInst, value.GetOpcode())
	require.Equal(t, "42", value.String())
	require.Same(t, editor, value.GetRange().GetEditor())
}
