package yakgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func query(t *testing.T, filter *ypb.SSAProgramFilter, local ypb.YakClient, name string) *ypb.SSAProgram {
	res, err := local.QuerySSAPrograms(context.Background(), &ypb.QuerySSAProgramRequest{
		Filter: filter,
	})
	require.NoError(t, err)
	for _, prog := range res.Data {
		if prog.Name == name {
			return prog
		}
	}
	return nil
}

func TestGRPCMUSTPASS_SyntaxFlow_SSAPrograms_Query(t *testing.T) {
	name := uuid.NewString()
	desc := `
	this is simple yaklang code example 
	`

	prog, err := ssaapi.Parse(`print("a")`,
		ssaapi.WithProgramName(name),
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramDescription(desc),
	)
	_ = prog
	defer func() {

		ssadb.DeleteProgram(ssadb.GetDB(), name)
	}()
	require.NoError(t, err)

	local, err := NewLocalClient(true)
	require.NoError(t, err)

	queryByFilter := func(t *testing.T, filter *ypb.SSAProgramFilter) *ypb.SSAProgram {
		return query(t, filter, local, name)
	}

	t.Run("query all", func(t *testing.T) {
		prog := queryByFilter(t, nil)
		require.NotNil(t, prog)
		require.Equal(t, prog.Name, name)
		require.Equal(t, prog.Language, string(ssaconfig.Yak))
		require.Equal(t, prog.Description, desc)
	})
	t.Run("query with filter name", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{
			ProgramNames: []string{name},
		}))
	})
	t.Run("query with filter description", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{
			Keyword: "simple",
		}))
	})
	t.Run("query with Language", func(t *testing.T) {
		require.NotNil(t, queryByFilter(t, &ypb.SSAProgramFilter{
			Languages: []string{string(ssaconfig.Yak)},
		}))
	})
	t.Run("query risk by filter", func(t *testing.T) {
		require.Nil(t, queryByFilter(t, &ypb.SSAProgramFilter{
			Languages: []string{string(ssaconfig.JAVA)},
		}))
	})

	t.Run("query risk", func(t *testing.T) {
		res, err := prog.SyntaxFlowWithError(`
		print(* as $a)
		alert $a for {
			level: 'high',
		}
		`)
		require.NoError(t, err)
		resultId, err := res.Save(schema.SFResultKindDebug)
		_ = resultId
		require.NoError(t, err)

		ssaProg := queryByFilter(t, nil)
		require.NotNil(t, ssaProg)
		require.Equal(t, ssaProg.HighRiskNumber, int64(1))
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Program_Delete_WithKeyword(t *testing.T) {
	name := uuid.NewString()
	desc := `
	this is simple yaklang code example 
	`
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	_, err = ssaapi.Parse(`print("a")`,
		ssaapi.WithProgramName(name),
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramDescription(desc),
	)
	defer func() {

		ssadb.DeleteProgram(ssadb.GetDB(), name)
	}()
	require.NoError(t, err)

	have := func(targetName string) bool {
		return query(t, nil, local, targetName) != nil
	}

	// have this program
	require.True(t, have(name))

	// delete program
	_, err = local.DeleteSSAPrograms(context.Background(), &ypb.DeleteSSAProgramRequest{
		Filter: &ypb.SSAProgramFilter{
			ProgramNames: []string{name},
		},
	})
	require.NoError(t, err)

	// no this program
	require.False(t, have(name))
}

func TestGRPCMUSTPASS_SyntaxFlow_Program_Update(t *testing.T) {

	name := uuid.NewString()
	desc := `
	this is simple yaklang code example 
	`
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	_, err = ssaapi.Parse(`print("a")`,
		ssaapi.WithProgramName(name),
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramDescription(desc),
	)
	defer func() {

		ssadb.DeleteProgram(ssadb.GetDB(), name)
	}()
	require.NoError(t, err)

	get := func() *ypb.SSAProgram {
		return query(t, nil, local, name)
	}
	prog := get()
	require.NotNil(t, prog)
	require.Equal(t, prog.Description, desc)

	newDesc := "new desc"
	_, err = local.UpdateSSAProgram(context.Background(), &ypb.UpdateSSAProgramRequest{
		ProgramInput: &ypb.SSAProgramInput{
			Name:        name,
			Description: newDesc,
		},
	})
	require.NoError(t, err)

	prog = get()
	require.NotNil(t, prog)
	require.Equal(t, prog.Description, newDesc)
}

// IncrementalCompileInfoCheck 定义单个 program 的增量编译信息检查配置
type IncrementalCompileInfoCheck struct {
	ProgramIndex         int    // program 索引（0表示base，1表示diff1，2表示diff2，-1表示normal program）
	IsIncrementalCompile bool   // 是否应该是增量编译
	IncrementalGroupId   string // 预期的增量编译组ID（空字符串表示使用 base program name）
	HeadProgramName      string // 预期的头部 program name（空字符串表示使用最新的 program name）
}

// IncrementalCompileInfoTestConfig 统一的增量编译信息查询测试配置结构
type IncrementalCompileInfoTestConfig struct {
	Name                     string                        // 测试名称
	FileSystems              []map[string]string           // 多个文件系统，第一个是基础，其他是增量修改
	NormalProgramFileSystem  map[string]string             // 普通编译的文件系统（可选，nil表示不创建）
	EnableIncrementalForBase bool                          // 是否为基础程序启用增量编译（WithEnableIncrementalCompile）
	ExpectedIncrementalInfo  []IncrementalCompileInfoCheck // 预期的增量编译信息检查配置
}

// checkIncrementalCompileInfo 统一的增量编译信息查询测试检查函数
func checkIncrementalCompileInfo(t *testing.T, config IncrementalCompileInfoTestConfig) {
	ctx := context.Background()

	// 验证配置：至少需要一个文件系统（基础）
	require.GreaterOrEqual(t, len(config.FileSystems), 1, "至少需要一个文件系统（基础）")

	// 计算总的 program 数量
	totalProgramCount := len(config.FileSystems)
	if config.NormalProgramFileSystem != nil {
		totalProgramCount++
	}

	// 创建程序名称
	programNames := make([]string, totalProgramCount)
	for i := range programNames {
		programNames[i] = uuid.NewString()
	}

	// 清理函数
	defer func() {
		for _, name := range programNames {
			ssadb.DeleteProgram(ssadb.GetDB(), name)
		}
	}()

	// Step 1: 创建基础程序（第一个文件系统）
	baseFS := filesys.NewVirtualFs()
	for path, content := range config.FileSystems[0] {
		baseFS.AddFile(path, content)
	}

	var basePrograms ssaapi.Programs
	var err error
	if config.EnableIncrementalForBase {
		basePrograms, err = ssaapi.ParseProject(
			ssaapi.WithFileSystem(baseFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programNames[0]),
			ssaapi.WithEnableIncrementalCompile(true),
		)
	} else {
		basePrograms, err = ssaapi.ParseProject(
			ssaapi.WithFileSystem(baseFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programNames[0]),
		)
	}
	require.NoError(t, err)
	require.NotNil(t, basePrograms)
	require.Greater(t, len(basePrograms), 0)

	// Step 2: 创建增量程序（如果有多个文件系统）
	for i := 1; i < len(config.FileSystems); i++ {
		diffFS := filesys.NewVirtualFs()
		for path, content := range config.FileSystems[i] {
			diffFS.AddFile(path, content)
		}

		// 使用增量编译 API
		var baseProgName string
		if i == 1 {
			baseProgName = programNames[0]
		} else {
			baseProgName = programNames[i-1]
		}

		diffProgs, err := ssaapi.ParseProjectWithIncrementalCompile(
			diffFS,
			baseProgName, programNames[i],
			ssaconfig.JAVA,
			ssaapi.WithContext(ctx),
		)
		require.NoError(t, err)
		require.NotNil(t, diffProgs)
		require.Greater(t, len(diffProgs), 0)
	}

	// Step 3: 创建普通编译的 program（如果配置了）
	normalProgramIndex := -1
	if config.NormalProgramFileSystem != nil {
		normalProgramIndex = len(config.FileSystems)
		normalFS := filesys.NewVirtualFs()
		for path, content := range config.NormalProgramFileSystem {
			normalFS.AddFile(path, content)
		}

		normalPrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(normalFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programNames[normalProgramIndex]),
		)
		require.NoError(t, err)
		require.NotNil(t, normalPrograms)
		require.Greater(t, len(normalPrograms), 0)
	}

	// Step 4: 通过 gRPC API 查询所有 program
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	res, err := local.QuerySSAPrograms(ctx, &ypb.QuerySSAProgramRequest{
		Filter: &ypb.SSAProgramFilter{
			ProgramNames: programNames,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Data)
	require.Len(t, res.Data, totalProgramCount)

	// 创建 program name 到 program 的映射
	programMap := make(map[string]*ypb.SSAProgram)
	for _, prog := range res.Data {
		programMap[prog.Name] = prog
	}

	// Step 5: 验证预期的增量编译信息
	for _, check := range config.ExpectedIncrementalInfo {
		var programName string
		if check.ProgramIndex == -1 {
			// normal program
			if normalProgramIndex == -1 {
				t.Fatalf("ExpectedIncrementalInfo contains normal program check but NormalProgramFileSystem is nil")
			}
			programName = programNames[normalProgramIndex]
		} else {
			if check.ProgramIndex >= len(programNames) {
				t.Fatalf("ProgramIndex %d is out of range (max: %d)", check.ProgramIndex, len(programNames)-1)
			}
			programName = programNames[check.ProgramIndex]
		}

		prog, ok := programMap[programName]
		require.True(t, ok, "program %s (index %d) should be found", programName, check.ProgramIndex)

		// 验证 IsIncrementalCompile
		require.Equal(t, check.IsIncrementalCompile, prog.IsIncrementalCompile,
			"program %s (index %d) IsIncrementalCompile should be %v", programName, check.ProgramIndex, check.IsIncrementalCompile)

		// 验证 IncrementalGroupId
		expectedGroupId := check.IncrementalGroupId
		if expectedGroupId == "" {
			if check.ProgramIndex == -1 {
				// normal program: 使用自己的名字
				expectedGroupId = programName
			} else {
				// 增量编译 program: 使用 base program name
				expectedGroupId = programNames[0]
			}
		}
		require.Equal(t, expectedGroupId, prog.IncrementalGroupId,
			"program %s (index %d) IncrementalGroupId should be %s", programName, check.ProgramIndex, expectedGroupId)

		// 验证 HeadProgramName
		expectedHeadName := check.HeadProgramName
		if expectedHeadName == "" {
			if check.ProgramIndex == -1 {
				// normal program: 使用自己的名字
				expectedHeadName = programName
			} else {
				// 增量编译 program: 使用最新的 program name（最后一个增量 program）
				if len(config.FileSystems) > 1 {
					expectedHeadName = programNames[len(config.FileSystems)-1]
				} else {
					expectedHeadName = programNames[0]
				}
			}
		}
		require.Equal(t, expectedHeadName, prog.HeadProgramName,
			"program %s (index %d) HeadProgramName should be %s", programName, check.ProgramIndex, expectedHeadName)
	}
}

// TestGRPCMUSTPASS_SyntaxFlow_SSAPrograms_QueryIncrementalCompileInfo 测试查询增量编译信息的功能
// 验证 QuerySSAPrograms 返回的增量编译信息是否正确
func TestGRPCMUSTPASS_SyntaxFlow_SSAPrograms_QueryIncrementalCompileInfo(t *testing.T) {
	t.Run("test query incremental compile info", func(t *testing.T) {
		checkIncrementalCompileInfo(t, IncrementalCompileInfoTestConfig{
			Name: "test query incremental compile info",
			FileSystems: []map[string]string{
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Diff1";
		public String getValue() {
			return "Value from Modified A in Diff1";
		}
	}`,
					"B.java": `
	public class B {
		public static void process() {
			System.out.println("Process from B");
		}
	}`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Diff2";
		public String getValue() {
			return "Value from Modified A in Diff2";
		}
	}`,
					"C.java": `
	public class C {
		public static void compute() {
			System.out.println("Compute from C");
		}
	}`,
				},
			},
			NormalProgramFileSystem: map[string]string{
				"Normal.java": `
	public class Normal {
		public static void main(String[] args) {
			System.out.println("Normal compilation");
		}
	}`,
			},
			EnableIncrementalForBase: true,
			ExpectedIncrementalInfo: []IncrementalCompileInfoCheck{
				{
					ProgramIndex:         0, // base program
					IsIncrementalCompile: true,
					IncrementalGroupId:   "", // 空字符串表示使用 base program name (programNames[0])
					HeadProgramName:      "", // 空字符串表示使用最新的 program name (programNames[2])
				},
				{
					ProgramIndex:         1, // diff1 program
					IsIncrementalCompile: true,
					IncrementalGroupId:   "", // 空字符串表示使用 base program name
					HeadProgramName:      "", // 空字符串表示使用最新的 program name
				},
				{
					ProgramIndex:         2, // diff2 program
					IsIncrementalCompile: true,
					IncrementalGroupId:   "", // 空字符串表示使用 base program name
					HeadProgramName:      "", // 空字符串表示使用最新的 program name (自己)
				},
				{
					ProgramIndex:         -1, // normal program
					IsIncrementalCompile: false,
					IncrementalGroupId:   "", // 对于普通编译，groupId 应该是自己
					HeadProgramName:      "", // 对于普通编译，head 应该是自己
				},
			},
		})
	})
}
