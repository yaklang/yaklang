package ssaapi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// An imported declaration has no source in this compilation. The importer is
// a reference site, not the declaration of the shared synthetic type.
func TestGoIRStateVirtualTypeHasNoImporterDeclaration(t *testing.T) {
	for _, bFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("b-first-%v", bFirst), func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
			vf.AddFile("client/a.go", "package client\nimport \"net/http\"\ntype A struct { request *http.Request }\n")
			vf.AddFile("client/b.go", "package client\nimport \"net/http\"\ntype B struct { response *http.Request }\n")
			paths := []string{"go.mod", "client/a.go", "client/b.go"}
			if bFirst {
				paths[1], paths[2] = paths[2], paths[1]
			}
			app := buildGoBodiesInOrder(t, vf, paths, nil)
			lib, _ := app.GetLibrary("http")
			require.NotNil(t, lib)
			typ, ok := lib.GetExportType("Request")
			require.True(t, ok)
			bp, ok := ssa.ToBluePrintType(typ)
			require.True(t, ok)
			require.Nil(t, bp.Range, "the first import reference is not an external declaration")
			container := bp.Container()
			require.NotNil(t, container)
			require.Nil(t, container.GetRange(), "shared synthetic IR must not inherit the first importer's cursor")
			require.Same(t, lib, container.GetProgram())
			client, _ := app.GetLibrary("client")
			for _, field := range []struct{ owner, name string }{{"A", "request"}, {"B", "response"}} {
				owner, ok := client.GetExportType(field.owner)
				require.True(t, ok)
				object, ok := ssa.ToObjectType(owner)
				require.True(t, ok)
				pointer, ok := ssa.ToObjectType(object.GetField(ssa.NewConst(field.name)))
				require.True(t, ok)
				require.Same(t, bp, pointer.FieldType, "both uses must refer to one external type, not consumer-owned copies")
			}
		})
	}
}

func goIRStateFunction(t *testing.T, app *ssa.Program, library, name string) *ssa.Function {
	t.Helper()
	lib, _ := app.GetLibrary(library)
	require.NotNil(t, lib)
	fn, ok := lib.Funcs.Get(name)
	require.True(t, ok, name)
	return fn
}

func goIRStateCalls(t *testing.T, fn *ssa.Function) []*ssa.Call {
	t.Helper()
	var calls []*ssa.Call
	for _, blockID := range fn.Blocks {
		block, ok := fn.GetBasicBlockByID(blockID)
		require.True(t, ok)
		for _, id := range block.Insts {
			value, ok := fn.GetValueById(id)
			require.True(t, ok)
			if call, ok := ssa.ToCall(value); ok {
				calls = append(calls, call)
			}
		}
	}
	return calls
}

// This is the ARTEX Compactor.overflow failure without a scan, map iteration,
// sleeps or retries: deliver a completed file before/after its field's type.
// Both orders must preserve the same declared struct and its observable zeros.
func TestGoIRStateForwardStructZeroValue(t *testing.T) {
	for _, innerFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("inner-file-first-%v", innerFirst), func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
			vf.AddFile("state/outer.go", `package state
 type Compactor struct { overflow overflowState }
 func Use() {
     c := Compactor{}
     println(c.overflow.learnedWindow, c.overflow.armed)
 }
 `)
			vf.AddFile("state/inner.go", `package state
 type overflowState struct { learnedWindow int; armed bool }
 `)
			paths := []string{"go.mod", "state/outer.go", "state/inner.go"}
			if innerFirst {
				paths[1], paths[2] = paths[2], paths[1]
			}
			app := buildGoBodiesInOrder(t, vf, paths, []string{"state:Use"})
			lib, _ := app.GetLibrary("state")
			typ, ok := lib.GetExportType("Compactor")
			require.True(t, ok)
			outer, ok := ssa.ToObjectType(typ)
			require.True(t, ok)
			inner := outer.GetField(ssa.NewConst("overflow"))
			require.NotNil(t, inner)
			require.Equal(t, ssa.StructTypeKind, inner.GetTypeKind(), "a forward struct field must not stay any or fall back to number(0)")
			decl, ok := lib.GetExportType("overflowState")
			require.True(t, ok)
			require.Same(t, decl, inner, "field and declaration must share the resolved type identity")
			calls := goIRStateCalls(t, goIRStateFunction(t, app, "state", "Use"))
			require.Len(t, calls, 1)
			args := calls[0].GetValuesByIDs(calls[0].Args)
			require.Len(t, args, 2)
			require.Equal(t, "0", args[0].String())
			require.Equal(t, ssa.NumberTypeKind, args[0].GetType().GetTypeKind())
			require.Equal(t, "false", args[1].String())
			require.Equal(t, ssa.BooleanTypeKind, args[1].GetType().GetTypeKind())
		})
	}
}

// Parameter-member reads and declared callees cannot become invalid members
// just because the struct that types the parameter arrived in another file.
func TestGoIRStateForwardMemberAndCallee(t *testing.T) {
	for _, typesFirst := range []bool{false, true} {
		for _, calleeFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("types-first-%v/callee-first-%v", typesFirst, calleeFirst), func(t *testing.T) {
				vf := filesys.NewVirtualFs()
				vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
				vf.AddFile("tasks/context.go", `package tasks
 type Context struct { Tasks Store }
 func Use(tc *Context) { info := tc.Tasks.Get(); println(info.Status) }
 `)
				vf.AddFile("tasks/store.go", `package tasks
 type Info struct { Status string }
 type Store struct{}
 func (s Store) Get() Info { return Info{Status: "ready"} }
 `)
				paths := []string{"go.mod", "tasks/context.go", "tasks/store.go"}
				if typesFirst {
					paths[1], paths[2] = paths[2], paths[1]
				}
				order := []string{"tasks:Use", "tasks:Store$Get"}
				if calleeFirst {
					order[0], order[1] = order[1], order[0]
				}
				app := buildGoBodiesInOrder(t, vf, paths, order)
				fn := goIRStateFunction(t, app, "tasks", "Use")
				calls := goIRStateCalls(t, fn)
				require.Len(t, calls, 2)
				callee, ok := calls[0].GetValueById(calls[0].Method)
				require.True(t, ok)
				if unknown, ok := ssa.ToUndefined(callee); ok {
					require.Equal(t, ssa.UndefinedMemberValid, unknown.Kind, "a declared Store.Get is not an invalid member")
				}
				receiver := ssa.GetLatestObject(callee)
				require.NotNil(t, receiver)
				require.Equal(t, ssa.SSAOpcodeParameterMember, receiver.GetOpcode(), "tc.Tasks must retain its dependence on the receiver parameter")
				decl := goIRStateFunction(t, app, "tasks", "Store$Get")
				require.Same(t, decl, ssa.GetMethod(receiver.GetType(), "Get", true))
				arg := calls[1].GetValuesByIDs(calls[1].Args)
				require.Len(t, arg, 1)
				require.Equal(t, ssa.StringTypeKind, arg[0].GetType().GetTypeKind())
				if unknown, ok := ssa.ToUndefined(arg[0]); ok {
					require.Equal(t, ssa.UndefinedMemberValid, unknown.Kind, "Info.Status exists independently of when Get's body is built")
				}
				for _, pair := range ssa.GetObjectKeyPairs(callee) {
					if pair.Object.GetId() == receiver.GetId() {
						member, found := ssa.GetLatestMemberByKey(receiver, pair.Key)
						require.True(t, found)
						require.Equal(t, callee.GetId(), member.GetId(), "forward and reverse member relations must agree")
					}
				}
			})
		}
	}
}
