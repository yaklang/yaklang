package consts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestPathsRespectYakitHome(t *testing.T) {
	tmp := t.TempDir()
	yakitHome := filepath.Join(tmp, "yakit-home")
	require.NoError(t, os.MkdirAll(yakitHome, 0o755))
	t.Setenv("YAKIT_HOME", yakitHome)

	require.Equal(t, yakitHome, GetDefaultYakitBaseDir())
	require.True(t, strings.HasPrefix(GetDefaultYakitBaseTempDir(), yakitHome))
	require.True(t, strings.HasPrefix(GetNucleiTemplatesDir(), yakitHome))
	require.Equal(t, filepath.Join(yakitHome, "nuclei-templates"), GetNucleiTemplatesDir())
	require.Equal(t, filepath.Join(yakitHome, ".ym-id"), utils.GetMachineIdFilePath())

	f, err := utils.OpenTempFile("yakit-home-path-probe.tmp")
	require.NoError(t, err)
	tempPath := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(tempPath))
	require.True(t, strings.HasPrefix(tempPath, filepath.Join(yakitHome, "temp")))
}
