package yak2ssa

import (
	"fmt"
	"path"
	"sort"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type fileBuildState uint8

const (
	filePending fileBuildState = iota
	fileBuilding
	fileCompleted
	fileFailed
)

// Yak bodies are built serially. Project roots keep their original builder;
// includes share the same file result, even when ASTs arrive in another order.
type fileBuild struct {
	hash        string
	state       fileBuildState
	builderID   int64
	definitions map[string]int64
	exports     map[string]int64
	err         error
}

func (s *SSABuilder) fileRecord(b *ssa.FunctionBuilder, editor *memedit.MemEditor) (*fileBuild, error) {
	app := b.GetProgram().GetApplication()
	if s.fileBuilds == nil {
		s.fileBuilds = make(map[*ssa.Program]map[string]*fileBuild)
	}
	files := s.fileBuilds[app]
	if files == nil {
		files = make(map[string]*fileBuild)
		s.fileBuilds[app] = files
	}
	key := path.Clean(editor.GetUrl())
	hash := editor.GetIrSourceHash()
	if record := files[key]; record != nil {
		if record.hash != hash {
			return nil, fmt.Errorf("Yak source changed during compilation: %s", key)
		}
		return record, nil
	}
	record := &fileBuild{hash: hash, exports: make(map[string]int64)}
	files[key] = record
	return record, nil
}

func (s *SSABuilder) buildFile(ast ssa.FrontAST, b *ssa.FunctionBuilder) (err error) {
	b.SupportClosure = true
	if b.GetEditor() == nil {
		return fmt.Errorf("Yak file has no source editor")
	}
	record, err := s.fileRecord(b, b.GetEditor())
	if err != nil {
		return err
	}
	switch record.state {
	case fileCompleted:
		// An include-only file can become a project root in a later batch.
		if record.builderID != b.GetId() {
			if err := b.AddIncludedValues(record.exports); err != nil {
				return err
			}
			if err := importFileValues(b, record.definitions); err != nil {
				return err
			}
		}
		return nil
	case fileBuilding:
		return fmt.Errorf("cyclic Yak include: %s", b.GetEditor().GetUrl())
	case fileFailed:
		return record.err
	}
	record.state = fileBuilding
	record.builderID = b.GetId()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("build Yak file %s: %v", b.GetEditor().GetUrl(), recovered)
		}
		if err != nil {
			record.state, record.err = fileFailed, err
			record.exports = nil
		} else {
			record.state = fileCompleted
		}
	}()
	before := make(map[string]int)
	for _, variable := range b.CurrentBlock.ScopeTable.GetAllVariables() {
		before[variable.GetName()] = variable.GetGlobalIndex()
	}
	astBuilder := &astbuilder{FunctionBuilder: b, frontend: s, file: record}
	astBuilder.build(ast)
	if astBuilder.err != nil {
		return astBuilder.err
	}
	if b.IsStop() {
		return fmt.Errorf("Yak file build canceled: %s", b.GetEditor().GetUrl())
	}
	// Save stable values produced by this file, not the shared mutable root
	// scope (which later files may overwrite). Include exports are retained
	// even when the same binding was already visible before this file.
	exports := record.exports
	record.definitions = make(map[string]int64)
	for _, variable := range b.CurrentBlock.ScopeTable.GetAllVariables() {
		name := variable.GetName()
		value := variable.GetValue()
		if value == nil {
			continue
		}
		previous, existed := before[name]
		if !existed || previous != variable.GetGlobalIndex() {
			record.definitions[name] = value.GetId()
			exports[name] = value.GetId()
		}
	}
	record.exports = exports
	return nil
}

func importFileValues(b *ssa.FunctionBuilder, exports map[string]int64) error {
	names := make([]string, 0, len(exports))
	for name := range exports {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value, ok := b.GetValueById(exports[name])
		if !ok || value == nil {
			return fmt.Errorf("included Yak binding %s is unavailable", name)
		}
		if current := ssa.ReadVariableFromScope(b.CurrentBlock.ScopeTable, name); current != nil && current.GetValue() == value {
			continue
		}
		b.AssignVariable(b.CreateVariable(name), value)
	}
	return nil
}

func (s *SSABuilder) includeFile(caller *astbuilder, filename string) error {
	p := caller.GetProgram()
	includePaths := p.Loader.GetIncludeFiles()
	p.Loader.AddIncludePath(caller.GetEditor().GetFolderPath())
	defer p.Loader.SetIncludePaths(includePaths)
	resolved, loaded, err := p.Loader.LoadFilePackage(filename, false)
	if err != nil {
		return err
	}
	app := p.GetApplication()
	editor := app.CreateEditor([]byte(loaded.GetSourceCode()), path.Clean(resolved))
	record, err := s.fileRecord(caller.FunctionBuilder, editor)
	if err != nil {
		return err
	}
	switch record.state {
	case fileBuilding:
		return fmt.Errorf("cyclic Yak include: %s", resolved)
	case fileFailed:
		return record.err
	case filePending:
		// Includes keep an independent control-flow entry: a top-level return
		// in the included file must not terminate the caller. Copy build
		// configuration without making the include a lexical closure.
		var ast ssa.FrontAST
		ast, err = s.ParseAST(editor.GetSourceCode(), s.GetAntlrCache())
		if err == nil {
			program := app.GetSubProgram(editor.GetIrSourceHash())
			builder := program.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
			builder.SetBuildSupport(caller.FunctionBuilder)
			builder.DefineFunc = caller.DefineFunc
			builder.SetContext(caller.GetContext())
			err = app.Build(ast, editor, builder)
			if err == nil {
				program.LazyBuild()
				builder.Finish()
			}
		}
		if err != nil {
			record.state, record.err = fileFailed, err
			record.exports = nil
			return err
		}
	}
	if err := caller.AddIncludedValues(record.exports); err != nil {
		return err
	}
	// Function-local includes must not become exports of the enclosing file.
	if caller.GetId() == caller.file.builderID {
		for name, value := range record.exports {
			caller.file.exports[name] = value
		}
	}
	return nil
}
