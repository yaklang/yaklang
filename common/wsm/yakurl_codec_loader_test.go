package wsm

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestLoadWebShellAndCodecScriptsUsesProfileDatabase(t *testing.T) {
	projectDB, err := consts.CreateProjectDatabase(filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	profileDB, err := consts.CreateProfileDatabase(filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)

	shell, err := yakit.CreateOrUpdateWebShell(projectDB, "test-shell", &schema.WebShell{
		Url:              "http://example.com/shell.jsp",
		SecretKey:        "rebeyond",
		ShellType:        ypb.ShellType_Behinder.String(),
		ShellScript:      ypb.ShellScript_JSP.String(),
		PacketCodecName:  "packet-codec",
		PayloadCodecName: "payload-codec",
	})
	require.NoError(t, err)

	require.NoError(t, yakit.CreateOrUpdateYakScriptByName(profileDB, "packet-codec", &schema.YakScript{
		ScriptName: "packet-codec",
		Content:    "packet-content",
	}))
	require.NoError(t, yakit.CreateOrUpdateYakScriptByName(profileDB, "payload-codec", &schema.YakScript{
		ScriptName: "payload-codec",
		Content:    "payload-content",
	}))

	grpcShell, packetScript, payloadScript, err := loadWebShellAndCodecScripts(projectDB, profileDB, int64(shell.ID))
	require.NoError(t, err)
	require.Equal(t, shell.ID, uint(grpcShell.GetId()))
	require.Equal(t, "packet-content", packetScript)
	require.Equal(t, "payload-content", payloadScript)
}
