package ssaapi_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// checkDiffProgramMetadataConfig 配置结构体，用于检查差量程序的元数据
type checkDiffProgramMetadataConfig struct {
	// BaseProgramName 期望的基础程序名称
	BaseProgramName string
	// ExpectedFiles 期望的文件及其 hash 状态映射
	// key: 文件路径, value: hash 状态 (-1: 删除, 0: 修改, 1: 新增)
	ExpectedFiles map[string]int
	// ExcludedFiles 不应该出现在 FileHashMap 中的文件列表
	ExcludedFiles []string
}

// checkDiffProgramMetadata 检查差量程序的元数据（BaseProgramName 和 FileHashMap）
func checkDiffProgramMetadata(t *testing.T, diffProgram *ssaapi.Program, config checkDiffProgramMetadataConfig) {
	require.NotNil(t, diffProgram.Program, "diffProgram.Program should not be nil")
	require.Equal(t, config.BaseProgramName, diffProgram.Program.BaseProgramName, "BaseProgramName should be set in ssa.Program struct")
	require.NotNil(t, diffProgram.Program.FileHashMap, "FileHashMap should be set in ssa.Program struct")
	require.NotEmpty(t, diffProgram.Program.FileHashMap, "FileHashMap should not be empty")

	// 验证 FileHashMap 包含预期的文件变更
	for filePath, expectedHash := range config.ExpectedFiles {
		require.Contains(t, diffProgram.Program.FileHashMap, filePath, "FileHashMap should contain %s", filePath)
		require.Equal(t, expectedHash, diffProgram.Program.FileHashMap[filePath], "%s should be marked as %d", filePath, expectedHash)
	}

	// 验证不应该出现在 FileHashMap 中的文件
	for _, filePath := range config.ExcludedFiles {
		require.NotContains(t, diffProgram.Program.FileHashMap, filePath, "%s should not be in FileHashMap", filePath)
	}
}

// checkDiffProgramMetadataInDB 检查数据库中保存的差量程序元数据
// 用于检查 ssadb.IrProgram 中的字段（FileHashMap 是 StringMap 格式）
func checkDiffProgramMetadataInDB(t *testing.T, irProgram *ssadb.IrProgram, config checkDiffProgramMetadataConfig) {
	require.Equal(t, config.BaseProgramName, irProgram.BaseProgramName, "BaseProgramName should be set in database")
	require.NotEmpty(t, irProgram.FileHashMap, "FileHashMap should be saved in database")

	// 验证 FileHashMap 包含预期的文件变更（数据库中的格式是 string）
	for filePath, expectedHash := range config.ExpectedFiles {
		require.Contains(t, irProgram.FileHashMap, filePath, "FileHashMap in database should contain %s", filePath)
		require.Equal(t, expectedHash, parseHashFromString(irProgram.FileHashMap[filePath]), "%s should be marked as %d in database", filePath, expectedHash)
	}

	// 验证不应该出现在 FileHashMap 中的文件
	for _, filePath := range config.ExcludedFiles {
		require.NotContains(t, irProgram.FileHashMap, filePath, "%s should not be in FileHashMap in database", filePath)
	}
}

// parseHashFromString 将字符串转换为 int（用于解析数据库中的 FileHashMap）
func parseHashFromString(s string) int {
	var hash int
	if _, err := fmt.Sscanf(s, "%d", &hash); err != nil {
		return 0
	}
	return hash
}

func TestCompileDiffProgramAndSaveToDB(t *testing.T) {
	baseProgramName := uuid.NewString()
	diffProgramName := uuid.NewString()

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgramName)
	}()

	// 创建基础文件系统
	baseFS := filesys.NewVirtualFs()
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
	baseFS.AddFile("Utils.java", `
	public class Utils {
		public static void helper() {
			System.out.println("Helper from Utils");
		}
	}
	`)

	// 创建新文件系统（修改 A.java，删除 Utils.java，新增 B.java）
	newFS := filesys.NewVirtualFs()
	newFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Diff";
		public String getValue() {
			return "Value from Modified A";
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
	newFS.AddFile("B.java", `
	public class B {
		public static void process() {
			System.out.println("Process from B");
		}
	}
	`)

	t.Run("Step 1: Compile base program", func(t *testing.T) {
		t.Logf("Compiling base program: %s", baseProgramName)
		basePrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(baseFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(baseProgramName),
		)
		require.NoError(t, err)
		require.NotNil(t, basePrograms)
		require.Greater(t, len(basePrograms), 0, "Should have at least one program")
	})

	var diffProgram *ssaapi.Program
	t.Run("Step 2: Compile diff program", func(t *testing.T) {
		t.Logf("Compiling diff program: %s", diffProgramName)
		ctx := context.Background()
		var err error
		diffProgram, err = ssaapi.CompileDiffProgramAndSaveToDB(
			ctx,
			nil, newFS,
			baseProgramName, diffProgramName,
			ssaconfig.JAVA,
		)
		require.NoError(t, err)
		require.NotNil(t, diffProgram)
	})

	t.Run("Step 3: Verify diff program metadata in memory", func(t *testing.T) {
		t.Logf("Checking diff program metadata in memory")
		checkDiffProgramMetadata(t, diffProgram, checkDiffProgramMetadataConfig{
			BaseProgramName: baseProgramName,
			ExpectedFiles: map[string]int{
				"A.java":     0,  // 修改
				"B.java":     1,  // 新增
				"Utils.java": -1, // 删除（注意：删除的文件不会出现在 diffProgram 中，但会在 FileHashMap 中标记为 -1）
			},
			ExcludedFiles: []string{"Main.java"}, // Main.java 没有变化，不应该在 FileHashMap 中
		})
	})

	t.Run("Step 4: Verify diff program metadata in database", func(t *testing.T) {
		t.Logf("Loading diff program from database")
		irProg, err := ssadb.GetProgram(diffProgramName, ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, irProg)
		checkDiffProgramMetadataInDB(t, irProg, checkDiffProgramMetadataConfig{
			BaseProgramName: baseProgramName,
			ExpectedFiles: map[string]int{
				"A.java":     0,
				"B.java":     1,
				"Utils.java": -1,
			},
			ExcludedFiles: []string{"Main.java"},
		})
	})

	t.Run("Step 5: Verify diff program contains only changed files", func(t *testing.T) {
		t.Logf("Verifying diff program contains only changed files")
		fileCount := 0
		diffProgram.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
			fileCount++
			normalizedPath := normalizeFilePathForTest(filePath)
			t.Logf("  Found file in diff program: %s", normalizedPath)
			// diffProgram 应该只包含修改和新增的文件
			require.Contains(t, []string{"A.java", "B.java"}, normalizedPath, "diffProgram should only contain modified or new files")
			return true
		})
		require.Equal(t, 2, fileCount, "diffProgram should contain exactly 2 files (A.java modified, B.java new)")
	})

	t.Run("Step 6: Verify diff program compilation correctness", func(t *testing.T) {
		t.Logf("Verifying diff program compilation correctness")
		// 检查修改的文件
		aClass := diffProgram.Ref("A")
		require.NotEmpty(t, aClass, "diffProgram should contain class A (modified)")

		// 检查新增的文件
		bClass := diffProgram.Ref("B")
		require.NotEmpty(t, bClass, "diffProgram should contain class B (new)")

		// 检查删除的文件（不应该出现在 diffProgram 中）
		utilsClass := diffProgram.Ref("Utils")
		require.Empty(t, utilsClass, "diffProgram should not contain class Utils (deleted)")
	})
}

// normalizeFilePathForTest 规范化文件路径用于测试（去掉 program name 前缀）
// 输入格式可能是: /d00c28ac-28e7-4f24-947c-8dc854e6161e/A.java 或 /programName/folder/file.java
// 输出格式: A.java 或 folder/file.java
func normalizeFilePathForTest(filePath string) string {
	if filePath == "" {
		return ""
	}

	path := strings.TrimPrefix(filePath, "/")
	if path == "" {
		return filePath
	}
	firstSlashIndex := strings.Index(path, "/")
	if firstSlashIndex == -1 {
		return filePath
	}
	result := path[firstSlashIndex+1:]
	return result
}

// TestIncrementalCompile_Twice 测试两次增量编译的场景
// 第一次增量编译：base program -> diff program 1
// 第二次增量编译：diff program 1 -> diff program 2
func TestIncrementalCompile_Twice(t *testing.T) {
	// for i := 0; i < 50; i++ {
	baseProgramName := uuid.NewString()
	diffProgram1Name := uuid.NewString()
	diffProgram2Name := uuid.NewString()

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgram1Name)
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgram2Name)
	}()

	// ========== Step 1: 创建并编译基础程序 ==========
	t.Logf("Step 1: Creating and compiling base program: %s", baseProgramName)
	baseFS := filesys.NewVirtualFs()
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
	baseFS.AddFile("Utils.java", `
	public class Utils {
		public static void helper() {
			System.out.println("Helper from Utils");
		}
	}
	`)

	basePrograms, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(baseProgramName),
	)
	require.NoError(t, err)
	require.NotNil(t, basePrograms)
	require.Greater(t, len(basePrograms), 0)

	// ========== Step 2: 第一次增量编译 ==========
	t.Logf("Step 2: First incremental compile: %s -> %s", baseProgramName, diffProgram1Name)
	// 创建第一次增量编译的新文件系统（修改 A.java，删除 Utils.java，新增 B.java）
	diff1FS := filesys.NewVirtualFs()
	diff1FS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Diff1";
		public String getValue() {
			return "Value from Modified A in Diff1";
		}
	}`)
	diff1FS.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)
	diff1FS.AddFile("B.java", `
	public class B {
		public static void process() {
			System.out.println("Process from B");
		}
	}
	`)

	diffProgram1s, err := ssaapi.ParseProjectWithIncrementalCompile(
		diff1FS,          // 新的文件系统
		baseProgramName,  // base program name（差量 program，系统会从数据库构建基础文件系统）
		diffProgram1Name, // diff program name
		ssaconfig.JAVA,   // language
	)
	require.NoError(t, err)
	require.NotNil(t, diffProgram1s)
	require.Greater(t, len(diffProgram1s), 0)
	diffProgram1 := diffProgram1s[0]

	// 验证第一次增量编译的元数据
	t.Logf("Step 2.1: Verifying first diff program metadata")
	checkDiffProgramMetadata(t, diffProgram1, checkDiffProgramMetadataConfig{
		BaseProgramName: baseProgramName,
		ExpectedFiles: map[string]int{
			"A.java":     0,  // 修改
			"B.java":     1,  // 新增
			"Utils.java": -1, // 删除
		},
		ExcludedFiles: []string{"Main.java"},
	})

	// ========== Step 3: 从数据库加载 diff program 1，验证 overlay 自动创建 ==========
	t.Logf("Step 3: Loading diff program 1 from database and verifying overlay")
	reloadedDiffProgram1, err := ssaapi.FromDatabase(diffProgram1Name)
	require.NoError(t, err)
	require.NotNil(t, reloadedDiffProgram1)

	// 验证 overlay 已从数据库加载并重建
	overlay1 := reloadedDiffProgram1.GetOverlay()
	require.NotNil(t, overlay1, "diff program 1 should have overlay after loading from database")
	require.Equal(t, 2, overlay1.GetLayerCount(), "overlay should have 2 layers (base + diff1)")

	// ========== Step 4: 第二次增量编译（基于 diff program 1） ==========
	t.Logf("Step 4: Second incremental compile: %s -> %s", diffProgram1Name, diffProgram2Name)
	// 创建第二次增量编译的新文件系统（修改 A.java，新增 C.java，删除 B.java）
	diff2FS := filesys.NewVirtualFs()
	diff2FS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Diff2";
		public String getValue() {
			return "Value from Modified A in Diff2";
		}
	}`)
	diff2FS.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)
	diff2FS.AddFile("C.java", `
	public class C {
		public static void compute() {
			System.out.println("Compute from C");
		}
	}
	`)

	// 检查 diff2FS 创建后的文件列表
	diff2FSFiles := make([]string, 0)
	filesys.Recursive(".", filesys.WithFileSystem(diff2FS), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if !isDir {
			diff2FSFiles = append(diff2FSFiles, pathname)
		}
		return nil
	}))
	fmt.Fprintf(os.Stderr, "[TEST] diff2FS files after creation: %v (total: %d)\n", diff2FSFiles, len(diff2FSFiles))
	t.Logf("diff2FS files after creation: %v (total: %d)", diff2FSFiles, len(diff2FSFiles))

	// 检查 overlay1 的聚合文件系统
	aggregatedFS := overlay1.GetAggregatedFileSystem()
	if aggregatedFS != nil {
		aggregatedFSFiles := make([]string, 0)
		filesys.Recursive(".", filesys.WithFileSystem(aggregatedFS), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
			if !isDir {
				aggregatedFSFiles = append(aggregatedFSFiles, pathname)
			}
			return nil
		}))
		fmt.Fprintf(os.Stderr, "[TEST] overlay1.GetAggregatedFileSystem() files: %v (total: %d)\n", aggregatedFSFiles, len(aggregatedFSFiles))
		t.Logf("overlay1.GetAggregatedFileSystem() files: %v (total: %d)", aggregatedFSFiles, len(aggregatedFSFiles))
	}

	// 使用 ParseProjectWithIncrementalCompile 进行第二次增量编译
	diffProgram2s, err := ssaapi.ParseProjectWithIncrementalCompile(
		diff2FS,          // 新的文件系统
		diffProgram1Name, // base program name（差量 program，系统会从数据库构建基础文件系统）
		diffProgram2Name, // diff program name
		ssaconfig.JAVA,   // language
	)
	require.NoError(t, err)
	require.NotNil(t, diffProgram2s)
	require.Greater(t, len(diffProgram2s), 0)
	diffProgram2 := diffProgram2s[0]

	// 验证第二次增量编译的元数据
	t.Logf("Step 4.1: Verifying second diff program metadata")
	checkDiffProgramMetadata(t, diffProgram2, checkDiffProgramMetadataConfig{
		BaseProgramName: diffProgram1Name, // base 应该是 diff program 1
		ExpectedFiles: map[string]int{
			"A.java": 0,  // 修改
			"C.java": 1,  // 新增
			"B.java": -1, // 删除（在 diff1 中新增，在 diff2 中删除）
		},
		ExcludedFiles: []string{"Main.java"},
	})

	// ========== Step 5: 验证 diff program 2 的 overlay ==========
	t.Logf("Step 5: Verifying diff program 2 overlay")
	overlay2 := diffProgram2.GetOverlay()
	require.NotNil(t, overlay2, "diff program 2 should have overlay")
	require.Equal(t, 3, overlay2.GetLayerCount(), "overlay should have 3 layers (base + diff1 + diff2)")

	// 验证各层的文件
	layer1Files := overlay2.GetFilesInLayer(1)
	layer2Files := overlay2.GetFilesInLayer(2)
	layer3Files := overlay2.GetFilesInLayer(3)
	t.Logf("Layer 1 files: %v", layer1Files)
	t.Logf("Layer 2 files: %v", layer2Files)
	t.Logf("Layer 3 files: %v", layer3Files)

	// Layer 1 应该包含基础文件
	require.Contains(t, layer1Files, "Main.java", "Layer 1 should contain Main.java")
	require.Contains(t, layer1Files, "Utils.java", "Layer 1 should contain Utils.java")

	// Layer 2 应该包含 diff1 的变更文件
	require.Contains(t, layer2Files, "A.java", "Layer 2 should contain modified A.java")
	require.Contains(t, layer2Files, "B.java", "Layer 2 should contain new B.java")

	// Layer 3 应该包含 diff2 的变更文件
	require.Contains(t, layer3Files, "A.java", "Layer 3 should contain modified A.java")
	require.Contains(t, layer3Files, "C.java", "Layer 3 should contain new C.java")

	// ========== Step 6: 从数据库加载 diff program 2，验证 overlay 自动创建 ==========
	t.Logf("Step 6: Loading diff program 2 from database and verifying overlay")
	reloadedDiffProgram2, err := ssaapi.FromDatabase(diffProgram2Name)
	require.NoError(t, err)
	require.NotNil(t, reloadedDiffProgram2)

	// 验证 overlay 已从数据库加载并重建
	reloadedOverlay2 := reloadedDiffProgram2.GetOverlay()
	require.NotNil(t, reloadedOverlay2, "diff program 2 should have overlay after loading from database")
	require.Equal(t, 3, reloadedOverlay2.GetLayerCount(), "reloaded overlay should have 3 layers")

	// ========== Step 7: 验证 overlay 的功能（查找值） ==========
	t.Logf("Step 7: Verifying overlay functionality (finding values)")
	// 从 overlay 查找 valueStr 字段，应该返回最上层（diff2）的值
	valueStrValues := reloadedOverlay2.Ref("valueStr")
	require.NotEmpty(t, valueStrValues, "overlay should find valueStr")
	require.Contains(t, valueStrValues.String(), "Value from Diff2", "overlay should return value from top layer (diff2)")

	// 查找 C 类（只在 diff2 中），应该能找到
	cValues := reloadedOverlay2.Ref("C")
	require.NotEmpty(t, cValues, "overlay should find class C from diff2")

	// 查找 B 类（在 diff1 中新增，在 diff2 中删除），不应该找到
	bValues := reloadedOverlay2.Ref("B")
	require.Empty(t, bValues, "overlay should not find class B (deleted in diff2)")

	// 查找 Utils 类（在 base 中存在，在 diff1 中删除），不应该找到
	utilsValues := reloadedOverlay2.Ref("Utils")
	require.Empty(t, utilsValues, "overlay should not find class Utils (deleted in diff1)")

	// 验证 A 类存在（应该是最上层的版本）
	aValues := reloadedOverlay2.Ref("A")
	require.NotEmpty(t, aValues, "overlay should find class A")

	t.Logf("Test completed successfully: base -> diff1 -> diff2")
	// }
}

// TestIsOverlayFieldInDatabase 测试 IsOverlay 字段在数据库中的保存和读取
// 从编译层面验证增量编译时 IsOverlay 字段是否正确设置
func TestIsOverlayFieldInDatabase(t *testing.T) {
	baseProgramName := uuid.NewString()
	diffProgramName := uuid.NewString()

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgramName)
	}()

	// 创建基础文件系统
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`)

	// 创建新文件系统（修改 A.java）
	newFS := filesys.NewVirtualFs()
	newFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Diff";
		public String getValue() {
			return "Value from Modified A";
		}
	}`)

	t.Run("Compile base program", func(t *testing.T) {
		t.Logf("Compiling base program: %s", baseProgramName)
		basePrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(baseFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(baseProgramName),
		)
		require.NoError(t, err)
		require.NotNil(t, basePrograms)
		require.Len(t, basePrograms, 1)
		require.False(t, basePrograms[0].Program.GetIrProgram().IsOverlay, "IsOverlay should be false for base program (no BaseProgramName or FileHashMap)")
	})

	t.Run("Compile diff program", func(t *testing.T) {
		t.Logf("Compiling diff program: %s", diffProgramName)
		ctx := context.Background()
		diffProgram, err := ssaapi.CompileDiffProgramAndSaveToDB(
			ctx,
			nil, newFS,
			baseProgramName, diffProgramName,
			ssaconfig.JAVA,
		)
		require.NoError(t, err)
		require.NotNil(t, diffProgram)
	})

	t.Run("Verify base program IsOverlay is false", func(t *testing.T) {
		t.Logf("Verifying base program IsOverlay field")
		baseIrProg, err := ssadb.GetProgram(baseProgramName, ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, baseIrProg)
		require.False(t, baseIrProg.IsOverlay, "IsOverlay should be false for base program (no BaseProgramName or FileHashMap)")
	})

	t.Run("Verify diff program IsOverlay is true", func(t *testing.T) {
		t.Logf("Verifying diff program IsOverlay field")
		diffIrProg, err := ssadb.GetProgram(diffProgramName, ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, diffIrProg)
		require.True(t, diffIrProg.IsOverlay, "IsOverlay should be true for diff program (has BaseProgramName and FileHashMap)")
	})

	t.Run("Compile base program with EnableIncremental", func(t *testing.T) {
		t.Logf("Compiling base program: %s", baseProgramName)
		basePrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(baseFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(baseProgramName),
			ssaapi.WithEnableIncrementalCompile(true),
		)
		require.NoError(t, err)
		require.NotNil(t, basePrograms)
		require.Len(t, basePrograms, 1)
		require.True(t, basePrograms[0].Program.GetIrProgram().IsOverlay, "IsOverlay should be false for base program (no BaseProgramName or FileHashMap)")
	})
}

// TestRecompileAutoDetectIncrementalCompile 测试重新编译时自动检测增量编译的逻辑
func TestRecompileAutoDetectIncrementalCompile(t *testing.T) {
	baseProgramName := uuid.NewString()
	diffProgramName := uuid.NewString()

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgramName)
	}()

	// 创建基础文件系统
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`)

	// 创建新文件系统（修改 A.java）
	newFS := filesys.NewVirtualFs()
	newFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Diff";
		public String getValue() {
			return "Value from Modified A";
		}
	}`)

	t.Run("Step 1: Compile base program with incremental compile enabled", func(t *testing.T) {
		// 第一次编译：启用增量编译，创建 base program（IsOverlay=true, BaseProgramName="")
		basePrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(baseFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(baseProgramName),
			ssaapi.WithEnableIncrementalCompile(true),
		)
		require.NoError(t, err)
		require.NotNil(t, basePrograms)
		require.Len(t, basePrograms, 1)

		// 验证 base program 的 IsOverlay 为 true
		baseIrProg, err := ssadb.GetProgram(baseProgramName, ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, baseIrProg)
		require.True(t, baseIrProg.IsOverlay, "IsOverlay should be true for base program with incremental compile enabled")
		require.Empty(t, baseIrProg.BaseProgramName, "BaseProgramName should be empty for base program")
	})

	t.Run("Step 2: Recompile base program without explicit incremental compile flag", func(t *testing.T) {
		// 重新编译 base program，不显式设置增量编译标志
		// 应该自动检测到 IsOverlay=true，并启用增量编译逻辑
		basePrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(newFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(diffProgramName),
			ssaapi.WithBaseProgramName(baseProgramName),
			ssaapi.WithReCompile(true),
			// 注意：不设置 WithEnableIncrementalCompile(true)
		)
		require.NoError(t, err)
		require.NotNil(t, basePrograms)
		require.Len(t, basePrograms, 1)

		// 验证重新编译后，base program 仍然存在（没有被删除）
		baseIrProg, err := ssadb.GetProgram(baseProgramName, ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, baseIrProg, "Base program should still exist after recompile (incremental compile keeps base program)")
		require.True(t, baseIrProg.IsOverlay, "IsOverlay should still be true after recompile")
		require.Empty(t, baseIrProg.BaseProgramName, "BaseProgramName should still be empty for base program")
	})
}
