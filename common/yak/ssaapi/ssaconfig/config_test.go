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

	require.NotNil(t, cfg.BaseInfo)
	require.NotNil(t, cfg.SyntaxFlow)
	require.NotNil(t, cfg.SyntaxFlowScan)
	require.NotNil(t, cfg.SyntaxFlowRule)

	require.Equal(t, uint32(5), cfg.SyntaxFlowScan.Concurrency)
	require.False(t, cfg.SyntaxFlowScan.IgnoreLanguage)
	require.Equal(t, SFResultSaveNone, cfg.SyntaxFlow.ResultSaveKind)
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
