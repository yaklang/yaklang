package yakit

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestResolveSSAReadTargetsDedicatedIncludesDefault(t *testing.T) {
	dedicatedPath := fmt.Sprintf("%s/dedicated-%s.db", t.TempDir(), uuid.NewString())
	_, err := consts.GetOrOpenSSADB(dedicatedPath)
	require.NoError(t, err)

	project := &schema.SSAProject{
		ProjectName:  fmt.Sprintf("multi-db-%s", uuid.NewString()),
		Language:     ssaconfig.GO,
		URL:          "/tmp/multi-db-src",
		DatabasePath: dedicatedPath,
	}
	require.NoError(t, consts.GetGormProfileDatabase().Create(project).Error)
	t.Cleanup(func() {
		_ = consts.CloseSSADBPath(dedicatedPath)
		consts.GetGormProfileDatabase().Unscoped().Delete(project)
	})
	require.True(t, ProjectUsesDedicatedSSADB(project))

	targets, err := ResolveSSAReadTargets(uint64(project.ID))
	require.NoError(t, err)
	require.Len(t, targets, 3)
	kinds := map[SSAReadTargetKind]bool{}
	for _, tg := range targets {
		kinds[tg.Kind] = true
		require.NotNil(t, tg.DB)
	}
	require.True(t, kinds[SSAReadTargetDedicated])
	require.True(t, kinds[SSAReadTargetDefaultMigrated])
	require.True(t, kinds[SSAReadTargetDefaultLegacy])
}
