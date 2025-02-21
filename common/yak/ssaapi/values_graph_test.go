package ssaapi_test

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"strings"
	"testing"
	"time"
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
				b.show(b.get());
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
	prog, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	require.NoError(t, err)
	require.NotNil(t, prog)

	// TODO：hook会产生随机结果
	query := `
		target* as $target
		$target (* #-> as $para_top_def)
		` // , "`*  as $a`"

	require.Len(t, prog, 1)
	res, err := prog[0].SyntaxFlowWithError(query)
	require.NoError(t, err)
	resultID, err := res.Save(schema.SFResultKindDebug)
	require.NoError(t, err)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	result, err := ssaapi.LoadResultByID(resultID)
	require.NoError(t, err)

	// memory
	var memPath [][]string
	// var memTime time.Duration
	{
		res.Show()
		start := time.Now()
		valueMem := res.GetValues("para_top_def")
		require.NotNil(t, valueMem)
		require.Equal(t, len(valueMem), 1)
		value := valueMem[0]
		graph := ssaapi.NewValueGraph(value)
		dotStr := graph.Dot()
		since := time.Since(start)
		log.Infof("memory graph time: %v", since)
		log.Infof("memory graph time: %d", since)
		log.Infof("dot graph: \n%v", dotStr)
		memPath = graph.DeepFirstGraphPrev(value)
		// memTime = since
	}

	// database
	var dbPath [][]string
	// var dbTime time.Duration
	{
		start := time.Now()
		valueDB := result.GetValues("para_top_def")
		require.Equal(t, len(valueDB), 1)
		value := valueDB[0]
		graphDB := ssaapi.NewValueGraph(value)
		dotStrDB := graphDB.Dot()
		since := time.Since(start)
		log.Infof("db graph time: %v", since)
		log.Infof("db graph time: %d", since)
		log.Infof("dot graph from db: \n%v", dotStrDB)
		dbPath = graphDB.DeepFirstGraphPrev(value)
		// dbTime = since
	}
	log.Infof("memory path: %v", memPath)
	log.Infof("db path: %v", dbPath)

	require.Equal(t, 2, len(memPath))
	require.Equal(t, len(memPath), len(dbPath))
}

func TestGraph2(t *testing.T) {
	code := `
public interface RemoteLogService
{
	@PostMapping("/operlog")
   public R<Boolean> saveLog(@RequestBody SysOperLog sysOperLog, @RequestHeader(SecurityConstants.FROM_SOURCE) String source) throws Exception;

	@Override
   public T deserialize1() throws SerializationException
   {
       return JSON.parseObject(str, clazz, AUTO_TYPE_FILTER);
   }
	@Override
   public T deserialize2() throws SerializationException
   {
       return JSON.parseObject(str, clazz, AUTO_TYPE_FILTER);
   }
	@Override
   public T deserialize3() throws SerializationException
   {
       return JSON.parseObject(str, clazz, AUTO_TYPE_FILTER);
   }
	@Override
   public T deserialize4() throws SerializationException
   {
       return JSON.parseObject(str, clazz, AUTO_TYPE_FILTER);
   }
}
	`

	ProgName := uuid.NewString()
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithProgramName(ProgName),
	)
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(`
	// <include('java-spring-param')> as $entry;
	JSON.parse*() as $entry;
	`)
	require.NoError(t, err)
	entrys := res.GetValues("entry")
	require.Greater(t, len(entrys), 0)
	entry := entrys[0]
	graph := ssaapi.NewValueGraph(entry)
	path := graph.DeepFirstGraphPrev(entry)
	log.Infof("path: %v", path)
	memDot := entry.DotGraph()
	log.Infof("dot: \n%v", memDot)
	require.Equal(t, len(path), 1)

	resultID, err := res.Save(schema.SFResultKindDebug)
	require.NoError(t, err)

	result, err := ssaapi.LoadResultByID(resultID)
	require.NoError(t, err)
	entrysDB := result.GetValues("entry")
	require.Greater(t, len(entrysDB), 0)
	entryDB := entrysDB[0]
	graphDB := ssaapi.NewValueGraph(entryDB)
	pathDB := graphDB.DeepFirstGraphPrev(entryDB)
	require.Equal(t, len(pathDB), 1)

	log.Infof("path from db: %v", pathDB)
	dbDot := entryDB.DotGraph()
	log.Infof("dot from db: \n%v", dbDot)
}

func Test_Values_Graph_Dot(t *testing.T) {
	t.Run("test dfs simple", func(t *testing.T) {
		progName := uuid.NewString()
		prog, err := ssaapi.Parse(``, ssaapi.WithProgramName(progName))
		require.NoError(t, err)
		value1 := CreateValue(prog, 1)
		value2 := CreateValue(prog, 2)
		value3_1 := CreateValue(prog, 3)
		value3_2 := CreateValue(prog, 3)
		value4 := CreateValue(prog, 4)
		value1.AppendDependOn(value2)
		value2.AppendDependOn(value3_1)
		value1.AppendDependOn(value3_2)
		value3_2.AppendDependOn(value4)

		graph := ssaapi.NewValueGraph(value1)
		graph.ShowDot()

		result := graph.DeepFirstGraphNext(value1)
		require.Equal(t, 2, len(result))
		require.Equal(t, strings.Count(graph.Dot(), "t3: 3"), 2)
	})

	t.Run("test dfs with predecessor", func(t *testing.T) {
		progName := uuid.NewString()
		prog, err := ssaapi.Parse(``, ssaapi.WithProgramName(progName))
		require.NoError(t, err)
		value1 := CreateValue(prog, 1)
		value2 := CreateValue(prog, 2)
		value3 := CreateValue(prog, 3)
		value4 := CreateValue(prog, 4)

		value1.AppendDependOn(value2)
		value2.AppendDependOn(value3)
		value3.AppendDependOn(value4)

		value1.AppendPredecessor(value2, sfvm.WithAnalysisContext_Label("Test1"), sfvm.WithAnalysisContext_Step(1))
		value1.AppendPredecessor(value3, sfvm.WithAnalysisContext_Label("Test2"), sfvm.WithAnalysisContext_Step(2))
		value3.AppendPredecessor(value4, sfvm.WithAnalysisContext_Label("Test3"), sfvm.WithAnalysisContext_Step(-1))
		graph := ssaapi.NewValueGraph(value1)
		graph.ShowDot()

		require.Contains(t, graph.Dot(), "step[1]: Test1")
		require.Contains(t, graph.Dot(), "step[2]: Test2")
		require.Contains(t, graph.Dot(), "Test3")
	})
}
