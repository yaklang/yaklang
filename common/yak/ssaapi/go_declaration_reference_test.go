package ssaapi

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

// Observe the actual end-of-file prehandler boundary, before library bodies.
// The test supplies the parser's completed-file queue in an explicit order,
// rather than depending on sleeps or a lucky worker interleaving.
type goDeclarationProbe struct {
	ssa.Builder
	after func(*ssa.FunctionBuilder, *memedit.MemEditor)
}

func (*goDeclarationProbe) SupportsConcurrentUnitBuild() bool      { return true }
func (*goDeclarationProbe) SupportsConcurrentUnitPreHandler() bool { return false }
func (*goDeclarationProbe) DeferMethodBuildOnLookup() bool         { return true }

func (p *goDeclarationProbe) PreHandlerProject(fs fi.FileSystem, ast ssa.FrontAST, b *ssa.FunctionBuilder, editor *memedit.MemEditor) error {
	if err := p.Builder.PreHandlerProject(fs, ast, b, editor); err != nil {
		return err
	}
	p.after(b, editor)
	return nil
}

func prehandleGoFilesInOrder(t *testing.T, cfg *Config, vf *filesys.VirtualFS, paths []string) *ssa.Program {
	t.Helper()
	prog, builder, err := cfg.init(vf, len(paths))
	require.NoError(t, err)
	prog.SetConcurrentUnitBuild(true)
	prog.SetDeferMethodBuildOnLookup(true)
	prog.SetPreHandler(true)
	plan := buildCompileUnitPlan(cfg.LanguageBuilder, ssaconfig.GO, vf, paths)
	prog.CompileUnits = flattenCompileUnits(plan)
	units := make(map[string]string)
	for _, unit := range prog.CompileUnits {
		for _, file := range unit.Files {
			units[file] = unit.Key
		}
	}
	files := make(chan *ssareducer.FileContent, len(paths))
	set := make(map[string]struct{})
	for _, path := range paths {
		source, err := vf.ReadFile(path)
		require.NoError(t, err)
		var ast ssa.FrontAST
		if cfg.LanguageBuilder.FilterParseAST(path) {
			ast, err = cfg.LanguageBuilder.ParseAST(string(source), cfg.LanguageBuilder.GetAntlrCache())
			require.NoError(t, err)
		}
		files <- &ssareducer.FileContent{Path: path, Content: source, AST: ast, Status: ssareducer.FileStatusSuccess}
		set[path] = struct{}{}
	}
	close(files)
	env := &prehandleBatchEnv{c: cfg, prog: prog, builder: builder, filesystem: vf, fileUnits: units,
		preHandlerFileSet: set, handlerFileSet: set, preHandlerBuildsFiles: true,
		preHandlerProcess: func() {}, concurrency: 1}
	require.False(t, env.consume(files))
	require.NoError(t, env.astErr)
	t.Cleanup(cfg.LanguageBuilder.Clearup)
	return prog
}

func TestGoPreHandlerMethodDeclarationIdentity(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
	vf.AddFile("db/types.go", "package db\ntype DB struct{}\n")
	vf.AddFile("db/methods.go", `package db
func (d *DB) SetBool(key string, value bool) {}
func New() *DB { return &DB{} }
`)
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.GO), WithMemory(), ssaconfig.WithCompileConcurrency(4))
	require.NoError(t, err)
	type snapshot struct {
		method     *ssa.Function
		classified bool
		exports    []string
		ordinary   ssa.Value
	}
	observed := make(chan snapshot, 1)
	cfg.LanguageBuilder = &goDeclarationProbe{Builder: cfg.LanguageBuilder, after: func(b *ssa.FunctionBuilder, editor *memedit.MemEditor) {
		if !strings.HasSuffix(editor.GetFilePath(), "db/methods.go") {
			return
		}
		lib, _ := b.GetProgram().GetApplication().GetLibrary("db")
		if lib == nil {
			observed <- snapshot{}
			return
		}
		method, _ := lib.Funcs.Get("DB$SetBool")
		s := snapshot{method: method, ordinary: lib.GetExportValue("New")}
		if method != nil {
			s.classified = method.IsMethod()
		}
		lib.ExportValue.ForEach(func(name string, _ ssa.Value) bool { s.exports = append(s.exports, name); return true })
		sort.Strings(s.exports)
		observed <- s
	}}
	// A completed method AST is delivered before the receiver type's file.
	// That ordering is supported by the real unordered parser pool.
	app := prehandleGoFilesInOrder(t, cfg, vf, []string{"go.mod", "db/methods.go", "db/types.go"})
	close(observed)
	s, ok := <-observed
	require.True(t, ok, "method file must reach the prehandler observation boundary")
	require.NotNil(t, s.method, "first prehandler must publish a stable method function identity")
	require.True(t, s.classified, "receiver methods must be classified before the receiver file or lazy body is built")
	require.NotContains(t, s.exports, "DB$SetBool", "receiver methods are not package-level exports")
	require.NotNil(t, s.ordinary, "ordinary exported functions must be available before bodies build")
	keys := make([]string, 0, len(app.CompileUnits))
	for _, unit := range app.CompileUnits {
		keys = append(keys, unit.Key)
	}
	require.True(t, app.RunDeferredBuildsForUnitsOfKind(keys, ssa.DeferredBuildKindBinding, nil, nil))
	require.True(t, app.RunDeferredBuildsForUnitsOfKind(keys, ssa.DeferredBuildKindHierarchy, nil, nil))
	app.SetPreHandler(false)
	s.method.Build()
	lib, _ := app.GetLibrary("db")
	final, _ := lib.Funcs.Get("DB$SetBool")
	require.Same(t, s.method, final, "body build must fill the declaration's function identity in place")
}

func TestGoPreHandlerEarlyImportBindsCompleteExportSet(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
	vf.AddFile("client/use.go", "package client\nimport \"fixture/db\"\nfunc Use() {}\n")
	vf.AddFile("db/types.go", "package db\ntype DB struct{}\n")
	vf.AddFile("db/methods.go", `package db
func (d *DB) SetBool(key string, value bool) {}
func New() *DB { return &DB{} }
func hidden() {}
`)
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.GO), WithMemory())
	require.NoError(t, err)
	app := prehandleGoFilesInOrder(t, cfg, vf, []string{"go.mod", "client/use.go", "db/methods.go", "db/types.go"})
	client, _ := app.GetLibrary("client")
	require.NotNil(t, client)
	var namespace ssa.Value
	client.Funcs.ForEach(func(_ string, fn *ssa.Function) bool {
		if fn.GetName() == string(ssa.MainFunctionName) {
			b := fn.GetOrCreateBuilder()
			if v := ssa.ReadVariableFromScopeAndParent(b.CurrentBlock.ScopeTable, "db"); v != nil {
				namespace = v.Value
			}
		}
		return true
	})
	require.NotNil(t, namespace, "an early import must have a stable namespace identity before its dependency's declarations arrive")
	namespaceID := namespace.GetId()
	keys := make([]string, 0, len(app.CompileUnits))
	for _, unit := range app.CompileUnits {
		keys = append(keys, unit.Key)
	}
	require.True(t, app.RunDeferredBuildsForUnitsOfKind(keys, ssa.DeferredBuildKindBinding, nil, nil))
	db, _ := app.GetLibrary("db")
	require.NotNil(t, db)
	target, ok := db.Funcs.Get("New")
	require.True(t, ok)
	member, ok := ssa.GetLatestMemberByKeyString(namespace, "New")
	require.True(t, ok, "binding must complete early import namespaces after every declaration has been discovered, before any body/query")
	require.Equal(t, target.GetId(), member.GetId(), "early import must bind the declaration's real function identity")
	require.Equal(t, namespaceID, namespace.GetId(), "binding must not replace the early namespace identity")
	var forward, reverse []ssa.Value
	for _, pair := range ssa.GetMemberPairs(namespace) {
		if pair.KeyString() == "New" {
			forward = append(forward, pair.Key)
			require.Equal(t, target.GetId(), pair.Member.GetId())
		}
		require.NotEqual(t, "hidden", pair.KeyString(), "private package functions must not be visible through the caller's imported namespace")
		require.NotEqual(t, "DB$SetBool", pair.KeyString(), "receiver methods belong to their receiver, not the package namespace")
	}
	for _, pair := range ssa.GetObjectKeyPairs(member) {
		if pair.Object.GetId() == namespaceID && pair.KeyString() == "New" {
			reverse = append(reverse, pair.Key)
		}
	}
	require.Len(t, forward, 1, "one imported symbol must have exactly one forward member key")
	require.Len(t, reverse, 1, "one imported symbol must have exactly one reverse owner key")
	require.Equal(t, forward[0].GetId(), reverse[0].GetId(), "both directions describe the same member relation and must use its same key identity")
	require.True(t, app.RunDeferredBuildsForUnitsOfKind(keys, ssa.DeferredBuildKindBinding, nil, nil))
	require.Len(t, ssa.GetMemberPairs(namespace), 1, "re-running completed binding must not duplicate namespace members")
}

func TestGoPreHandlerImportRelationUsesSingleKey(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
	vf.AddFile("db/api.go", "package db\nfunc Exported() {}\n")
	vf.AddFile("client/use.go", "package client\nimport \"fixture/db\"\nfunc Use() {}\n")
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.GO), WithMemory())
	require.NoError(t, err)
	app := prehandleGoFilesInOrder(t, cfg, vf, []string{"go.mod", "db/api.go", "client/use.go"})
	keys := make([]string, 0, len(app.CompileUnits))
	for _, unit := range app.CompileUnits {
		keys = append(keys, unit.Key)
	}
	require.True(t, app.RunDeferredBuildsForUnitsOfKind(keys, ssa.DeferredBuildKindBinding, nil, nil))
	client, _ := app.GetLibrary("client")
	require.NotNil(t, client)
	var namespace ssa.Value
	client.Funcs.ForEach(func(_ string, fn *ssa.Function) bool {
		if fn.GetName() == string(ssa.MainFunctionName) {
			b := fn.GetOrCreateBuilder()
			if v := ssa.ReadVariableFromScopeAndParent(b.CurrentBlock.ScopeTable, "db"); v != nil {
				namespace = v.Value
			}
		}
		return true
	})
	require.NotNil(t, namespace)
	member, ok := ssa.GetLatestMemberByKeyString(namespace, "Exported")
	require.True(t, ok)
	var forward, reverse []ssa.Value
	for _, pair := range ssa.GetMemberPairs(namespace) {
		if pair.KeyString() == "Exported" && pair.Member.GetId() == member.GetId() {
			forward = append(forward, pair.Key)
		}
	}
	for _, pair := range ssa.GetObjectKeyPairs(member) {
		if pair.Object.GetId() == namespace.GetId() && pair.KeyString() == "Exported" {
			reverse = append(reverse, pair.Key)
		}
	}
	require.Len(t, forward, 1)
	require.Len(t, reverse, 1)
	require.Equal(t, forward[0].GetId(), reverse[0].GetId(), "same namespace, member function, and symbol relation must retain one key identity in both directions")
}
