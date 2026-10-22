package ssaapi

import (
	"sort"

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
	store sfpattern.SourceStore
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
		store: sfpattern.MapSource(files),
	}
}

// NewSourceQueryTargetFromStore scans a store without copying every file body
// into the target.
func NewSourceQueryTargetFromStore(name string, store sfpattern.SourceStore) *SourceQueryTarget {
	if name == "" {
		name = "source"
	}
	return &SourceQueryTarget{
		name:  name,
		lang:  ssaconfig.General,
		store: store,
	}
}

// NewSourceQueryTargetFromFS lists fsys and reads bodies on demand.
func NewSourceQueryTargetFromFS(name string, fsys fi.FileSystem) (*SourceQueryTarget, error) {
	store, err := sfpattern.OpenStreamingSource(fsys, nil, sfpattern.DefaultSourceInFlight)
	if err != nil {
		return nil, err
	}
	return NewSourceQueryTargetFromStore(name, store), nil
}

// NewSourceQueryTargetFromProgram builds a source-mode target from the
// compiled program snapshot (FileList / ExtraFile / IrSource editors).
// Live workspace files are not used; the snapshot is what was compiled.
func NewSourceQueryTargetFromProgram(prog *Program) *SourceQueryTarget {
	if prog == nil || prog.Program == nil {
		return NewSourceQueryTarget("", nil)
	}
	target := NewSourceQueryTargetFromStore(prog.GetProgramName(), newProgramSource(prog))
	if lang := prog.GetLanguage(); lang != "" {
		target.lang = lang
	}
	return target
}

// programSource reads one compiled file when a rule asks for it. The program
// already owns the editors; this does not copy them into another map.
type programSource struct {
	prog  *Program
	paths []string
	raw   map[string]string
	hash  map[string]string
}

// NewProgramSourceStore reads a compiled program one file at a time.
func NewProgramSourceStore(prog *Program) sfpattern.SourceStore {
	if prog == nil || prog.Program == nil {
		return sfpattern.MapSource(nil)
	}
	return newProgramSource(prog)
}

func newProgramSource(prog *Program) sfpattern.SourceStore {
	name := prog.GetProgramName()
	raw := map[string]string{}
	hash := map[string]string{}
	var paths []string
	for _, listing := range []map[string]string{prog.Program.FileList, prog.Program.ExtraFile} {
		for path, fileHash := range listing {
			key := normalizeOverlayFilePath(path, name)
			if key == "" {
				continue
			}
			if _, seen := raw[key]; seen {
				continue
			}
			raw[key] = path
			hash[key] = fileHash
			paths = append(paths, key)
		}
	}
	sort.Strings(paths)
	return &programSource{prog: prog, paths: paths, raw: raw, hash: hash}
}

func (p *programSource) Paths() []string {
	if p == nil {
		return nil
	}
	return p.paths
}

func (p *programSource) Read(path string) (string, func(), error) {
	if p == nil || p.prog == nil {
		return "", nil, utils.Error("nil program source")
	}
	raw, ok := p.raw[path]
	if !ok {
		return "", nil, utils.Errorf("source file %q not found", path)
	}
	editor, err := p.prog.getEditor(raw, p.hash[path])
	if err != nil || editor == nil {
		if content, ok := readProgramFileContent(p.prog, raw); ok {
			return content, func() {}, nil
		}
		if err == nil {
			err = utils.Errorf("source file %q has no editor", path)
		}
		return "", nil, err
	}
	return editor.GetSourceCode(), func() {}, nil
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

// Paths returns the scan list without reading file bodies.
func (t *SourceQueryTarget) Paths() []string {
	if t == nil || t.store == nil {
		return nil
	}
	return t.store.Paths()
}

// SourceStore returns the reader used by the source pre-scan and rule root.
func (t *SourceQueryTarget) SourceStore() sfpattern.SourceStore {
	if t == nil {
		return nil
	}
	if t.store != nil {
		return t.store
	}
	return sfpattern.MapSource(t.files)
}

func (t *SourceQueryTarget) SyntaxFlowWithError(i string, opts ...QueryOption) (*SyntaxFlowResult, error) {
	return t.syntaxFlow(opts, QueryWithRuleContent(i))
}

func (t *SourceQueryTarget) SyntaxFlowRule(rule *schema.SyntaxFlowRule, opts ...QueryOption) (*SyntaxFlowResult, error) {
	if t == nil {
		return nil, utils.Error("nil SourceQueryTarget")
	}
	if rule != nil && !rule.IsSourceMode() {
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
	root := sfpattern.NewRootFromStore(t.SourceStore())
	root.SetProgramName(t.name)
	prog := NewTmpProgram(t.name)
	all := make([]QueryOption, 0, len(opts)+3)
	all = append(all, QueryWithValue(root), ruleOpt, QueryWithResultProgram(prog))
	all = append(all, opts...)
	return QuerySyntaxflow(all...)
}

func existingPatternRoot(config *queryConfig) *sfvm.PatternRoot {
	if config == nil {
		return nil
	}
	for _, v := range config.value {
		if root, ok := v.(*sfvm.PatternRoot); ok && root != nil && !root.IsEmpty() {
			return root
		}
	}
	return nil
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
