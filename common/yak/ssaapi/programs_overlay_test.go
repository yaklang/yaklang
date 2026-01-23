package ssaapi_test

import (
	"context"
	"io/fs"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// 测试 ProgramOverLay 功能：多层增量编译的虚拟视图
// - 差量 program 从数据库加载时自动创建 overlay
// - 基于差量 program 进行增量编译时自动聚合生成 overlay

var valueName = "valueStr"

var baseValueStr = "Value from Base"
var extendValueStr = "Value from Extend"

// SyntaxFlowRuleCheck 定义 syntaxflow 规则检查配置
type SyntaxFlowRuleCheck struct {
	Rule               string   // syntaxflow 规则
	ExpectedValues     []string // 预期的值（包含这些字符串）
	ExpectedCount      int      // 预期的结果数量（0表示不限制，-1表示应该为空）
	ExpectedOverridden bool     // 是否预期被覆盖（仅对base/extend program有效，对overlay无效）
	QueryInstance      string   // 查询实例："base", "extend", "overlay", "all"
	VariableName       string   // 如果规则中有变量绑定（如 $data），指定变量名来获取值
}

// OverlayTestConfig 统一的 overlay 测试配置结构
type OverlayTestConfig struct {
	Name                    string                // 测试名称
	FileSystems             []map[string]string   // 多个文件系统，第一个是基础，其他是增量修改
	SyntaxFlowRules         []SyntaxFlowRuleCheck // syntaxflow 规则和预期结果
	ExpectedAggregatedFiles []string              // 预期的聚合文件系统文件列表（应该存在的文件）
	ExpectedExcludedFiles   []string              // 预期排除的文件列表（不应该存在的文件）
	LoadFromDatabase        bool                  // 是否从数据库加载程序（用于测试数据库加载场景）
	IsDatabase              bool                  // 是否从数据库加载程序（与 LoadFromDatabase 相同，提供更明确的命名）
}

// checkOverlayTest 统一的 overlay 测试检查函数
func checkOverlayTest(t *testing.T, config OverlayTestConfig) {
	ctx := context.Background()

	// 创建程序名称
	programNames := make([]string, len(config.FileSystems))
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

	basePrograms, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(programNames[0]),
	)
	require.NoError(t, err)
	require.NotNil(t, basePrograms)
	require.Greater(t, len(basePrograms), 0, "Should have at least one program")
	baseProgram := basePrograms[0]

	// Step 2: 创建增量程序（如果有多个文件系统）
	var programs []*ssaapi.Program
	programs = append(programs, baseProgram)

	for i := 1; i < len(config.FileSystems); i++ {
		diffFS := filesys.NewVirtualFs()
		for path, content := range config.FileSystems[i] {
			diffFS.AddFile(path, content)
		}

		diffProgram, err := ssaapi.CompileDiffProgramAndSaveToDB(
			ctx,
			nil, diffFS,
			programNames[i-1], programNames[i],
			ssaconfig.JAVA,
		)
		require.NoError(t, err)
		require.NotNil(t, diffProgram)
		programs = append(programs, diffProgram)
	}

	// Step 3: 从数据库加载（如果需要）
	if config.LoadFromDatabase || config.IsDatabase {
		for i, name := range programNames {
			loadedProg, err := ssaapi.FromDatabase(name)
			require.NoError(t, err)
			require.NotNil(t, loadedProg)
			programs[i] = loadedProg
		}
	}

	// Step 4: 创建 overlay
	overlay := ssaapi.NewProgramOverLay(programs...)
	require.NotNil(t, overlay)

	// Step 5: 检查聚合文件系统
	if len(config.ExpectedAggregatedFiles) > 0 || len(config.ExpectedExcludedFiles) > 0 {
		aggFS := overlay.GetAggregatedFileSystem()
		require.NotNil(t, aggFS, "AggregatedFS should not be nil")

		overlayFiles := make(map[string]bool)
		filesys.Recursive(".", filesys.WithFileSystem(aggFS), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			if !info.IsDir() {
				normalizedPath := s
				if len(normalizedPath) > 0 && normalizedPath[0] == '/' {
					normalizedPath = normalizedPath[1:]
				}
				overlayFiles[normalizedPath] = true
			}
			return nil
		}))

		// 检查应该存在的文件
		for _, expectedFile := range config.ExpectedAggregatedFiles {
			normalizedExpected := expectedFile
			if len(normalizedExpected) > 0 && normalizedExpected[0] == '/' {
				normalizedExpected = normalizedExpected[1:]
			}
			require.True(t, overlayFiles[normalizedExpected], "Aggregated FS should contain %s", expectedFile)
		}

		// 检查不应该存在的文件
		for _, excludedFile := range config.ExpectedExcludedFiles {
			normalizedExcluded := excludedFile
			if len(normalizedExcluded) > 0 && normalizedExcluded[0] == '/' {
				normalizedExcluded = normalizedExcluded[1:]
			}
			require.False(t, overlayFiles[normalizedExcluded], "Aggregated FS should not contain %s", excludedFile)
		}
	}

	// Step 6: 检查 syntaxflow 规则
	for _, ruleCheck := range config.SyntaxFlowRules {
		var queryInstance ssaapi.SyntaxFlowQueryInstance

		switch ruleCheck.QueryInstance {
		case "base":
			queryInstance = baseProgram
		case "extend":
			if len(programs) > 1 {
				queryInstance = programs[len(programs)-1]
			} else {
				t.Fatalf("Cannot use 'extend' query instance when there is only one program")
			}
		case "overlay":
			queryInstance = overlay
		case "all":
			queryInstance = ssaapi.Programs(programs)
		default:
			queryInstance = overlay // 默认为 overlay
		}

		res, err := queryInstance.SyntaxFlowWithError(ruleCheck.Rule)
		require.NoError(t, err)
		require.NotNil(t, res)

		// 根据是否有变量名来决定获取值的方式
		var values []*ssaapi.Value
		if ruleCheck.VariableName != "" {
			values = res.GetValues(ruleCheck.VariableName)
		} else {
			values = res.GetAllValuesChain()
		}

		// 检查结果数量
		if ruleCheck.ExpectedCount == -1 {
			require.Empty(t, values, "Should not find values for rule: %s", ruleCheck.Rule)
		} else if ruleCheck.ExpectedCount > 0 {
			require.Len(t, values, ruleCheck.ExpectedCount, "Should find exactly %d values for rule: %s", ruleCheck.ExpectedCount, ruleCheck.Rule)
		} else if ruleCheck.ExpectedCount == 0 {
			require.NotEmpty(t, values, "Should find values for rule: %s", ruleCheck.Rule)
		}

		// 检查预期值
		if len(ruleCheck.ExpectedValues) > 0 && len(values) > 0 {
			for _, expectedValue := range ruleCheck.ExpectedValues {
				found := false
				for _, v := range values {
					if contains(v.String(), expectedValue) {
						found = true
						break
					}
				}
				require.True(t, found, "Should find value containing '%s' in rule: %s", expectedValue, ruleCheck.Rule)
			}
		}

		// 检查是否被覆盖（仅对单个值有效）
		if ruleCheck.ExpectedOverridden && len(values) > 0 && overlay != nil {
			v := values[0]
			isOverridden := overlay.IsOverridden(v)
			require.True(t, isOverridden, "Value %s should be overridden", v.String())
		}
	}
}

// contains 检查字符串是否包含子字符串（不区分大小写）
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	// 简单的包含检查，可以扩展为不区分大小写
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// InitProgram 初始化基础程序和差量程序
func InitProgram(t *testing.T) (progBase *ssaapi.Program, progExtend *ssaapi.Program, progNameBaseUUID string, progNameExtendUUID string) {
	progNameBaseUUID = uuid.NewString()
	progNameExtendUUID = uuid.NewString()

	baseFS := filesys.NewVirtualFs()
	var err error

	t.Logf("Creating new progBase")
	baseFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`)
	baseFS.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)

	progBases, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(progNameBaseUUID),
	)
	progBase = progBases[0]
	require.NoError(t, err)
	require.NotNil(t, progBase)
	require.Greater(t, len(progBases), 0, "Should have at least one program")

	{
		vs := progBase.Ref(valueName)
		require.NotEmpty(t, vs, "Should find value in base program")
		require.Contains(t, vs.String(), baseValueStr, "Base value should match")
	}

	t.Logf("Creating new progExtend (diff program)")
	newFS := filesys.NewVirtualFs()
	newFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}	
	}`)
	newFS.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)

	ctx := context.Background()
	progExtend, err = ssaapi.CompileDiffProgramAndSaveToDB(
		ctx,
		nil, newFS,
		progNameBaseUUID, progNameExtendUUID,
		ssaconfig.JAVA,
	)
	require.NoError(t, err)
	require.NotNil(t, progExtend)

	return
}

// TestOverlay_Easy 测试基本的 overlay 功能
func TestOverlay_Easy(t *testing.T) {
	t.Run("test basic overlay functionality", func(t *testing.T) {
		config := OverlayTestConfig{
			Name: "test basic overlay functionality",
			FileSystems: []map[string]string{
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`,
					"Main.java": `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}	
	}`,
					"Main.java": `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`,
				},
			},
			SyntaxFlowRules: []SyntaxFlowRuleCheck{
				{
					Rule:               "valueStr as $res",
					ExpectedValues:     []string{baseValueStr},
					ExpectedCount:      1,
					ExpectedOverridden: true,
					QueryInstance:      "base",
				},
				{
					Rule:               "valueStr as $res",
					ExpectedValues:     []string{extendValueStr},
					ExpectedCount:      1,
					ExpectedOverridden: false,
					QueryInstance:      "extend",
				},
				{
					Rule:               "valueStr as $res",
					ExpectedValues:     []string{extendValueStr},
					ExpectedCount:      1,
					ExpectedOverridden: false,
					QueryInstance:      "overlay",
				},
				{
					Rule:               "A.valueStr as $res",
					ExpectedValues:     []string{baseValueStr},
					ExpectedCount:      1,
					ExpectedOverridden: true,
					QueryInstance:      "base",
				},
				{
					Rule:               "A.valueStr as $res",
					ExpectedValues:     []string{extendValueStr},
					ExpectedCount:      1,
					ExpectedOverridden: false,
					QueryInstance:      "extend",
				},
				{
					Rule:               "A.valueStr as $res",
					ExpectedValues:     []string{extendValueStr},
					ExpectedCount:      1,
					ExpectedOverridden: false,
					QueryInstance:      "overlay",
				},
			},
			ExpectedAggregatedFiles: []string{"A.java", "Main.java"},
		}

		checkOverlayTest(t, config)
	})

	// 额外的 Relocate 测试
	t.Run("test Relocate method", func(t *testing.T) {
		progBase, progExtend, progNameBaseUUID, progNameExtendUUID := InitProgram(t)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progNameBaseUUID)
			ssadb.DeleteProgram(ssadb.GetDB(), progNameExtendUUID)
		}()

		overProg := ssaapi.NewProgramOverLay(progBase, progExtend)
		require.NotNil(t, overProg)

		baseValues := progBase.Ref(valueName)
		require.Equal(t, baseValues.Len(), 1)

		baseValue := baseValues[0]
		relocatedValue := overProg.Relocate(baseValue)
		require.NotNil(t, relocatedValue)

		t.Logf("Original value: %s", baseValue.String())
		t.Logf("Relocated value: %s", relocatedValue.String())

		require.Equalf(t, relocatedValue.GetProgramName(), progNameExtendUUID, "Relocated value should come from extend program")
	})
}

// TestOverlay_CrossLayer_Flow 测试跨层调用图链接
func TestOverlay_CrossLayer_Flow(t *testing.T) {
	t.Run("test cross-layer call graph linking", func(t *testing.T) {
		config := OverlayTestConfig{
			Name: "test cross-layer call graph linking",
			FileSystems: []map[string]string{
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`,
					"Main.java": `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}	
	}`,
					"Main.java": `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`,
				},
			},
			SyntaxFlowRules: []SyntaxFlowRuleCheck{
				{
					Rule:           "println(, * as $arg); $arg #->  as $data",
					ExpectedValues: []string{"Value from A"},
					ExpectedCount:  1,
					QueryInstance:  "base",
					VariableName:   "data",
				},
				{
					Rule:           "println(, * as $arg); $arg #->  as $data",
					ExpectedValues: []string{},
					ExpectedCount:  -1, // 应该为空
					QueryInstance:  "extend",
					VariableName:   "data",
				},
				{
					Rule:           "println(, * as $arg); $arg #->  as $data",
					ExpectedValues: []string{"Value from A"},
					ExpectedCount:  1,
					QueryInstance:  "all",
					VariableName:   "data",
				},
				{
					Rule:           "println(, * as $arg); $arg #->  as $data",
					ExpectedValues: []string{"Value from Extended A"},
					ExpectedCount:  1,
					QueryInstance:  "overlay",
					VariableName:   "data",
				},
			},
		}

		checkOverlayTest(t, config)
	})
}

// InitProgramWithFileChanges 创建测试文件变更的程序（修改、新增、删除）
func InitProgramWithFileChanges(t *testing.T) (progBase *ssaapi.Program, progExtend *ssaapi.Program, progNameBaseUUID string, progNameExtendUUID string) {
	progNameBaseUUID = uuid.NewString()
	progNameExtendUUID = uuid.NewString()

	vf1 := filesys.NewVirtualFs()
	var err error

	t.Logf("Creating new progBase with files: A.java, Main.java, Utils.java")
	vf1.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`)

	vf1.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)

	vf1.AddFile("Utils.java", `
	public class Utils {
		public static void helper() {
			System.out.println("Helper from Base");
		}
	}
	`)

	progBases, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(vf1),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(progNameBaseUUID),
	)
	progBase = progBases[0]
	require.NoError(t, err)
	require.NotNil(t, progBase)
	require.Greater(t, len(progBases), 0, "Should have at least one program")

	t.Logf("Creating new progExtend: override A.java, remove Utils.java, add NewFile.java")
	vf2 := filesys.NewVirtualFs()
	vf2.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}
	}`)

	vf2.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)

	vf2.AddFile("NewFile.java", `
	public class NewFile {
		public static void newMethod() {
			System.out.println("New method from Extend");
		}
	}
	`)

	ctx := context.Background()
	progExtend, err = ssaapi.CompileDiffProgramAndSaveToDB(
		ctx,
		nil, vf2,
		progNameBaseUUID, progNameExtendUUID,
		ssaconfig.JAVA,
	)
	require.NoError(t, err)
	require.NotNil(t, progExtend)

	irProgram := progExtend.Program.GetIrProgram()
	require.NotNil(t, irProgram, "irProgram should exist")
	require.Equal(t, progNameBaseUUID, irProgram.BaseProgramName, "BaseProgramName should be set")
	require.NotEmpty(t, irProgram.FileHashMap, "FileHashMap should be saved in database")

	require.Contains(t, irProgram.FileHashMap, "A.java", "FileHashMap should contain A.java (modified)")
	require.Equal(t, "0", irProgram.FileHashMap["A.java"], "A.java should be marked as modified (0)")
	require.Contains(t, irProgram.FileHashMap, "NewFile.java", "FileHashMap should contain NewFile.java (new)")
	require.Equal(t, "1", irProgram.FileHashMap["NewFile.java"], "NewFile.java should be marked as new (1)")
	require.Contains(t, irProgram.FileHashMap, "Utils.java", "FileHashMap should contain Utils.java (deleted)")
	require.Equal(t, "-1", irProgram.FileHashMap["Utils.java"], "Utils.java should be marked as deleted (-1)")

	return
}

// TestOverlay_FileSystem 测试文件系统聚合功能
func TestOverlay_FileSystem(t *testing.T) {
	t.Run("test file system aggregation", func(t *testing.T) {
		config := OverlayTestConfig{
			Name: "test file system aggregation",
			FileSystems: []map[string]string{
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`,
					"Main.java": `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`,
					"Utils.java": `
	public class Utils {
		public static void helper() {
			System.out.println("Helper from Base");
		}
	}
	`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}
	}`,
					"Main.java": `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`,
					"NewFile.java": `
	public class NewFile {
		public static void newMethod() {
			System.out.println("New method from Extend");
		}
	}
	`,
				},
			},
			SyntaxFlowRules: []SyntaxFlowRuleCheck{
				{
					Rule:           "NewFile as $res",
					ExpectedValues: []string{"NewFile"},
					ExpectedCount:  0, // 至少找到一个
					QueryInstance:  "overlay",
				},
				{
					Rule:           "Main as $res",
					ExpectedValues: []string{"Main"},
					ExpectedCount:  0,
					QueryInstance:  "overlay",
				},
				{
					Rule:           "Utils as $res",
					ExpectedValues: []string{},
					ExpectedCount:  -1, // 应该为空（已删除）
					QueryInstance:  "overlay",
				},
			},
			ExpectedAggregatedFiles: []string{"A.java", "Main.java", "NewFile.java"},
			ExpectedExcludedFiles:   []string{"Utils.java"},
			IsDatabase:              false, // 从数据库加载测试
		}

		checkOverlayTest(t, config)
	})
}

// TestOverlay_FileSystem_FromDataBase 测试文件系统聚合功能（数据库）
func TestOverlay_FileSystem_FromDataBase(t *testing.T) {
	t.Run("test file system aggregation from database", func(t *testing.T) {
		config := OverlayTestConfig{
			Name: "test file system aggregation from database",
			FileSystems: []map[string]string{
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`,
					"Main.java": `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`,
					"Utils.java": `
	public class Utils {
		public static void helper() {
			System.out.println("Helper from Base");
		}
	}
	`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}
	}`,
					"Main.java": `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`,
					"NewFile.java": `
	public class NewFile {
		public static void newMethod() {
			System.out.println("New method from Extend");
		}
	}
	`,
				},
			},
			SyntaxFlowRules: []SyntaxFlowRuleCheck{
				{
					Rule:           "NewFile as $res",
					ExpectedValues: []string{"NewFile"},
					ExpectedCount:  0,
					QueryInstance:  "overlay",
				},
				{
					Rule:           "Main as $res",
					ExpectedValues: []string{"Main"},
					ExpectedCount:  0,
					QueryInstance:  "overlay",
				},
				{
					Rule:           "Utils as $res",
					ExpectedValues: []string{},
					ExpectedCount:  -1,
					QueryInstance:  "overlay",
				},
			},
			ExpectedAggregatedFiles: []string{"A.java", "Main.java", "NewFile.java"},
			ExpectedExcludedFiles:   []string{"Utils.java"},
			LoadFromDatabase:        true,
		}

		checkOverlayTest(t, config)
	})
}
