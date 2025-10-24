package ssatest

import (
	"context"
	"fmt"
	"io/fs"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestCompilePeephole(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				// for test 1: A->B
				target1(b.get());
				// for test 2: B->A
				b.show(1);
			}
		}
	`)

	vf.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", `
		package com.example.bpackage.sub; 
		import com.example.cpackage.C;
		class B {
			public  int get() {
				return 	 1;
			}
			public void show(int a) {
				var c = new C();
				target2(a);
			}
		}
	`)

	vf.AddFile("example/src/main/java/com/example/cpackage/c.java", `
	package com.example.cpackage;
	class C {
		public static void CFunc(String[] args) {
			System.out.println("Hello, World");
		}
	}
	`)

	checkCrossFile := func(valueOpt ssaapi.QueryOption) {
		res, err := ssaapi.QuerySyntaxflow(
			valueOpt,
			ssaapi.QueryWithRuleContent(`
target2(* as $para)
$para #-> * as $target
			`),
		)
		require.NoError(t, err)
		_ = res
		CompareResult(t, false, res, map[string][]string{
			"target": {"Parameter-a"}, // no cross file
		})
	}

	t.Run("test compile", func(t *testing.T) {
		progs, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(vf),
			ssaapi.WithPeepholeSize(1),
			ssaapi.WithRawLanguage("java"),
		)

		require.NoError(t, err)
		require.Equal(t, len(progs), 3)

		for _, prog := range progs {
			prog.Show()
		}
		checkCrossFile(ssaapi.QueryWithPrograms(progs))
	})

	t.Run("test compile and load from db", func(t *testing.T) {
		t.Skip("peephole will use multiple program-id same key in irCode and irType")
		progName := uuid.NewString()
		progs, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(vf),
			ssaapi.WithPeepholeSize(1),
			ssaapi.WithRawLanguage("java"),
			ssaapi.WithProgramName(progName),
		)
		require.NoError(t, err)
		require.Greater(t, len(progs), 0)

		prog, err := ssaapi.FromDatabase(progName)
		require.NoError(t, err)
		require.NotNil(t, prog)

		prog.Show()

		count := 0
		filesys.Recursive(
			fmt.Sprintf("/%s", progName),
			filesys.WithFileSystem(ssadb.NewIrSourceFs()),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				log.Infof("file: %s", s)
				count++
				return nil
			}),
		)
		require.Equal(t, count, 3)

		checkCrossFile(ssaapi.QueryWithProgram(prog))
	})

	t.Run("test compile process", func(t *testing.T) {
		maxProcess := 0.0
		reduce := false
		greaterThanOne := false
		hasProcess := false

		_, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(vf),
			ssaapi.WithRawLanguage("java"),
			ssaapi.WithPeepholeSize(1),
			ssaapi.WithProcess(func(msg string, process float64) {
				log.Infof("%f : %s", process, msg)
				if 0 < process && process < 1 {
					hasProcess = true
				}
				if process > 1 {
					greaterThanOne = true
				}

				if maxProcess < process {
					maxProcess = process
				}

				if process < maxProcess {
					t.Logf("process reduce ")
					reduce = true
				}
			}),
		)
		require.NoError(t, err)
		require.False(t, reduce)
		require.False(t, greaterThanOne)
		require.True(t, hasProcess)
		require.Equal(t, maxProcess, 1.0)
	})

	t.Run("test context cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		_, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(vf),
			ssaapi.WithRawLanguage("java"),
			ssaapi.WithPeepholeSize(1),
			ssaapi.WithContext(ctx),
			ssaapi.WithProcess(func(msg string, process float64) {
				log.Infof("%f : %s", process, msg)
				if process > 0.3 {
					cancel()
				}
				if process > 0.5 {
					t.Errorf("should not reach here")
				}
			}),
		)
		_ = err
		require.Contains(t, err.Error(), ssaapi.ErrContextCancel.Error())
	})
}
