package ssaconfig

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
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
		WithProjectLanguage(GO),
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

	stringGot, ok := cfg.GetExtraInfo("string")
	require.True(t, ok)
	require.Equal(t, 1, len(stringGot))
	require.Equal(t, "value", stringGot[0])

	intGot, ok := cfg.GetExtraInfo("int")
	require.True(t, ok)
	require.Equal(t, 1, len(intGot))
	require.Equal(t, 123, intGot[0])

	_, ok = cfg.GetExtraInfo("missing")
	require.False(t, ok)
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

		err = WithProjectLanguage("Go")(cfg)
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

		err = WithScanLanguage(GO)(cfg)
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

type OtherScanConfig struct {
	*Config
	OtherField string
}

const otherKey = "other_scan/otherField"

var WithOtherField = SetOption(otherKey, func(c *OtherScanConfig, val string) {
	c.OtherField = val
})

func NewOtherScanConfig(opts ...Option) (*OtherScanConfig, error) {
	cfg := &OtherScanConfig{
		Config: &Config{},
	}
	var err error
	cfg.Config, err = New(ModeSyntaxFlowScan, opts...)
	if err != nil {
		return nil, err
	}

	ApplyExtraOptions(cfg, cfg.Config)
	return cfg, nil
}

// --- 测试 ---

func TestApplyExtraOptions(t *testing.T) {

	t.Run("Correctly applies extra option to derived config", func(t *testing.T) {
		key := uuid.NewString()
		prefix := "other_scan/"
		run := false
		withCheckCallback := SetOption(key, func(c *OtherScanConfig, value string) {
			run = true
			c.OtherField = prefix + value
		})

		value := uuid.NewString()
		want := prefix + value

		cfg, err := NewOtherScanConfig(
			withCheckCallback(value),
		)

		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.True(t, run, "Callback should have been executed")
		require.Equal(t, want, cfg.OtherField, "OtherField should be set correctly by ApplyExtraOptions")

	})

	t.Run("handler has string with option", func(t *testing.T) {
		key := uuid.NewString()
		cfg, err := NewOtherScanConfig(
			WithOtherField(key),
		)

		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Equal(t, key, cfg.OtherField, "OtherField should be set correctly by ApplyExtraOptions")
	})

	t.Run("Handles nil options without error", func(t *testing.T) {
		// 1. 不传入任何选项
		cfg, err := NewOtherScanConfig()

		// 2. 断言
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// 3. 验证特定字段为 nil
		require.Empty(t, cfg.OtherField, "OtherField should be empty when no option is provided")
	})

	t.Run("Safely ignores extra option for a different type", func(t *testing.T) {
		// 1. 传入一个为 *OtherScanConfig 准备的选项
		cfg, err := NewSyntaxFlowScanConfig(
			WithOtherField("this-should-be-ignored"),
		)
		ApplyExtraOptions(cfg, cfg)

		// 2. 断言
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// 4. 验证 ExtraInfo 仍然包含了这个"不相关"的选项 (它只是没被应用)
		require.Contains(t, cfg.ExtraInfo, otherKey, "ExtraInfo should still contain the other key")
		require.IsType(t, ExtraOption[*OtherScanConfig]{}, cfg.ExtraInfo[otherKey], "The stored type should be for OtherScanConfig")
	})
}

type MapConfig struct {
	*Config
	FieldMap map[string]string
}

var withFeildMap = SetOption("map_config/field_map", func(c *MapConfig, val struct {
	name  string
	value string
}) {
	if c.FieldMap == nil {
		c.FieldMap = make(map[string]string)
	}
	c.FieldMap[val.name] = val.value
})

func WithFieldMap(name, value string) Option {
	return withFeildMap(struct {
		name  string
		value string
	}{
		name:  name,
		value: value,
	})
}

func NewMapConfig(opts ...Option) (*MapConfig, error) {
	cfg := &MapConfig{
		Config: &Config{},
	}
	var err error
	cfg.Config, err = New(ModeProjectBase, opts...)
	if err != nil {
		return nil, err
	}

	ApplyExtraOptions(cfg, cfg.Config)
	return cfg, nil
}

func TestWrapOption(t *testing.T) {
	t.Run("Correctly applies map option to derived config", func(t *testing.T) {
		cfg, err := NewMapConfig(
			WithFieldMap("key1", "value1"),
			WithFieldMap("key2", "value2"),
		)

		log.Errorf("%+v", cfg)

		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.NotNil(t, cfg.FieldMap)
		require.Equal(t, 2, len(cfg.FieldMap))

		// check key value
		value1, ok1 := cfg.FieldMap["key1"]
		require.True(t, ok1)
		require.Equal(t, "value1", value1)

		value2, ok2 := cfg.FieldMap["key2"]
		require.True(t, ok2)
		require.Equal(t, "value2", value2)
	})
}
