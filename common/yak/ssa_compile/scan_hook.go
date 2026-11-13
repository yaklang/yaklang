package ssa_compile

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
)

func init() {
	syntaxflow_scan.CompileProject = compileForProductScan
}

// compileForProductScan is the production compiler for a project scan.
//
// syntaxflow_scan cannot import this package: ssa_compile -> yakscript -> yak
// -> syntaxflow_scan. The init registration is what keeps auto-detect
// (ParseProjectWithConfig) on the product path. When this package is not
// linked, CompileProject stays nil and the scan falls back to ParseProject.
func compileForProductScan(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
	if err := resolveProductLanguage(ctx, cfg); err != nil {
		return nil, err
	}
	res, err := ParseProjectWithConfig(ctx, cfg, extra...)
	if err != nil {
		return nil, err
	}
	if res == nil || res.Program == nil {
		return nil, utils.Errorf("compile result is empty")
	}
	return res.Program, nil
}

// resolveProductLanguage fills the project language when the caller omitted it.
//
// The product compile runs in-process (struct rules and callbacks force
// ExtraInfo), so it never passes through the "SSA 项目探测" step that the CLI
// compile command uses. An omitted --language therefore reached the compiler as
// "": no builder was selected, the compiler warned "no language builder
// specified ... extra file analyzing disabled", and every language-typed rule
// was skipped as a language mismatch. Detection only fills an empty language;
// an explicit language is an input fact and always wins.
func resolveProductLanguage(ctx context.Context, cfg *ssaconfig.Config) error {
	if cfg == nil || cfg.GetLanguage() != "" {
		return nil
	}
	// Only local inputs are inspected: a git/svn URL is collected (and
	// retargeted to its local tree) before the compile reaches this point.
	switch cfg.GetCodeSourceKind() {
	case ssaconfig.CodeSourceLocal, ssaconfig.CodeSourceCompression, ssaconfig.CodeSourceJar:
	default:
		return nil
	}
	target := strings.TrimSpace(cfg.GetCodeSourceLocalFileOrURL())
	if target == "" {
		return nil
	}
	info, err := detectProject(ctx, target, "")
	if err != nil {
		// An undetectable language is not a reason to fail a scan that used to
		// compile with the all-builders fallback; keep that path and say why.
		log.Warnf("detect project language for %s failed: %v", target, err)
		return nil
	}
	if info == nil || info.Config == nil {
		log.Warnf("detect project language for %s returned no config", target)
		return nil
	}
	language := info.Config.GetLanguage()
	if language == "" {
		log.Warnf("detect project language for %s returned an empty language", target)
		return nil
	}
	if err := cfg.Update(ssaconfig.WithProjectRawLanguage(string(language))); err != nil {
		return utils.Wrapf(err, "apply detected project language %q", language)
	}
	log.Infof("[ssa-compile] detected project language %q for %s", language, target)
	return nil
}
