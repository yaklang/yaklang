package coreplugin

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestSSAAutoDetective(t *testing.T) {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	check := func(t *testing.T, input string) *programInfo {
		info, prog, err := ParseProjectWithAutoDetective(context.Background(), input, "")
		_ = err
		_ = prog
		return info
	}

	t.Run("check compile jar", func(t *testing.T) {
		jarPath, err := ssatest.GetJarFile()
		require.NoError(t, err)
		info, prog, err := ParseProjectWithAutoDetective(context.Background(), jarPath, "")
		require.NoError(t, err)
		require.NotNil(t, prog)
		log.Infof("info: %v", info)
	})

	t.Run("check jar", func(t *testing.T) {
		jarPath, err := ssatest.GetJarFile()
		require.NoError(t, err)
		info := check(t, jarPath)
		log.Infof("info: %v", info)
		require.Equal(t, info.Language, "java")
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "")
		require.NotNil(t, info.Info)
		require.Equal(t, info.Info.Kind, "jar")
		require.Equal(t, info.Info.LocalFile, jarPath)
	})

	t.Run("check zip", func(t *testing.T) {
		zipPath, err := ssatest.GetZipFile()
		require.NoError(t, err)
		info := check(t, zipPath)
		log.Infof("info: %v", info)
		require.Equal(t, info.Language, "java")
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "")
		require.NotNil(t, info.Info)
		require.Equal(t, info.Info.Kind, "compression")
		require.Equal(t, info.Info.LocalFile, zipPath)
	})

	t.Run("check error path", func(t *testing.T) {
		dir := os.TempDir()
		// create a not exist dir
		dir = path.Join(dir, uuid.NewString(), uuid.NewString())
		info := check(t, dir)

		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "fileNotFoundException")
	})

	t.Run("check unsupported file ", func(t *testing.T) {
		dir := os.TempDir()
		file := path.Join(dir, "test.txt")
		err := os.WriteFile(file, []byte("test"), 0644)
		require.NoError(t, err)

		info := check(t, file)
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "fileTypeException")
	})

	t.Run("check git", func(t *testing.T) {
		url, err := ssatest.GetLocalGit()
		require.NoError(t, err)
		info := check(t, url)
		log.Infof("info: %v", info)
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "languageNeedSelectException")
	})

	t.Run("check un access url", func(t *testing.T) {
		info := check(t, "http://127.0.0.1:7777/1123/5"+uuid.NewString())
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "connectFailException")
	})
}
