package coreplugin

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"strconv"
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

		// 3. 准备更新数据
		updateConfig := map[string]interface{}{
			"CodeSource": map[string]interface{}{
				"kind":       "local",
				"local_file": tempDir,
			},
			"SSACompile": map[string]interface{}{
				"strict_mode":         true,
				"peephole_size":       10,
				"exclude_files":       []string{"**/test/**"},
				"re_compile":          true,
				"memory_compile":      false,
				"compile_concurrency": 5,
			},
			"SyntaxFlow": map[string]interface{}{
				"memory": false,
			},
			"SyntaxFlowScan": map[string]interface{}{
				"concurrency": 3,
			},
		}

		projectDataMap := map[string]interface{}{
			"project_name": projectName,
			"description":  "更新后的描述",
			"language":     "java",
			"config":       updateConfig,
			"tags":         "tag1,tag2,tag3",
		}

		projectDataJSON, err := json.Marshal(projectDataMap)
		require.NoError(t, err)

		log.Infof("Updating SSA project with data:\n%s", string(projectDataJSON))

		// 4. 调用 SSA 项目更新脚本
		pluginName := "SSA 项目更新"
		param := make(map[string]string)
		param["project_id"] = strconv.Itoa(int(schemaProject.ID))
		param["project_data"] = string(projectDataJSON)

		err = yakgrpc.ExecScriptWithParam(context.Background(), pluginName, param,
			"", func(exec *ypb.ExecResult) error {
				return nil
			},
		)
		require.NoError(t, err)

		// 5. 验证更新结果
		var updatedProject schema.SSAProject
		err = db.First(&updatedProject, schemaProject.ID).Error
		require.NoError(t, err)

		// 验证基础字段
		require.Equal(t, projectName, updatedProject.ProjectName)
		require.Equal(t, "更新后的描述", updatedProject.Description)
		require.Equal(t, ssaconfig.JAVA, updatedProject.Language)

		// 验证配置
		updatedConfig, err := updatedProject.GetConfig()
		require.NoError(t, err)
		require.NotNil(t, updatedConfig)

		// 验证编译配置
		require.True(t, updatedConfig.GetCompileStrictMode())
		require.Equal(t, 10, updatedConfig.GetCompilePeepholeSize())
		require.Equal(t, uint32(5), updatedConfig.GetCompileConcurrency())
		require.Contains(t, updatedConfig.GetCompileExcludeFiles(), "**/test/**")

		// 验证扫描配置
		require.Equal(t, uint32(3), updatedConfig.SyntaxFlowScan.Concurrency)

		// 验证BaseInfo同步
		require.Equal(t, projectName, updatedConfig.BaseInfo.ProjectName)
		require.Equal(t, "更新后的描述", updatedConfig.BaseInfo.ProjectDescription)
		require.Equal(t, ssaconfig.JAVA, updatedConfig.BaseInfo.Language)
		require.Equal(t, []string{"tag1", "tag2", "tag3"}, updatedConfig.BaseInfo.Tags)

		log.Infof("SSA project updated successfully!")
		log.Infof("  ProjectName: %s", updatedProject.ProjectName)
		log.Infof("  Description: %s", updatedProject.Description)
		log.Infof("  Tags: %s", updatedProject.Tags)
		log.Infof("  StrictMode: %v", updatedConfig.GetCompileStrictMode())
		log.Infof("  PeepholeSize: %d", updatedConfig.GetCompilePeepholeSize())
		log.Infof("  Concurrency: %d", updatedConfig.GetCompileConcurrency())
	})
}
