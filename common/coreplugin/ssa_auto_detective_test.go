package coreplugin

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSSAAutoDetective(t *testing.T) {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	check := func(t *testing.T, input string) *programInfo {
		info, prog, err := ParseProjectWithAutoDetective(context.Background(), input, "", true)
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

	t.Run("create SSA project via gRPC with params raw data", func(t *testing.T) {
		// 创建一个临时目录
		tempDir := path.Join(os.TempDir(), uuid.NewString())
		err := os.MkdirAll(tempDir, 0755)
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// 创建测试文件
		javaFile := path.Join(tempDir, "Main.java")
		err = os.WriteFile(javaFile, []byte("public class Main { public static void main(String[] args) {} }"), 0644)
		require.NoError(t, err)

		// 使用 SSA 探测获取 params
		info, prog, err := ParseProjectWithAutoDetective(
			context.Background(),
			tempDir,
			"",
			false,
		)
		require.NoError(t, err)
		require.Nil(t, prog)

		// 将 info 转换为 JSON
		paramsJSON, err := json.Marshal(info)
		require.NoError(t, err)
		log.Infof("Creating SSA project with params:\n%s", string(paramsJSON))

		// 通过 gRPC 创建 SSA 项目
		req := &ypb.CreateSSAProjectRequest{
			ProjectRawData: string(paramsJSON),
		}

		db := consts.GetGormProfileDatabase()
		project, err := yakit.CreateSSAProject(db, req)
		require.NoError(t, err)
		require.NotNil(t, project)

		log.Infof("SSA Project created successfully:")
		log.Infof("  ID: %d", project.ID)
		log.Infof("  ProjectName: %s", project.ProjectName)
		log.Infof("  Language: %s", project.Language)
		log.Infof("  Description: %s", project.Description)
		// Check
		require.Equal(t, info.ProjectName, project.ProjectName)
		require.Equal(t, info.Language, string(project.Language))
		require.Equal(t, info.Description, project.Description)

		defer func() {
			deleteReq := &ypb.DeleteSSAProjectRequest{
				Filter: &ypb.SSAProjectFilter{
					IDs: []int64{int64(project.ID)},
				},
			}
			count, err := yakit.DeleteSSAProject(db, deleteReq)
			if err != nil {
				log.Errorf("Failed to delete SSA project: %v", err)
			} else {
				log.Infof("Deleted %d SSA project(s)", count)
			}
		}()
	})
}
