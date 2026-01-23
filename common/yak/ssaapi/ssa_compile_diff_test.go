package ssaapi_test

import (
	"context"
	"fmt"
	"io/fs"
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

// DiffRefCheck 定义 diff program Ref 检查配置
type DiffRefCheck struct {
	ClassName     string // 类名
	ShouldExist   bool   // 是否应该存在
	ExpectedValue string // 预期的值（包含此字符串，空表示不检查）
}

// LayerFileCheck 定义 layer 文件检查配置
type LayerFileCheck struct {
	LayerIndex int      // layer 索引（1-based）
	Files      []string // 该 layer 应该包含的文件
}

// DiffProgramFileHashMapCheck 定义每个 diff program 的 FileHashMap 检查配置
type DiffProgramFileHashMapCheck struct {
	DiffIndex               int            // diff program 索引（1-based，1表示第一个diff）
	ExpectedFileHashMap     map[string]int // 预期的 FileHashMap
	ExcludedFromFileHashMap []string       // 不应该出现在 FileHashMap 中的文件列表
}

// DiffCompileTestConfig 统一的 diff 编译测试配置结构
type DiffCompileTestConfig struct {
	Name                     string                        // 测试名称
	FileSystems              []map[string]string           // 多个文件系统，第一个是基础，其他是增量修改
	SyntaxFlowRules          []SyntaxFlowRuleCheck         // syntaxflow 规则和预期结果（针对最后一个 diff program）
	ExpectedAggregatedFiles  []string                      // 预期的聚合文件系统文件列表（应该存在的文件）
	ExpectedExcludedFiles    []string                      // 预期排除的文件列表（不应该存在的文件）
	ExpectedFileHashMap      map[string]int                // 预期的 FileHashMap（最后一个 diff program，文件变更状态：-1删除，0修改，1新增）
	ExcludedFromFileHashMap  []string                      // 不应该出现在 FileHashMap 中的文件列表（最后一个 diff program）
	DiffProgramFileHashMaps  []DiffProgramFileHashMapCheck // 每个 diff program 的 FileHashMap 检查（支持多层）
	ExpectedDiffFiles        []string                      // 差量程序应该包含的文件（最后一个 diff program，只包含修改和新增的文件）
	DiffProgramFiles         map[int][]string              // 每个 diff program 应该包含的文件（key: diff index 1-based）
	RefChecks                []DiffRefCheck                // diff program Ref 检查配置（针对最后一个 diff program）
	OverlayRefChecks         []OverlayRefCheck             // overlay Ref 检查配置（针对最后一个 overlay）
	LayerFileChecks          []LayerFileCheck              // layer 文件检查配置
	ExpectedLayerCount       int                           // 预期的 layer 数量（0表示不检查）
	TestDatabaseLoad         bool                          // 是否测试从数据库加载
	TestOverlay              bool                          // 是否测试 overlay（从数据库加载时）
	EnableIncrementalForBase bool                          // 是否为基础程序启用增量编译（WithEnableIncrementalCompile）
	TestRecompile            bool                          // 是否测试重新编译（使用 WithReCompile 和 WithBaseProgramName）
	BaseProgramIsOverlay     *bool                         // 验证 base program 的 IsOverlay 字段（nil表示不检查）
	DiffProgramIsOverlay     *bool                         // 验证 diff program 的 IsOverlay 字段（nil表示不检查）
}

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

// check 统一的 diff 编译测试检查函数（简化入口）
// 输入配置结构包含：
// 1. 多个文件系统（默认第一个为初始化，其他的都是修改）
// 2. syntaxflow规则
// 3. 预期的聚合文件系统
// 4. 预期的结果
func checkDiff(t *testing.T, config DiffCompileTestConfig) {
	checkDiffCompileTest(t, config)
}

// checkDiffCompileTest 统一的 diff 编译测试检查函数
func checkDiffCompileTest(t *testing.T, config DiffCompileTestConfig) {
	ctx := context.Background()

	// 验证配置：至少需要一个文件系统（基础）
	require.GreaterOrEqual(t, len(config.FileSystems), 1, "至少需要一个文件系统（基础）")
	if len(config.FileSystems) == 1 && !config.TestRecompile {
		t.Logf("警告：只有一个文件系统，将只创建基础程序，不会创建 diff program")
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

	// 验证 base program 的 IsOverlay 字段
	if config.BaseProgramIsOverlay != nil {
		baseIrProg, err := ssadb.GetProgram(programNames[0], ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, baseIrProg)
		require.Equal(t, *config.BaseProgramIsOverlay, baseIrProg.IsOverlay, "Base program IsOverlay should be %v", *config.BaseProgramIsOverlay)
	}

	// Step 2: 创建增量程序（如果有多个文件系统）或重新编译
	var diffPrograms []*ssaapi.Program
	if config.TestRecompile && len(config.FileSystems) >= 2 {
		// 重新编译场景：使用 WithReCompile 和 WithBaseProgramName
		diffFS := filesys.NewVirtualFs()
		for path, content := range config.FileSystems[1] {
			diffFS.AddFile(path, content)
		}

		diffProgs, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(diffFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programNames[1]),
			ssaapi.WithBaseProgramName(programNames[0]),
			ssaapi.WithReCompile(true),
		)
		require.NoError(t, err)
		require.NotNil(t, diffProgs)
		require.Greater(t, len(diffProgs), 0)
		diffPrograms = append(diffPrograms, diffProgs[0])

		// 验证重新编译后，base program 仍然存在
		baseIrProg, err := ssadb.GetProgram(programNames[0], ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, baseIrProg, "Base program should still exist after recompile")
	} else {
		// 正常的增量编译场景
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

			// 验证每个 diff program 的 FileHashMap（如果配置了）
			for _, fileHashMapCheck := range config.DiffProgramFileHashMaps {
				if fileHashMapCheck.DiffIndex == i {
					var expectedBaseProgName string
					if i == 1 {
						expectedBaseProgName = programNames[0]
					} else {
						expectedBaseProgName = programNames[i-1]
					}
					checkDiffProgramMetadata(t, diffProgs[0], checkDiffProgramMetadataConfig{
						BaseProgramName: expectedBaseProgName,
						ExpectedFiles:   fileHashMapCheck.ExpectedFileHashMap,
						ExcludedFiles:   fileHashMapCheck.ExcludedFromFileHashMap,
					})
				}
			}

			// 验证每个 diff program 包含的文件（如果配置了）
			if expectedFiles, ok := config.DiffProgramFiles[i]; ok {
				fileCount := 0
				foundFiles := make(map[string]bool)
				diffProgs[0].ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
					fileCount++
					normalizedPath := normalizeFilePathForTest(filePath)
					foundFiles[normalizedPath] = true
					return true
				})

				for _, expectedFile := range expectedFiles {
					require.True(t, foundFiles[expectedFile], "diffProgram %d should contain file %s", i, expectedFile)
				}
				require.Equal(t, len(expectedFiles), fileCount, "diffProgram %d should contain exactly %d files", i, len(expectedFiles))
			}
		}
	}

	// Step 3: 验证最后一个增量程序
	if len(diffPrograms) > 0 {
		lastDiffProgram := diffPrograms[len(diffPrograms)-1]

		// 验证 FileHashMap
		if len(config.ExpectedFileHashMap) > 0 || len(config.ExcludedFromFileHashMap) > 0 {
			var baseProgName string
			if len(diffPrograms) == 1 {
				baseProgName = programNames[0]
			} else {
				baseProgName = programNames[len(programNames)-2]
			}

			checkDiffProgramMetadata(t, lastDiffProgram, checkDiffProgramMetadataConfig{
				BaseProgramName: baseProgName,
				ExpectedFiles:   config.ExpectedFileHashMap,
				ExcludedFiles:   config.ExcludedFromFileHashMap,
			})
		}

		// 验证差量程序包含的文件（只包含修改和新增的文件）
		if len(config.ExpectedDiffFiles) > 0 {
			fileCount := 0
			foundFiles := make(map[string]bool)
			lastDiffProgram.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
				fileCount++
				normalizedPath := normalizeFilePathForTest(filePath)
				foundFiles[normalizedPath] = true
				return true
			})

			for _, expectedFile := range config.ExpectedDiffFiles {
				require.True(t, foundFiles[expectedFile], "diffProgram should contain file %s", expectedFile)
			}
			require.Equal(t, len(config.ExpectedDiffFiles), fileCount, "diffProgram should contain exactly %d files", len(config.ExpectedDiffFiles))
		}

		// Step 4: 检查 Ref 查询
		for _, refCheck := range config.RefChecks {
			values := lastDiffProgram.Ref(refCheck.ClassName)
			if refCheck.ShouldExist {
				require.NotEmpty(t, values, "diffProgram should contain class %s", refCheck.ClassName)
				if refCheck.ExpectedValue != "" {
					require.Contains(t, values.String(), refCheck.ExpectedValue, "diffProgram should contain value %s for class %s", refCheck.ExpectedValue, refCheck.ClassName)
				}
			} else {
				require.Empty(t, values, "diffProgram should not contain class %s", refCheck.ClassName)
			}
		}

		// 验证 diff program 的 IsOverlay 字段
		if config.DiffProgramIsOverlay != nil {
			diffIrProg, err := ssadb.GetProgram(programNames[len(programNames)-1], ssadb.Application)
			require.NoError(t, err)
			require.NotNil(t, diffIrProg)
			require.Equal(t, *config.DiffProgramIsOverlay, diffIrProg.IsOverlay, "Diff program IsOverlay should be %v", *config.DiffProgramIsOverlay)
		}

		// Step 5: 检查 syntaxflow 规则
		for _, ruleCheck := range config.SyntaxFlowRules {
			res, err := lastDiffProgram.SyntaxFlowWithError(ruleCheck.Rule)
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
						if containsDiff(v.String(), expectedValue) {
							found = true
							break
						}
					}
					require.True(t, found, "Should find value containing '%s' in rule: %s", expectedValue, ruleCheck.Rule)
				}
			}
		}

		// Step 6: 检查 overlay
		if config.TestOverlay {
			overlay := lastDiffProgram.GetOverlay()
			if overlay != nil {
				// 验证 layer 数量
				if config.ExpectedLayerCount > 0 {
					require.Equal(t, config.ExpectedLayerCount, overlay.GetLayerCount(), "overlay should have %d layers", config.ExpectedLayerCount)
				}

				// 验证各层的文件
				for _, layerCheck := range config.LayerFileChecks {
					layerFiles := overlay.GetFilesInLayer(layerCheck.LayerIndex)
					for _, expectedFile := range layerCheck.Files {
						require.Contains(t, layerFiles, expectedFile, "Layer %d should contain file %s", layerCheck.LayerIndex, expectedFile)
					}
				}

				// 检查聚合文件系统
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

				// 检查 overlay Ref 查询
				for _, overlayRefCheck := range config.OverlayRefChecks {
					values := overlay.Ref(overlayRefCheck.ClassName)
					if overlayRefCheck.ShouldExist {
						require.NotEmpty(t, values, "overlay should contain class %s", overlayRefCheck.ClassName)
						if overlayRefCheck.ExpectedValue != "" {
							require.Contains(t, values.String(), overlayRefCheck.ExpectedValue, "overlay should contain value %s for class %s", overlayRefCheck.ExpectedValue, overlayRefCheck.ClassName)
						}
					} else {
						require.Empty(t, values, "overlay should not contain class %s", overlayRefCheck.ClassName)
					}
				}
			}
		}

		// Step 7: 测试从数据库加载（如果需要）
		if config.TestDatabaseLoad {
			reloadedDiffProgram, err := ssaapi.FromDatabase(programNames[len(programNames)-1])
			require.NoError(t, err)
			require.NotNil(t, reloadedDiffProgram)

			// 验证数据库中的元数据
			if len(config.ExpectedFileHashMap) > 0 || len(config.ExcludedFromFileHashMap) > 0 {
				var baseProgName string
				if len(diffPrograms) == 1 {
					baseProgName = programNames[0]
				} else {
					baseProgName = programNames[len(programNames)-2]
				}

				irProg, err := ssadb.GetProgram(programNames[len(programNames)-1], ssadb.Application)
				require.NoError(t, err)
				require.NotNil(t, irProg)
				checkDiffProgramMetadataInDB(t, irProg, checkDiffProgramMetadataConfig{
					BaseProgramName: baseProgName,
					ExpectedFiles:   config.ExpectedFileHashMap,
					ExcludedFiles:   config.ExcludedFromFileHashMap,
				})
			}

			// 如果启用了 overlay 测试，验证 overlay
			if config.TestOverlay {
				reloadedOverlay := reloadedDiffProgram.GetOverlay()
				require.NotNil(t, reloadedOverlay, "overlay should be loaded from database")

				// 验证 layer 数量
				if config.ExpectedLayerCount > 0 {
					require.Equal(t, config.ExpectedLayerCount, reloadedOverlay.GetLayerCount(), "reloaded overlay should have %d layers", config.ExpectedLayerCount)
				}

				// 验证各层的文件
				for _, layerCheck := range config.LayerFileChecks {
					layerFiles := reloadedOverlay.GetFilesInLayer(layerCheck.LayerIndex)
					for _, expectedFile := range layerCheck.Files {
						require.Contains(t, layerFiles, expectedFile, "Reloaded Layer %d should contain file %s", layerCheck.LayerIndex, expectedFile)
					}
				}

				// 检查 overlay Ref 查询（从数据库加载后）
				for _, overlayRefCheck := range config.OverlayRefChecks {
					values := reloadedOverlay.Ref(overlayRefCheck.ClassName)
					if overlayRefCheck.ShouldExist {
						require.NotEmpty(t, values, "reloaded overlay should contain class %s", overlayRefCheck.ClassName)
						if overlayRefCheck.ExpectedValue != "" {
							require.Contains(t, values.String(), overlayRefCheck.ExpectedValue, "reloaded overlay should contain value %s for class %s", overlayRefCheck.ExpectedValue, overlayRefCheck.ClassName)
						}
					} else {
						require.Empty(t, values, "reloaded overlay should not contain class %s", overlayRefCheck.ClassName)
					}
				}
			}
		}
	}
}

// containsDiff 检查字符串是否包含子字符串（用于 diff 测试）
func containsDiff(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCompileDiffProgramAndSaveToDB(t *testing.T) {
	t.Run("test compile diff program and save to database", func(t *testing.T) {
		checkDiff(t, DiffCompileTestConfig{
			Name: "test compile diff program and save to database",
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
			System.out.println("Helper from Utils");
		}
	}
	`,
				},
				{
					"A.java": `
	public class A {
		static string valueStr = "Value from Diff";
		public String getValue() {
			return "Value from Modified A";
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
			},
			ExpectedFileHashMap: map[string]int{
				"A.java":     0,  // 修改
				"B.java":     1,  // 新增
				"Utils.java": -1, // 删除
			},
			ExcludedFromFileHashMap: []string{"Main.java"},        // Main.java 没有变化，不应该在 FileHashMap 中
			ExpectedDiffFiles:       []string{"A.java", "B.java"}, // 差量程序应该只包含修改和新增的文件
			RefChecks: []DiffRefCheck{
				{ClassName: "A", ShouldExist: true},      // 修改的文件应该存在
				{ClassName: "B", ShouldExist: true},      // 新增的文件应该存在
				{ClassName: "Utils", ShouldExist: false}, // 删除的文件不应该存在
			},
			TestDatabaseLoad: true, // 测试从数据库加载
		})
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
	t.Run("test incremental compile twice", func(t *testing.T) {
		checkDiff(t, DiffCompileTestConfig{
			Name: "test incremental compile twice",
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
			System.out.println("Helper from Utils");
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
			DiffProgramFileHashMaps: []DiffProgramFileHashMapCheck{
				{
					DiffIndex: 1,
					ExpectedFileHashMap: map[string]int{
						"A.java":     0,  // 修改
						"B.java":     1,  // 新增
						"Utils.java": -1, // 删除
					},
					ExcludedFromFileHashMap: []string{"Main.java"},
				},
				{
					DiffIndex: 2,
					ExpectedFileHashMap: map[string]int{
						"A.java": 0,  // 修改
						"C.java": 1,  // 新增
						"B.java": -1, // 删除（在 diff1 中新增，在 diff2 中删除）
					},
					ExcludedFromFileHashMap: []string{"Main.java"},
				},
			},
			DiffProgramFiles: map[int][]string{
				1: {"A.java", "B.java"}, // diff1 应该包含修改和新增的文件
				2: {"A.java", "C.java"}, // diff2 应该包含修改和新增的文件
			},
			ExpectedLayerCount: 3,
			LayerFileChecks: []LayerFileCheck{
				{
					LayerIndex: 1,
					Files:      []string{"Main.java", "Utils.java"},
				},
				{
					LayerIndex: 2,
					Files:      []string{"A.java", "B.java"},
				},
				{
					LayerIndex: 3,
					Files:      []string{"A.java", "C.java"},
				},
			},
			OverlayRefChecks: []OverlayRefCheck{
				{ClassName: "valueStr", ShouldExist: true, ExpectedValue: "Value from Diff2"},
				{ClassName: "C", ShouldExist: true},
				{ClassName: "B", ShouldExist: false},
				{ClassName: "Utils", ShouldExist: false},
				{ClassName: "A", ShouldExist: true},
			},
			TestDatabaseLoad: true,
			TestOverlay:      true,
		})
	})

	t.Run("test incremental compile add then delete file", func(t *testing.T) {
		checkDiff(t, DiffCompileTestConfig{
			Name: "test incremental compile add then delete file",
			FileSystems: []map[string]string{
				{
					"A.java": `
	public class A {
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
					"Temp.java": `
	public class Temp {
		public static void process() {
			System.out.println("Process from Temp");
		}
	}
	`,
				},
				{
					"A.java": `
	public class A {
		// Modified in diff2 to ensure diffFS is not empty
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
			},
			DiffProgramFileHashMaps: []DiffProgramFileHashMapCheck{
				{
					DiffIndex: 1,
					ExpectedFileHashMap: map[string]int{
						"Temp.java": 1, // 新增
					},
					ExcludedFromFileHashMap: []string{"A.java", "Main.java"},
				},
				{
					DiffIndex: 2,
					ExpectedFileHashMap: map[string]int{
						"A.java":    0,  // 修改（添加注释以确保 diffFS 不为空）
						"Temp.java": -1, // 删除（在 diff1 中新增，在 diff2 中删除）
					},
					ExcludedFromFileHashMap: []string{"Main.java"},
				},
			},
			DiffProgramFiles: map[int][]string{
				1: {"Temp.java"}, // diff1 应该只包含新增的文件
				2: {"A.java"},    // diff2 应该包含修改的文件（A.java 添加了注释）
			},
			ExpectedLayerCount: 3,
			LayerFileChecks: []LayerFileCheck{
				{
					LayerIndex: 1,
					Files:      []string{"A.java", "Main.java"},
				},
				{
					LayerIndex: 2,
					Files:      []string{"Temp.java"},
				},
				{
					LayerIndex: 3,
					Files:      []string{"A.java"}, // layer 3 包含修改后的 A.java
				},
			},
			RefChecks: []DiffRefCheck{
				{ClassName: "Temp", ShouldExist: false}, // diff2 中不应该找到 Temp（已删除）
			},
			OverlayRefChecks: []OverlayRefCheck{
				{ClassName: "Temp", ShouldExist: false}, // overlay 中不应该找到 Temp（在 diff2 中被删除）
				{ClassName: "A", ShouldExist: true},     // A 应该存在
				{ClassName: "Main", ShouldExist: true},  // Main 应该存在
			},
			TestDatabaseLoad: true,
			TestOverlay:      true,
		})
	})
}

// TestIsOverlayFieldInDatabase 测试 IsOverlay 字段在数据库中的保存和读取
// 从编译层面验证增量编译时 IsOverlay 字段是否正确设置
func TestIsOverlayFieldInDatabase(t *testing.T) {
	// 测试 1: base program 不启用增量编译，IsOverlay 应该为 false
	t.Run("test base program IsOverlay false", func(t *testing.T) {
		checkDiff(t, DiffCompileTestConfig{
			Name: "test base program IsOverlay false",
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
		static string valueStr = "Value from Diff";
		public String getValue() {
			return "Value from Modified A";
		}
	}`,
				},
			},
			BaseProgramIsOverlay: boolPtr(false),
			DiffProgramIsOverlay: boolPtr(true),
			ExpectedFileHashMap: map[string]int{
				"A.java": 0, // 修改
			},
			TestDatabaseLoad: true,
		})
	})

	// 测试 2: base program 启用增量编译，IsOverlay 应该为 true
	t.Run("test base program IsOverlay true with incremental compile", func(t *testing.T) {
		baseProgramName := uuid.NewString()
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		}()

		baseFS := filesys.NewVirtualFs()
		baseFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`)

		basePrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(baseFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(baseProgramName),
			ssaapi.WithEnableIncrementalCompile(true),
		)
		require.NoError(t, err)
		require.NotNil(t, basePrograms)
		require.Len(t, basePrograms, 1)

		baseIrProg, err := ssadb.GetProgram(baseProgramName, ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, baseIrProg)
		require.True(t, baseIrProg.IsOverlay, "IsOverlay should be true for base program with incremental compile enabled")
	})
}

// boolPtr 返回 bool 指针的辅助函数
func boolPtr(b bool) *bool {
	return &b
}

// TestRecompileAutoDetectIncrementalCompile 测试重新编译时自动检测增量编译的逻辑
func TestRecompileAutoDetectIncrementalCompile(t *testing.T) {
	t.Run("test recompile auto detect incremental compile", func(t *testing.T) {
		checkDiff(t, DiffCompileTestConfig{
			Name: "test recompile auto detect incremental compile",
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
		static string valueStr = "Value from Diff";
		public String getValue() {
			return "Value from Modified A";
		}
	}`,
				},
			},
			EnableIncrementalForBase: true,
			TestRecompile:            true,
			BaseProgramIsOverlay:     boolPtr(true),
			ExpectedFileHashMap: map[string]int{
				"A.java": 0, // 修改
			},
		})
	})
}
