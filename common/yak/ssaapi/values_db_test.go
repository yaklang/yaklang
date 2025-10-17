package ssaapi_test

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/log"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func CreateValue(prog *ssaapi.Program, nodeId int) *ssaapi.Value {
	constInst := ssa.NewConst(nodeId)
	constInst.SetId(int64(nodeId))
	value, err := prog.NewValue(constInst)
	_ = err
	// require.NoError(t, err)
	return value
}

func TestValuesDB_Save_Audit_Node(t *testing.T) {
	t.Run("test save entry node", func(t *testing.T) {
		code := `
		a = {}
		a.c=1
		`
		programName := uuid.NewString()
		prog, err := ssaapi.Parse(code, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(consts.Yak))
		t.Cleanup(func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programName)
		})

		require.NoError(t, err)
		res, err := prog.SyntaxFlowWithError(`a.c<getObject> as $res;`)
		require.NoError(t, err)
		res.GetValues("res").ShowDot()
		_, err = res.Save(schema.SFResultKindDebug)
		require.NoError(t, err)

		nodes, err := ssadb.GetResultNodeByVariable(ssadb.GetDB(), res.GetResultID(), "res")
		require.NoError(t, err)
		require.Equal(t, len(nodes), 1)
	})

	t.Run("test recursiveSaveValue ", func(t *testing.T) {
		progName := uuid.NewString()
		fmt.Println(progName)
		prog, err := ssaapi.Parse(``, ssaapi.WithProgramName(progName))
		require.NoError(t, err)
		/*
			1->2->3
			1->3->4
			1->red->3
		*/
		value1 := CreateValue(prog, 1)
		value2 := CreateValue(prog, 2)
		value3_1 := CreateValue(prog, 3)
		value3_2 := CreateValue(prog, 3)
		value4 := CreateValue(prog, 4)
		value1.AppendPredecessor(value2)
		value2.AppendPredecessor(value3_1)
		value1.AppendPredecessor(value3_2)
		value3_2.AppendPredecessor(value4)

		value3_1.Predecessors = []*ssaapi.PredecessorValue{{
			Node: value1,
			Info: &sfvm.AnalysisContext{
				Step:  -1,
				Label: "dataflow_topdef",
			},
		}}

		values := []*ssaapi.Value{value1, value2, value3_1, value3_2, value4}

		for _, v := range values {
			err := ssaapi.SaveValue(v,
				ssaapi.OptionSaveValue_ProgramName(prog.GetProgramName()),
				ssaapi.OptionSaveValue_ResultVariable("res"),
			)
			require.NoError(t, err)
		}

		t.Cleanup(func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progName)
		})

		db := ssadb.GetDB()
		// check save nodes
		getEntryNodesFromDb := func() []*ssadb.AuditNode {
			var ids []int64
			for _, v := range values {
				ids = append(ids, v.GetId())
			}
			var nodes []*ssadb.AuditNode
			db.Model(&ssadb.AuditNode{}).Where("program_name = ? AND ir_code_id IN (?) AND is_entry_node = true", progName, ids).Find(&nodes)
			return nodes
		}
		entryNodes := getEntryNodesFromDb()
		for _, v := range values {
			log.Infof("value: %v", v)
		}
		for _, n := range entryNodes {
			log.Infof("entry node: %v", n)
		}
		require.Equal(t, len(values), len(entryNodes))

		// // check Edge
		// getNodeByIrCodeId := func(irCodeId int64) []int64 {
		// 	var nodes []*ssadb.AuditNode
		// 	db.Model(&ssadb.AuditNode{}).Where("program_name = ? AND ir_code_id = ?", progName, irCodeId).Find(&nodes)
		// 	ids := lo.Map(nodes, func(item *ssadb.AuditNode, index int) int64 {
		// 		return int64(item.ID)
		// 	})
		// 	return ids
		// }

		// {
		// 	node1 := getNodeByIrCodeId(1)
		// 	node2 := getNodeByIrCodeId(2)
		// 	node4 := getNodeByIrCodeId(4)
		// 	node3 := getNodeByIrCodeId(3)

		// 	var (
		// 		edge3a []uint
		// 		edge3b []uint
		// 	)

		// 	var edge3_1 []*ssadb.AuditEdge
		// 	db.Model(&ssadb.AuditEdge{}).Where("program_name = ? AND from_node IN (?) AND to_node in (?) ", progName, node2, node1).Find(&edge3_1)
		// 	require.Greater(t, len(edge3_1), 1)
		// 	for _, e := range edge3_1 {
		// 		edge3a = append(edge3a, e.FromNode)
		// 		fmt.Printf("edge3_1: fromNode:%v,toNode:%v,edgeType:%v\n", e.FromNode, e.ToNode, e.EdgeType)
		// 	}

		// 	var edge3_2 []*ssadb.AuditEdge
		// 	db.Model(&ssadb.AuditEdge{}).Where("program_name = ? AND from_node IN (?) AND to_node in (?) ", progName, node2, node3).Find(&edge3_2)
		// 	require.Equal(t, 1, len(edge3_2))
		// 	for _, e := range edge3_2 {
		// 		edge3b = append(edge3b, e.FromNode)
		// 		fmt.Printf("edge3_2: fromNode:%v,toNode:%v,edgeType:%v\n", e.FromNode, e.ToNode, e.EdgeType)
		// 	}

		// 	var edge3_4 []*ssadb.AuditEdge
		// 	// db = db.Debug()
		// 	db.Model(&ssadb.AuditEdge{}).Where("program_name = ? AND from_node IN (?) AND to_node IN (?) ", progName, node3, node4).Find(&edge3_4)
		// 	// node3 -> node4 位于范围外，不会构建边
		// 	require.Equal(t, 0, len(edge3_4))
		// }
	})
}

func TestDataFlowPath(t *testing.T) {
	vf := filesys.NewVirtualFs()

	vf.AddFile("org/joychou/controller/Cookies.java", `
package org.joychou.controller;

public class Cookies {
    @GetMapping(value = "/vuln06")
    public String vuln06(@CookieValue(value = "nick") String nick) {
		nick = "123" + nick;
        return "Cookie nick: " + nick;
    }
}
	`)

	progId := uuid.NewString()
	prog, err := ssaapi.ParseProject(
		ssaapi.WithProgramName(progId),
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithFileSystem(vf),
	)
	log.Infof("prog: %v", progId)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progId)
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(`
	GetMapping.__ref__(* as $param)
	$param<getFunc><getReturns> as $returns;
	$returns #{until:"* & $param"}-> as $result;
		`)
	require.NoError(t, err)
	_ = res

	t.Run("test memory result", func(t *testing.T) {

		for _, v := range res.GetValues("result") {
			// v.ShowDot()
			dotGraph := v.DotGraph()
			fmt.Println(dotGraph)

			// check syntaxflow step
			require.Contains(t, dotGraph, "getFunc")
			require.Contains(t, dotGraph, "getReturns")

			// check dataflow step
			// // this data not exist in dataflow path,
			// require.Contains(t, dotGraph, `"Cookie nick: "`)
			// require.Contains(t, dotGraph, `"123"`)

			// check no dataflow edge
			require.NotContains(t, dotGraph, ssaapi.Predecessors_TopDefLabel)
		}
	})

	t.Run("test save result", func(t *testing.T) {
		// database
		id, err := res.Save(schema.SFResultKindDebug)
		require.NoError(t, err)

		result, err := ssaapi.LoadResultByID(id)
		require.NoError(t, err)
		require.Equal(t, result.GetResultID(), id)

		for _, v := range result.GetValues("result") {
			fmt.Println(v.GetId(), v.GetName())
			// v.ShowDot()
			dotGraph := v.DotGraph()
			fmt.Println(dotGraph)

			// check syntaxflow step
			require.Contains(t, dotGraph, "getFunc")
			require.Contains(t, dotGraph, "getReturns")

			/*
				+----------------+          +----------------+
				|  "Cookie nick: "| <--------| add("Cookie nick: ", |
				|                |          |  add("123", Param))  |
				+----------------+          +----------------+
											|
											|
											|
										+----------------+    +-----------------+
										| add("123",     | --→| Parameter-nick  |
										|  Parameter-nick)|    |                 |
										+----------------+    +-----------------+
											|
											|
										+----------------+
										|     "123"      |
										+----------------+
			*/

			// check dataflow step, this data not exist in dataflow path,
			require.NotContains(t, dotGraph, `"\"Cookie nick: \""`)
			require.NotContains(t, dotGraph, `"\"123\""`)

			require.Contains(t, dotGraph, `\"Cookie nick: \" + nick`)
			require.Contains(t, dotGraph, `\"123\" + nick`)
			require.Contains(t, dotGraph, `@CookieValue(value = \"nick\") String nick`)

			// check no dataflow edge
			require.NotContains(t, dotGraph, ssaapi.Predecessors_TopDefLabel)
		}
	})

}
