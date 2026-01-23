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

// OverlayRefCheck 定义 overlay Ref 检查配置
type OverlayRefCheck struct {
	ClassName       string // 类名
	ShouldExist     bool   // 是否应该存在
	ExpectedInLayer int    // 预期在哪个 layer（0 表示不检查）
	ExpectedValue   string // 预期的值（包含此字符串，空表示不检查）
}

// OverlayCompileTestConfig 统一的 overlay 编译测试配置结构
type OverlayCompileTestConfig struct {
	Name                    string                // 测试名称
	FileSystems             []map[string]string   // 多个文件系统，第一个是基础，其他是增量修改
	SyntaxFlowRules         []SyntaxFlowRuleCheck // syntaxflow 规则和预期结果
	ExpectedAggregatedFiles []string              // 预期的聚合文件系统文件列表（应该存在的文件）
	ExpectedExcludedFiles   []string              // 预期排除的文件列表（不应该存在的文件）
	ExpectedFileCount       int                   // 预期的文件数量（0表示不检查）
	RefChecks               []OverlayRefCheck     // overlay Ref 检查配置
	ExpectedLayerCount      int                   // 预期的 layer 数量（0表示不检查）
	TestDatabaseLoad        bool                  // 是否测试从数据库加载
	TestMultipleLayers      bool                  // 是否测试多层 overlay
}

// check 统一的 overlay 编译测试检查函数（简化入口）
// 输入配置结构包含：
// 1. 多个文件系统（默认第一个为初始化，其他的都是修改）
// 2. syntaxflow规则
// 3. 预期的聚合文件系统
// 4. 预期的结果
func check(t *testing.T, config OverlayCompileTestConfig) {
	checkOverlayCompileTest(t, config)
}

// checkOverlayCompileTest 统一的 overlay 编译测试检查函数
func checkOverlayCompileTest(t *testing.T, config OverlayCompileTestConfig) {
	ctx := context.Background()

	// 验证配置：至少需要一个文件系统（基础）
	require.GreaterOrEqual(t, len(config.FileSystems), 1, "至少需要一个文件系统（基础）")
	if len(config.FileSystems) == 1 {
		t.Logf("警告：只有一个文件系统，将只创建基础程序，不会创建 overlay")
	}

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
	require.Greater(t, len(basePrograms), 0)

	// Step 2: 创建增量程序（如果有多个文件系统）
	var diffPrograms []*ssaapi.Program
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
		diffPrograms = append(diffPrograms, diffProgs[0])
	}

	// Step 3: 验证最后一个增量程序的 overlay
	if len(diffPrograms) > 0 {
		lastDiffProgram := diffPrograms[len(diffPrograms)-1]

		// 验证 overlay 已创建
		overlay := lastDiffProgram.GetOverlay()
		require.NotNil(t, overlay, "overlay should be created")

		// 验证 layer 数量
		if config.ExpectedLayerCount > 0 {
			require.Equal(t, config.ExpectedLayerCount, len(overlay.Layers), "overlay should have %d layers", config.ExpectedLayerCount)
		} else {
			require.GreaterOrEqual(t, len(overlay.Layers), len(config.FileSystems), "overlay should have at least %d layers", len(config.FileSystems))
		}

		// 验证数据库中的 overlay 信息
		irProgram := lastDiffProgram.Program.GetIrProgram()
		require.NotNil(t, irProgram, "irProgram should exist")
		require.True(t, irProgram.IsOverlay, "IsOverlay should be true in database")
		require.NotEmpty(t, irProgram.OverlayLayers, "OverlayLayers should be saved in database")

		// 验证 layer 顺序
		if len(irProgram.OverlayLayers) > 0 {
			require.Equal(t, programNames[0], irProgram.OverlayLayers[0], "base program should be the first layer")
		}

		// 验证所有 layer 的 program 都已保存到数据库
		for _, layerName := range irProgram.OverlayLayers {
			layerProg, err := ssaapi.FromDatabase(layerName)
			require.NoError(t, err, "layer program %s should be saved to database", layerName)
			require.NotNil(t, layerProg, "layer program %s should not be nil", layerName)
		}

		// Step 4: 检查聚合文件系统
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

		// Step 5: 检查文件数量
		if config.ExpectedFileCount > 0 {
			fileCount := overlay.GetFileCount()
			require.Equal(t, config.ExpectedFileCount, fileCount, "overlay should have %d files", config.ExpectedFileCount)
		}

		// Step 6: 检查 Ref 查询
		for _, refCheck := range config.RefChecks {
			values := overlay.Ref(refCheck.ClassName)
			if refCheck.ShouldExist {
				require.NotEmpty(t, values, "overlay should contain class %s", refCheck.ClassName)
			} else {
				require.Empty(t, values, "overlay should not contain class %s", refCheck.ClassName)
			}
		}

		// Step 7: 检查 syntaxflow 规则
		for _, ruleCheck := range config.SyntaxFlowRules {
			var queryInstance ssaapi.SyntaxFlowQueryInstance
			queryInstance = overlay // 默认使用 overlay

			res, err := queryInstance.SyntaxFlowWithError(ruleCheck.Rule)
			require.NoError(t, err)
			require.NotNil(t, res)

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
		}

		// Step 8: 测试从数据库加载（如果需要）
		if config.TestDatabaseLoad {
			reloadedDiffProgram, err := ssaapi.FromDatabase(programNames[len(programNames)-1])
			require.NoError(t, err)
			require.NotNil(t, reloadedDiffProgram)

			reloadedOverlay := reloadedDiffProgram.GetOverlay()
			require.NotNil(t, reloadedOverlay, "overlay should be loaded from database")
			require.GreaterOrEqual(t, len(reloadedOverlay.Layers), len(config.FileSystems), "reloaded overlay should have at least %d layers", len(config.FileSystems))

			// 验证 layer 的 program names
			layerNames := reloadedOverlay.GetLayerProgramNames()
			require.Equal(t, len(config.FileSystems), len(layerNames), "reloaded overlay should have %d layer names", len(config.FileSystems))

			// 验证每个 layer 的 program 都已正确加载
			for i, layer := range reloadedOverlay.Layers {
				require.NotNil(t, layer, "layer %d should not be nil", i)
				require.NotNil(t, layer.Program, "layer %d program should not be nil", i)
				require.NotEmpty(t, layer.Program.GetProgramName(), "layer %d program should have a name", i)
			}

			// 验证所有 layer 的 program 都在数据库中
			reloadedIrProgram := reloadedDiffProgram.Program.GetIrProgram()
			require.NotNil(t, reloadedIrProgram)
			require.True(t, reloadedIrProgram.IsOverlay)

			for _, layerName := range reloadedIrProgram.OverlayLayers {
				layerIrProgram, err := ssadb.GetProgram(layerName, ssadb.Application)
				require.NoError(t, err, "layer program %s should exist in database", layerName)
				require.NotNil(t, layerIrProgram, "layer program %s should not be nil", layerName)
				require.Equal(t, layerName, layerIrProgram.ProgramName, "layer program name should match")
			}
		}
	}
}

func TestOverlaySaveAndLoadFromDatabase(t *testing.T) {
	// 测试 overlay 保存和加载的基本功能
	t.Run("test overlay save and load from database", func(t *testing.T) {
		check(t, OverlayCompileTestConfig{
			Name: "test overlay save and load from database",
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
			ExpectedLayerCount:      2,
			ExpectedAggregatedFiles: []string{"A.java", "Main.java", "NewFile.java"},
			ExpectedExcludedFiles:   []string{"Utils.java"},
			ExpectedFileCount:       3,
			RefChecks: []OverlayRefCheck{
				{ClassName: "A", ShouldExist: true},
				{ClassName: "NewFile", ShouldExist: true},
				{ClassName: "Main", ShouldExist: true},
				{ClassName: "Utils", ShouldExist: false},
			},
			TestDatabaseLoad: true,
		})
	})
}

func TestOverlayWithMultipleLayers(t *testing.T) {
	// 测试多层 overlay
	// 注意：这个测试需要先创建第一个 overlay，然后基于它创建第二个
	// 由于 check 函数会自动处理清理，我们需要分两步进行

	// 第一步：创建基础 overlay
	config1 := OverlayCompileTestConfig{
		Name: "test first overlay",
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
				"NewFile.java": `
	public class NewFile {
		public static void newMethod() {
			System.out.println("New method from Extend");
		}
	}
	`,
			},
		},
		ExpectedLayerCount: 2,
		TestDatabaseLoad:   false, // 不清理，保留在数据库中供下一步使用
	}

	// 手动执行第一步，保留程序名称
	ctx := context.Background()
	programNames := make([]string, len(config1.FileSystems))
	for i := range programNames {
		programNames[i] = uuid.NewString()
	}

	defer func() {
		for _, name := range programNames {
			ssadb.DeleteProgram(ssadb.GetDB(), name)
		}
	}()

	// 创建基础程序
	baseFS := filesys.NewVirtualFs()
	for path, content := range config1.FileSystems[0] {
		baseFS.AddFile(path, content)
	}

	basePrograms, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(programNames[0]),
	)
	require.NoError(t, err)
	require.NotNil(t, basePrograms)
	require.Greater(t, len(basePrograms), 0)

	// 创建第一个增量程序
	diffFS1 := filesys.NewVirtualFs()
	for path, content := range config1.FileSystems[1] {
		diffFS1.AddFile(path, content)
	}

	diffPrograms1, err := ssaapi.ParseProjectWithIncrementalCompile(
		diffFS1,
		programNames[0], programNames[1],
		ssaconfig.JAVA,
		ssaapi.WithContext(ctx),
	)
	require.NoError(t, err)
	require.NotNil(t, diffPrograms1)
	require.Greater(t, len(diffPrograms1), 0)

	// 第二步：基于第一个增量程序创建第二个增量程序
	diffProgramName2 := uuid.NewString()
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgramName2)
	}()

	diffFS2 := filesys.NewVirtualFs()
	diffFS2.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Second Extend";
		public String getValue() {
			return "Value from Second Extended A";
		}
	}`)
	diffFS2.AddFile("AnotherFile.java", `
	public class AnotherFile {
		public static void anotherMethod() {
			System.out.println("Another method");
		}
	}
	`)

	diffPrograms2, err := ssaapi.ParseProjectWithIncrementalCompile(
		diffFS2,
		programNames[1], diffProgramName2,
		ssaconfig.JAVA,
		ssaapi.WithContext(ctx),
	)
	require.NoError(t, err)
	require.NotNil(t, diffPrograms2)
	require.Greater(t, len(diffPrograms2), 0)
	diffProgram2 := diffPrograms2[0]

	// 验证第二个 overlay 包含 3 个 layers
	overlay2 := diffProgram2.GetOverlay()
	require.NotNil(t, overlay2, "second overlay should be created")
	require.GreaterOrEqual(t, len(overlay2.Layers), 3, "second overlay should have at least 3 layers")

	// 验证数据库中的 overlay 信息
	irProgram2 := diffProgram2.Program.GetIrProgram()
	require.NotNil(t, irProgram2)
	require.True(t, irProgram2.IsOverlay, "second overlay IsOverlay should be true")
	require.GreaterOrEqual(t, len(irProgram2.OverlayLayers), 3, "second overlay should have at least 3 layer names")

	// 从数据库重新加载并验证
	reloadedDiffProgram2, err := ssaapi.FromDatabase(diffProgramName2)
	require.NoError(t, err)
	require.NotNil(t, reloadedDiffProgram2)

	reloadedOverlay2 := reloadedDiffProgram2.GetOverlay()
	require.NotNil(t, reloadedOverlay2, "second overlay should be loaded from database")
	require.GreaterOrEqual(t, len(reloadedOverlay2.Layers), 3, "reloaded second overlay should have at least 3 layers")

	// 验证可以查找最新的类
	classA := reloadedOverlay2.Ref("A")
	require.NotEmpty(t, classA, "overlay should contain class A (from latest layer)")

	anotherFileClass := reloadedOverlay2.Ref("AnotherFile")
	require.NotEmpty(t, anotherFileClass, "overlay should contain class AnotherFile (from latest layer)")
}

// TestOverlayWithTwiceIncrementalCompile 测试二次增量编译的场景
// 第一次增量编译：base program -> diff program 1
// 第二次增量编译：diff program 1 -> diff program 2
func TestOverlayWithTwiceIncrementalCompile(t *testing.T) {
	// 测试 TestDatabaseLoad = false 的情况
	t.Run("test twice incremental compile without database load", func(t *testing.T) {
		check(t, OverlayCompileTestConfig{
			Name: "test twice incremental compile without database load",
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
		static string valueStr = "Value from Diff1";
		public String getValue() {
			return "Value from Modified A in Diff1";
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
					"B.java": `
	public class B {
		public static void process() {
			System.out.println("Process from B");
		}
	}
	`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Diff2";
		public String getValue() {
			return "Value from Modified A in Diff2";
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
					"C.java": `
	public class C {
		public static void compute() {
			System.out.println("Compute from C");
		}
	}
	`,
				},
			},
			ExpectedLayerCount:      3,
			ExpectedAggregatedFiles: []string{"A.java", "Main.java", "C.java"},
			ExpectedExcludedFiles:   []string{"Utils.java", "B.java"},
			ExpectedFileCount:       3,
			RefChecks: []OverlayRefCheck{
				{ClassName: "A", ShouldExist: true},
				{ClassName: "C", ShouldExist: true},
				{ClassName: "Main", ShouldExist: true},
				{ClassName: "Utils", ShouldExist: false},
				{ClassName: "B", ShouldExist: false},
			},
			TestDatabaseLoad: false,
		})
	})

	// 测试 TestDatabaseLoad = true 的情况
	t.Run("test twice incremental compile with database load", func(t *testing.T) {
		check(t, OverlayCompileTestConfig{
			Name: "test twice incremental compile with database load",
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
		static string valueStr = "Value from Diff1";
		public String getValue() {
			return "Value from Modified A in Diff1";
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
					"B.java": `
	public class B {
		public static void process() {
			System.out.println("Process from B");
		}
	}
	`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Diff2";
		public String getValue() {
			return "Value from Modified A in Diff2";
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
					"C.java": `
	public class C {
		public static void compute() {
			System.out.println("Compute from C");
		}
	}
	`,
				},
			},
			ExpectedLayerCount:      3,
			ExpectedAggregatedFiles: []string{"A.java", "Main.java", "C.java"},
			ExpectedExcludedFiles:   []string{"Utils.java", "B.java"},
			ExpectedFileCount:       3,
			RefChecks: []OverlayRefCheck{
				{ClassName: "A", ShouldExist: true},
				{ClassName: "C", ShouldExist: true},
				{ClassName: "Main", ShouldExist: true},
				{ClassName: "Utils", ShouldExist: false},
				{ClassName: "B", ShouldExist: false},
			},
			TestDatabaseLoad: true,
		})
	})
}
