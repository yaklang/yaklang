package scannode

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

func TestScanNodeParentConsumesSSADatabaseEnv(t *testing.T) {
	_, original := consts.GetSSADataBaseInfo()
	t.Cleanup(func() { consts.SetSSADatabaseInfo(original) })
	const configured = "postgres://fixture:fixture@127.0.0.1:15432/maintenance_ir?sslmode=disable"
	t.Setenv(consts.ENV_SSA_DATABASE_RAW, configured)
	t.Setenv(ssagitworkdir.WorkDirEnv, t.TempDir())

	scanNode, err := NewScanNode(node.BaseConfig{
		AgentInstallationID: "maintenance-database-fixture",
		BaseDir:             t.TempDir(),
		EnrollmentToken:     "fixture-enrollment-token",
		PlatformAPIBaseURL:  "http://platform.test",
		TransportClient:     &bootstrapSessionTransport{},
	})
	require.NoError(t, err)
	t.Cleanup(scanNode.Shutdown)
	dialect, raw := consts.GetSSADataBaseInfo()
	require.Equal(t, consts.Postgres, dialect)
	require.Equal(t, configured, raw, "parent maintenance must not silently use default SQLite")
}
