package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSSAProjectCRUDOperations(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()

	// 测试用的代码源配置
	codeSourceConfig := &schema.CodeSourceInfo{
		Kind:      schema.CodeSourceLocal,
		LocalFile: "/tmp/test-project",
		Auth: &schema.AuthConfigInfo{
			Kind:     "password",
			UserName: "test",
			Password: "test123",
		},
		Proxy: &schema.ProxyConfigInfo{
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
		require.True(t, project.CompileConfig.StrictMode)
		require.Equal(t, int64(200), project.CompileConfig.PeepholeSize)
		require.Equal(t, []string{"*.test.go", "*.mock.go"}, project.CompileConfig.ExcludeFiles)
		require.False(t, project.CompileConfig.ReCompile)
		require.Equal(t, uint32(10), project.ScanConfig.Concurrency)
		require.True(t, project.ScanConfig.Memory)
		require.Equal(t, []string{"sql-injection", "xss"}, project.RuleConfig.RuleFilter.RuleNames)
		require.False(t, project.ScanConfig.IgnoreLanguage)
		require.Equal(t, []string{"test", "local"}, project.Tags)

		// 保存项目ID用于后续测试
		projectID = uint64(project.ID)

		t.Logf("Found SSA project: ID=%d, Name=%s", project.ID, project.ProjectName)
	})

	// 3. 测试更新SSA项目
	t.Run("UpdateSSAProject", func(t *testing.T) {
		// 修改代码源配置为Git类型
		gitConfig := &schema.CodeSourceInfo{
			Kind:   schema.CodeSourceGit,
			URL:    "https://github.com/test/repo.git",
			Branch: "main",
			Path:   "src/main",
			Auth: &schema.AuthConfigInfo{
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
		require.False(t, updatedProject.CompileConfig.StrictMode)
		require.Equal(t, int64(300), updatedProject.CompileConfig.PeepholeSize)
		require.Equal(t, []string{"*.test.go"}, updatedProject.CompileConfig.ExcludeFiles)
		require.True(t, updatedProject.CompileConfig.ReCompile)
		require.Equal(t, uint32(5), updatedProject.ScanConfig.Concurrency)
		require.False(t, updatedProject.ScanConfig.Memory)
		require.Equal(t, []string{"sql-injection"}, updatedProject.RuleConfig.RuleFilter.RuleNames)
		require.True(t, updatedProject.ScanConfig.IgnoreLanguage)
		require.Equal(t, []string{newProjectName, "git"}, updatedProject.Tags)
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

func TestSSAProjectValidation(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("CreateWithInvalidConfig", func(t *testing.T) {
		// 测试无效的配置 - 缺少required字段
		invalidConfig := &schema.CodeSourceInfo{
			Kind: schema.CodeSourceLocal,
			// 缺少 LocalFile
		}

		configBytes, err := json.Marshal(invalidConfig)
		require.NoError(t, err)

		req := &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      fmt.Sprintf("invalid-project-%v", uuid.NewString()),
				CodeSourceConfig: string(configBytes),
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
			},
		}

		_, err = client.CreateSSAProject(ctx, req)
		require.Error(t, err)
		t.Logf("Expected unsupported kind error: %v", err)
	})
}

func TestSSAProjectDifferentSourceTypes(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()

	testCases := []struct {
		name   string
		config *schema.CodeSourceInfo
	}{
		{
			name: "CompressionSource",
			config: &schema.CodeSourceInfo{
				Kind:      schema.CodeSourceCompression,
				LocalFile: "/tmp/test.zip",
			},
		},
		{
			name: "JarSource",
			config: &schema.CodeSourceInfo{
				Kind: schema.CodeSourceJar,
				URL:  "https://repo1.maven.org/maven2/org/example/example/1.0.0/example-1.0.0.jar",
			},
		},
		{
			name: "GitSourceWithBranch",
			config: &schema.CodeSourceInfo{
				Kind:   schema.CodeSourceGit,
				URL:    "https://github.com/example/repo.git",
				Branch: "develop",
				Path:   "src",
				Auth: &schema.AuthConfigInfo{
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
