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
		info, prog, err := ParseProjectWithAutoDetective(context.Background(), input, "", true)
		_ = prog
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
		info, prog, err := ParseProjectWithAutoDetective(
			context.Background(),
			tempDir,
			"",
			false,
		)
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

	t.Run("detective with immediate compile", func(t *testing.T) {
		// 使用项目探测立即编译的项目应该也有一次编译次数
		javaCode := `public class ImmediateTest {
    public static void main(String[] args) {
        System.out.println("Immediate Compile Test");
    }
}`
		tempDir, cleanupDir := setupTempDirWithJavaFile(t, "ImmediateTest.java", javaCode)
		defer cleanupDir()

		// 使用立即编译模式探测项目
		log.Infof("Starting SSA auto detective with immediate compile...")
		info, prog, err := ParseProjectWithAutoDetective(
			context.Background(),
			tempDir,
			"java",
			true, // 立即编译
		)
		require.NoError(t, err)
		require.NotNil(t, prog, "Program should be compiled immediately")
		require.NotEmpty(t, info.GetProgramName(), "Program name should not be empty")
		require.NotEmpty(t, info.GetProjectName(), "Project name should not be empty")

		log.Infof("Immediate compile completed:")
		log.Infof("  ProgramName: %s", info.GetProgramName())
		log.Infof("  ProjectName: %s", info.GetProjectName())
		log.Infof("  Language: %s", info.GetLanguage())

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

		// 验证项目信息
		require.Equal(t, info.GetProjectName(), project.ProjectName)
		require.Equal(t, string(info.GetLanguage()), project.Language)
		require.Equal(t, int64(1), project.CompileTimes, "CompileTimes should be 1 after immediate compilation")

		// 清理项目
		defer func() {
			deleteReq := &ypb.DeleteSSAProjectRequest{
				DeleteMode: string(yakit.SSAProjectDeleteAll),
				Filter: &ypb.SSAProjectFilter{
					IDs: []int64{project.ID},
				},
			}
			_, _ = client.DeleteSSAProject(context.Background(), deleteReq)
		}()

		log.Infof("✅ Test passed: SSA project compiled immediately with CompileTimes = 1")
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
		info, prog, err := ParseProjectWithAutoDetective(
			context.Background(),
			tempDir,
			"java",
			false,
		)
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
		pluginName := "SSA 项目编译V2"

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
