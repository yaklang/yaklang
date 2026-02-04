package ssa_compile_test

import (
	"context"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var initDB sync.Once

func TestSSAAutoDetective(t *testing.T) {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	check := func(t *testing.T, input string) (*ssaconfig.Config, error) {
		res, err := ssa_compile.ParseProjectWithAutoDetective(context.Background(), &ssa_compile.SSADetectConfig{
			Target: input,
		})
		if res == nil || res.Info == nil {
			return nil, err
		}
		return res.Info.Config, err
	}

	t.Run("check compile jar", func(t *testing.T) {
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

func TestSSAProjectComprehensive(t *testing.T) {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	t.Run("create SSA project via gRPC with params raw data", func(t *testing.T) {
		tempDir, cleanupDir := setupTempDirWithJavaFile(t, "Main.java",
			"public class Main { public static void main(String[] args) {} }")
		defer cleanupDir()

		res, err := ssa_compile.ParseProjectWithAutoDetective(context.Background(), &ssa_compile.SSADetectConfig{
			Target: tempDir,
		})
		require.NoError(t, err)
		info := res.Info
		prog := res.Program
		cleanup := res.Cleanup
		_ = cleanup
		require.Nil(t, prog)

		project, cleanupProject := createSSAProject(t, info.Config)
		defer cleanupProject()

		require.Equal(t, info.GetProjectName(), project.ProjectName)
		require.Equal(t, string(info.GetLanguage()), string(project.Language))
		require.Equal(t, info.GetProjectDescription(), project.Description)
	})

	t.Run("detective with compile_immediately flag", func(t *testing.T) {
		javaCode := `public class ImmediateTest {
    public static void main(String[] args) {
        System.out.println("Immediate Compile Test");
    }
}`
		tempDir, cleanupDir := setupTempDirWithJavaFile(t, "ImmediateTest.java", javaCode)
		defer cleanupDir()

		log.Infof("Starting SSA auto detective with compile_immediately=true...")
		res, err := ssa_compile.ParseProjectWithAutoDetective(context.Background(), &ssa_compile.SSADetectConfig{
			Target:             tempDir,
			Language:           "java",
			CompileImmediately: true,
		})
		require.NoError(t, err)
		info := res.Info
		prog := res.Program
		cleanup := res.Cleanup
		require.NotNil(t, cleanup, "cleanup function should be returned")
		defer cleanup()

		require.NotNil(t, prog, "compile_immediately=true 时应该返回编译后的程序")
		require.NotEmpty(t, info.GetProgramName(), "Program name should not be empty")
		require.NotEmpty(t, info.GetProjectName(), "Project name should not be empty")

		log.Infof("Immediate compile completed:")
		log.Infof("  ProgramName: %s", info.GetProgramName())
		log.Infof("  ProjectName: %s", info.GetProjectName())
		log.Infof("  Language: %s", info.GetLanguage())
		log.Infof("  CompileImmediately: %v", info.CompileImmediately)
		log.Infof("  Program: %v", prog)

		require.True(t, info.CompileImmediately, "CompileImmediately should be true")

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

		require.Equal(t, int64(1), project.CompileTimes, "CompileTimes should be 1 after immediate compilation")

		log.Infof("✅ Test passed: compile_immediately flag works correctly with automatic compilation")
	})

	t.Run("check project exists and use existing config", func(t *testing.T) {
		tempDir, cleanupDir := setupTempDirWithJavaFile(t, "ExistsTest.java", `
public class ExistsTest {
    public static void main(String[] args) {
        System.out.println("Project exists test");
    }
}
`)
		defer cleanupDir()

		log.Infof("First detection - project should not exist")
		res1, err := ssa_compile.ParseProjectWithAutoDetective(context.Background(), &ssa_compile.SSADetectConfig{
			Target:   tempDir,
			Language: "java",
		})
		require.NoError(t, err)
		info1 := res1.Info
		require.NotNil(t, info1)
		require.False(t, info1.ProjectExists, "First detection: project should not exist")

		firstConfig := info1.Config
		require.NotNil(t, firstConfig)
		firstProjectName := firstConfig.GetProjectName()
		log.Infof("First detection: project_name=%s, project_exists=%v", firstProjectName, info1.ProjectExists)

		log.Infof("Creating SSA project...")
		project, cleanupProj := createSSAProject(t, info1.Config)
		defer cleanupProj()
		log.Infof("Project created with ID: %d, Name: %s", project.ID, project.ProjectName)

		log.Infof("Second detection - project should exist and use existing config")
		res2, err := ssa_compile.ParseProjectWithAutoDetective(context.Background(), &ssa_compile.SSADetectConfig{
			Target:   tempDir,
			Language: "java",
		})
		require.NoError(t, err)
		info2 := res2.Info
		require.NotNil(t, info2)
		require.True(t, info2.ProjectExists, "Second detection: project should exist")

		secondConfig := info2.Config
		require.NotNil(t, secondConfig)
		require.Equal(t, project.ProjectName, secondConfig.GetProjectName(), "Should use existing project name")
		require.Equal(t, project.Language, string(secondConfig.GetLanguage()), "Should use existing project language")

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

		ssaDB := consts.GetGormSSAProjectDataBase()

		log.Infof("Step 1: Starting SSA auto detective without compile...")
		res, err := ssa_compile.ParseProjectWithAutoDetective(context.Background(), &ssa_compile.SSADetectConfig{
			Target:   tempDir,
			Language: "java",
		})
		require.NoError(t, err)
		info := res.Info
		prog := res.Program
		cleanup := res.Cleanup
		_ = cleanup
		require.Nil(t, prog, "Program should not be compiled yet")
		require.NotEmpty(t, info.GetProgramName(), "Program name should not be empty")

		log.Infof("Detective completed:")
		log.Infof("  ProgramName: %s", info.GetProgramName())
		log.Infof("  ProjectName: %s", info.GetProjectName())
		log.Infof("  Language: %s", info.GetLanguage())

		log.Infof("Step 2: Creating SSA project...")
		project, cleanupProject := createSSAProject(t, info.Config)
		defer cleanupProject()

		log.Infof("Step 3: Compiling SSA project using compile script...")
		pluginName := "SSA 项目编译"

		configJSON := project.JSONStringConfig
		require.NotEmpty(t, configJSON, "Project JSON config should not be empty")

		compileParam := map[string]string{
			"config": configJSON,
		}

		for i := 1; i <= 3; i++ {
			log.Infof("Compiling project - attempt %d/3", i)
			err = yakgrpc.ExecScriptWithParam(context.Background(), pluginName, compileParam,
				"", func(exec *ypb.ExecResult) error {
					return nil
				},
			)
			require.NoError(t, err, "Compilation %d should succeed", i)
			time.Sleep(1 * time.Second)
			currentCompileTimes := yakit.QuerySSACompileTimesByProjectID(ssaDB, uint(project.ID))
			log.Infof("After compilation %d, CompileTimes: %d", i, currentCompileTimes)
			require.Equal(t, int64(i), currentCompileTimes, "CompileTimes should be %d after %d compilation(s)", i, i)
		}

		log.Infof("All 3 compilations completed successfully")

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

		finalCompileTimes := yakit.QuerySSACompileTimesByProjectID(ssaDB, uint(project.ID))
		log.Infof("Final CompileTimes: %d", finalCompileTimes)
		require.Equal(t, int64(3), finalCompileTimes, "CompileTimes should be 3 after three compilations")

		log.Infof("✅ Test passed: SSA project compiled successfully 3 times with CompileTimes = 3")
	})
}

func TestExcludeFile(t *testing.T) {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	setup := func() string {
		tempDir := path.Join(os.TempDir(), uuid.NewString())
		require.NoError(t, os.MkdirAll(path.Join(tempDir, "a"), 0o755))
		require.NoError(t, os.MkdirAll(path.Join(tempDir, "b"), 0o755))

		writeFile := func(relPath, content string) {
			fullPath := path.Join(tempDir, relPath)
			require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
		}

		writeFile("a.java", "public class A{}")
		writeFile("a/a.java", "public class A1 {}")
		writeFile("a/b.java", "public class B1 {}")
		writeFile("b/a.java", "public class A2 {}")
		return tempDir
	}

	t.Run("exclude file", func(t *testing.T) {
		tempDir := setup()
		defer os.RemoveAll(tempDir)
		res, err := ssa_compile.ParseProjectWithAutoDetective(context.Background(), &ssa_compile.SSADetectConfig{
			Target:   tempDir,
			Language: "java",
			Params: map[string]any{
				"excludeFile": "a.java",
			},
			CompileImmediately: true,
		})
		require.NoError(t, err)
		info := res.Info
		require.NotNil(t, info)
		require.NotNil(t, info.Config)

		excludes := info.Config.GetCompileExcludeFiles()
		require.Contains(t, excludes, "a.java")

		prog := res.Program
		defer ssadb.DeleteProgram(ssadb.GetDB(), prog.GetProgramName())
		require.NotNil(t, prog)

		fileList := make([]string, 0, len(prog.Program.FileList))
		prog.Show().ForEachAllFile(func(s string, me *memedit.MemEditor) bool {
			s = strings.TrimPrefix(s, "/"+prog.GetProgramName())
			fileList = append(fileList, s)
			return true
		})

		log.Infof("FileList: %#v", fileList)

		require.Contains(t, fileList, "/a/a.java")
		require.Contains(t, fileList, "/a/b.java")
		require.Contains(t, fileList, "/b/a.java")
		require.NotContains(t, fileList, "/a.java")
	})

	t.Run("exclude folder", func(t *testing.T) {
		tempDir := setup()
		defer os.RemoveAll(tempDir)
		res, err := ssa_compile.ParseProjectWithAutoDetective(context.Background(), &ssa_compile.SSADetectConfig{
			Target:   tempDir,
			Language: "java",
			Params: map[string]any{
				"excludeFile": "a,a/",
			},
			CompileImmediately: true,
		})
		require.NoError(t, err)
		info := res.Info
		require.NotNil(t, info)
		require.NotNil(t, info.Config)

		excludes := info.Config.GetCompileExcludeFiles()
		require.Contains(t, excludes, "a")
		require.Contains(t, excludes, "a/")

		prog := res.Program
		defer ssadb.DeleteProgram(ssadb.GetDB(), prog.GetProgramName())
		require.NotNil(t, prog)

		fileList := make([]string, 0, len(prog.Program.FileList))
		prog.Show().ForEachAllFile(func(s string, me *memedit.MemEditor) bool {
			s = strings.TrimPrefix(s, "/"+prog.GetProgramName())
			fileList = append(fileList, s)
			return true
		})
		log.Infof("FileList: %#v", fileList)

		require.Contains(t, fileList, "/b/a.java")
		require.Contains(t, fileList, "/a.java")
		require.NotContains(t, fileList, "/a/a.java")
		require.NotContains(t, fileList, "/a/b.java")
	})
}
