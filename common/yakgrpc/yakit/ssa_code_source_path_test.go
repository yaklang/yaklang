package yakit

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestLocalPathFromCompileConfigJSON(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ssaconfig.New(ssaconfig.ModeAll, ssaconfig.WithCodeSourceInfo(&ssaconfig.CodeSourceInfo{
		Kind:      ssaconfig.CodeSourceLocal,
		LocalFile: dir,
	}))
	require.NoError(t, err)
	raw, err := cfg.ToJSONRaw()
	require.NoError(t, err)
	require.Equal(t, dir, localPathFromCompileConfigJSON(string(raw)))
}

func TestResolveCodeSourceLocalPath_FromIrProgram(t *testing.T) {
	db := consts.GetGormSSAProjectDataBase()
	if db == nil {
		t.Skip("ssa database is not initialized")
	}
	dir := t.TempDir()
	cfg, err := ssaconfig.New(ssaconfig.ModeAll, ssaconfig.WithCodeSourceInfo(&ssaconfig.CodeSourceInfo{
		Kind:      ssaconfig.CodeSourceLocal,
		LocalFile: dir,
	}))
	require.NoError(t, err)
	raw, err := cfg.ToJSONRaw()
	require.NoError(t, err)

	name := "Heap-Exploitation(2026-08-28 14:07:27)-" + uuid.NewString()
	prog := &ssadb.IrProgram{
		ProgramName: name,
		ConfigInput: string(raw),
	}
	require.NoError(t, db.Create(prog).Error)
	t.Cleanup(func() {
		db.Unscoped().Delete(prog)
	})

	got := ResolveCodeSourceLocalPath(name)
	require.Equal(t, dir, got)

	st, err := os.Stat(got)
	require.NoError(t, err)
	require.True(t, st.IsDir())
}

func TestResolveCodeSourceLocalPath_IgnoresPlainRelativeName(t *testing.T) {
	require.Empty(t, ResolveCodeSourceLocalPath("not-a-real-ssa-program-"+uuid.NewString()))
}
