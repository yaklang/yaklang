package configtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Include getters which migrate or persist their result as well as setters.
// This test audits test entry points and their local setup helpers, so a new
// configuration-writing test cannot silently omit its precondition.
var persistedConfigCalls = strings.Fields(`
GetAIGlobalConfig GetGlobalNetworkConfig LoadGlobalNetworkConfig GetMCPToolList
SetAIGlobalConfig SetGlobalNetworkConfig ResetGlobalNetworkConfig ConfigureNetWork
SetGlobalHotPatchConfig ResetGlobalHotPatchConfig PutGlobalHotPatchConfigToKV
SaveFuzzerConfig DeleteFuzzerConfig SetMITMFilter ResetMITMFilter
SetMITMHijackFilter ResetMITMHijackFilter ImportMITMReplacerRules
DeduplicateMITMReplacerRules UpdateApiKey SetProcessEnvKey SetProjectKey
SetGlobalProxyRulesConfig UpdateMCPServerToolConfig UpdateIMControlConfig
UpsertAIProvider DeleteAIProvider GetOrCreateMCPClientToolConfig
GetOrCreateMCPClientToolConfigWithDefaultEnable SetMCPClientToolEnabled
UpsertMCPClientToolConfigDescription ReconcileMCPBuiltinToolTierDefaults
MigrateMCPClientBridgeToolConfigsServerName DeleteMCPClientToolConfigsByServerAndNames
EnsureConfigLoaded SaveTieredAIConfigToDB AddMCPServer UpdateMCPServer DeleteMCPServer
CreateMCPServer CreateOrUpdateMCPServer SyncAndCacheMCPServerTools
SetPluginEnv CreateOrUpdatePluginEnv CreateHotPatchTemplate UpdateHotPatchTemplate
DeleteHotPatchTemplate AddToNavigation AddOneNavigation DeleteAllNavigation
AddToMenu AddMenus ImportMenuItem DeleteAllMenuItem DeleteAllMenu
`)

type configTestFunction struct {
	file, pkg string
	body      *ast.BlockStmt
}

func TestPersistedConfigurationTestsHavePreconditions(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	functions := map[string]configTestFunction{}
	mutators := map[string]bool{}
	for _, name := range persistedConfigCalls {
		mutators[name] = true
	}
	err = filepath.WalkDir(filepath.Join(root, "common"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") || filepath.Dir(path) == filepath.Join(root, "common", "utils", "configtest") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		candidate := strings.Contains(string(raw), "RequirePristine") || strings.Contains(string(raw), "SetKey(") || strings.Contains(string(raw), "yakit.Set(")
		for _, name := range persistedConfigCalls {
			candidate = candidate || strings.Contains(string(raw), name)
		}
		if !candidate {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, raw, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil && fn.Recv == nil {
				functions[filepath.Dir(path)+"/"+file.Name.Name+"/"+fn.Name.Name] = configTestFunction{path, file.Name.Name, fn.Body}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	type result struct{ writes, guarded, unsafe bool }
	var inspect func(string, map[string]bool) result
	inspect = func(name string, visiting map[string]bool) (state result) {
		fn, exists := functions[name]
		if !exists || visiting[name] {
			return
		}
		visiting[name] = true
		defer delete(visiting, name)
		absorb := func(child result) {
			state.writes = state.writes || child.writes
			state.unsafe = state.unsafe || (child.unsafe && !state.guarded)
			state.guarded = state.guarded || child.guarded
		}
		ast.Inspect(fn.body, func(node ast.Node) bool {
			if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING &&
				(strings.Contains(literal.Value, "db.SetKey(") || strings.Contains(literal.Value, "db.Set(")) {
				state.writes = true
				state.unsafe = state.unsafe || !state.guarded
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch callee := call.Fun.(type) {
			case *ast.SelectorExpr:
				method := callee.Sel.Name
				if method == "GuardConfigurationForTest" || method == "RequirePristine" {
					state.guarded = true
					return true
				}
				receiver, _ := callee.X.(*ast.Ident)
				writes := mutators[method] || (receiver != nil && receiver.Name == "yakit" && (method == "SetKey" || method == "Set"))
				// CliApp.SetPluginEnv only builds parser options. It shares a
				// name with the gRPC persistence API, but has no database access.
				if fn.pkg == "cli" && strings.HasSuffix(filepath.Dir(fn.file), filepath.Join("common", "utils", "cli")) &&
					method == "SetPluginEnv" && receiver != nil && receiver.Name == "app" {
					writes = false
				}
				if writes {
					state.writes = true
					state.unsafe = state.unsafe || !state.guarded
				}
			case *ast.Ident:
				writes := mutators[callee.Name] || (fn.pkg == "yakit" && callee.Name == "SetKey")
				if writes {
					state.writes = true
					state.unsafe = state.unsafe || !state.guarded
				}
				absorb(inspect(filepath.Dir(fn.file)+"/"+fn.pkg+"/"+callee.Name, visiting))
			}
			for _, arg := range call.Args {
				if callback, ok := arg.(*ast.Ident); ok {
					absorb(inspect(filepath.Dir(fn.file)+"/"+fn.pkg+"/"+callback.Name, visiting))
				}
			}
			return true
		})
		return
	}
	var checked int
	for name, fn := range functions {
		if !strings.HasPrefix(filepath.Base(name), "Test") {
			continue
		}
		state := inspect(name, map[string]bool{})
		if state.writes {
			checked++
		}
		if state.writes && state.unsafe {
			relative, _ := filepath.Rel(root, fn.file)
			t.Errorf("%s: %s accesses persisted configuration before an ownership/empty-configuration guard", relative, filepath.Base(name))
		}
	}
	t.Logf("checked %d persisted-configuration test entry points and their setup helpers", checked)
}
