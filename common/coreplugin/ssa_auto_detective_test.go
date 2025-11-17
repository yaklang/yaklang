package coreplugin

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSSAAutoDetective(t *testing.T) {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	check := func(t *testing.T, input string) (*ssaconfig.Config, error) {
		info, prog, cleanup, err := ParseProjectWithAutoDetective(context.Background(), input, "", false)
		_ = prog
		_ = cleanup
		if info == nil {
			return nil, err
		}
		return info.Config, err
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
		config, err := check(t, jarPath)
		require.NoError(t, err)
		log.Infof("config: %v", config)
		require.Equal(t, string(config.GetLanguage()), "java")
		require.NotNil(t, config.CodeSource)
		require.Equal(t, string(config.GetCodeSourceKind()), "jar")
		require.Equal(t, config.GetCodeSourceLocalFile(), jarPath)
	})

	t.Run("check zip", func(t *testing.T) {
		zipPath, err := ssatest.GetZipFile()
		require.NoError(t, err)
		config, err := check(t, zipPath)
		require.NoError(t, err)
		log.Infof("config: %v", config)
		require.Equal(t, string(config.GetLanguage()), "java")
		require.NotNil(t, config.CodeSource)
		require.Equal(t, string(config.GetCodeSourceKind()), "compression")
		require.Equal(t, config.GetCodeSourceLocalFile(), zipPath)
	})

	t.Run("check error path", func(t *testing.T) {
		dir := os.TempDir()
		// create a not exist dir
		dir = path.Join(dir, uuid.NewString(), uuid.NewString())
		_, err := check(t, dir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "file not found")
	})

	t.Run("check unsupported file ", func(t *testing.T) {
		dir := os.TempDir()
		file := path.Join(dir, "test.txt")
		err := os.WriteFile(file, []byte("test"), 0644)
		require.NoError(t, err)

		_, err = check(t, file)
		require.Error(t, err)
		require.Contains(t, err.Error(), "input file type")
	})

	t.Run("check git", func(t *testing.T) {
		url, err := ssatest.GetLocalGit()
		require.NoError(t, err)
		_, err = check(t, url)
		require.Error(t, err)
		require.Contains(t, err.Error(), "language need select")
	})

	t.Run("check un access url", func(t *testing.T) {
		_, err := check(t, "http://127.0.0.1:7777/1123/5"+uuid.NewString())
		require.Error(t, err)
		require.Contains(t, err.Error(), "connect fail")
	})
}

// setupTempDirWithJavaFile 创建临时目录和Java测试文件
func setupTempDirWithJavaFile(t *testing.T, filename, code string) (string, func()) {
	tempDir := path.Join(os.TempDir(), uuid.NewString())
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	javaFile := path.Join(tempDir, filename)
	err = os.WriteFile(javaFile, []byte(code), 0644)
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(tempDir)
	}
	return tempDir, cleanup
}

// createSSAProject 使用gRPC接口创建SSA项目
func createSSAProject(t *testing.T, config *ssaconfig.Config) (*ypb.SSAProject, func()) {
	configJSON, err := config.ToJSONString()
	require.NoError(t, err)
	log.Infof("Creating SSA project with config:\n%s", configJSON)

	client, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	req := &ypb.CreateSSAProjectRequest{
		JSONStringConfig: configJSON,
	}

	resp, err := client.CreateSSAProject(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Project)

	project := resp.Project
	log.Infof("SSA Project created successfully:")
	log.Infof("  ID: %d", project.ID)
	log.Infof("  ProjectName: %s", project.ProjectName)
	log.Infof("  Language: %s", project.Language)
	log.Infof("  Description: %s", project.Description)

	cleanup := func() {
		deleteReq := &ypb.DeleteSSAProjectRequest{
			DeleteMode: string(yakit.SSAProjectDeleteAll),
			Filter: &ypb.SSAProjectFilter{
				IDs: []int64{project.ID},
			},
		}
		_, err := client.DeleteSSAProject(context.Background(), deleteReq)
		require.NoError(t, err)
	}
	return project, cleanup
}

// TestSSAProjectComprehensive 综合测试：项目探测、创建、编译
func TestSSAProjectComprehensive(t *testing.T) {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	t.Run("create SSA project via gRPC with params raw data", func(t *testing.T) {
		// 使用辅助函数创建临时目录和测试文件
		tempDir, cleanupDir := setupTempDirWithJavaFile(t, "Main.java",
			"public class Main { public static void main(String[] args) {} }")
		defer cleanupDir()

		// 使用 SSA 探测获取 config
		info, prog, cleanup, err := ParseProjectWithAutoDetective(
			context.Background(),
			tempDir,
			"",
			false,
		)
		_ = cleanup // compileImmediately=false 时不需要清理
		require.NoError(t, err)
		require.Nil(t, prog)

		// 使用辅助函数创建 SSA 项目
		project, cleanupProject := createSSAProject(t, info.Config)
		defer cleanupProject()

		// Check
		require.Equal(t, info.GetProjectName(), project.ProjectName)
		require.Equal(t, string(info.GetLanguage()), string(project.Language))
		require.Equal(t, info.GetProjectDescription(), project.Description)
	})

	t.Run("detective with compile_immediately flag", func(t *testing.T) {
		// 测试 compile_immediately=true 时会自动创建项目并编译
		javaCode := `public class ImmediateTest {
    public static void main(String[] args) {
        System.out.println("Immediate Compile Test");
    }
}`
		tempDir, cleanupDir := setupTempDirWithJavaFile(t, "ImmediateTest.java", javaCode)
		defer cleanupDir()

		// 使用 compile_immediately=true 探测项目
		log.Infof("Starting SSA auto detective with compile_immediately=true...")
		info, prog, cleanup, err := ParseProjectWithAutoDetective(
			context.Background(),
			tempDir,
			"java",
			true, // compile_immediately - 应该自动编译
		)
		require.NoError(t, err)
		require.NotNil(t, cleanup, "cleanup function should be returned")
		defer cleanup() // 使用返回的清理函数

		require.NotNil(t, prog, "compile_immediately=true 时应该返回编译后的程序")
		require.NotEmpty(t, info.GetProgramName(), "Program name should not be empty")
		require.NotEmpty(t, info.GetProjectName(), "Project name should not be empty")

		log.Infof("Immediate compile completed:")
		log.Infof("  ProgramName: %s", info.GetProgramName())
		log.Infof("  ProjectName: %s", info.GetProjectName())
		log.Infof("  Language: %s", info.GetLanguage())
		log.Infof("  CompileImmediately: %v", info.CompileImmediately)
		log.Infof("  Program: %v", prog)

		// 验证 compile_immediately 标志
		require.True(t, info.CompileImmediately, "CompileImmediately should be true")

		// 查询项目，验证已创建并编译
		client, err := yakgrpc.NewLocalClient()
		require.NoError(t, err)

		queryReq := &ypb.QuerySSAProjectRequest{
			Filter: &ypb.SSAProjectFilter{
				ProjectNames: []string{info.GetProjectName()},
			},
		}

		queryResp, err := client.QuerySSAProject(context.Background(), queryReq)
		require.NoError(t, err)
		require.NotNil(t, queryResp)
		require.Equal(t, 1, len(queryResp.Projects), "Should find exactly one project")

		project := queryResp.Projects[0]
		log.Infof("Found SSA Project:")
		log.Infof("  ID: %d", project.ID)
		log.Infof("  ProjectName: %s", project.ProjectName)
		log.Infof("  Language: %s", project.Language)
		log.Infof("  CompileTimes: %d", project.CompileTimes)

		// 验证编译次数
		require.Equal(t, int64(1), project.CompileTimes, "CompileTimes should be 1 after immediate compilation")

		log.Infof("✅ Test passed: compile_immediately flag works correctly with automatic compilation")
	})

	t.Run("check project exists and use existing config", func(t *testing.T) {
		// 创建一个临时目录和Java文件
		tempDir, cleanupDir := setupTempDirWithJavaFile(t, "ExistsTest.java", `
public class ExistsTest {
    public static void main(String[] args) {
        System.out.println("Project exists test");
    }
}
`)
		defer cleanupDir()

		// 第一次探测，项目不应该存在
		log.Infof("First detection - project should not exist")
		info1, _, _, err := ParseProjectWithAutoDetective(context.Background(), tempDir, "java", false)
		require.NoError(t, err)
		require.NotNil(t, info1)
		require.False(t, info1.ProjectExists, "First detection: project should not exist")

		// 记录第一次探测的配置
		firstConfig := info1.Config
		require.NotNil(t, firstConfig)
		firstProjectName := firstConfig.GetProjectName()
		log.Infof("First detection: project_name=%s, project_exists=%v", firstProjectName, info1.ProjectExists)

		// 创建SSA项目
		log.Infof("Creating SSA project...")
		project, cleanupProj := createSSAProject(t, info1.Config)
		defer cleanupProj()
		log.Infof("Project created with ID: %d, Name: %s", project.ID, project.ProjectName)

		// 第二次探测，项目应该存在，并使用已有配置
		log.Infof("Second detection - project should exist and use existing config")
		info2, _, _, err := ParseProjectWithAutoDetective(context.Background(), tempDir, "java", false)
		require.NoError(t, err)
		require.NotNil(t, info2)
		require.True(t, info2.ProjectExists, "Second detection: project should exist")

		// 验证使用的是已有项目的配置
		secondConfig := info2.Config
		require.NotNil(t, secondConfig)
		require.Equal(t, project.ProjectName, secondConfig.GetProjectName(), "Should use existing project name")
		require.Equal(t, project.Language, string(secondConfig.GetLanguage()), "Should use existing project language")

		// 验证ProjectID应该被设置为已存在项目的ID
		require.Equal(t, uint64(project.ID), secondConfig.GetProjectID(), "ProjectID should be set to existing project ID")

		log.Infof("✅ Test passed: project_exists flag works correctly and uses existing config")
		log.Infof("  First detection: project_exists=false, project_name=%s", firstProjectName)
		log.Infof("  Second detection: project_exists=true, project_id=%d, project_name=%s", secondConfig.GetProjectID(), secondConfig.GetProjectName())
	})

	t.Run("detective then compile and verify compile times", func(t *testing.T) {
		javaCode := `public class HelloWorld {
    public static void main(String[] args) {
        System.out.println("Hello World");
    }
}`
		tempDir, cleanupDir := setupTempDirWithJavaFile(t, "HelloWorld.java", javaCode)
		defer cleanupDir()

		ssaDB := consts.GetGormDefaultSSADataBase()

		// Step 1: 探测项目
		log.Infof("Step 1: Starting SSA auto detective without compile...")
		info, prog, cleanup, err := ParseProjectWithAutoDetective(
			context.Background(),
			tempDir,
			"java",
			false,
		)
		_ = cleanup
		require.NoError(t, err)
		require.Nil(t, prog, "Program should not be compiled yet")
		require.NotEmpty(t, info.GetProgramName(), "Program name should not be empty")

		log.Infof("Detective completed:")
		log.Infof("  ProgramName: %s", info.GetProgramName())
		log.Infof("  ProjectName: %s", info.GetProjectName())
		log.Infof("  Language: %s", info.GetLanguage())

		// Step 2: 创建项目
		log.Infof("Step 2: Creating SSA project...")
		project, cleanupProject := createSSAProject(t, info.Config)
		defer cleanupProject()

		// Step 3: 编译项目三次
		log.Infof("Step 3: Compiling SSA project using compile script...")
		pluginName := "SSA 项目编译"

		configJSON := project.JSONStringConfig
		require.NotEmpty(t, configJSON, "Project JSON config should not be empty")

		compileParam := map[string]string{
			"config": configJSON,
		}

		// 执行三次编译
		for i := 1; i <= 3; i++ {
			log.Infof("Compiling project - attempt %d/3", i)
			err = yakgrpc.ExecScriptWithParam(context.Background(), pluginName, compileParam,
				"", func(exec *ypb.ExecResult) error {
					return nil
				},
			)
			require.NoError(t, err, "Compilation %d should succeed", i)
			time.Sleep(1 * time.Second)
			// 验证当前编译次数
			currentCompileTimes := yakit.QuerySSACompileTimesByProjectID(ssaDB, uint(project.ID))
			log.Infof("After compilation %d, CompileTimes: %d", i, currentCompileTimes)
			require.Equal(t, int64(i), currentCompileTimes, "CompileTimes should be %d after %d compilation(s)", i, i)
		}

		log.Infof("All 3 compilations completed successfully")

		// Step 4: 验证最终编译结果
		irProgram, err := ssadb.GetProgramByProjectID(uint64(project.ID))
		require.NoError(t, err, "Should find the compiled IrProgram by projectID")

		log.Infof("Found IrProgram:")
		log.Infof("  ID: %d", irProgram.ID)
		log.Infof("  ProgramName: %s", irProgram.ProgramName)
		log.Infof("  Language: %s", irProgram.Language)
		log.Infof("  ProjectID: %d", irProgram.ProjectID)

		require.Equal(t, uint64(project.ID), irProgram.ProjectID, "ProjectID should match")
		require.Equal(t, string(info.GetLanguage()), string(irProgram.Language))
		require.Contains(t, irProgram.ProgramName, info.GetProjectName(), "ProgramName should contain projectName")

		// 最终验证编译次数为3
		finalCompileTimes := yakit.QuerySSACompileTimesByProjectID(ssaDB, uint(project.ID))
		log.Infof("Final CompileTimes: %d", finalCompileTimes)
		require.Equal(t, int64(3), finalCompileTimes, "CompileTimes should be 3 after three compilations")

		log.Infof("✅ Test passed: SSA project compiled successfully 3 times with CompileTimes = 3")
	})
}
