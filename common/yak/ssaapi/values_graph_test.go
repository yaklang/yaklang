package ssaapi_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestGraph(t *testing.T) {

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
		class B {
			public  int get() {
				return 	 1;
			}
			public void show(int a) {
				target2(a);
			}
		}
		`)
	progID := uuid.NewString()
	prog, err := ssaapi.ParseProject(vf,
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	require.NoError(t, err)
	require.NotNil(t, prog)

	query := `
		target* as $target 
		$target (* #{
			hook: <<<HOOK
				* as $a 
HOOK
		}-> as $para_top_def)
		` // , "`*  as $a`"

	require.Len(t, prog, 1)
	res, err := prog[0].SyntaxFlowWithError(query)
	require.NoError(t, err)
	resultID, err := res.Save()
	require.NoError(t, err)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	result, err := ssaapi.CreateResultByID(resultID)
	require.NoError(t, err)

	// memory
	var memPath [][]string
	var memTime time.Duration
	{
		start := time.Now()
		valueMem := res.GetValues("para_top_def")
		require.NotNil(t, valueMem)
		require.Greater(t, len(valueMem), 0)
		value := valueMem[0]
		graph := ssaapi.NewValueGraph(value)
		dotStr := graph.Dot()
		since := time.Since(start)
		log.Infof("memory graph time: %v", since)
		log.Infof("memory graph time: %d", since)
		log.Infof("dot graph: \n%v", dotStr)
		memPath = graph.DeepFirstGraph(value.GetId())
		memTime = since
	}

	// database
	var dbPath [][]string
	var dbTime time.Duration
	{
		start := time.Now()
		valueDB := result.GetValues("para_top_def")
		require.Greater(t, len(valueDB), 0)
		value := valueDB[0]
		graphDB := ssaapi.NewValueGraph(value)
		dotStrDB := graphDB.Dot()
		since := time.Since(start)
		log.Infof("db graph time: %v", since)
		log.Infof("db graph time: %d", since)
		log.Infof("dot graph from db: \n%v", dotStrDB)
		dbPath = graphDB.DeepFirstGraph(value.GetId())
		dbTime = since
	}

	require.True(t, memTime*20 > dbTime)
	require.Equal(t, len(memPath), len(dbPath))

}
