package yakgrpc

import (
	"context"
	"encoding/json"
	"testing"

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
	t.Run("CreateSSAProject", func(t *testing.T) {
		req := &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      "test-local-project",
				CodeSourceConfig: configJSON,
				Description:      "测试本地项目",
				StrictMode:       true,
				PeepholeSize:     200,
				ExcludeFiles:     "*.test.go,*.mock.go",
				ReCompile:        false,
				ScanConcurrency:  10,
				MemoryScan:       true,
				ScanRuleGroups:   "security,performance",
				ScanRuleNames:    "sql-injection,xss",
				IgnoreLanguage:   false,
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
				ProjectNames: []string{"test-local-project"},
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
		require.Equal(t, "test-local-project", project.ProjectName)
		require.Equal(t, configJSON, project.CodeSourceConfig)
		require.Equal(t, "测试本地项目", project.Description)
		require.True(t, project.StrictMode)
		require.Equal(t, int32(200), project.PeepholeSize)

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

		req := &ypb.UpdateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ID:               int64(projectID),
				ProjectName:      "test-git-project-updated",
				CodeSourceConfig: gitConfigJSON,
				Description:      "更新为Git项目",
				StrictMode:       false,
				PeepholeSize:     300,
				ExcludeFiles:     "*.test.go",
				ReCompile:        true,
				ScanConcurrency:  5,
				MemoryScan:       false,
				ScanRuleGroups:   "security",
				ScanRuleNames:    "sql-injection",
				IgnoreLanguage:   true,
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
		require.Equal(t, "test-git-project-updated", updatedProject.ProjectName)
		require.Equal(t, gitConfigJSON, updatedProject.CodeSourceConfig)
		require.Equal(t, "更新为Git项目", updatedProject.Description)
		require.False(t, updatedProject.StrictMode)
		require.Equal(t, int32(300), updatedProject.PeepholeSize)
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
				ProjectName:      "invalid-project",
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

		req := &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName:      "unsupported-project",
				CodeSourceConfig: unsupportedConfig,
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
					ProjectName:      tc.name + "-project",
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
