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
	progBase, progExtend, progNameBaseUUID, progNameExtendUUID := InitProgram(t)
	require.NotNil(t, progBase)
	require.NotNil(t, progExtend)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progNameBaseUUID)
		ssadb.DeleteProgram(ssadb.GetDB(), progNameExtendUUID)
	}()

	t.Logf("progBase: %v", progBase.GetProgramName())
	t.Logf("progExtend: %v", progExtend.GetProgramName())

	overProg := ssaapi.NewProgramOverLay(progBase, progExtend)
	require.NotNil(t, overProg)

	t.Run("test layer structure built correctly", func(t *testing.T) {
		layerCount := overProg.GetLayerCount()
		fileCount := overProg.GetFileCount()
		t.Logf("Layer count: %d", layerCount)
		t.Logf("Unique file count: %d", fileCount)
		require.Equal(t, layerCount, 2, "Should have 2 layers")

		layer1Files := overProg.GetFilesInLayer(1)
		t.Logf("Layer1 files: %v", layer1Files)
		require.Greater(t, len(layer1Files), 0, "Layer1 should have files")

		layer2Files := overProg.GetFilesInLayer(2)
		t.Logf("Layer2 files: %v", layer2Files)
		require.Greater(t, len(layer2Files), 0, "Layer2 should have files")
	})

	check := func(p ssaapi.SyntaxFlowQueryInstance, rule string, expectOverridden bool, expectData string) {
		res, err := p.SyntaxFlowWithError(rule)
		res.Show()
		require.NoError(t, err)
		values := res.GetAllValuesChain()
		require.NotEmpty(t, values, "Should find values for rule: %s", rule)
		require.Len(t, values, 1, "Should find exactly one value for rule: %s", rule)
		v := values[0]
		isOverridden := overProg.IsOverridden(v)
		require.Equalf(t, expectOverridden, isOverridden, "Value %s overridden status should be %v", v.String(), expectOverridden)
		require.Containsf(t, v.String(), expectData, "Value %s data should contain %s", v.String(), expectData)
	}

	t.Run("test IsOverridden method : valueStr", func(t *testing.T) {
		rule := "valueStr as $res"
		check(progBase, rule, true, baseValueStr)
		check(progExtend, rule, false, extendValueStr)
		check(overProg, rule, false, extendValueStr)
	})

	t.Run("test IsOverridden method : A.valueStr", func(t *testing.T) {
		rule := "A.valueStr as $res"
		check(progBase, rule, true, baseValueStr)
		check(progExtend, rule, false, extendValueStr)
		check(overProg, rule, false, extendValueStr)
	})

	t.Run("test Relocate method", func(t *testing.T) {
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
	progBase, progExtend, progNameBaseUUID, progNameExtendUUID := InitProgram(t)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progNameBaseUUID)
		ssadb.DeleteProgram(ssadb.GetDB(), progNameExtendUUID)
	}()

	overProg := ssaapi.NewProgramOverLay(progBase, progExtend)
	require.NotNil(t, overProg)

	rule := "println(, * as $arg); $arg #->  as $data"
	check := func(p ssaapi.SyntaxFlowQueryInstance, expectData string) {
		res, err := p.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		res.Show()
		values := res.GetValues("data")
		require.NotEmpty(t, values, "Should find values for rule: %s", rule)
		require.Len(t, values, 1, "Should find exactly one value for rule: %s", rule)
		v := values[0]
		require.Containsf(t, v.String(), expectData, "Value %s data should contain %s", v.String(), expectData)
	}

	t.Run("test Cross-Layer Call Graph linking - baseline", func(t *testing.T) {
		check(progBase, "Value from A")
	})

	t.Run("test extend ", func(t *testing.T) {
		res, err := progExtend.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		res.Show()
		values := res.GetAllValuesChain()
		require.Empty(t, values, "Should not find values in extend program alone")
	})

	t.Run("test Normal-Program Call Graph linking", func(t *testing.T) {
		check(ssaapi.Programs{progBase, progExtend}, "Value from A")
	})

	t.Run("test Cross-Layer Call Graph linking - overlay", func(t *testing.T) {
		check(overProg, "Value from Extended A")
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

func allFileSystemCheck(t *testing.T, overProg *ssaapi.ProgramOverLay, progBase, progExtend *ssaapi.Program) {
	overProg.Show()
	t.Run("test file count in overlay", func(t *testing.T) {
		layer1Files := overProg.GetFilesInLayer(1)
		layer2Files := overProg.GetFilesInLayer(2)
		t.Logf("Layer1 files: %v", layer1Files)
		t.Logf("Layer2 files: %v", layer2Files)

		require.GreaterOrEqual(t, len(layer1Files), 3, "Layer1 should have at least 3 files")
		require.GreaterOrEqual(t, len(layer2Files), 2, "Layer2 should have at least 2 files")

		hasNewFile := false
		for _, file := range layer2Files {
			if file == "/NewFile.java" || file == "NewFile.java" {
				hasNewFile = true
				break
			}
		}
		require.True(t, hasNewFile, "Layer2 should contain NewFile.java")

		totalFiles := overProg.GetFileCount()
		t.Logf("Total unique files in overlay: %d", totalFiles)
		require.Equal(t, 3, totalFiles, "Overlay should have exactly 3 unique files: A.java, Main.java, NewFile.java")

		aggFS := overProg.GetAggregatedFileSystem()
		require.NotNil(t, aggFS, "AggregatedFS should not be nil")

		overlayFiles := make(map[string]bool)
		filesys.Recursive(".", filesys.WithFileSystem(aggFS), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			if !info.IsDir() {
				// 规范化路径（去掉前导斜杠或保留）
				normalizedPath := s
				if len(normalizedPath) > 0 && normalizedPath[0] == '/' {
					normalizedPath = normalizedPath[1:]
				}
				overlayFiles[normalizedPath] = true
			}
			return nil
		}))

		t.Logf("Files in aggregated file system: %v", overlayFiles)

		require.True(t, overlayFiles["A.java"], "Aggregated FS should contain A.java")
		require.True(t, overlayFiles["Main.java"], "Aggregated FS should contain Main.java")
		require.True(t, overlayFiles["NewFile.java"], "Aggregated FS should contain NewFile.java")
		require.False(t, overlayFiles["Utils.java"], "Aggregated FS should not contain Utils.java (deleted)")
	})

	t.Run("test overlay can access new file", func(t *testing.T) {
		rule := "NewFile as $res"
		res, err := overProg.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.NotNil(t, res)

		values := res.GetAllValuesChain()
		require.NotEmpty(t, values, "Should find NewFile class in overlay")
		t.Logf("Found NewFile: %v", values)
	})

	t.Run("test overlay can access base file not in extend", func(t *testing.T) {
		rule := "Main as $res"
		res, err := overProg.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.NotNil(t, res)

		values := res.GetAllValuesChain()
		require.NotEmpty(t, values, "Should find Main class from base layer")
		t.Logf("Found Main: %v", values)
	})

	t.Run("test deleted file not accessible in overlay", func(t *testing.T) {
		rule := "Utils as $res"
		res, err := progBase.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		baseValues := res.GetAllValuesChain()
		require.NotEmpty(t, baseValues, "Base should have Utils class")

		res2, err := progExtend.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		extendValues := res2.GetAllValuesChain()
		require.Empty(t, extendValues, "Extend should not have Utils class")

		res3, err := overProg.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		overlayValues := res3.GetAllValuesChain()
		require.Empty(t, overlayValues, "Overlay should not have Utils class (deleted)")
		t.Logf("Overlay Utils query result: %v", overlayValues)
	})

	t.Run("test overlay file system aggregation", func(t *testing.T) {
		aggFS := overProg.GetAggregatedFileSystem()
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

		t.Logf("Files in aggregated file system: %v", overlayFiles)
		t.Logf("Total files in aggregated FS: %d", len(overlayFiles))

		require.True(t, overlayFiles["A.java"], "Aggregated FS should contain A.java")
		require.True(t, overlayFiles["Main.java"], "Aggregated FS should contain Main.java")
		require.True(t, overlayFiles["NewFile.java"], "Aggregated FS should contain NewFile.java")
		require.False(t, overlayFiles["Utils.java"], "Aggregated FS should not contain Utils.java (deleted)")
	})

	t.Run("test overlay file override", func(t *testing.T) {
		// 先查询 A 类
		ruleA := "A as $a"
		resA, err := overProg.SyntaxFlowWithError(ruleA, ssaapi.QueryWithEnableDebug(true))
		require.NoError(t, err)
		require.NotNil(t, resA)

		valuesA := resA.GetAllValuesChain()
		require.NotEmpty(t, valuesA, "Should find A class in overlay")

		// 打印 A 类信息
		for i, aVal := range valuesA {
			t.Logf("A class %d: %s", i, aVal.String())
			t.Logf("A class %d program: %s", i, aVal.GetProgramName())
		}

		// TODO: 有概率问题
		// 大概率查找到Make，小概率查找到Parameter-this
		t.Skip()

		// 从 A 中查找 valueStr 成员
		if len(valuesA) > 0 {
			aVal := valuesA[0]
			// 使用 GetMembersByString 查找成员
			valueStrVal, ok := aVal.GetMembersByString("valueStr")
			require.True(t, ok, "Should find valueStr member in A class")
			require.NotNil(t, valueStrVal, "valueStr member should not be nil")

			valueStr := valueStrVal.String()
			require.Contains(t, valueStr, "Value from Extend", "Overlay should return value from Extend layer")
			t.Logf("Overlay valueStr: %s", valueStr)
		}
	})

	t.Run("test SQL level file exclusion optimization", func(t *testing.T) {
		// 创建测试场景：两个 layer 都有同名文件，但内容不同
		progName1 := uuid.NewString()
		progName2 := uuid.NewString()

		vf1 := filesys.NewVirtualFs()
		vf1.AddFile("Test.java", `
		public class Test {
			public static String A = "Layer1 Value";
		}
		`)

		p1, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(vf1),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(progName1),
		)
		require.NoError(t, err)
		require.NotNil(t, p1)
		require.Greater(t, len(p1), 0)

		prog1, err := ssaapi.FromDatabase(progName1)
		require.NoError(t, err)
		require.NotNil(t, prog1)

		vf2 := filesys.NewVirtualFs()
		vf2.AddFile("Test.java", `
		public class Test {
			public static String A = "Layer2 Value";
		}
		`)

		ctx := context.Background()
		prog2, err := ssaapi.CompileDiffProgramAndSaveToDB(
			ctx,
			nil, vf2,
			progName1, progName2,
			ssaconfig.JAVA,
		)
		require.NoError(t, err)
		require.NotNil(t, prog2)

		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progName1)
			ssadb.DeleteProgram(ssadb.GetDB(), progName2)
		}()

		overProg := ssaapi.NewProgramOverLay(prog1, prog2)
		require.NotNil(t, overProg)

		// 使用 ExactMatch 测试（会触发 queryMatch）
		// 查询变量 A，应该只在 Layer2 中找到（因为 Layer2 覆盖了 Layer1）
		matched, vals, err := overProg.ExactMatch(ctx, ssadb.NameMatch, "A")
		require.NoError(t, err)
		require.True(t, matched, "Should find variable A")
		require.NotNil(t, vals)

		// Extract Values from ValueOperator (which is *sfvm.ValueList)
		values := ssaapi.SyntaxFlowVariableToValues(vals)
		require.NotEmpty(t, values, "Should find at least one value")

		// 应该只返回 Layer2 的值（Layer1 的值被 Layer2 覆盖）
		// 由于 SQL 层面的优化，当在 Layer2 中找到 Test.java 中的变量 A 后，
		// Layer1 的 SQL 查询应该排除 Test.java 文件，避免重复查询
		foundLayer2 := false
		for _, v := range values {
			progName := v.GetProgramName()
			if progName == progName2 {
				foundLayer2 = true
				require.Contains(t, v.String(), "Layer2 Value", "Should return value from Layer2")
			}
		}
		require.True(t, foundLayer2, "Should find value from Layer2")

		// 验证只返回一个值（Layer2 的值），而不是两个值
		// 如果优化没有生效，可能会返回两个值（Layer1 和 Layer2 各一个）
		// 但由于 SQL 层面的文件排除优化，Layer1 的查询会排除 Test.java，所以应该只返回 Layer2 的值
		require.LessOrEqual(t, len(values), 1, "Should return at most one value (optimization: exclude files in subsequent layers)")

		t.Logf("Found %d values for variable A (SQL optimization: files excluded in subsequent layers)", len(values))
		for i, v := range values {
			t.Logf("Value %d: %s (from program: %s)", i, v.String(), v.GetProgramName())
		}
	})
}

// TestOverlay_FileSystem 测试文件系统聚合功能
func TestOverlay_FileSystem(t *testing.T) {
	// for i := 0; i < 50; i++ {
	progBase, progExtend, progNameBaseUUID, progNameExtendUUID := InitProgramWithFileChanges(t)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progNameBaseUUID)
		ssadb.DeleteProgram(ssadb.GetDB(), progNameExtendUUID)
	}()

	overProg := ssaapi.NewProgramOverLay(progBase, progExtend)
	require.NotNil(t, overProg)
	allFileSystemCheck(t, overProg, progBase, progExtend)
	// }
}

// TestOverlay_FileSystem_FromDataBase 测试文件系统聚合功能（数据库）
func TestOverlay_FileSystem_FromDataBase(t *testing.T) {
	// for i := 0; i < 50; i++ {
	_, _, progNameBaseUUID, progNameExtendUUID := InitProgramWithFileChanges(t)
	progBase, err := ssaapi.FromDatabase(progNameBaseUUID)
	require.NoError(t, err)
	progExtend, err := ssaapi.FromDatabase(progNameExtendUUID)
	require.NoError(t, err)

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progNameBaseUUID)
		ssadb.DeleteProgram(ssadb.GetDB(), progNameExtendUUID)
	}()

	overProg := ssaapi.NewProgramOverLay(progBase, progExtend)
	require.NotNil(t, overProg)
	allFileSystemCheck(t, overProg, progBase, progExtend)
	// }
}
