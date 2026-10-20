package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func importedMethodFixture() *filesys.VirtualFS {
	vf := filesys.NewVirtualFs()
	vf.AddFile("go.mod", "module fixture\n\ngo 1.20\n")
	vf.AddFile("db/types.go", `package db

type DB struct{}
`)
	vf.AddFile("db/methods.go", `package db

func (d DB) Value(s string) string { return s }
func (d *DB) SetBool(s string, v bool) {}
func New() *DB { return &DB{} }
`)
	vf.AddFile("client/calls.go", `package client
import "fixture/db"

func CallValue() { d := db.DB{}; d.Value("value") }
func CallPointer() { d := &db.DB{}; d.SetBool("pointer", true) }
func CallReturned() { d := db.New(); d.SetBool("returned", true) }
`)
	return vf
}

func callsInGoFunction(t *testing.T, fn *ssa.Function) []*ssa.Call {
	t.Helper()
	var calls []*ssa.Call
	for _, blockID := range fn.Blocks {
		block, ok := fn.GetBasicBlockByID(blockID)
		require.True(t, ok)
		for _, id := range block.Insts {
			v, ok := fn.GetValueById(id)
			if !ok {
				continue
			}
			if call, ok := ssa.ToCall(v); ok {
				calls = append(calls, call)
			}
		}
	}
	return calls
}

func TestGoImportedStaticMethodCallees(t *testing.T) {
	// These are explicit diagnostic/test settings; product defaults are unchanged.
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprintf("workers-%d", workers), func(t *testing.T) {
			programs, err := ssaapi.ParseProjectWithFS(importedMethodFixture(), ssaapi.WithLanguage(ssaconfig.GO), ssaapi.WithMemory(), ssaconfig.WithCompileConcurrency(workers))
			require.NoError(t, err)
			require.Len(t, programs, 1)
			app := programs[0].Program
			db, _ := app.GetLibrary("db")
			client, _ := app.GetLibrary("client")
			require.NotNil(t, db)
			require.NotNil(t, client)
			for _, tc := range []struct{ caller, target string }{{"CallValue", "DB$Value"}, {"CallPointer", "DB$SetBool"}, {"CallReturned", "DB$SetBool"}} {
				t.Run(tc.caller, func(t *testing.T) {
					fn, ok := client.Funcs.Get(tc.caller)
					require.True(t, ok)
					target, ok := db.Funcs.Get(tc.target)
					require.True(t, ok)
					calls := callsInGoFunction(t, fn)
					require.NotEmpty(t, calls)
					call := calls[len(calls)-1]
					callee, ok := call.GetValueById(call.Method)
					require.True(t, ok)
					// The call keeps its member-call value, which carries the receiver and
					// arguments. What must be stable is the method it resolves to: the
					// function created when the method was declared, not a fresh
					// undefined with no relation to the declaration.
					receiver := ssa.GetLatestObject(callee)
					require.NotNil(t, receiver, "imported method call must be a member call on its receiver, got %s (%T)", callee.String(), callee)
					name := tc.target[strings.Index(tc.target, "$")+1:]
					resolved := ssa.GetMethod(receiver.GetType(), name)
					require.NotNil(t, resolved, "receiver type must resolve %s to its declaration", name)
					require.Equal(t, target.GetId(), resolved.GetId(), "imported method call must resolve to the declared method function")
				})
			}
		})
	}
}
