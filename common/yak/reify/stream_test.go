package reify_test

import (
	"bytes"
	"context"
	"io"
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

func buildMultiFunctionTestBinary(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "multi.c")
	err := os.WriteFile(src, []byte(`
#include <stdio.h>

int leaf_add(int a, int b) { return a + b; }
int leaf_mul(int a, int b) { return a * b; }
const char *get_tag(void) { return "reify-stream-tag"; }

int combine(int x) {
    int s = leaf_add(x, 3);
    int m = leaf_mul(s, 2);
    printf("%s:%d\n", get_tag(), m);
    return m;
}

int main(void) {
    int v = combine(5);
    printf("done:%d\n", v);
    return 0;
}
`), 0o644)
	require.NoError(t, err)
	bin := filepath.Join(dir, "multi")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("gcc", "-O0", "-fno-pie", "-no-pie", "-fno-stack-protector", "-o", bin, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot compile test binary: %v\n%s", err, out)
	}
	return bin
}

func isolatedStreamDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	database, err := consts.CreateSSAProjectDatabase(consts.SQLiteExtend, filepath.Join(t.TempDir(), "stream_ssa.db"))
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

func TestLiftStreamMultiBatchAllModes(t *testing.T) {
	bin := buildMultiFunctionTestBinary(t)
	ctx := context.Background()
	textSection := ".text"
	if runtime.GOOS == "darwin" {
		textSection = "__TEXT,__text"
	}

	cases := []struct {
		mode     reify.Mode
		contains []string
	}{
		{reify.ModeASM, []string{"0x"}},
		{reify.ModeLowIR, []string{"target:", "function ", "leaf_add"}},
		{reify.ModeSSAIR, []string{"extern ", "combine", "leaf_add"}},
		{reify.ModeSource, []string{"leaf_add(", "combine(", "reify-stream-tag"}},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			var buf bytes.Buffer
			headerSeen := false
			res, err := reify.LiftStream(ctx, bin, &buf,
				reify.WithMode(tc.mode),
				reify.WithSection(textSection),
				reify.WithBatchSize(2),
				reify.WithWorkers(4),
				reify.WithBeforeStream(func(w io.Writer, h reify.StreamHeader) error {
					headerSeen = true
					require.Equal(t, tc.mode, h.Mode)
					_, err := io.WriteString(w, "HEADER\n")
					return err
				}),
			)
			require.NoError(t, err)
			require.True(t, headerSeen)
			text := buf.String()
			require.True(t, strings.HasPrefix(text, "HEADER\n"), "missing header prefix: %q", text)
			for _, want := range tc.contains {
				require.Contains(t, text, want)
			}
			if tc.mode == reify.ModeSource && runtime.GOOS != "darwin" {
				require.Contains(t, text, "printf(")
			}
			if tc.mode == reify.ModeASM {
				require.Positive(t, res.Instructions)
			} else {
				require.Positive(t, res.Functions)
			}
		})
	}
}

func TestSaveMultiBatchPersistsAndQueries(t *testing.T) {
	bin := buildMultiFunctionTestBinary(t)
	db, cleanup := isolatedStreamDB(t)
	defer cleanup()
	textSection := ".text"
	if runtime.GOOS == "darwin" {
		textSection = "__TEXT,__text"
	}

	programName := "reify-stream-save-" + uuid.NewString()
	defer ssadb.DeleteProgram(db, programName)

	saveRes, err := reify.Save(context.Background(), bin, programName,
		reify.WithSection(textSection),
		reify.WithBatchSize(2),
		reify.WithWorkers(2),
	)
	require.NoError(t, err)
	require.Equal(t, programName, saveRes.ProgramName)
	require.Positive(t, saveRes.Functions)
	require.Positive(t, saveRes.Blocks)
	require.Positive(t, saveRes.Values)

	program, err := ssaapi.FromDatabase(programName)
	require.NoError(t, err)
	require.Equal(t, ssa.ProgramCacheDBRead, program.Program.DatabaseKind)

	assertCalls := func(rule string, minimum int) {
		t.Helper()
		matched, err := program.SyntaxFlowWithError(rule)
		require.NoError(t, err, "rule=%s", rule)
		calls, ok := matched.GetSFResult().SymbolTable.Get("call")
		require.True(t, ok, "rule=%s did not bind $call", rule)
		require.GreaterOrEqual(t, len(calls), minimum, "rule=%s", rule)
	}
	assertCalls("leaf_add(* as $args) as $call", 1)
	assertCalls("combine(* as $args) as $call", 1)
	if runtime.GOOS != "darwin" {
		assertCalls("printf(* as $args) as $call", 2)
	}
}
