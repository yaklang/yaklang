package coreplugin

import (
	"context"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSSAProjectUpdate(t *testing.T) {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	t.Run("update SSA project via yak script", func(t *testing.T) {
		// 1. 创建一个临时目录和测试文件
		tempDir := path.Join(os.TempDir(), uuid.NewString())
		err := os.MkdirAll(tempDir, 0755)
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// 创建测试文件
		javaFile := path.Join(tempDir, "Test.java")
		err = os.WriteFile(javaFile, []byte("public class Test { }"), 0644)
		require.NoError(t, err)

		// 2. 创建SSA项目
		projectName := "test-update-" + uuid.NewString()

		config, err := ssaconfig.New(ssaconfig.ModeAll,
			ssaconfig.WithProjectLanguage("java"),
			ssaconfig.WithProgramDescription("测试项目"),
			ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
			ssaconfig.WithCodeSourceLocalFile(tempDir),
			ssaconfig.WithCompileStrictMode(false),
			ssaconfig.WithCompilePeepholeSize(0),
			ssaconfig.WithCompileConcurrency(10),
		)
		require.NoError(t, err)

		// 手动设置 BaseInfo.ProjectName
		if config.BaseInfo == nil {
			config.BaseInfo = &ssaconfig.BaseInfo{}
		}
		config.BaseInfo.ProjectName = projectName
		config.BaseInfo.ProjectDescription = "测试项目"
		config.BaseInfo.Language = "java"
		config.BaseInfo.Tags = []string{"tag1", "tag2"}

		db := consts.GetGormProfileDatabase()

		// 创建 schema.SSAProject
		schemaProject := &schema.SSAProject{
			ProjectName: projectName,
			Description: "原始描述",
			Tags:        "tag1,tag2",
			Language:    "java",
		}
		err = schemaProject.SetConfig(config)
		require.NoError(t, err)

		err = db.Create(schemaProject).Error
		require.NoError(t, err)
		require.Greater(t, schemaProject.ID, uint(0))

		log.Infof("Created SSA project: ID=%d, Name=%s", schemaProject.ID, schemaProject.ProjectName)

		defer func() {
			// 清理
			db.Delete(&schema.SSAProject{}, schemaProject.ID)
			log.Infof("Deleted SSA project: ID=%d", schemaProject.ID)
		}()

		// 3. 准备更新配置数据
		updateConfig, err := ssaconfig.New(
			ssaconfig.ModeAll,
			ssaconfig.WithProjectID(uint64(schemaProject.ID)),
			ssaconfig.WithProjectName(projectName),
			ssaconfig.WithProgramDescription("更新后的描述"),
			ssaconfig.WithProjectLanguage(ssaconfig.JAVA),
			ssaconfig.WithProjectTags([]string{"tag1", "tag2", "tag3"}),
			// CodeSource
			ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
			ssaconfig.WithCodeSourceLocalFile(tempDir),
			// SSACompile
			ssaconfig.WithCompileStrictMode(true),
			ssaconfig.WithCompilePeepholeSize(10),
			ssaconfig.WithCompileExcludeFiles([]string{"**/test/**"}),
			ssaconfig.WithCompileReCompile(true),
			ssaconfig.WithCompileMemoryCompile(false),
			ssaconfig.WithCompileConcurrency(5),
		)
		require.NoError(t, err)

		log.Infof("Prepared update config with ProjectID=%d", updateConfig.GetProjectID())

		configDataJSON, err := updateConfig.ToJSONString()
		require.NoError(t, err)

		log.Infof("Updating SSA project with config data:\n%s", configDataJSON)

		// 4. 调用 SSA 项目更新脚本
		pluginName := "SSA 项目更新"
		param := make(map[string]string)
		param["project_id"] = strconv.Itoa(int(schemaProject.ID))
		param["config_data"] = configDataJSON

		log.Infof("Calling SSA update script with project_id=%d", schemaProject.ID)
		var scriptOutput []string
		err = yakgrpc.ExecScriptWithParam(context.Background(), pluginName, param,
			"", func(exec *ypb.ExecResult) error {
				if exec.IsMessage {
					log.Infof("[Script] %s", string(exec.Message))
					scriptOutput = append(scriptOutput, string(exec.Message))
				}
				return nil
			},
		)
		require.NoError(t, err, "Script execution should succeed")

		log.Infof("Script execution completed, output lines: %d", len(scriptOutput))

		// 5. 验证更新结果
		log.Infof("Verifying updated project...")
		var updatedProject schema.SSAProject
		err = db.First(&updatedProject, schemaProject.ID).Error
		require.NoError(t, err, "Should find updated project in database")

		log.Infof("Found updated project:")
		log.Infof("  ProjectName: %s", updatedProject.ProjectName)
		log.Infof("  Description: %s", updatedProject.Description)
		log.Infof("  Language: %s", updatedProject.Language)
		log.Infof("  Tags: %s", updatedProject.Tags)

		// 验证基础字段
		require.Equal(t, projectName, updatedProject.ProjectName, "ProjectName should match")
		require.Equal(t, "更新后的描述", updatedProject.Description, "Description should be updated")
		require.Equal(t, ssaconfig.JAVA, updatedProject.Language, "Language should remain java")

		// 验证配置
		updatedConfig, err := updatedProject.GetConfig()
		require.NoError(t, err, "Should get config from updated project")
		require.NotNil(t, updatedConfig, "Config should not be nil")

		log.Infof("Updated config values:")
		log.Infof("  StrictMode: %v", updatedConfig.GetCompileStrictMode())
		log.Infof("  PeepholeSize: %d", updatedConfig.GetCompilePeepholeSize())
		log.Infof("  Concurrency: %d", updatedConfig.GetCompileConcurrency())
		log.Infof("  ExcludeFiles: %v", updatedConfig.GetCompileExcludeFiles())

		// 验证编译配置
		require.True(t, updatedConfig.GetCompileStrictMode(), "StrictMode should be true")
		require.Equal(t, 10, updatedConfig.GetCompilePeepholeSize(), "PeepholeSize should be 10")
		require.Equal(t, 5, updatedConfig.GetCompileConcurrency(), "Concurrency should be 5")
		require.Contains(t, updatedConfig.GetCompileExcludeFiles(), "**/test/**", "Should contain exclude pattern")

		// 验证BaseInfo同步
		require.NotNil(t, updatedConfig.BaseInfo, "BaseInfo should not be nil")
		require.Equal(t, projectName, updatedConfig.BaseInfo.ProjectName, "BaseInfo.ProjectName should match")
		require.Equal(t, "更新后的描述", updatedConfig.BaseInfo.ProjectDescription, "BaseInfo.ProjectDescription should match")
		require.Equal(t, ssaconfig.JAVA, updatedConfig.BaseInfo.Language, "BaseInfo.Language should match")
		require.Equal(t, []string{"tag1", "tag2", "tag3"}, updatedConfig.BaseInfo.Tags, "BaseInfo.Tags should match")

		require.Equal(t, strings.Join([]string{"tag1", "tag2", "tag3"}, ","), updatedProject.Tags, "Schema.Tags should match")

		log.Infof("✅ SSA project update test passed successfully!")
	})
}
