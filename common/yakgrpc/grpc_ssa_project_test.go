package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SSAProjectCRUDOperations(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()

	// 测试用的代码源配置
	codeSourceConfig := &ssaconfig.CodeSourceInfo{
		Kind:      ssaconfig.CodeSourceLocal,
		LocalFile: "/tmp/test-project",
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
		require.Equal(t, "/tmp/test-project", project.URL)

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

	testCases := []struct {
		name   string
		config *ssaconfig.CodeSourceInfo
	}{
		{
			name: "CompressionSource",
			config: &ssaconfig.CodeSourceInfo{
				Kind:      ssaconfig.CodeSourceCompression,
				LocalFile: "/tmp/test.zip",
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
					LocalFile: "/tmp/test2",
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
	require.Equal(t, float64(100), finalPercent, "迁移进度应该达到100%")

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
