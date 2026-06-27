//go:build ssa_discovery_integration

package loop_ssa_api_discovery

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestSSACompileMinimalJavaFixture runs a full SSA compile against the bundled Spring Boot sample (slow; requires network for Maven deps if not cached).
func TestSSACompileMinimalJavaFixture(t *testing.T) {
	db, err := consts.GetTempSSADataBase()
	require.NoError(t, err)
	consts.SetGormSSAProjectDatabase(db)
	t.Cleanup(func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	root, err := filepath.Abs(filepath.Join("testfixtures", "minimal_java_webapp"))
	require.NoError(t, err)

	progName := "disc_test_" + uuid.NewString()
	progs, err := ssaapi.ParseProjectFromPath(root,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(progName),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	require.NotEmpty(t, progs[0].GetProgramName())

	t.Cleanup(func() {
		ssadb.DeleteProgram(db, progName)
	})
}
