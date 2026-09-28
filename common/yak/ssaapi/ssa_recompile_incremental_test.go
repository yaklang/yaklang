package ssaapi_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestRecompileFullProgramAfterProjectEnablesIncremental 复现并验证修复：
// 首次全量编译（未启用增量）后，项目开启"启用增量编译"，
// 对全量 program 重编译时应转为以该 program 为 base 的增量编译，
// 而不是继续同名全量覆盖。
func TestRecompileFullProgramAfterProjectEnablesIncremental(t *testing.T) {
	yakit.InitialDatabase()

	tempDir, err := os.MkdirTemp("", "ssatest-recompile-incremental-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	require.NoError(t, os.MkdirAll(tempDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "A.java"), []byte(`
public class A {
  public String getValue() {
    return "Value from A";
  }
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "Main.java"), []byte(`
public class Main {
  public static void main(String[] args) {
    A a = new A();
    System.out.println(a.getValue());
  }
}
`), 0o644))

	projectName := fmt.Sprintf("%s_recompile_incremental", filepath.Base(tempDir))

	// 第一次：全量编译（不启用增量编译），program 归属该项目
	prog, err := ssaapi.ParseProject(
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithLocalFs(tempDir),
		ssaconfig.WithProjectName(projectName),
		ssaapi.WithProgramName(projectName),
		ssaapi.WithReCompile(true),
		ssaapi.WithContext(context.Background()),
	)
	require.NoError(t, err)
	require.NotEmpty(t, prog)
	firstProg := prog[0]

	// 创建 SSA 项目并持久化 project_id（模拟前端 CreateSSAProject）
	configJSON := fmt.Sprintf(`{"project_name":%q,"program_name":%q}`, projectName, projectName)
	profileDB := consts.GetGormProfileDatabase()
	project, err := yakit.CreateSSAProject(profileDB, &ypb.CreateSSAProjectRequest{
		JSONStringConfig: configJSON,
	})
	require.NoError(t, err)
	require.NotZero(t, project.ID)
	projectIDForCleanup := int64(project.ID)
	t.Cleanup(func() {
		_, _ = yakit.DeleteSSAProject(profileDB, &ypb.DeleteSSAProjectRequest{
			DeleteMode: string(yakit.SSAProjectDeleteAll),
			Filter: &ypb.SSAProjectFilter{
				IDs: []int64{projectIDForCleanup},
			},
		})
	})

	// 手工把 project_id 写进 program，模拟真实链路（SaveToDB 注入 config.project_id
	// → 编译时 application.ProjectID）
	firstIr, err := ssadb.GetProgram(firstProg.GetProgramName(), ssadb.Application)
	require.NoError(t, err)
	require.False(t, firstIr.IsOverlay)
	firstIr.ProjectID = uint64(project.ID)
	require.NoError(t, ssadb.UpdateProgramWithError(firstIr))

	// 项目随后开启"启用增量编译"（模拟探测插件对已存在项目更新配置 + SaveToDB）
	saProject, err := ssaproject.LoadSSAProjectByID(uint(project.ID))
	require.NoError(t, err)
	require.NoError(t, saProject.UpdateConfig(ssaapi.WithEnableIncrementalCompile(true)))
	require.NoError(t, saProject.SaveToDB())

	// reload program（避免旧缓存），对全量 program 重编译
	ssaapi.ProgramCache.Remove(firstProg.GetProgramName())
	reloaded, err := ssaapi.FromDatabase(firstProg.GetProgramName())
	require.NoError(t, err)
	require.False(t, reloaded.IsIncrementalCompile())

	// 阶段一：文件无变更时重编译 —— 应复用 base program 且不报错
	require.NoError(t, reloaded.Recompile(ssaapi.WithContext(context.Background())))

	// 阶段二：修改文件后再重编译 —— 应产生以原 program 为 base 的增量 diff program
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "A.java"), []byte(`
public class A {
  public String getValue() {
    return "Value from A modified";
  }
}
`), 0o644))

	ssaapi.ProgramCache.Remove(firstProg.GetProgramName())
	reloaded2, err := ssaapi.FromDatabase(firstProg.GetProgramName())
	require.NoError(t, err)
	require.NoError(t, reloaded2.Recompile(ssaapi.WithContext(context.Background())))

	// 重编译后应产生一个以原 program 为 base 的增量 diff program
	// （按 updated_at 取最新，GetProgramByProjectID 的 First() 是按主键升序取最旧）
	var latest ssadb.IrProgram
	require.NoError(t, ssadb.GetDB().Where("project_id = ?", project.ID).
		Order("updated_at DESC").First(&latest).Error)
	require.NotEqual(t, firstProg.GetProgramName(), latest.ProgramName)
	require.Equal(t, firstProg.GetProgramName(), latest.BaseProgramName)
	require.True(t, latest.IsOverlay)
	require.NotEmpty(t, latest.FileHashMap)
}