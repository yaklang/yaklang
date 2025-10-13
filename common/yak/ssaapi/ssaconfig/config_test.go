package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestConfigInitializationByMode(t *testing.T) {
	cfg, err := New(ModeSyntaxFlowScan)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// New() no longer eagerly initializes nested structs - they should be nil
	require.Nil(t, cfg.BaseInfo)
	require.Nil(t, cfg.SyntaxFlow)
	require.Nil(t, cfg.SyntaxFlowScan)
	require.Nil(t, cfg.SyntaxFlowRule)

	// Test that Mode is set correctly
	require.Equal(t, ModeSyntaxFlowScan, cfg.Mode)
}

func TestConfigWithOptions(t *testing.T) {
	cfg, err := New(
		ModeAll,
		WithProgramNames("yak", "yaklang"),
		WithProgramDescription("yaklang project"),
		WithProgramLanguage("Go"),
		WithCompileStrictMode(true),
		WithCompilePeepholeSize(42),
		WithCompileExcludeFiles([]string{"*.tmp"}),
		WithCompileReCompile(true),
		WithCompileMemoryCompile(true),
		WithCompileConcurrency(17),
		WithCodeSourceKind(CodeSourceGit),
		WithCodeSourceLocalFile("/tmp/yak"),
		WithCodeSourceURL("https://example.com/yak.git"),
		WithCodeSourceBranch("dev"),
		WithCodeSourcePath("path/in/repo"),
		WithCodeSourceAuthKind("token"),
		WithCodeSourceAuthUserName("yak"),
		WithCodeSourceAuthPassword("secret"),
		WithSSAProjectCodeSourceAuthKeyPath("/tmp/key"),
		WithCodeSourceProxyURL("http://proxy:8080"),
		WithCodeSourceProxyAuth("proxyUser", "proxyPass"),
		WithSyntaxFlowMemory(true),
		WithScanConcurrency(23),
		WithScanIgnoreLanguage(true),
		WithScanControlMode(ControlModeResume),
		WithScanLanguage("go", "java"),
		WithRuleFilterLanguage("go"),
		WithRuleFilterKeyword("sql"),
		WithRuleFilterIncludeLibraryRule(true),
	)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	require.Equal(t, []string{"yak", "yaklang"}, cfg.BaseInfo.ProgramNames)
	require.Equal(t, "yaklang project", cfg.BaseInfo.ProjectDescription)
	require.Equal(t, "Go", cfg.BaseInfo.Language)

	require.True(t, cfg.GetCompileStrictMode())
	require.Equal(t, 42, cfg.GetCompilePeepholeSize())
	require.Equal(t, []string{"*.tmp"}, cfg.GetCompileExcludeFiles())
	require.True(t, cfg.GetCompileReCompile())
	require.True(t, cfg.GetCompileMemory())
	require.Equal(t, uint32(17), cfg.GetCompileConcurrency())

	require.Equal(t, CodeSourceGit, cfg.GetCodeSourceKind())
	require.Equal(t, "/tmp/yak", cfg.GetCodeSourceLocalFile())
	require.Equal(t, "https://example.com/yak.git", cfg.GetCodeSourceURL())
	require.Equal(t, "dev", cfg.GetCodeSourceBranch())
	require.Equal(t, "path/in/repo", cfg.GetCodeSourcePath())
	require.Equal(t, "token", cfg.GetCodeSourceAuthKind())
	require.Equal(t, "yak", cfg.GetCodeSourceAuthUserName())
	require.Equal(t, "secret", cfg.GetCodeSourceAuthPassword())
	require.Equal(t, "http://proxy:8080", cfg.GetCodeSourceProxyURL())
	proxyUser, proxyPass := cfg.GetCodeSourceProxyAuth()
	require.Equal(t, "proxyUser", proxyUser)
	require.Equal(t, "proxyPass", proxyPass)

	require.Equal(t, SFResultSaveNone, cfg.GetSyntaxFlowResultKind())
	require.True(t, cfg.GetScanMemory())
	require.True(t, cfg.SyntaxFlow.Memory)
	require.Equal(t, uint32(23), cfg.GetScanConcurrency())
	require.True(t, cfg.GetScanIgnoreLanguage())
	require.Equal(t, ControlModeResume, cfg.GetScanControlMode())
	require.Equal(t, []string{"go", "java"}, cfg.GetScanLanguage())

	require.NotNil(t, cfg.SyntaxFlowRule.RuleFilter)
	require.Equal(t, []string{"go"}, cfg.SyntaxFlowRule.RuleFilter.Language)
	require.Equal(t, "sql", cfg.SyntaxFlowRule.RuleFilter.Keyword)
	require.True(t, cfg.SyntaxFlowRule.RuleFilter.IncludeLibraryRule)
	require.Equal(t, "/tmp/key", cfg.GetCodeSourceAuth().KeyPath)
}

func TestWithScanRaw(t *testing.T) {
	cfg, err := New(ModeSyntaxFlowScan)
	require.NoError(t, err)

	req := &ypb.SyntaxFlowScanRequest{
		ControlMode:    string(ControlModeStart),
		IgnoreLanguage: true,
		ResumeTaskId:   "resume-001",
		Concurrency:    31,
		Memory:         true,
		ProgramName:    []string{"prog1", "prog2"},
		ProjectName:    []string{"project"},
		Filter: &ypb.SyntaxFlowRuleFilter{
			Keyword: "danger",
		},
		RuleInput: &ypb.SyntaxFlowRuleInput{
			RuleName: "rule-1",
		},
	}

	err = WithScanRaw(req)(cfg)
	require.NoError(t, err)

	require.Equal(t, ControlModeStart, cfg.GetScanControlMode())
	require.True(t, cfg.GetScanIgnoreLanguage())
	require.Equal(t, "resume-001", cfg.GetScanResumeTaskId())
	require.Equal(t, uint32(31), cfg.GetScanConcurrency())
	require.True(t, cfg.GetScanMemory())
	require.Equal(t, []string{"prog1", "prog2"}, cfg.BaseInfo.ProgramNames)
	require.Equal(t, "project", cfg.BaseInfo.ProjectName)
	require.Same(t, req.Filter, cfg.GetRuleFilter())
	require.Same(t, req.RuleInput, cfg.GetRuleInput())
}

func TestExtraInfo(t *testing.T) {
	cfg, err := New(ModeProjectBase)
	require.NoError(t, err)

	cfg.SetExtraInfo("string", "value")
	cfg.SetExtraInfo("int", 123)
	cfg.SetExtraInfo("bool", true)

	v, ok := cfg.GetExtraInfo("string")
	require.True(t, ok)
	require.Equal(t, "value", v)
	require.Equal(t, "value", cfg.GetExtraInfoString("string"))
	require.Equal(t, 123, cfg.GetExtraInfoInt("int"))
	require.True(t, cfg.GetExtraInfoBool("bool"))

	_, ok = cfg.GetExtraInfo("missing")
	require.False(t, ok)
	require.Equal(t, "", cfg.GetExtraInfoString("missing"))
	require.Equal(t, 0, cfg.GetExtraInfoInt("missing"))
	require.False(t, cfg.GetExtraInfoBool("missing"))
}

func TestOptionRequiresMode(t *testing.T) {
	cfg, err := New(ModeProjectBase)
	require.NoError(t, err)

	err = WithCompileStrictMode(true)(cfg)
	require.Error(t, err)

	err = WithScanConcurrency(3)(cfg)
	require.Error(t, err)

	err = WithRuleFilterLanguage("go")(cfg)
	require.Error(t, err)

	err = WithCodeSourceKind(CodeSourceGit)(cfg)
	require.Error(t, err)
}

// TestLazyInitialization verifies that nested structs are only created when options are applied
func TestLazyInitialization(t *testing.T) {
	t.Run("BaseInfo lazy initialization", func(t *testing.T) {
		cfg, err := New(ModeProjectBase)
		require.NoError(t, err)
		require.Nil(t, cfg.BaseInfo, "BaseInfo should be nil after New()")

		err = WithProgramNames("test")(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.BaseInfo, "BaseInfo should be created by WithProgramNames")
		require.Equal(t, []string{"test"}, cfg.BaseInfo.ProgramNames)
	})

	t.Run("SSACompile lazy initialization", func(t *testing.T) {
		cfg, err := New(ModeSSACompile)
		require.NoError(t, err)
		require.Nil(t, cfg.SSACompile, "SSACompile should be nil after New()")

		err = WithCompileStrictMode(true)(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.SSACompile, "SSACompile should be created by WithCompileStrictMode")
		require.True(t, cfg.SSACompile.StrictMode)
	})

	t.Run("SyntaxFlow lazy initialization", func(t *testing.T) {
		cfg, err := New(ModeSyntaxFlow)
		require.NoError(t, err)
		require.Nil(t, cfg.SyntaxFlow, "SyntaxFlow should be nil after New()")

		err = WithSyntaxFlowMemory(true)(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.SyntaxFlow, "SyntaxFlow should be created by WithSyntaxFlowMemory")
		require.True(t, cfg.SyntaxFlow.Memory)
	})

	t.Run("SyntaxFlowScan lazy initialization", func(t *testing.T) {
		cfg, err := New(ModeSyntaxFlowScanManager)
		require.NoError(t, err)
		require.Nil(t, cfg.SyntaxFlowScan, "SyntaxFlowScan should be nil after New()")

		err = WithScanConcurrency(10)(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.SyntaxFlowScan, "SyntaxFlowScan should be created by WithScanConcurrency")
		require.Equal(t, uint32(10), cfg.SyntaxFlowScan.Concurrency)
	})

	t.Run("SyntaxFlowRule lazy initialization", func(t *testing.T) {
		cfg, err := New(ModeSyntaxFlowRule)
		require.NoError(t, err)
		require.Nil(t, cfg.SyntaxFlowRule, "SyntaxFlowRule should be nil after New()")

		err = WithRuleFilterKeyword("test")(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.SyntaxFlowRule, "SyntaxFlowRule should be created by WithRuleFilterKeyword")
		require.NotNil(t, cfg.SyntaxFlowRule.RuleFilter)
		require.Equal(t, "test", cfg.SyntaxFlowRule.RuleFilter.Keyword)
	})

	t.Run("CodeSource lazy initialization", func(t *testing.T) {
		cfg, err := New(ModeCodeSource)
		require.NoError(t, err)
		require.Nil(t, cfg.CodeSource, "CodeSource should be nil after New()")

		err = WithCodeSourceKind(CodeSourceGit)(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.CodeSource, "CodeSource should be created by WithCodeSourceKind")
		require.Equal(t, CodeSourceGit, cfg.CodeSource.Kind)
	})
}

// TestDefaultFactoryFunctions verifies that default factory functions create proper defaults
func TestDefaultFactoryFunctions(t *testing.T) {
	t.Run("defaultBaseInfo", func(t *testing.T) {
		info := defaultBaseInfo()
		require.NotNil(t, info)
		require.Empty(t, info.ProgramNames)
		require.Empty(t, info.ProjectName)
	})

	t.Run("defaultSSACompileConfig", func(t *testing.T) {
		config := defaultSSACompileConfig()
		require.NotNil(t, config)
		require.False(t, config.StrictMode)
		require.Equal(t, 0, config.PeepholeSize)
		require.Empty(t, config.ExcludeFiles)
		require.False(t, config.ReCompile)
		require.False(t, config.MemoryCompile)
		require.Equal(t, uint32(1), config.Concurrency)
	})

	t.Run("defaultSyntaxFlowConfig", func(t *testing.T) {
		config := defaultSyntaxFlowConfig()
		require.NotNil(t, config)
		require.False(t, config.Memory)
		require.Equal(t, SFResultSaveNone, config.ResultSaveKind)
	})

	t.Run("defaultSyntaxFlowScanConfig", func(t *testing.T) {
		config := defaultSyntaxFlowScanConfig()
		require.NotNil(t, config)
		require.False(t, config.IgnoreLanguage)
		require.Empty(t, config.Language)
		require.Equal(t, uint32(5), config.Concurrency)
	})

	t.Run("defaultSyntaxFlowRuleConfig", func(t *testing.T) {
		config := defaultSyntaxFlowRuleConfig()
		require.NotNil(t, config)
		require.Empty(t, config.RuleNames)
		require.Nil(t, config.RuleFilter)
		require.Nil(t, config.RuleInput)
	})

	t.Run("defaultCodeSourceConfig", func(t *testing.T) {
		config := defaultCodeSourceConfig()
		require.NotNil(t, config)
		require.NotNil(t, config.Auth)
		require.NotNil(t, config.Proxy)
		require.Empty(t, config.Kind)
		require.Empty(t, config.LocalFile)
		require.Empty(t, config.URL)
	})
}

// TestModeBitmaskValidation verifies that options properly validate Mode bitmask
func TestModeBitmaskValidation(t *testing.T) {
	t.Run("BaseInfo options require ModeProjectBase", func(t *testing.T) {
		cfg, _ := New(ModeSSACompile) // Wrong mode

		err := WithProgramNames("test")(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Base mode")

		err = WithProgramDescription("desc")(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Base mode")

		err = WithProgramLanguage("Go")(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Base mode")
	})

	t.Run("Compile options require ModeSSACompile", func(t *testing.T) {
		cfg, _ := New(ModeProjectBase) // Wrong mode

		err := WithCompileStrictMode(true)(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Compile mode")

		err = WithCompilePeepholeSize(10)(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Compile mode")

		err = WithCompileConcurrency(5)(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Compile mode")
	})

	t.Run("SyntaxFlow options require ModeSyntaxFlow or ModeSyntaxFlowScanManager", func(t *testing.T) {
		cfg, _ := New(ModeProjectBase) // Wrong mode

		err := WithSyntaxFlowMemory(true)(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Scan mode")
	})

	t.Run("Scan options require ModeSyntaxFlowScanManager", func(t *testing.T) {
		cfg, _ := New(ModeProjectBase) // Wrong mode

		err := WithScanConcurrency(10)(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Scan mode")

		err = WithScanIgnoreLanguage(true)(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Scan mode")

		err = WithScanLanguage("go")(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Scan mode")
	})

	t.Run("Rule options require ModeSyntaxFlowRule", func(t *testing.T) {
		cfg, _ := New(ModeProjectBase) // Wrong mode

		err := WithRuleFilterKeyword("test")(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Rule mode")

		err = WithRuleFilterLanguage("go")(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Rule mode")

		err = WithRuleInput(&ypb.SyntaxFlowRuleInput{RuleName: "test"})(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Rule mode")
	})

	t.Run("CodeSource options require ModeCodeSource", func(t *testing.T) {
		cfg, _ := New(ModeProjectBase) // Wrong mode

		err := WithCodeSourceKind(CodeSourceGit)(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Code Source mode")

		err = WithCodeSourceURL("http://example.com")(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Code Source mode")

		err = WithCodeSourceBranch("main")(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Code Source mode")
	})
}

// TestCompositeModeInitialization verifies that composite modes work correctly
func TestCompositeModeInitialization(t *testing.T) {
	t.Run("ModeSyntaxFlowScan includes multiple modes", func(t *testing.T) {
		cfg, err := New(ModeSyntaxFlowScan)
		require.NoError(t, err)

		// Should allow BaseInfo options
		err = WithProgramNames("test")(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.BaseInfo)

		// Should allow SyntaxFlow options
		err = WithSyntaxFlowMemory(true)(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.SyntaxFlow)

		// Should allow Scan options
		err = WithScanConcurrency(10)(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.SyntaxFlowScan)

		// Should allow Rule options
		err = WithRuleFilterKeyword("test")(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.SyntaxFlowRule)
	})

	t.Run("ModeAll includes all modes", func(t *testing.T) {
		cfg, err := New(ModeAll)
		require.NoError(t, err)

		// Should allow all option types
		err = WithProgramNames("test")(cfg)
		require.NoError(t, err)

		err = WithCompileStrictMode(true)(cfg)
		require.NoError(t, err)

		err = WithSyntaxFlowMemory(true)(cfg)
		require.NoError(t, err)

		err = WithScanConcurrency(10)(cfg)
		require.NoError(t, err)

		err = WithRuleFilterKeyword("test")(cfg)
		require.NoError(t, err)

		err = WithCodeSourceKind(CodeSourceGit)(cfg)
		require.NoError(t, err)

		// All nested structs should be initialized
		require.NotNil(t, cfg.BaseInfo)
		require.NotNil(t, cfg.SSACompile)
		require.NotNil(t, cfg.SyntaxFlow)
		require.NotNil(t, cfg.SyntaxFlowScan)
		require.NotNil(t, cfg.SyntaxFlowRule)
		require.NotNil(t, cfg.CodeSource)
	})
}

// TestMultipleOptionsOnSameField verifies that multiple options can modify the same field
func TestMultipleOptionsOnSameField(t *testing.T) {
	cfg, err := New(ModeProjectBase,
		WithProgramNames("prog1"),
		WithProgramNames("prog2", "prog3"),
	)
	require.NoError(t, err)
	require.NotNil(t, cfg.BaseInfo)
	require.Equal(t, []string{"prog1", "prog2", "prog3"}, cfg.BaseInfo.ProgramNames)
}
