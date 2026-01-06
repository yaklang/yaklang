package ssaapi_test

import (
	"context"
	"fmt"
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
			System.out.println("Helper from Base");
		}
	}
	`)

	// 编译基础程序
	basePrograms, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(baseProgramName),
	)
	require.NoError(t, err)
	require.NotNil(t, basePrograms)
	require.Greater(t, len(basePrograms), 0)

	// 创建新文件系统（包含修改、新增、删除）
	newFS := filesys.NewVirtualFs()
	// 修改 A.java
	newFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}
	}`)
	// Main.java 保持不变（不包含在差量中）
	newFS.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)
	// 删除 Utils.java（不添加到 newFS）
	// 新增 NewFile.java
	newFS.AddFile("NewFile.java", `
	public class NewFile {
		public static void newMethod() {
			System.out.println("New method from Extend");
		}
	}
	`)

	// 使用 CompileDiffProgramAndSaveToDB 编译差量程序
	ctx := context.Background()
	diffProgram, err := ssaapi.CompileDiffProgramAndSaveToDB(
		ctx,
		baseFS, newFS,
		baseProgramName, diffProgramName,
		ssaconfig.JAVA,
	)
	require.NoError(t, err)
	require.NotNil(t, diffProgram)

	t.Run("test BaseProgramName and FileHashMap in Program struct", func(t *testing.T) {
		checkDiffProgramMetadata(t, diffProgram, checkDiffProgramMetadataConfig{
			BaseProgramName: baseProgramName,
			ExpectedFiles: map[string]int{
				"/A.java":       0,  // modified
				"/NewFile.java": 1,  // new
				"/Utils.java":   -1, // deleted
			},
			ExcludedFiles: []string{"/Main.java"}, // unchanged, should not be in FileHashMap
		})
	})

	t.Run("test BaseProgramName and FileHashMap saved to database", func(t *testing.T) {
		// 从数据库重新加载程序
		reloadedProgram, err := ssaapi.FromDatabase(diffProgramName)
		require.NoError(t, err)
		require.NotNil(t, reloadedProgram)

		// 验证数据库中的信息
		irProgram := reloadedProgram.Program.GetIrProgram()
		require.NotNil(t, irProgram, "irProgram should exist")

		// 使用 checkDiffProgramMetadataInDB 验证数据库中的信息
		checkDiffProgramMetadataInDB(t, irProgram, checkDiffProgramMetadataConfig{
			BaseProgramName: baseProgramName,
			ExpectedFiles: map[string]int{
				"/A.java":       0,  // modified
				"/NewFile.java": 1,  // new
				"/Utils.java":   -1, // deleted
			},
			ExcludedFiles: []string{"/Main.java"}, // unchanged, should not be in FileHashMap
		})
	})

	t.Run("test diffProgram contains only changed files", func(t *testing.T) {
		// 验证 diffProgram 只包含变更的文件（新增+修改），不包含未变更的文件
		// 使用 checkDiffProgramMetadata 验证 FileHashMap，FileHashMap 正确则文件列表也正确
		checkDiffProgramMetadata(t, diffProgram, checkDiffProgramMetadataConfig{
			BaseProgramName: baseProgramName,
			ExpectedFiles: map[string]int{
				"/A.java":       0,  // modified
				"/NewFile.java": 1,  // new
				"/Utils.java":   -1, // deleted
			},
			ExcludedFiles: []string{"/Main.java"}, // unchanged, should not be in FileHashMap
		})
	})

	t.Run("test diffProgram compilation correctness", func(t *testing.T) {
		// 验证 diffProgram 可以正常编译和使用
		// 使用 checkDiffProgramMetadata 验证 FileHashMap，确保删除的文件（hash=-1）不在 diffProgram 中
		checkDiffProgramMetadata(t, diffProgram, checkDiffProgramMetadataConfig{
			BaseProgramName: baseProgramName,
			ExpectedFiles: map[string]int{
				"/A.java":       0,  // modified - should be in diffProgram
				"/NewFile.java": 1,  // new - should be in diffProgram
				"/Utils.java":   -1, // deleted - should NOT be in diffProgram
			},
			ExcludedFiles: []string{"/Main.java"}, // unchanged, should not be in FileHashMap or diffProgram
		})

		// 验证 diffProgram 基本属性
		require.NotNil(t, diffProgram.Program, "diffProgram.Program should not be nil")
		require.NotEmpty(t, diffProgram.GetProgramName(), "diffProgram should have a program name")

		// 验证 diffProgram 包含修改后的 A.java 内容（FileHashMap hash=0）
		classA := diffProgram.Ref("A")
		require.NotEmpty(t, classA, "diffProgram should contain class A (modified, hash=0)")

		// 验证 diffProgram 包含新增的 NewFile 类（FileHashMap hash=1）
		newFileClass := diffProgram.Ref("NewFile")
		require.NotEmpty(t, newFileClass, "diffProgram should contain class NewFile (new, hash=1)")

		// 验证 diffProgram 不包含 Utils 类（已删除，FileHashMap hash=-1）
		// Ref 返回 Values（切片类型），如果类不存在，应该返回空切片
		utilsClass := diffProgram.Ref("Utils")
		require.Empty(t, utilsClass, "diffProgram should not contain class Utils (deleted, hash=-1)")
	})

	t.Run("test diffProgram file count", func(t *testing.T) {
		// 验证 diffProgram 的文件数量
		// diffProgram 应该只包含变更的文件：A.java (修改) 和 NewFile.java (新增)
		// 总共应该是 2 个文件

		fileCount := 0
		diffProgram.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
			fileCount++
			return true
		})

		require.Equal(t, 2, fileCount, "diffProgram should contain exactly 2 files (A.java modified, NewFile.java new)")
	})
}
