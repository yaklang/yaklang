package syntaxflow_services

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
)

var programNameSanitize = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// InferredSSAProgramNameForPath returns a stable, DB-suitable program name for same-process
// SyntaxFlow / code-scan runs when the user did not set program_name in JSON.
func InferredSSAProgramNameForPath(localPath string) string {
	clean := filepath.Clean(strings.TrimSpace(localPath))
	base := filepath.Base(clean)
	base = programNameSanitize.ReplaceAllString(base, "_")
	if base == "" || base == "." {
		base = "proj"
	}
	sum := sha256.Sum256([]byte(clean))
	return fmt.Sprintf("ai_sf_%s_%x", base, sum[:6])
}

// BuildCodeScanJSONForLocalPath builds a minimal code-scan JSON for a local file or directory
// (in-process compile + SyntaxFlow; no language guessing from full user text).
func BuildCodeScanJSONForLocalPath(localPath string) (string, error) {
	p := strings.TrimSpace(localPath)
	if p == "" {
		return "", errors.New("empty local path")
	}
	if st, err := os.Stat(p); err != nil {
		return "", err
	} else if !st.IsDir() {
		p = filepath.Dir(p)
	}
	raw, err := buildMinimalInProcessCodeScanJSON(p)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildMinimalInProcessCodeScanJSON(localPath string) ([]byte, error) {
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		return nil, errors.New("empty path")
	}
	cfg, err := ssaconfig.NewCLIScanConfig(
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(localPath),
		ssaconfig.WithCompileMemoryCompile(false),
		ssaconfig.WithSyntaxFlowMemory(false),
		ssaconfig.WithSetProgramName(InferredSSAProgramNameForPath(localPath)),
	)
	if err != nil {
		return nil, err
	}
	return cfg.ToJSONRaw()
}

// LoadProgramsFromCodeScanJSON parses code-scan JSON (same family as `yak code-scan --config`) and loads SSA Programs.
func LoadProgramsFromCodeScanJSON(ctx context.Context, jsonRaw []byte) (cfg *ssaconfig.Config, progs []*ssaapi.Program, err error) {
	if len(jsonRaw) == 0 {
		return nil, nil, utils.Error("empty code-scan config json")
	}
	cfg, err = ssaconfig.NewCLIScanConfig(ssaconfig.WithJsonRawConfig(jsonRaw))
	if err != nil {
		return nil, nil, err
	}
	if db := consts.GetGormProfileDatabase(); db != nil {
		cfg, _, err = ssa_compile.EnsureSSAProjectRowForCodeScan(ctx, db, cfg)
		if err != nil {
			return nil, nil, err
		}
	}
	progs, err = loadProgramsForCodeScanConfig(ctx, cfg)
	if err != nil {
		return cfg, nil, err
	}
	return cfg, progs, nil
}

func loadProgramsForCodeScanConfig(ctx context.Context, cfg *ssaconfig.Config) ([]*ssaapi.Program, error) {
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}
	targetPath := cfg.GetCodeSourceLocalFileOrURL()
	programName := strings.TrimSpace(cfg.GetProgramName())

	if targetPath != "" {
		if !cfg.GetCompileMemory() && strings.TrimSpace(cfg.GetProgramName()) == "" {
			cfg.SetProgramName(InferredSSAProgramNameForPath(targetPath))
		}
		configJSON, err := cfg.ToJSONString()
		if err != nil {
			return nil, err
		}
		ps, err := ssaapi.ParseProject(
			ssaconfig.WithConfigJson(configJSON),
			ssaconfig.WithContext(ctx),
		)
		if err != nil {
			return nil, err
		}
		if len(ps) == 0 {
			return nil, utils.Errorf("内存编译未产生任何 Program")
		}
		return []*ssaapi.Program(ps), nil
	}
	if programName != "" {
		ret := ssaapi.LoadProgramRegexp(programName)
		if len(ret) == 0 {
			return nil, utils.Errorf("数据库中未找到 SSA Program: %s", programName)
		}
		return ret, nil
	}
	return nil, utils.Errorf("code-scan JSON 需包含 CodeSource 本地路径，或 BaseInfo.program_names 指向已编译 Program")
}

// CodeScanToSyntaxFlowRuleOptions aligns with yak code-scan useConfigMode options for StartScan (subset; no WithPrograms / WithContext).
func CodeScanToSyntaxFlowRuleOptions(cfg *ssaconfig.Config) []ssaconfig.Option {
	if cfg == nil {
		return nil
	}
	out := make([]ssaconfig.Option, 0, 4)
	out = append(out, ssaconfig.WithRuleFilterLibRuleKind("noLib"))
	out = append(out, ssaconfig.WithSyntaxFlowMemory(cfg.GetSyntaxFlowMemory()))
	if rf := cfg.GetRuleFilter(); rf != nil {
		out = append(out, ssaconfig.WithRuleFilter(rf))
	}
	return out
}

// StartSyntaxFlowScanBackground runs [syntaxflow_scan.StartScanInBackground] with compiled programs + rule-aligned options from cfg.
func StartSyntaxFlowScanBackground(ctx context.Context, cfg *ssaconfig.Config, progs []*ssaapi.Program) (taskID string, err error) {
	opts := make([]ssaconfig.Option, 0, 8)
	if ctx != nil {
		opts = append(opts, ssaconfig.WithContext(ctx))
	}
	opts = append(opts, syntaxflow_scan.WithPrograms(progs...))
	opts = append(opts, CodeScanToSyntaxFlowRuleOptions(cfg)...)
	return syntaxflow_scan.StartScanInBackground(ctx, opts...)
}

// StartSyntaxFlowScanBackgroundWithRuleFile is like [StartSyntaxFlowScanBackground] but appends inlined rule content from disk.
func StartSyntaxFlowScanBackgroundWithRuleFile(ctx context.Context, cfg *ssaconfig.Config, progs []*ssaapi.Program, rulePath string) (taskID string, err error) {
	raw, err := os.ReadFile(rulePath)
	if err != nil {
		return "", err
	}
	opts := make([]ssaconfig.Option, 0, 10)
	if ctx != nil {
		opts = append(opts, ssaconfig.WithContext(ctx))
	}
	opts = append(opts, syntaxflow_scan.WithPrograms(progs...))
	opts = append(opts, CodeScanToSyntaxFlowRuleOptions(cfg)...)
	opts = append(opts, ssaconfig.WithRuleInputRaw(strings.TrimSpace(string(raw))))
	return syntaxflow_scan.StartScanInBackground(ctx, opts...)
}
