package ssaapi_test

import (
	"context"
	"io/fs"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
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
	UseSyntaxFlowRule  bool     // 是否使用 SyntaxFlowRule 方法（true）还是 SyntaxFlowWithError（false，默认）
	RuleName           string   // 如果使用 SyntaxFlowRule，指定规则名称
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
	Language                ssaconfig.Language    // 编译语言（默认为 JAVA）
}

// checkOverlayTest 统一的 overlay 测试检查函数
func checkOverlayTest(t *testing.T, config OverlayTestConfig) {
	ctx := context.Background()

	log.Infof("[checkOverlayTest] Starting test: %s", config.Name)

	// 创建程序名称
	programNames := make([]string, len(config.FileSystems))
	for i := range programNames {
		programNames[i] = uuid.NewString()
	}
	log.Infof("[checkOverlayTest] Created %d program names", len(programNames))

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
	log.Infof("[checkOverlayTest] Step 1: Created base filesystem with %d files", len(config.FileSystems[0]))

	language := config.Language
	basePrograms, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(language),
		ssaapi.WithProgramName(programNames[0]),
	)
	require.NoError(t, err)
	require.NotNil(t, basePrograms)
	require.Greater(t, len(basePrograms), 0, "Should have at least one program")
	baseProgram := basePrograms[0]
	log.Infof("[checkOverlayTest] Step 1: Created base program: %s", baseProgram.GetProgramName())

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
			language,
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
	log.Infof("[checkOverlayTest] Step 6: Checking %d syntaxflow rules", len(config.SyntaxFlowRules))
	for i, ruleCheck := range config.SyntaxFlowRules {
		log.Infof("[checkOverlayTest] Step 6: Checking rule %d/%d: %s (queryInstance: %s)", i+1, len(config.SyntaxFlowRules), ruleCheck.Rule, ruleCheck.QueryInstance)
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
		res.Show()

		// 根据是否有变量名来决定获取值的方式
		var values []*ssaapi.Value
		if ruleCheck.VariableName != "" {
			values = res.GetValues(ruleCheck.VariableName)
			log.Infof("[checkOverlayTest] Step 6: Got %d values for variable '%s'", len(values), ruleCheck.VariableName)
		} else {
			values = res.GetAllValuesChain()
			log.Infof("[checkOverlayTest] Step 6: Got %d values from GetAllValuesChain()", len(values))
		}

		// 检查结果数量
		if ruleCheck.ExpectedCount == -1 {
			log.Infof("[checkOverlayTest] Step 6: Expecting empty result")
			require.Empty(t, values, "Should not find values for rule: %s", ruleCheck.Rule)
		} else if ruleCheck.ExpectedCount > 0 {
			log.Infof("[checkOverlayTest] Step 6: Expecting exactly %d values, got %d", ruleCheck.ExpectedCount, len(values))
			require.Len(t, values, ruleCheck.ExpectedCount, "Should find exactly %d values for rule: %s", ruleCheck.ExpectedCount, ruleCheck.Rule)
		} else if ruleCheck.ExpectedCount == 0 {
			log.Infof("[checkOverlayTest] Step 6: Expecting non-empty result, got %d values", len(values))
			require.NotEmpty(t, values, "Should find values for rule: %s", ruleCheck.Rule)
		}

		// 检查预期值
		if len(ruleCheck.ExpectedValues) > 0 && len(values) > 0 {
			log.Infof("[checkOverlayTest] Step 6: Checking %d expected values", len(ruleCheck.ExpectedValues))
			for _, expectedValue := range ruleCheck.ExpectedValues {
				found := false
				for _, v := range values {
					if contains(v.String(), expectedValue) {
						found = true
						break
					}
				}
				log.Infof("[checkOverlayTest] Step 6: Expected value '%s' found: %v", expectedValue, found)
				require.True(t, found, "Should find value containing '%s' in rule: %s", expectedValue, ruleCheck.Rule)
			}
		}

		// 检查是否被覆盖（仅对单个值有效）
		if ruleCheck.ExpectedOverridden && len(values) > 0 && overlay != nil {
			v := values[0]
			isOverridden := overlay.IsOverridden(v)
			log.Infof("[checkOverlayTest] Step 6: Value %s is overridden: %v", v.String(), isOverridden)
			require.True(t, isOverridden, "Value %s should be overridden", v.String())
		}
	}
	log.Infof("[checkOverlayTest] Completed test: %s", config.Name)
}

// checkOverlayTestWithSyntaxFlowRule 统一的 overlay 测试检查函数（使用 SyntaxFlowRule 方法）
// 与 checkOverlayTest 的区别：使用 SyntaxFlowRule 而不是 SyntaxFlowWithError
func checkOverlayTestWithSyntaxFlowRule(t *testing.T, config OverlayTestConfig) {
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

	language := config.Language
	if language == "" {
		language = ssaconfig.JAVA // 默认为 JAVA
	}

	basePrograms, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(language),
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
			language,
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

	// Step 6: 检查 syntaxflow 规则（使用 SyntaxFlowRule 方法）
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

		// 创建或加载 SyntaxFlowRule 对象
		var rule *schema.SyntaxFlowRule
		var err error

		// 如果 RuleName 不为空，尝试从数据库加载规则
		if ruleCheck.RuleName != "" {
			rule, err = sfdb.GetRule(ruleCheck.RuleName)
			if err != nil {
				// 如果加载失败，使用 Rule 字段作为内容创建新规则
				rule = &schema.SyntaxFlowRule{
					RuleName: ruleCheck.RuleName,
					Content:  ruleCheck.Rule,
					Language: baseProgram.GetLanguage(),
				}
			}
		} else {
			t.Fatalf("rule name is empty")
		}

		// 根据 queryInstance 类型调用相应的方法
		var res *ssaapi.SyntaxFlowResult
		switch inst := queryInstance.(type) {
		case *ssaapi.ProgramOverLay:
			res, err = inst.SyntaxFlowRule(rule)
		case *ssaapi.Program:
			res, err = inst.SyntaxFlowRule(rule)
		case ssaapi.Programs:
			res, err = inst.SyntaxFlowRule(rule)
		default:
			t.Fatalf("Unsupported query instance type for SyntaxFlowRule: %T", inst)
		}

		require.NoError(t, err)
		require.NotNil(t, res)
		res.Show()

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
				// {
				// 	Rule:               "A.valueStr as $res",
				// 	ExpectedValues:     []string{baseValueStr},
				// 	ExpectedCount:      1,
				// 	ExpectedOverridden: true,
				// 	QueryInstance:      "base",
				// },
				// {
				// 	Rule:               "A.valueStr as $res",
				// 	ExpectedValues:     []string{extendValueStr},
				// 	ExpectedCount:      1,
				// 	ExpectedOverridden: false,
				// 	QueryInstance:      "extend",
				// },
				// {
				// 	Rule:               "A.valueStr as $res",
				// 	ExpectedValues:     []string{extendValueStr},
				// 	ExpectedCount:      1,
				// 	ExpectedOverridden: false,
				// 	QueryInstance:      "overlay",
				// },
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

// TestOverlay_SyntaxFlowRule 测试 ProgramOverLay.SyntaxFlowRule 方法
// 测试场景：增量编译后，使用 SyntaxFlowRule 扫描聚合文件系统
func TestOverlay_SyntaxFlowRule(t *testing.T) {
	// 创建合并后的规则（不使用 include）
	ruleName := "golang-" + uuid.NewString() + ".sf"
	ruleContent := `
http?{<fullTypeName>?{have: "net/http"}} as $http
$http.ListenAndServe as $mid

alert $mid for {
	level: "mid",
	type: "vuln",
	risk: "不安全连接",
}
`
	ruleName2 := "golang-" + uuid.NewString() + ".sf"
	ruleContent2 := `
	exec?{<fullTypeName>?{have: 'os/exec'}} as $entry
	$entry.Command(* #-> as $sink)

	r?{<fullTypeName>?{have: 'net/http'}}.URL.Query().Get(* #-> as $input)
	r?{<fullTypeName>?{have: 'net/http'}}.FormValue(* #-> as $input)
	r?{<fullTypeName>?{have: 'net/http'}}.PostFormValue(* #-> as $input)
	r?{<fullTypeName>?{have: 'net/http'}}.Header.Get(* #-> as $input)

	$sink & $input as $high

	alert $high for {
		level: "high",
		type: "vuln",
		risk: "命令注入",
	}
		`

	// 创建规则
	rule, err := sfdb.CreateRuleByContent(ruleName, ruleContent, false)
	require.NoError(t, err)
	require.NotNil(t, rule)
	rule2, err := sfdb.CreateRuleByContent(ruleName2, ruleContent2, false)
	require.NoError(t, err)
	require.NotNil(t, rule2)
	defer func() {
		sfdb.DeleteRuleByRuleName(ruleName)
		sfdb.DeleteRuleByRuleName(ruleName2)
	}()

	// 共享的测试配置
	// 模拟真实增量编译场景：
	// - Layer1 (base): main.go 和 safe.go，main.go 中没有 http.ListenAndServe
	// - Layer2 (diff): main.go（添加 http.ListenAndServe）和 unsafe.go（新增）
	// 这样 http 值可能来自 layer1（没有 ListenAndServe 成员），而 ListenAndServe 只在 layer2 中使用
	config := OverlayTestConfig{
		Name: "test SyntaxFlowRule with incremental compilation",
		FileSystems: []map[string]string{
			{
				"main.go": `package main

import (
    "fmt"
    "net/http"
)

func main() {
    http.HandleFunc("/unsafe", unsafeHandler)
    http.HandleFunc("/safe", safeHandler)
    fmt.Println("Server starting on :8080...")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        fmt.Printf("Server error: %v\n", err)
    }
}`,
				"safe.go": `package main

import (
    "fmt"
    "net/http"
    "os/exec"
)

func safeHandler(w http.ResponseWriter, r *http.Request) {
    output, err := executeCommandSafe("hello")
    if err != nil {
        http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    fmt.Fprint(w, output)
}

func executeCommandSafe(userInput string) (string, error) {
    cmd := exec.Command("echo", userInput)

    output, err := cmd.CombinedOutput()
    if err != nil {
        return "", fmt.Errorf("command execution failed: %v", err)
    }

    return string(output), nil
}`,
			},
			{
				"main.go": `package main

import (
    "fmt"
    "net/http"
)

func main() {
    http.HandleFunc("/unsafe", unsafeHandler)
    http.HandleFunc("/safe", safeHandler)
    fmt.Println("Server starting on :8080...")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        fmt.Printf("Server error: %v\n", err)
    }
}`,
				"unsafe.go": `package main

import (
    "fmt"
    "net/http"
    "os/exec"
)

func unsafeHandler(w http.ResponseWriter, r *http.Request) {
    cmdParam := r.URL.Query().Get("cmd")
    if cmdParam == "" {
        http.Error(w, "Missing 'cmd' parameter", http.StatusBadRequest)
        return
    }

    output, err := executeCommandUnsafe(cmdParam)
    if err != nil {
        http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    fmt.Fprint(w, output)
}

func executeCommandUnsafe(userInput string) (string, error) {
    cmd := exec.Command("sh", "-c", "echo "+userInput)

    output, err := cmd.CombinedOutput()
    if err != nil {
        return "", fmt.Errorf("command execution failed: %v", err)
    }

    return string(output), nil
}`,
			},
		},
		SyntaxFlowRules: []SyntaxFlowRuleCheck{
			{
				Rule:              ruleName,
				ExpectedValues:    []string{"ListenAndServe"},
				ExpectedCount:     0,
				QueryInstance:     "overlay",
				UseSyntaxFlowRule: true,
				RuleName:          ruleName,
			},
			{
				Rule:              ruleName2,
				ExpectedValues:    []string{"Parameter-r", "\"cmd\""},
				ExpectedCount:     0,
				QueryInstance:     "overlay",
				UseSyntaxFlowRule: true,
				RuleName:          ruleName2,
			},
		},
		ExpectedAggregatedFiles: []string{"main.go", "unsafe.go"},
		Language:                ssaconfig.GO,
	}

	t.Run("test SyntaxFlowRule with incremental compilation", func(t *testing.T) {
		checkOverlayTestWithSyntaxFlowRule(t, config)
	})

	t.Run("test SyntaxFlowWithError with incremental compilation", func(t *testing.T) {
		// 使用共享的 config，但修改 Rule 字段为规则内容
		configWithRuleContent := config
		configWithRuleContent.Name = "test SyntaxFlowWithError with incremental compilation"
		configWithRuleContent.SyntaxFlowRules = []SyntaxFlowRuleCheck{
			{
				Rule:           ruleContent,
				ExpectedValues: []string{"ListenAndServe"},
				ExpectedCount:  0,
				QueryInstance:  "overlay",
			}, {
				Rule:           ruleContent2,
				ExpectedValues: []string{"Parameter-r", "\"cmd\""},
				ExpectedCount:  0,
				QueryInstance:  "overlay",
			},
		}

		checkOverlayTest(t, configWithRuleContent)
	})
}
