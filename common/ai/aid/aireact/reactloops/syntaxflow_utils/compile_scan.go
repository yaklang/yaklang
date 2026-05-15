package syntaxflow_utils

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
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

// CodeScanConfigResolveSource describes how a code-scan config was obtained before compile.
type CodeScanConfigResolveSource string

const (
	CodeScanConfigSourceExplicitJSON    CodeScanConfigResolveSource = "explicit_json"
	CodeScanConfigSourceExistingProject CodeScanConfigResolveSource = "existing_project"
	CodeScanConfigSourceAutoDetectNew   CodeScanConfigResolveSource = "auto_detect_new"
)

// CodeScanConfigResolveOutcome is the resolved config plus metadata for user-facing stage markdown.
type CodeScanConfigResolveOutcome struct {
	Config       *ssaconfig.Config
	Source       CodeScanConfigResolveSource
	ProjectName  string
	ProjectID    uint64
	ResolvedPath string
}

func normalizeLocalProjectPath(localPath string) (string, error) {
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		return "", utils.Error("empty project path")
	}
	st, err := os.Stat(localPath)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		localPath = filepath.Dir(localPath)
	}
	return filepath.Clean(localPath), nil
}

func lookupExistingSSAProject(projectName, localPath string) (*schema.SSAProject, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, nil
	}
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		return nil, nil
	}
	q := db.Where("url = ?", localPath)
	if projectName = strings.TrimSpace(projectName); projectName != "" {
		q = q.Where("project_name = ?", projectName)
	}
	var project schema.SSAProject
	err := q.First(&project).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func configFromExistingSSAProject(ctx context.Context, project *schema.SSAProject) (*ssaconfig.Config, error) {
	if project == nil {
		return nil, utils.Error("ssa project is nil")
	}
	cfg, err := project.GetConfig()
	if err != nil {
		return nil, err
	}
	if project.ID > 0 {
		if err := cfg.Update(ssaconfig.WithProjectID(uint64(project.ID))); err != nil {
			return nil, err
		}
	}
	if ctx != nil {
		if err := cfg.Update(ssaconfig.WithContext(ctx)); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// ResolveCodeScanConfigForLocalPath resolves config for path-only intake: reuse SSAProject by url (+ optional project_name) or auto-detect and sync project row.
func ResolveCodeScanConfigForLocalPath(ctx context.Context, localPath, projectName string) (*CodeScanConfigResolveOutcome, error) {
	resolvedPath, err := normalizeLocalProjectPath(localPath)
	if err != nil {
		return nil, err
	}
	if existing, err := lookupExistingSSAProject(projectName, resolvedPath); err == nil && existing != nil {
		cfg, err := configFromExistingSSAProject(ctx, existing)
		if err != nil {
			return nil, err
		}
		out := &CodeScanConfigResolveOutcome{
			Config:       cfg,
			Source:       CodeScanConfigSourceExistingProject,
			ProjectName:  cfg.GetProjectName(),
			ProjectID:    uint64(existing.ID),
			ResolvedPath: resolvedPath,
		}
		if out.ProjectName == "" {
			out.ProjectName = existing.ProjectName
		}
		return out, nil
	} else if err != nil {
		return nil, err
	}

	res, err := ssa_compile.ParseProjectWithAutoDetective(ctx, &ssa_compile.SSADetectConfig{
		Target:             resolvedPath,
		CompileImmediately: false,
	})
	if err != nil {
		return nil, err
	}
	if res == nil || res.Info == nil || res.Info.Config == nil {
		return nil, utils.Error("ssa auto-detect returned no config")
	}
	cfg := res.Info.Config
	source := CodeScanConfigSourceAutoDetectNew
	if res.Info.ProjectExists {
		source = CodeScanConfigSourceExistingProject
	}
	if db := consts.GetGormProfileDatabase(); db != nil {
		cfg, _, err = ssa_compile.EnsureSSAProjectRowForCodeScan(ctx, db, cfg)
		if err != nil {
			return nil, err
		}
	}
	return &CodeScanConfigResolveOutcome{
		Config:       cfg,
		Source:       source,
		ProjectName:  cfg.GetProjectName(),
		ProjectID:    cfg.GetProjectID(),
		ResolvedPath: resolvedPath,
	}, nil
}

// ResolveCodeScanConfigFromJSON 从 code-scan 族 JSON 得到落库后的编译配置（不编译）。
func ResolveCodeScanConfigFromJSON(ctx context.Context, jsonRaw []byte) (*CodeScanConfigResolveOutcome, error) {
	if len(jsonRaw) == 0 {
		return nil, utils.Error("empty code-scan config json")
	}
	cfg, err := ssaconfig.NewCLIScanConfig(ssaconfig.WithJsonRawConfig(jsonRaw))
	if err != nil {
		return nil, err
	}
	if db := consts.GetGormProfileDatabase(); db != nil {
		cfg, _, err = ssa_compile.EnsureSSAProjectRowForCodeScan(ctx, db, cfg)
		if err != nil {
			return nil, err
		}
	}
	return &CodeScanConfigResolveOutcome{
		Config:      cfg,
		Source:      CodeScanConfigSourceExplicitJSON,
		ProjectName: cfg.GetProjectName(),
		ProjectID:   cfg.GetProjectID(),
	}, nil
}

// ResolveCodeScanConfigFromProjectPath 仅做「SSA 项目探测」+ SSAProject 行同步（不编译）。path 可为文件或目录。
func ResolveCodeScanConfigFromProjectPath(ctx context.Context, localPath string) (*ssaconfig.Config, error) {
	out, err := ResolveCodeScanConfigForLocalPath(ctx, localPath, "")
	if err != nil {
		return nil, err
	}
	return out.Config, nil
}

// LoadCompiledProgramsByName loads already-compiled SSA Programs from the database by program_name.
func LoadCompiledProgramsByName(programName string) ([]*ssaapi.Program, error) {
	programName = strings.TrimSpace(programName)
	if programName == "" {
		return nil, utils.Error("program_name is empty")
	}
	ret := ssa_compile.LoadProgramsMatchingName(programName)
	if len(ret) == 0 {
		return nil, utils.Errorf("数据库中未找到 SSA Program: %s", programName)
	}
	return ret, nil
}

// CompileProgramsFromCodeScanConfig 在已解析的 cfg 上执行编译或从 DB 装载 Program（仅这一步触碰 ssa_compile 编译/加载）。
func CompileProgramsFromCodeScanConfig(ctx context.Context, cfg *ssaconfig.Config) ([]*ssaapi.Program, error) {
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}
	targetPath := cfg.GetCodeSourceLocalFileOrURL()
	programName := strings.TrimSpace(cfg.GetProgramName())

	if targetPath != "" {
		if !cfg.GetCompileMemory() && programName == "" {
			cfg.SetProgramName(InferredSSAProgramNameForPath(targetPath))
		}
		compileRes, err := ssa_compile.ParseProjectWithConfig(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if compileRes == nil || compileRes.Program == nil {
			return nil, utils.Errorf("ssa_compile 未产生 Program")
		}
		return []*ssaapi.Program{compileRes.Program}, nil
	}
	if programName != "" {
		ret := ssa_compile.LoadProgramsMatchingName(programName)
		if len(ret) == 0 {
			return nil, utils.Errorf("数据库中未找到 SSA Program: %s", programName)
		}
		return ret, nil
	}
	return nil, utils.Errorf("code-scan 配置需包含 CodeSource 路径/URL，或 program_name 指向已编译 Program")
}

// LoadProgramsFromCodeScanJSON parses code-scan JSON、解析 cfg 并编译/装载 Programs。
func LoadProgramsFromCodeScanJSON(ctx context.Context, jsonRaw []byte) (cfg *ssaconfig.Config, progs []*ssaapi.Program, err error) {
	out, err := ResolveCodeScanConfigFromJSON(ctx, jsonRaw)
	if err != nil {
		return nil, nil, err
	}
	cfg = out.Config
	progs, err = CompileProgramsFromCodeScanConfig(ctx, cfg)
	if err != nil {
		return cfg, nil, err
	}
	return cfg, progs, nil
}

// LoadProgramsFromProjectPath 探测路径得到 cfg 后，再 [CompileProgramsFromCodeScanConfig]。
func LoadProgramsFromProjectPath(ctx context.Context, localPath string) (cfg *ssaconfig.Config, progs []*ssaapi.Program, err error) {
	cfg, err = ResolveCodeScanConfigFromProjectPath(ctx, localPath)
	if err != nil {
		return nil, nil, err
	}
	progs, err = CompileProgramsFromCodeScanConfig(ctx, cfg)
	if err != nil {
		return cfg, nil, err
	}
	return cfg, progs, nil
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
