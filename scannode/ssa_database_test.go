package scannode

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func TestParentSSAConfigurationSurvivesDistYakExtraction(t *testing.T) {
	_, before := consts.GetSSADataBaseInfo()
	t.Cleanup(func() { consts.SetSSADatabaseInfo(before) })
	const configured = "postgres://fixture:fixture@127.0.0.1:15432/parent_ir?sslmode=disable"
	t.Setenv(consts.ENV_SSA_DATABASE_RAW, configured)
	applySSADatabaseFromEnv()
	dialect, raw := consts.GetSSADataBaseInfo()
	require.Equal(t, consts.Postgres, dialect)
	require.Equal(t, configured, raw)
}
