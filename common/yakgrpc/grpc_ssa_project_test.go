package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SSAProjectCRUDOperations(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()
	localSrcDir := filepath.Join(t.TempDir(), "crud-src")

	// 测试用的代码源配置
	codeSourceConfig := &ssaconfig.CodeSourceInfo{
		Kind:      ssaconfig.CodeSourceLocal,
		LocalFile: localSrcDir,
		Auth: &ssaconfig.AuthConfigInfo{
			Kind:     "password",
			UserName: "test",
			Password: "test123",
		},
		Proxy: &ssaconfig.ProxyConfigInfo{
			URL:      "http://127.0.0.1:8080",
			User:     "proxy_user",
			Password: "proxy_pass",
		},
	}

	// 将配置序列化为JSON
	configBytes, err := json.Marshal(codeSourceConfig)
	require.NoError(t, err)
	configJSON := string(configBytes)

	// 1. 测试创建SSA项目
	projectName := fmt.Sprintf("test-project-%v", uuid.NewString())

	t.Run("CreateSSAProject", func(t *testing.T) {
		req := &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      projectName,
				CodeSourceConfig: configJSON,
				Description:      "测试本地项目",
				Language:         "go",
				CompileConfig: &ypb.SSAProjectCompileConfig{
					StrictMode:   true,
					PeepholeSize: 200,
					ExcludeFiles: []string{"*.test.go", "*.mock.go"},
					ReCompile:    false,
				},
				ScanConfig: &ypb.SSAProjectScanConfig{
					Concurrency:    10,
					Memory:         true,
					IgnoreLanguage: false,
				},
				RuleConfig: &ypb.SSAProjectScanRuleConfig{
					RuleFilter: &ypb.SyntaxFlowRuleFilter{
						RuleNames: []string{"sql-injection", "xss"},
					},
				},
				Tags: []string{"test", "local"},
			},
		}

		resp, err := client.CreateSSAProject(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Message)
		require.NotEmpty(t, resp.Message.ExtraMessage)

		t.Logf("Created SSA project successfully: %s", resp.Message.ExtraMessage)
	})

	// 2. 测试查询SSA项目
	var projectID uint64
	t.Run("QuerySSAProject", func(t *testing.T) {
		req := &ypb.QuerySSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{
				ProjectNames: []string{projectName},
			},
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   10,
				OrderBy: "created_at",
				Order:   "desc",
			},
		}

		resp, err := client.QuerySSAProject(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.True(t, len(resp.Projects) > 0)

		// 验证项目信息
		project := resp.Projects[0]
		require.Equal(t, projectName, project.ProjectName)
		require.Equal(t, configJSON, project.CodeSourceConfig)
		require.Equal(t, "测试本地项目", project.Description)
		require.Equal(t, string(ssaconfig.GO), project.Language)
		require.True(t, project.CompileConfig.StrictMode)
		require.Equal(t, int64(200), project.CompileConfig.PeepholeSize)
		require.Equal(t, []string{"*.test.go", "*.mock.go"}, project.CompileConfig.ExcludeFiles)
		require.False(t, project.CompileConfig.ReCompile)
		require.Equal(t, uint32(10), project.ScanConfig.Concurrency)
		require.True(t, project.ScanConfig.Memory)
		require.Equal(t, []string{"sql-injection", "xss"}, project.RuleConfig.RuleFilter.RuleNames)
		require.False(t, project.ScanConfig.IgnoreLanguage)
		require.Equal(t, []string{"test", "local"}, project.Tags)
		require.Equal(t, localSrcDir, project.URL)

		// 保存项目ID用于后续测试
		projectID = uint64(project.ID)

		t.Logf("Found SSA project: ID=%d, Name=%s", project.ID, project.ProjectName)
	})

	// 3. 测试更新SSA项目
	t.Run("UpdateSSAProject", func(t *testing.T) {
		// 修改代码源配置为Git类型
		gitConfig := &ssaconfig.CodeSourceInfo{
			Kind:   ssaconfig.CodeSourceGit,
			URL:    "https://github.com/test/repo.git",
			Branch: "main",
			Path:   "src/main",
			Auth: &ssaconfig.AuthConfigInfo{
				Kind:     "ssh_key",
				UserName: "git",
				KeyPath:  "/home/user/.ssh/id_rsa",
			},
		}

		gitConfigBytes, err := json.Marshal(gitConfig)
		require.NoError(t, err)
		gitConfigJSON := string(gitConfigBytes)

		newProjectName := fmt.Sprintf("test-git-project-updated-%v", uuid.NewString())
		req := &ypb.UpdateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ID:               int64(projectID),
				ProjectName:      newProjectName,
				CodeSourceConfig: gitConfigJSON,
				Description:      "更新为Git项目",
				Language:         "java",
				CompileConfig: &ypb.SSAProjectCompileConfig{
					StrictMode:   false,
					PeepholeSize: 300,
					ExcludeFiles: []string{"*.test.go"},
					ReCompile:    true,
				},
				ScanConfig: &ypb.SSAProjectScanConfig{
					Concurrency: 5,
					Memory:      false,

					IgnoreLanguage: true,
				},
				RuleConfig: &ypb.SSAProjectScanRuleConfig{
					RuleFilter: &ypb.SyntaxFlowRuleFilter{
						RuleNames: []string{"sql-injection"},
					},
				},
				Tags: []string{newProjectName, "git"},
			},
		}

		resp, err := client.UpdateSSAProject(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Message)

		t.Logf("Updated SSA project successfully: %s", resp.Message.ExtraMessage)

		// 验证更新结果
		queryReq := &ypb.QuerySSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{
				IDs: []int64{int64(projectID)},
			},
		}

		queryResp, err := client.QuerySSAProject(ctx, queryReq)
		require.NoError(t, err)
		require.Equal(t, 1, len(queryResp.Projects))

		updatedProject := queryResp.Projects[0]

		require.Equal(t, gitConfigJSON, updatedProject.CodeSourceConfig)
		require.Equal(t, "更新为Git项目", updatedProject.Description)
		require.Equal(t, "java", updatedProject.Language)
		require.False(t, updatedProject.CompileConfig.StrictMode)
		require.Equal(t, int64(300), updatedProject.CompileConfig.PeepholeSize)
		require.Equal(t, []string{"*.test.go"}, updatedProject.CompileConfig.ExcludeFiles)
		require.True(t, updatedProject.CompileConfig.ReCompile)
		require.Equal(t, uint32(5), updatedProject.ScanConfig.Concurrency)
		require.False(t, updatedProject.ScanConfig.Memory)
		require.Equal(t, []string{"sql-injection"}, updatedProject.RuleConfig.RuleFilter.RuleNames)
		require.True(t, updatedProject.ScanConfig.IgnoreLanguage)
		require.Equal(t, []string{newProjectName, "git"}, updatedProject.Tags)
		require.Equal(t, "https://github.com/test/repo.git", updatedProject.URL)
	})

	// 4. 测试删除SSA项目
	t.Run("DeleteSSAProject", func(t *testing.T) {
		req := &ypb.DeleteSSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{
				IDs: []int64{int64(projectID)},
			},
		}

		resp, err := client.DeleteSSAProject(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Message)

		t.Logf("Deleted SSA project successfully: %s", resp.Message.ExtraMessage)

		// 验证删除结果 - 项目应该不存在了
		queryReq := &ypb.QuerySSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{
				IDs: []int64{int64(projectID)},
			},
		}

		queryResp, err := client.QuerySSAProject(ctx, queryReq)
		require.NoError(t, err)
		require.Equal(t, 0, len(queryResp.Projects))
	})

	t.Run("IdempotentWhenExists", func(t *testing.T) {
		cfg, err := json.Marshal(&ssaconfig.CodeSourceInfo{
			Kind:      ssaconfig.CodeSourceLocal,
			LocalFile: filepath.Join(t.TempDir(), "idem-src"),
		})
		require.NoError(t, err)
		projectName := fmt.Sprintf("idem-%s", uuid.NewString())

		createReq := &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      projectName,
				CodeSourceConfig: string(cfg),
				Language:         "go",
			},
		}
		first, err := client.CreateSSAProject(ctx, createReq)
		require.NoError(t, err)
		require.NotZero(t, first.GetProject().GetID())

		second, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
			JSONStringConfig: first.GetProject().GetJSONStringConfig(),
		})
		require.NoError(t, err)
		require.Equal(t, first.GetProject().GetID(), second.GetProject().GetID())

		t.Cleanup(func() {
			schemaProj, err := yakit.GetSSAProjectById(uint64(first.GetProject().GetID()))
			if err == nil && schemaProj.DatabasePath != "" {
				_ = os.Remove(schemaProj.DatabasePath)
			}
			consts.GetGormProfileDatabase().Unscoped().Delete(&schema.SSAProject{}, first.GetProject().GetID())
		})
	})
}

func TestGRPCMUSTPASS_SSAProjectValidation(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("CreateWithInvalidConfig", func(t *testing.T) {
		// 测试无效的配置 - 缺少required字段
		invalidConfig := &ssaconfig.CodeSourceInfo{
			Kind: ssaconfig.CodeSourceLocal,
			// 缺少 LocalFile
		}

		configBytes, err := json.Marshal(invalidConfig)
		require.NoError(t, err)

		req := &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      fmt.Sprintf("invalid-project-%v", uuid.NewString()),
				CodeSourceConfig: string(configBytes),
				Language:         "go",
			},
		}

		_, err = client.CreateSSAProject(ctx, req)
		require.Error(t, err)
		t.Logf("Expected validation error: %v", err)
	})

	t.Run("CreateWithUnsupportedKind", func(t *testing.T) {
		// 测试不支持的源码类型
		unsupportedConfig := `{"kind":"unsupported","local_file":"/tmp/test"}`
		unsupportedConfigBytes, err := json.Marshal(unsupportedConfig)
		require.NoError(t, err)

		req := &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      fmt.Sprintf("unsupported-project-%v", uuid.NewString()),
				CodeSourceConfig: string(unsupportedConfigBytes),
				Language:         "go",
			},
		}

		_, err = client.CreateSSAProject(ctx, req)
		require.Error(t, err)
		t.Logf("Expected unsupported kind error: %v", err)
	})
}

func TestGRPCMUSTPASS_SSAProjectDifferentSourceTypes(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()
	tmpDir := t.TempDir()

	testCases := []struct {
		name   string
		config *ssaconfig.CodeSourceInfo
	}{
		{
			name: "CompressionSource",
			config: &ssaconfig.CodeSourceInfo{
				Kind:      ssaconfig.CodeSourceCompression,
				LocalFile: filepath.Join(tmpDir, "test.zip"),
			},
		},
		{
			name: "JarSource",
			config: &ssaconfig.CodeSourceInfo{
				Kind: ssaconfig.CodeSourceJar,
				URL:  "https://repo1.maven.org/maven2/org/example/example/1.0.0/example-1.0.0.jar",
			},
		},
		{
			name: "GitSourceWithBranch",
			config: &ssaconfig.CodeSourceInfo{
				Kind:   ssaconfig.CodeSourceGit,
				URL:    "https://github.com/example/repo.git",
				Branch: "develop",
				Path:   "src",
				Auth: &ssaconfig.AuthConfigInfo{
					Kind:     "password",
					UserName: "user",
					Password: "token",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configBytes, err := json.Marshal(tc.config)
			require.NoError(t, err)

			req := &ypb.CreateSSAProjectRequest{
				Project: &ypb.SSAProject{
					ProjectName:      fmt.Sprintf("%s-project-%v", tc.name, uuid.NewString()),
					CodeSourceConfig: string(configBytes),
					Description:      "测试" + tc.name,
					Language:         "go",
				},
			}

			resp, err := client.CreateSSAProject(ctx, req)
			require.NoError(t, err)
			require.NotNil(t, resp.Message)

			t.Logf("Successfully created %s project", tc.name)

			// 清理：删除创建的项目
			defer func(projectName string) {
				// 查询项目ID
				queryReq := &ypb.QuerySSAProjectRequest{
					Filter: &ypb.SSAProjectFilter{
						ProjectNames: []string{projectName},
					},
				}
				queryResp, err := client.QuerySSAProject(ctx, queryReq)
				if err == nil && len(queryResp.Projects) > 0 {
					deleteReq := &ypb.DeleteSSAProjectRequest{
						Filter: &ypb.SSAProjectFilter{
							IDs: []int64{queryResp.Projects[0].ID},
						},
					}
					client.DeleteSSAProject(ctx, deleteReq)
				}
			}(req.Project.ProjectName)
		})
	}
}

func TestGRPCMUSTPASS_SSAProjectMigrateSSAProject(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()
	migrateLocalFile := filepath.Join(t.TempDir(), "migrate-src")

	// 1. 准备测试数据：创建3个没有 projectId 的 IrProgram
	testPrograms := []struct {
		name        string
		code        string
		language    ssaconfig.Language
		description string
		configInput string
	}{
		{
			name:        uuid.NewString(),
			code:        `println("test1")`,
			language:    ssaconfig.Yak,
			description: "测试程序1",
			configInput: "",
		},
		{
			name:        uuid.NewString(),
			code:        `println("test2")`,
			language:    ssaconfig.Yak,
			description: "测试程序2 - 带配置",
			configInput: func() string {
				config, _ := ssaconfig.New(ssaconfig.ModeCodeSource, ssaconfig.WithCodeSourceInfo(&ssaconfig.CodeSourceInfo{
					Kind:      ssaconfig.CodeSourceLocal,
					LocalFile: migrateLocalFile,
				}))
				str, _ := config.ToJSONString()
				return str
			}(),
		},
		{
			name:        uuid.NewString(),
			code:        `println("test3")`,
			language:    ssaconfig.Yak,
			description: "测试程序3 - Git配置",
			configInput: func() string {
				config, _ := ssaconfig.New(ssaconfig.ModeCodeSource, ssaconfig.WithCodeSourceInfo(&ssaconfig.CodeSourceInfo{
					Kind:   ssaconfig.CodeSourceGit,
					URL:    "https://github.com/test/repo.git",
					Branch: "main",
				}))
				str, _ := config.ToJSONString()
				return str
			}(),
		},
	}

	// 创建程序但不指定 projectId（模拟旧数据）
	ssaDB := consts.GetGormSSAProjectDataBase()
	createdProgramNames := make([]string, 0)

	for _, tp := range testPrograms {
		t.Logf("创建测试程序: %s", tp.name)

		// 使用 ssaapi.Parse 创建程序，只指定 programName，不指定 projectId
		prog, err := ssaapi.Parse(tp.code,
			ssaapi.WithProgramName(tp.name),
			ssaapi.WithLanguage(tp.language),
			ssaapi.WithProgramDescription(tp.description),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		// 如果有 ConfigInput，手动更新到数据库（模拟旧数据情况）
		if tp.configInput != "" {
			err = ssaDB.Model(&ssadb.IrProgram{}).
				Where("program_name = ?", tp.name).
				Update("config_input", tp.configInput).Error
			require.NoError(t, err)
		}

		// 验证创建的程序没有 projectId
		irProg, err := yakit.GetSSAProgramByName(ssaDB, tp.name)
		require.NoError(t, err)
		require.Equal(t, uint64(0), irProg.ProjectID, "新创建的程序应该没有 project_id")

		createdProgramNames = append(createdProgramNames, tp.name)
	}

	// 确保测试后清理数据
	defer func() {
		for _, name := range createdProgramNames {
			ssadb.DeleteProgram(ssaDB, name)
		}
		// 清理创建的 SSAProject
		profileDB := consts.GetGormProfileDatabase()
		for _, tp := range testPrograms {
			profileDB.Where("project_name = ?", tp.name).Unscoped().Delete(&schema.SSAProject{})
		}
	}()

	// 2. 执行迁移操作
	t.Log("开始执行数据迁移...")

	stream, err := client.MigrateSSAProject(ctx, &ypb.MigrateSSAProjectRequest{})
	require.NoError(t, err)

	// 收集所有的流式响应
	messages := make([]string, 0)
	var finalPercent float64

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		t.Logf("进度: %.2f%% - %s", resp.Percent, resp.Message)
		messages = append(messages, resp.Message)
		finalPercent = resp.Percent
	}

	// 验证迁移完成
	require.Equal(t, float64(1), finalPercent, "迁移进度应该达到100%")

	// 3. 验证迁移结果
	t.Log("验证迁移结果...")
	time.Sleep(1 * time.Second)
	for i, tp := range testPrograms {
		t.Run("验证程序_"+tp.name, func(t *testing.T) {
			// 检查 IrProgram 的 projectId 是否已更新
			irProg, err := yakit.GetSSAProgramByName(ssaDB, tp.name)
			require.NoError(t, err)
			require.NotEqual(t, uint64(0), irProg.ProjectID, "迁移后程序应该有 project_id")

			t.Logf("程序 %s 的 project_id: %d", tp.name, irProg.ProjectID)

			// 查询对应的 SSAProject
			queryReq := &ypb.QuerySSAProjectRequest{
				Filter: &ypb.SSAProjectFilter{
					IDs: []int64{int64(irProg.ProjectID)},
				},
			}
			queryResp, err := client.QuerySSAProject(ctx, queryReq)
			require.NoError(t, err)
			require.Equal(t, 1, len(queryResp.Projects), "应该能找到对应的 SSAProject")

			project := queryResp.Projects[0]

			// 验证基本信息
			require.Equal(t, tp.name, project.ProjectName, "项目名称应该匹配")
			require.Equal(t, string(tp.language), project.Language, "语言应该匹配")
			require.Equal(t, tp.description, project.Description, "描述应该匹配")

			t.Logf("✓ 程序 %d 验证通过: ID=%d, Name=%s", i+1, project.ID, project.ProjectName)
		})
	}

}

func getDefaultSSAProfileProjectID(t *testing.T, profileDB *gorm.DB) int64 {
	t.Helper()
	var proj schema.Project
	err := profileDB.Where("type = ? AND project_name = ?", yakit.TypeSSAProject, yakit.INIT_DATABASE_RECORD_NAME).First(&proj).Error
	require.NoError(t, err)
	return int64(proj.ID)
}

func switchToDedicatedSSAProfile(t *testing.T, client ypb.YakClient, ctx context.Context, profileDB *gorm.DB) func() {
	t.Helper()
	name := fmt.Sprintf("dedicated-profile-%s", uuid.NewString())
	resp, err := client.NewProject(ctx, &ypb.NewProjectRequest{
		ProjectName: name,
		Type:        yakit.TypeSSAProject,
	})
	require.NoError(t, err)
	_, err = client.SetCurrentProject(ctx, &ypb.SetCurrentProjectRequest{
		Id:   resp.GetId(),
		Type: yakit.TypeSSAProject,
	})
	require.NoError(t, err)
	defaultID := getDefaultSSAProfileProjectID(t, profileDB)
	return func() {
		_, _ = client.SetCurrentProject(ctx, &ypb.SetCurrentProjectRequest{
			Id:   defaultID,
			Type: yakit.TypeSSAProject,
		})
	}
}

func setCurrentSSAProfileByID(t *testing.T, client ypb.YakClient, ctx context.Context, id int64) {
	t.Helper()
	_, err := client.SetCurrentProject(ctx, &ypb.SetCurrentProjectRequest{
		Id:   id,
		Type: yakit.TypeSSAProject,
	})
	require.NoError(t, err)
}

// TestGRPCMUSTPASS_SSAProjectDedicatedDatabase covers per-project SSA IR sqlite files:
// cache/switch, multi-db read merge, delete modes, and legacy projects on default DB.
func TestGRPCMUSTPASS_SSAProjectDedicatedDatabase(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	ctx := context.Background()
	tmpDir := t.TempDir()
	profileDB := consts.GetGormProfileDatabase()
	t.Cleanup(switchToDedicatedSSAProfile(t, client, ctx, profileDB))

	marshalLocalConfig := func(subdir string) []byte {
		cfg, err := json.Marshal(&ssaconfig.CodeSourceInfo{
			Kind:      ssaconfig.CodeSourceLocal,
			LocalFile: filepath.Join(tmpDir, subdir),
		})
		require.NoError(t, err)
		return cfg
	}

	createDedicatedProject := func(t *testing.T, namePrefix string) (projectID int64, dbPath string, configBytes []byte) {
		t.Helper()
		configBytes = marshalLocalConfig(namePrefix)
		resp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      fmt.Sprintf("%s-%s", namePrefix, uuid.NewString()),
				CodeSourceConfig: string(configBytes),
				Language:         "go",
			},
		})
		require.NoError(t, err)
		id := resp.GetProject().GetID()
		path := resp.GetProject().GetDatabasePath()
		require.NotEmpty(t, path)
		_, err = os.Stat(path)
		require.NoError(t, err)
		return id, path, configBytes
	}

	openSSAProjectViaGRPC := func(projectID int64) {
		_, err := client.QuerySSAPrograms(ctx, &ypb.QuerySSAProgramRequest{
			Filter:     &ypb.SSAProgramFilter{ProjectIds: []uint64{uint64(projectID)}},
			Pagination: &ypb.Paging{Page: 1, Limit: 1},
		})
		require.NoError(t, err)
	}

	requireSSADBCacheConnected := func(t *testing.T, dbPath string) *gorm.DB {
		t.Helper()
		db, err := consts.GetOrOpenSSADB(dbPath)
		require.NoError(t, err)
		require.NotNil(t, db.DB())
		require.NoError(t, db.DB().Ping())
		return db
	}

	cleanupDedicatedProject := func(projectID int64, dbPath string, programNames ...string) {
		if projectID > 0 {
			_ = yakit.EnsureSSAProjectDatabaseOpen(uint64(projectID))
			for _, name := range programNames {
				if name != "" {
					ssadb.DeleteProgram(consts.GetGormSSAProjectDataBase(), name)
				}
			}
		}
		if dbPath != "" {
			_ = consts.CloseSSADBPath(dbPath)
			_ = os.Remove(dbPath)
		}
		profileDB.Unscoped().Delete(&schema.SSAProject{}, projectID)
	}

	t.Run("DatabaseBinding", func(t *testing.T) {
		projectIDA, pathA, _ := createDedicatedProject(t, "bind-a")
		projectIDB, pathB, _ := createDedicatedProject(t, "bind-b")
		t.Cleanup(func() {
			cleanupDedicatedProject(projectIDA, pathA)
			cleanupDedicatedProject(projectIDB, pathB)
		})

		openSSAProjectViaGRPC(projectIDA)
		dbA := requireSSADBCacheConnected(t, pathA)

		openSSAProjectViaGRPC(projectIDB)
		dbB := requireSSADBCacheConnected(t, pathB)

		dbAAfterB, err := consts.GetOrOpenSSADB(pathA)
		require.NoError(t, err)
		require.Same(t, dbA, dbAAfterB)
		require.NoError(t, dbAAfterB.DB().Ping())

		_, err = consts.GetOrOpenSSADB(pathB)
		require.NoError(t, err)
		require.Same(t, dbB, consts.GetGormSSAProjectDataBase())
	})

	t.Run("MultiDBReadMerge", func(t *testing.T) {
		configBytes := marshalLocalConfig("merge-src")
		projectName := fmt.Sprintf("multi-db-%s", uuid.NewString())
		legacyProgramName := projectName
		dedicatedProgramName := fmt.Sprintf("dedicated-prog-%s", uuid.NewString())

		require.NoError(t, yakit.EnsureSSAProjectDatabaseOpen(0))
		_, err := ssaapi.Parse(`package main; func legacyMain() {}`,
			ssaapi.WithProgramName(legacyProgramName),
			ssaapi.WithLanguage(ssaconfig.GO),
			ssaconfig.WithJsonRawConfig(configBytes),
		)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = yakit.EnsureSSAProjectDatabaseOpen(0)
			ssadb.DeleteProgram(consts.GetGormSSAProjectDataBase(), legacyProgramName)
		})

		createResp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      projectName,
				CodeSourceConfig: string(configBytes),
				Description:      "multi-db read merge",
				Language:         "go",
			},
		})
		require.NoError(t, err)
		projectID := createResp.GetProject().GetID()
		dbPath := createResp.GetProject().GetDatabasePath()
		t.Cleanup(func() {
			cleanupDedicatedProject(projectID, dbPath, dedicatedProgramName)
		})

		require.NoError(t, yakit.EnsureSSAProjectDatabaseOpen(uint64(projectID)))
		_, err = ssaapi.Parse(`package main; func dedicatedMain() {}`,
			ssaapi.WithProgramName(dedicatedProgramName),
			ssaapi.WithLanguage(ssaconfig.GO),
			ssaconfig.WithProjectID(uint64(projectID)),
		)
		require.NoError(t, err)

		progResp, err := client.QuerySSAPrograms(ctx, &ypb.QuerySSAProgramRequest{
			Filter:     &ypb.SSAProgramFilter{ProjectIds: []uint64{uint64(projectID)}},
			Pagination: &ypb.Paging{Page: 1, Limit: 100},
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(progResp.Data), 2)

		names := make(map[string]struct{}, len(progResp.Data))
		for _, p := range progResp.Data {
			names[p.Name] = struct{}{}
		}
		require.Contains(t, names, legacyProgramName)
		require.Contains(t, names, dedicatedProgramName)
	})

	t.Run("DeleteClosesDatabase", func(t *testing.T) {
		projectID, dbPath, _ := createDedicatedProject(t, "del")
		progName := fmt.Sprintf("del-prog-%s", uuid.NewString())
		t.Cleanup(func() {
			cleanupDedicatedProject(projectID, dbPath)
		})

		_, err := ssaapi.Parse(`package main; func main() {}`,
			ssaapi.WithProgramName(progName),
			ssaapi.WithLanguage(ssaconfig.GO),
			ssaconfig.WithProjectID(uint64(projectID)),
		)
		require.NoError(t, err)

		_, err = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
			Filter:     &ypb.SSAProjectFilter{IDs: []int64{projectID}},
			DeleteMode: string(yakit.SSAProjectClearCompileHistory),
		})
		require.NoError(t, err)
		_, err = os.Stat(dbPath)
		require.NoError(t, err, "dedicated sqlite should remain after clear_compile_history")

		progResp, err := client.QuerySSAPrograms(ctx, &ypb.QuerySSAProgramRequest{
			Filter:     &ypb.SSAProgramFilter{ProjectIds: []uint64{uint64(projectID)}},
			Pagination: &ypb.Paging{Page: 1, Limit: 10},
		})
		require.NoError(t, err)
		require.Empty(t, progResp.Data)

		_, err = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
			Filter:     &ypb.SSAProjectFilter{IDs: []int64{projectID}},
			DeleteMode: string(yakit.SSAProjectDeleteAll),
		})
		require.NoError(t, err)
		_, err = os.Stat(dbPath)
		require.True(t, os.IsNotExist(err), "dedicated sqlite should be removed after delete_all")

		queryResp, err := client.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{IDs: []int64{projectID}},
		})
		require.NoError(t, err)
		require.Empty(t, queryResp.Projects)
		require.Equal(t, uint64(0), yakit.GetCurrentSSAProjectID())
	})

	t.Run("LegacyOpenUsesDefaultDB", func(t *testing.T) {
		legacyURL := filepath.Join(tmpDir, "legacy")
		project := &schema.SSAProject{
			ProjectName: fmt.Sprintf("legacy-%s", uuid.NewString()),
			Language:    ssaconfig.GO,
			Description: "legacy without dedicated db",
			URL:         legacyURL,
		}
		require.NoError(t, profileDB.Create(project).Error)
		require.Empty(t, project.DatabasePath)
		t.Cleanup(func() {
			profileDB.Unscoped().Delete(project)
		})

		require.NoError(t, yakit.OpenSSAProjectDatabase(project))
		_, defaultPath := consts.GetSSADataBaseInfo()
		require.Equal(t, defaultPath, yakit.ResolveSSAProjectDatabasePath(project))
	})

	t.Cleanup(func() {
		_ = yakit.EnsureSSAProjectDatabaseOpen(0)
	})
}

// TestGRPCMUSTPASS_SSAProjectSharedProfileScope verifies list/create scoping for default vs dedicated profiles.
func TestGRPCMUSTPASS_SSAProjectSharedProfileScope(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	ctx := context.Background()
	profileDB := consts.GetGormProfileDatabase()
	tmpDir := t.TempDir()

	defaultProfileID := getDefaultSSAProfileProjectID(t, profileDB)
	t.Cleanup(func() {
		setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)
	})

	marshalLocalConfig := func(subdir string) []byte {
		cfg, err := json.Marshal(&ssaconfig.CodeSourceInfo{
			Kind:      ssaconfig.CodeSourceLocal,
			LocalFile: filepath.Join(tmpDir, subdir),
		})
		require.NoError(t, err)
		return cfg
	}

	setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)

	sharedName := fmt.Sprintf("shared-scope-%s", uuid.NewString())
	sharedCfg := marshalLocalConfig("shared")
	sharedResp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
		Project: &ypb.SSAProject{
			ProjectName:      sharedName,
			CodeSourceConfig: string(sharedCfg),
			Language:         "go",
		},
	})
	require.NoError(t, err)
	sharedID := sharedResp.GetProject().GetID()
	require.Empty(t, sharedResp.GetProject().GetDatabasePath())

	listBeforeCompile, err := client.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{
		Filter:     &ypb.SSAProjectFilter{ProjectNames: []string{sharedName}},
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	require.NotEmpty(t, listBeforeCompile.Projects, "uncompiled shared project should appear in default profile list")

	progName := fmt.Sprintf("shared-prog-%s", uuid.NewString())
	require.NoError(t, yakit.EnsureSSAProjectDatabaseOpen(0))
	_, err = ssaapi.Parse(`package main; func main() {}`,
		ssaapi.WithProgramName(progName),
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaconfig.WithProjectID(uint64(sharedID)),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		ssadb.DeleteProgram(consts.GetGormSSAProjectDataBase(), progName)
		_, _ = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{IDs: []int64{sharedID}},
		})
	})

	restoreDedicatedProfile := switchToDedicatedSSAProfile(t, client, ctx, profileDB)
	dedicatedName := fmt.Sprintf("dedicated-scope-%s", uuid.NewString())
	dedicatedCfg := marshalLocalConfig("dedicated")
	dedicatedResp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
		Project: &ypb.SSAProject{
			ProjectName:      dedicatedName,
			CodeSourceConfig: string(dedicatedCfg),
			Language:         "go",
		},
	})
	require.NoError(t, err)
	dedicatedID := dedicatedResp.GetProject().GetID()
	dedicatedPath := dedicatedResp.GetProject().GetDatabasePath()
	require.NotEmpty(t, dedicatedPath)
	t.Cleanup(func() {
		restoreDedicatedProfile()
		_, _ = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{IDs: []int64{dedicatedID}},
		})
		_ = os.Remove(dedicatedPath)
	})

	setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)
	listDefault, err := client.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{
		Filter:     &ypb.SSAProjectFilter{ProjectNames: []string{sharedName, dedicatedName}},
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	defaultNames := make(map[string]struct{})
	for _, p := range listDefault.Projects {
		defaultNames[p.ProjectName] = struct{}{}
	}
	require.Contains(t, defaultNames, sharedName)
	require.NotContains(t, defaultNames, dedicatedName)

	switchToDedicatedSSAProfile(t, client, ctx, profileDB)
	listDedicated, err := client.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{
		Filter:     &ypb.SSAProjectFilter{ProjectNames: []string{sharedName, dedicatedName}},
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	dedicatedNames := make(map[string]struct{})
	for _, p := range listDedicated.Projects {
		dedicatedNames[p.ProjectName] = struct{}{}
	}
	require.Contains(t, dedicatedNames, dedicatedName)
	require.NotContains(t, dedicatedNames, sharedName)
}

// TestGRPCMUSTPASS_SSAProjectListPool verifies SHARED vs DEDICATED list filters are mutually exclusive.
func TestGRPCMUSTPASS_SSAProjectListPool(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	ctx := context.Background()
	tmpDir := t.TempDir()
	profileDB := consts.GetGormProfileDatabase()

	defaultProfileID := getDefaultSSAProfileProjectID(t, profileDB)
	t.Cleanup(func() {
		setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)
	})

	marshalLocalConfig := func(subdir string) []byte {
		cfg, err := json.Marshal(&ssaconfig.CodeSourceInfo{
			Kind:      ssaconfig.CodeSourceLocal,
			LocalFile: filepath.Join(tmpDir, subdir),
		})
		require.NoError(t, err)
		return cfg
	}

	setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)

	sharedName := fmt.Sprintf("listpool-shared-%s", uuid.NewString())
	sharedResp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
		Project: &ypb.SSAProject{
			ProjectName:      sharedName,
			CodeSourceConfig: string(marshalLocalConfig("shared")),
			Language:         "go",
		},
		DatabaseBindMode: ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_SHARED,
	})
	require.NoError(t, err)
	sharedID := sharedResp.GetProject().GetID()
	t.Cleanup(func() {
		_, _ = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{IDs: []int64{sharedID}},
		})
	})

	dedicatedName := fmt.Sprintf("listpool-dedicated-%s", uuid.NewString())
	dedicatedResp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
		Project: &ypb.SSAProject{
			ProjectName:      dedicatedName,
			CodeSourceConfig: string(marshalLocalConfig("dedicated")),
			Language:         "go",
		},
		DatabaseBindMode: ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_DEDICATED,
	})
	require.NoError(t, err)
	dedicatedID := dedicatedResp.GetProject().GetID()
	dedicatedPath := dedicatedResp.GetProject().GetDatabasePath()
	require.NotEmpty(t, dedicatedPath)
	t.Cleanup(func() {
		_, _ = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
			Filter:     &ypb.SSAProjectFilter{IDs: []int64{dedicatedID}},
			DeleteMode: string(yakit.SSAProjectDeleteAll),
		})
		_ = os.Remove(dedicatedPath)
	})

	queryNames := func(pool ypb.SSAProjectListPool) map[string]struct{} {
		resp, err := client.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{
				ProjectNames: []string{sharedName, dedicatedName},
				ListPool:     pool,
			},
			Pagination: &ypb.Paging{Page: 1, Limit: 50},
		})
		require.NoError(t, err)
		names := make(map[string]struct{})
		for _, p := range resp.Projects {
			names[p.ProjectName] = struct{}{}
		}
		return names
	}

	sharedNames := queryNames(ypb.SSAProjectListPool_SSA_PROJECT_LIST_SHARED)
	require.Contains(t, sharedNames, sharedName)
	require.NotContains(t, sharedNames, dedicatedName)

	dedicatedNames := queryNames(ypb.SSAProjectListPool_SSA_PROJECT_LIST_DEDICATED)
	require.Contains(t, dedicatedNames, dedicatedName)
	require.NotContains(t, dedicatedNames, sharedName)

	// ListPool takes precedence over profile: on default profile, DEDICATED still lists dedicated rows.
	setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)
	dedicatedOnDefault := queryNames(ypb.SSAProjectListPool_SSA_PROJECT_LIST_DEDICATED)
	require.Contains(t, dedicatedOnDefault, dedicatedName)

	// SHARED create must appear in SHARED list (internal audit projects).
	sharedOnly, err := client.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{
		Filter: &ypb.SSAProjectFilter{
			ProjectNames: []string{sharedName},
			ListPool:     ypb.SSAProjectListPool_SSA_PROJECT_LIST_SHARED,
		},
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	require.Len(t, sharedOnly.Projects, 1)
	require.Empty(t, sharedOnly.Projects[0].GetDatabasePath())
}

// TestGRPCMUSTPASS_SSAProjectSameNameDualPool allows the same project name in shared and dedicated pools.
func TestGRPCMUSTPASS_SSAProjectSameNameDualPool(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	ctx := context.Background()
	tmpDir := t.TempDir()
	defaultProfileID := getDefaultSSAProfileProjectID(t, consts.GetGormProfileDatabase())
	t.Cleanup(func() {
		setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)
	})

	marshalLocalConfig := func(subdir string) []byte {
		cfg, err := json.Marshal(&ssaconfig.CodeSourceInfo{
			Kind:      ssaconfig.CodeSourceLocal,
			LocalFile: filepath.Join(tmpDir, subdir),
		})
		require.NoError(t, err)
		return cfg
	}

	setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)
	name := fmt.Sprintf("dual-pool-%s", uuid.NewString())

	sharedResp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
		Project: &ypb.SSAProject{
			ProjectName:      name,
			CodeSourceConfig: string(marshalLocalConfig("same")),
			Language:         "go",
		},
		DatabaseBindMode: ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_SHARED,
	})
	require.NoError(t, err)
	sharedID := sharedResp.GetProject().GetID()
	require.Empty(t, sharedResp.GetProject().GetDatabasePath())
	t.Cleanup(func() {
		_, _ = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{IDs: []int64{sharedID}},
		})
	})

	dedicatedResp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
		Project: &ypb.SSAProject{
			ProjectName:      name,
			CodeSourceConfig: string(marshalLocalConfig("same")),
			Language:         "go",
		},
		DatabaseBindMode: ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_DEDICATED,
	})
	require.NoError(t, err)
	dedicatedID := dedicatedResp.GetProject().GetID()
	require.NotEqual(t, sharedID, dedicatedID)
	require.NotEmpty(t, dedicatedResp.GetProject().GetDatabasePath())
	t.Cleanup(func() {
		_, _ = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
			Filter:     &ypb.SSAProjectFilter{IDs: []int64{dedicatedID}},
			DeleteMode: string(yakit.SSAProjectDeleteAll),
		})
	})

	listShared, err := client.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{
		Filter:     &ypb.SSAProjectFilter{ProjectNames: []string{name}, ListPool: ypb.SSAProjectListPool_SSA_PROJECT_LIST_SHARED},
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	require.Len(t, listShared.Projects, 1)
	require.Equal(t, sharedID, listShared.Projects[0].ID)

	listDedicated, err := client.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{
		Filter:     &ypb.SSAProjectFilter{ProjectNames: []string{name}, ListPool: ypb.SSAProjectListPool_SSA_PROJECT_LIST_DEDICATED},
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	require.Len(t, listDedicated.Projects, 1)
	require.Equal(t, dedicatedID, listDedicated.Projects[0].ID)
}

// TestGRPCMUSTPASS_SSAProjectCreateDatabaseBindMode verifies explicit bind modes on CreateSSAProject.
func TestGRPCMUSTPASS_SSAProjectCreateDatabaseBindMode(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	ctx := context.Background()
	tmpDir := t.TempDir()
	profileDB := consts.GetGormProfileDatabase()

	defaultProfileID := getDefaultSSAProfileProjectID(t, profileDB)
	t.Cleanup(func() {
		setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)
	})

	marshalLocalConfig := func(subdir string) []byte {
		cfg, err := json.Marshal(&ssaconfig.CodeSourceInfo{
			Kind:      ssaconfig.CodeSourceLocal,
			LocalFile: filepath.Join(tmpDir, subdir),
		})
		require.NoError(t, err)
		return cfg
	}

	setCurrentSSAProfileByID(t, client, ctx, defaultProfileID)

	t.Run("DefaultProfileForceDedicated", func(t *testing.T) {
		name := fmt.Sprintf("bind-dedicated-%s", uuid.NewString())
		cfg := marshalLocalConfig("dedicated")
		resp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      name,
				CodeSourceConfig: string(cfg),
				Language:         "go",
			},
			DatabaseBindMode: ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_DEDICATED,
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.GetProject().GetDatabasePath())
		t.Cleanup(func() {
			_, _ = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
				Filter:     &ypb.SSAProjectFilter{IDs: []int64{resp.GetProject().GetID()}},
				DeleteMode: string(yakit.SSAProjectDeleteAll),
			})
		})
	})

	t.Run("DefaultProfileForceShared", func(t *testing.T) {
		name := fmt.Sprintf("bind-shared-%s", uuid.NewString())
		cfg := marshalLocalConfig("shared")
		resp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      name,
				CodeSourceConfig: string(cfg),
				Language:         "go",
			},
			DatabaseBindMode: ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_SHARED,
		})
		require.NoError(t, err)
		require.Empty(t, resp.GetProject().GetDatabasePath())
		t.Cleanup(func() {
			_, _ = client.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{
				Filter:     &ypb.SSAProjectFilter{IDs: []int64{resp.GetProject().GetID()}},
				DeleteMode: string(yakit.SSAProjectDeleteAll),
			})
		})
	})

	t.Run("DedicatedProfileForceSharedFails", func(t *testing.T) {
		restore := switchToDedicatedSSAProfile(t, client, ctx, profileDB)
		defer restore()

		name := fmt.Sprintf("bind-shared-fail-%s", uuid.NewString())
		cfg := marshalLocalConfig("fail")
		_, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      name,
				CodeSourceConfig: string(cfg),
				Language:         "go",
			},
			DatabaseBindMode: ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_SHARED,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not default or temporary")
	})
}
