package ssaapi

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Run the real Go prehandler with explicitly supplied file/body orders. No
// retry, timing or map iteration is used to choose the interleaving under test.
func buildGoBodiesInOrder(t *testing.T, vf *filesys.VirtualFS, paths, order []string) *ssa.Program {
	t.Helper()
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.GO), WithMemory(), ssaconfig.WithCompileConcurrency(1))
	require.NoError(t, err)
	app := prehandleGoFilesInOrder(t, cfg, vf, paths)
	keys := make([]string, 0, len(app.CompileUnits))
	for _, unit := range app.CompileUnits {
		keys = append(keys, unit.Key)
	}
	require.True(t, app.RunDeferredBuildsForUnitsOfKind(keys, ssa.DeferredBuildKindBinding, nil, nil))
	require.True(t, app.RunDeferredBuildsForUnitsOfKind(keys, ssa.DeferredBuildKindHierarchy, nil, nil))
	app.SetPreHandler(false)
	for _, ref := range order {
		library, name, ok := strings.Cut(ref, ":")
		require.True(t, ok)
		lib, _ := app.GetLibrary(library)
		require.NotNil(t, lib)
		fn, ok := lib.Funcs.Get(name)
		require.True(t, ok, ref)
		fn.Build()
	}
	return app
}

type goBodyOrderCall struct {
	calleeOpcode   ssa.Opcode
	undefinedKind  ssa.UndefinedKind
	calleeType     string
	returnType     string
	receiverType   string
	resolvedMethod string
	forwardMember  bool
	reverseMember  bool
}

var goBodyOrderGeneratedID = regexp.MustCompile(`#\d+`)

func snapshotGoBodyOrderCall(t *testing.T, app *ssa.Program, callerLib, callerName, methodLib, methodName string) goBodyOrderCall {
	t.Helper()
	lib, _ := app.GetLibrary(callerLib)
	require.NotNil(t, lib)
	fn, ok := lib.Funcs.Get(callerName)
	require.True(t, ok)
	var last *ssa.Call
	for _, blockID := range fn.Blocks {
		block, ok := fn.GetBasicBlockByID(blockID)
		require.True(t, ok)
		for _, id := range block.Insts {
			value, ok := fn.GetValueById(id)
			if !ok {
				continue
			}
			if call, ok := ssa.ToCall(value); ok {
				last = call
			}
		}
	}
	require.NotNil(t, last)
	callee, ok := last.GetValueById(last.Method)
	require.True(t, ok)
	normalize := func(s string) string { return goBodyOrderGeneratedID.ReplaceAllString(s, "#<value>") }
	s := goBodyOrderCall{calleeOpcode: callee.GetOpcode(), calleeType: normalize(callee.GetType().String()), returnType: normalize(last.GetType().String())}
	if un, ok := ssa.ToUndefined(callee); ok {
		s.undefinedKind = un.Kind
	}
	receiver := ssa.GetLatestObject(callee)
	require.NotNil(t, receiver)
	s.receiverType = normalize(receiver.GetType().String())
	key := strings.SplitN(methodName, "$", 2)[1]
	if method := ssa.GetMethod(receiver.GetType(), key, true); method != nil {
		declLib, _ := app.GetLibrary(methodLib)
		require.NotNil(t, declLib)
		decl, ok := declLib.Funcs.Get(methodName)
		require.True(t, ok)
		if method.GetId() == decl.GetId() {
			s.resolvedMethod = methodName
		}
	}
	for _, member := range receiver.GetMembersByKeyString(key) {
		if member.GetId() == callee.GetId() {
			s.forwardMember = true
		}
	}
	for _, pair := range ssa.GetObjectKeyPairs(callee) {
		if pair.Object.GetId() == receiver.GetId() && pair.KeyString() == key {
			s.reverseMember = true
		}
	}
	return s
}

func TestGoSerialBodyOrderPreservesStaticCallee(t *testing.T) {
	for _, factory := range []struct{ name, result, body, binding string }{
		{"pointer", "*Service", "return &Service{}", "s := New()"},
		{"value", "Service", "return Service{}", "s := New()"},
		{"multi-return", "(*Service, error)", "return &Service{}, nil", "s, _ := New()"},
		{"multi-return-value", "(Service, error)", "return Service{}, nil", "s, _ := New()"},
		{"named-multi-return", "(s *Service, err error)", "return &Service{}, nil", "s, _ := New()"},
	} {
		t.Run(factory.name, func(t *testing.T) {
			var got []goBodyOrderCall
			for _, order := range [][]string{{"service:Service$Value", "service:New", "service:Use"}, {"service:Use", "service:New", "service:Service$Value"}} {
				vf := filesys.NewVirtualFs()
				vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
				vf.AddFile("service/service.go", fmt.Sprintf(`package service

type Service struct{}
func (s *Service) Value() string { return "value" }
func New() %s { %s }
func Use() { %s; s.Value() }
`, factory.result, factory.body, factory.binding))
				app := buildGoBodiesInOrder(t, vf, []string{"go.mod", "service/service.go"}, order)
				got = append(got, snapshotGoBodyOrderCall(t, app, "service", "Use", "service", "Service$Value"))
			}
			require.Equal(t, got[0], got[1], "static callee validity, return type and both member directions must not depend on body order")
			require.NotEqual(t, ssa.UndefinedMemberInValid, got[0].undefinedKind)
			require.Equal(t, "Service$Value", got[0].resolvedMethod)
			require.Equal(t, "string", got[0].returnType)
			require.True(t, got[0].forwardMember)
			require.True(t, got[0].reverseMember)
		})
	}
}

func TestGoDeclarationSignaturesReadyBeforeBodies(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
	vf.AddFile("service/factory.go", `package service
func New(n int) (*Service, error) { return &Service{}, nil }
func (s *Service) Value(x string) string { return x }
`)
	vf.AddFile("service/types.go", "package service\ntype Service struct{}\n")
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.GO), WithMemory(), ssaconfig.WithCompileConcurrency(1))
	require.NoError(t, err)
	app := prehandleGoFilesInOrder(t, cfg, vf, []string{"go.mod", "service/factory.go", "service/types.go"})
	var keys []string
	for _, unit := range app.CompileUnits {
		keys = append(keys, unit.Key)
	}
	require.True(t, app.RunDeferredBuildsForUnitsOfKind(keys, ssa.DeferredBuildKindBinding, nil, nil))
	lib, _ := app.GetLibrary("service")
	require.NotNil(t, lib)
	typ, ok := lib.GetExportType("Service")
	require.True(t, ok)
	factory, ok := lib.Funcs.Get("New")
	require.True(t, ok)
	require.Len(t, factory.Params, 1, "declaration creates parameters once")
	require.Empty(t, factory.Return, "signature readiness must not eagerly build a body")
	require.NotNil(t, factory.Type)
	tuple, ok := ssa.ToObjectType(factory.Type.ReturnType)
	require.True(t, ok)
	require.Equal(t, ssa.TupleTypeKind, tuple.GetTypeKind())
	first := tuple.GetField(ssa.NewConst(0))
	second := tuple.GetField(ssa.NewConst(1))
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.Equal(t, typ.String(), first.String(), "tuple position zero must not be overwritten by error")
	require.Equal(t, ssa.ErrorTypeKind, second.GetTypeKind())
	method, ok := lib.Funcs.Get("Service$Value")
	require.True(t, ok)
	require.Len(t, method.Params, 2, "receiver and explicit parameter are ready before body")
	require.Empty(t, method.Return)
	require.True(t, method.IsMethod())
	require.NotNil(t, method.Type.ObjectType)
	require.Equal(t, typ.String(), method.Type.ObjectType.String())
	app.SetPreHandler(false)
	factory.Build()
	method.Build()
	require.Len(t, factory.Params, 1, "body must not rebuild signature parameters")
	require.Len(t, method.Params, 2, "body must not duplicate the receiver")
}

func TestGoSerialBodyOrderCrossPackageEarlyImport(t *testing.T) {
	for _, methodFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("method-file-first-%v", methodFirst), func(t *testing.T) {
			var got []goBodyOrderCall
			for _, order := range [][]string{{"db:DB$Value", "db:New", "client:Use"}, {"client:Use", "db:New", "db:DB$Value"}} {
				vf := filesys.NewVirtualFs()
				vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
				vf.AddFile("db/types.go", "package db\ntype DB struct{}\n")
				vf.AddFile("db/methods.go", "package db\nfunc (d *DB) Value() string { return \"value\" }\n")
				vf.AddFile("db/factory.go", "package db\nfunc New() *DB { return &DB{} }\n")
				vf.AddFile("client/use.go", "package client\nimport \"fixture/db\"\nfunc Use() { d := db.New(); d.Value() }\n")
				paths := []string{"go.mod", "client/use.go", "db/types.go", "db/methods.go", "db/factory.go"}
				if methodFirst {
					paths = []string{"go.mod", "client/use.go", "db/methods.go", "db/factory.go", "db/types.go"}
				}
				app := buildGoBodiesInOrder(t, vf, paths, order)
				got = append(got, snapshotGoBodyOrderCall(t, app, "client", "Use", "db", "DB$Value"))
				db, _ := app.GetLibrary("db")
				require.Nil(t, db.GetExportValue("DB$Value"), "receiver method is not a package export")
			}
			require.Equal(t, got[0], got[1], "early import and cross-file receiver declarations must not expose body order")
			require.NotEqual(t, ssa.UndefinedMemberInValid, got[0].undefinedKind)
			require.Equal(t, "DB$Value", got[0].resolvedMethod)
			require.Equal(t, "string", got[0].returnType)
			require.True(t, got[0].forwardMember)
			require.True(t, got[0].reverseMember)
		})
	}
}
