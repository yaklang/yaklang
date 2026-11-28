package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/reify"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// TestBinarySSAIsPersistedAndQueriedBySyntaxFlow covers the whole route:
// C source -> ELF -> reify lift -> ssadb -> ssaapi.FromDatabase ->
// syntaxflow. The query must find both an imported libc call (printf) and a
// recovered local call (helper) in the persisted program.
func TestBinarySSAIsPersistedAndQueriedBySyntaxFlow(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}

	directory := t.TempDir()
	source := filepath.Join(directory, "demo.c")
	if err := os.WriteFile(source, []byte(`
#include <stdio.h>

int helper(int x) { return x + 1; }

int compute(int x) {
    int y = helper(x);
    printf("compute: %d\n", y);
    return y;
}

int main(void) {
    int value = compute(41);
    printf("result: %d\n", value);
    return 0;
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "demo")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	compile := exec.Command("gcc", "-O0", "-fno-pie", "-no-pie", "-fno-stack-protector", "-o", binary, source)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Skipf("cannot build demo: %v\n%s", err, output)
	}

	// File-mode audit: lift into memory and query the program directly.
	memoryLift, err := reify.Lift(context.Background(), binary,
		reify.WithMode(reify.ModeSSAIR),
		reify.WithSection(".text"),
	)
	require.NoError(t, err)
	require.NotNil(t, memoryLift.Program)
	memoryLift.Program.FlushInstruction()
	memoryProgram := ssaapi.NewProgram(memoryLift.Program, nil)
	memoryResult, err := memoryProgram.SyntaxFlowWithError("helper(* as $args) as $call")
	require.NoError(t, err)
	memoryCalls, ok := memoryResult.GetSFResult().SymbolTable.Get("call")
	require.True(t, ok, "in-memory audit did not bind call")
	require.GreaterOrEqual(t, len(memoryCalls), 1)
	_ = memoryLift.Close()

	programName := "reify-e2e-" + uuid.NewString()
	db, cleanup := isolatedSSADB(t)
	defer cleanup()
	defer ssadb.DeleteProgram(db, programName)
	result, err := reify.Lift(context.Background(), binary,
		reify.WithMode(reify.ModeSSAIR),
		reify.WithSection(".text"),
		reify.WithProgramName(programName),
		reify.WithProgramCache(ssa.ProgramCacheDBWrite),
	)
	require.NoError(t, err)
	defer func() { _ = result.Close() }()
	require.NotNil(t, result.Program)
	require.Equal(t, programName, result.ProgramName)
	require.NoError(t, result.Program.SaveToDatabase())

	program, err := ssaapi.FromDatabase(programName)
	require.NoError(t, err)
	require.Equal(t, ssa.ProgramCacheDBRead, program.Program.DatabaseKind,
		"the query program must be loaded from ssadb, not from the lift cache")

	assertQuery := func(rule, symbol string, wantAtLeast int) {
		t.Helper()
		matched, err := program.SyntaxFlowWithError(rule)
		require.NoError(t, err, "rule: %s", rule)
		frame := matched.GetSFResult()
		require.NotNil(t, frame)
		values, ok := frame.SymbolTable.Get(strings.TrimPrefix(symbol, "$"))
		require.True(t, ok, "rule %q did not bind %s\n%s", rule, symbol, matched.String())
		require.GreaterOrEqual(t, len(values), wantAtLeast, "rule: %s", rule)
	}

	// Imported libc function: the loader names the PLT/GOT target, so the call
	// keeps the symbol name in ssadb.
	assertQuery("printf(* as $args) as $call", "$call", 2)
	assertQuery("printf(* as $args) as $call", "$args", 4)
	// Recovered local function: the call target is an internal function.
	assertQuery("helper(* as $args) as $call", "$call", 1)

	// The persisted call must expose its target through the normal value API,
	// not only through the syntaxflow string matcher.
	matched, err := program.SyntaxFlowWithError("printf(* as $args) as $call")
	require.NoError(t, err)
	calls, ok := matched.GetSFResult().SymbolTable.Get(strings.TrimPrefix("$call", "$"))
	require.True(t, ok)
	require.NotEmpty(t, calls)
	first, ok := calls[0].(*ssaapi.Value)
	require.True(t, ok)
	callee := first.GetCallee()
	require.NotNil(t, callee)
	require.Equal(t, "printf", callee.GetName())
}

// isolatedSSADB points the ssadb globals at one temporary SQLite database and
// restores the previous handles afterwards.
func isolatedSSADB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	database, err := consts.CreateSSAProjectDatabase(consts.SQLiteExtend, filepath.Join(t.TempDir(), "ssa.db"))
	require.NoError(t, err)
	oldSSA := ssadb.GetDB()
	oldProject := consts.GetGormSSAProjectDataBase()
	ssadb.SetDB(database)
	consts.SetGormSSAProjectDatabase(database)
	return database, func() {
		ssadb.SetDB(oldSSA)
		consts.SetGormSSAProjectDatabase(oldProject)
		_ = database.Close()
	}
}
