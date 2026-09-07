package ssaapi

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// SourceQueryTarget is a SyntaxFlowQueryInstance backed only by raw source
// files (no SSA IR). Source-mode rules run via sfpattern against these files.
type SourceQueryTarget struct {
	name  string
	lang  ssaconfig.Language
	files map[string]string
}

var _ SyntaxFlowQueryInstance = (*SourceQueryTarget)(nil)

// NewSourceQueryTarget builds a no-IR query target from path→content.
func NewSourceQueryTarget(name string, files map[string]string) *SourceQueryTarget {
	if files == nil {
		files = map[string]string{}
	}
	if name == "" {
		name = "source"
	}
	return &SourceQueryTarget{
		name:  name,
		lang:  ssaconfig.General,
		files: files,
	}
}

// NewSourceQueryTargetFromFS loads all files then NewSourceQueryTarget.
func NewSourceQueryTargetFromFS(name string, fsys fi.FileSystem) (*SourceQueryTarget, error) {
	files, err := sfpattern.LoadFilesFromFS(fsys)
	if err != nil {
		return nil, err
	}
	return NewSourceQueryTarget(name, files), nil
}

func (t *SourceQueryTarget) GetProgramName() string {
	if t == nil {
		return ""
	}
	return t.name
}

func (t *SourceQueryTarget) GetLanguage() ssaconfig.Language {
	if t == nil || t.lang == "" {
		return ssaconfig.General
	}
	return t.lang
}

func (t *SourceQueryTarget) SetLanguage(lang ssaconfig.Language) *SourceQueryTarget {
	if t != nil {
		t.lang = lang
	}
	return t
}

func (t *SourceQueryTarget) IsIncrementalCompile() bool { return false }
func (t *SourceQueryTarget) IsBaseProgram() bool        { return true }
func (t *SourceQueryTarget) GetBaseProgramName() string { return t.GetProgramName() }
func (t *SourceQueryTarget) Recompile(...ssaconfig.Option) error {
	return nil
}

func (t *SourceQueryTarget) Files() map[string]string {
	if t == nil {
		return nil
	}
	return t.files
}

func (t *SourceQueryTarget) SyntaxFlowWithError(i string, opts ...QueryOption) (*SyntaxFlowResult, error) {
	return t.syntaxFlow(opts, QueryWithRuleContent(i))
}

func (t *SourceQueryTarget) SyntaxFlowRule(rule *schema.SyntaxFlowRule, opts ...QueryOption) (*SyntaxFlowResult, error) {
	if t == nil {
		return nil, utils.Error("nil SourceQueryTarget")
	}
	if rule != nil && !sfvm.RuleIsSourceMode(rule, nil) {
		return nil, utils.Errorf(
			"source target cannot execute non-source rule %s (mode=%s)",
			rule.RuleName,
			schema.ValidRuleMode(rule.Mode),
		)
	}
	return t.syntaxFlow(opts, QueryWithRule(rule))
}

func (t *SourceQueryTarget) syntaxFlow(opts []QueryOption, ruleOpt QueryOption) (*SyntaxFlowResult, error) {
	if t == nil {
		return nil, utils.Error("nil SourceQueryTarget")
	}
	root := sfpattern.NewRoot(t.files)
	root.SetProgramName(t.name)
	prog := NewTmpProgram(t.name)
	all := make([]QueryOption, 0, len(opts)+3)
	all = append(all, QueryWithValue(root), ruleOpt, QueryWithResultProgram(prog))
	all = append(all, opts...)
	return QuerySyntaxflow(all...)
}

// collectFilesForSourceMode gathers path→content for mode=source frames.
func collectFilesForSourceMode(config *queryConfig) (map[string]string, error) {
	if config == nil {
		return nil, utils.Error("source mode: nil query config")
	}
	for _, v := range config.value {
		if r, ok := v.(*sfvm.PatternRoot); ok && r != nil {
			return r.Files(), nil
		}
		if p, ok := v.(*Program); ok && p != nil {
			return p.CollectSourceFiles(), nil
		}
	}
	if config.program != nil {
		return config.program.CollectSourceFiles(), nil
	}
	return nil, utils.Error("source mode: no source files (need PatternRoot, SourceQueryTarget, or Program)")
}

// CollectSourceFiles returns path→content for all FileList + ExtraFile editors.
func (p *Program) CollectSourceFiles() map[string]string {
	out := make(map[string]string)
	if p == nil {
		return out
	}
	p.forEachFileListAndExtraFile(func(path string, me *memedit.MemEditor) bool {
		if me == nil {
			return true
		}
		out[path] = me.GetSourceCode()
		return true
	})
	return out
}

// QueryWithResultProgram attaches a Program for result/risk metadata without
// replacing the feed ValueOperator (used by source-mode PatternRoot feeds).
func QueryWithResultProgram(program *Program) QueryOption {
	return func(c *queryConfig) {
		c.program = program
	}
}
