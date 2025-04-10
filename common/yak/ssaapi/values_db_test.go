package ssaapi_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
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
		_, err = res.Save(schema.SFResultKindDebug)
		require.NoError(t, err)

		nodes, err := ssadb.GetResultNodeByVariable(ssadb.GetDB(), res.GetResultID(), "res")
		require.NoError(t, err)
		require.Equal(t, 1, len(nodes))
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
		value3 := CreateValue(prog, 3)
		value4 := CreateValue(prog, 4)
		value1.AppendDependOn(value2)
		value2.AppendDependOn(value3)
		value1.AppendDependOn(value3)
		value3.AppendDependOn(value4)

		value3.Predecessors = []*ssaapi.PredecessorValue{{
			Node: value1,
			Info: &sfvm.AnalysisContext{
				Step:  -1,
				Label: "dataflow_topdef",
			},
		}}

		values := []*ssaapi.Value{value1, value2, value3, value4}

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
		require.Equal(t, len(values), len(entryNodes))

		// check Edge
		getNodeByIrCodeId := func(irCodeId int64) []int64 {
			var nodes []*ssadb.AuditNode
			db.Model(&ssadb.AuditNode{}).Where("program_name = ? AND ir_code_id = ?", progName, irCodeId).Find(&nodes)
			ids := lo.Map(nodes, func(item *ssadb.AuditNode, index int) int64 {
				return int64(item.ID)
			})
			return ids
		}

		{
			node1 := getNodeByIrCodeId(1)
			node2 := getNodeByIrCodeId(2)
			node4 := getNodeByIrCodeId(4)
			node3 := getNodeByIrCodeId(3)

			var (
				edge3a []uint
				edge3b []uint
			)

			var edge3_1 []*ssadb.AuditEdge
			db.Model(&ssadb.AuditEdge{}).Where("program_name = ? AND from_node IN (?) AND to_node in (?) ", progName, node2, node1).Find(&edge3_1)
			require.Equal(t, 1, len(edge3_1))
			for _, e := range edge3_1 {
				edge3a = append(edge3a, e.FromNode)
				fmt.Printf("edge3_1: fromNode:%v,toNode:%v,edgeType:%v\n", e.FromNode, e.ToNode, e.EdgeType)
			}

			var edge3_2 []*ssadb.AuditEdge
			db.Model(&ssadb.AuditEdge{}).Where("program_name = ? AND from_node IN (?) AND to_node in (?) ", progName, node2, node3).Find(&edge3_2)
			require.Equal(t, 1, len(edge3_2))
			for _, e := range edge3_2 {
				edge3b = append(edge3b, e.FromNode)
				fmt.Printf("edge3_2: fromNode:%v,toNode:%v,edgeType:%v\n", e.FromNode, e.ToNode, e.EdgeType)
			}

			var edge3_4 []*ssadb.AuditEdge
			db = db.Debug()
			db.Model(&ssadb.AuditEdge{}).Where("program_name = ? AND from_node IN (?) AND to_node IN (?) ", progName, node3, node4).Find(&edge3_4)
			// node3 -> node4 位于范围外，不会构建边
			require.Equal(t, 0, len(edge3_4))
		}
	})
}
