package yakgrpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SyntaxFlow_Result(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	syntaxFlowCode := `println(* as $para)`

	progName := uuid.NewString()
	prog, err := ssaapi.Parse(`println("araa")`,
		ssaapi.WithProgramName(progName), ssaapi.WithLanguage(ssaapi.Yak),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	require.NoError(t, err)
	taskID1 := uuid.NewString()
	res := prog.SyntaxFlow(syntaxFlowCode)
	resultID1, err := res.Save(taskID1)
	require.NoError(t, err)

	taskID2 := uuid.NewString()
	res = prog.SyntaxFlow(syntaxFlowCode)
	resultID2, err := res.Save(taskID2)
	require.NoError(t, err)
	res = prog.SyntaxFlow(syntaxFlowCode)
	resultID3, err := res.Save(taskID2)
	require.NoError(t, err)

	t.Run("test query result by taskID", func(t *testing.T) {
		// taskID1 (resultID1)
		rsp, err := local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{},
			Filter:     &ypb.SyntaxFlowResultFilter{TaskIDs: []string{taskID1}},
		})
		require.NoError(t, err)
		spew.Dump(rsp)

		require.Equal(t, 1, len(rsp.GetResults()))
		result := rsp.GetResults()[0]
		require.Equal(t, taskID1, result.GetTaskID())
		require.Equal(t, resultID1, uint(result.GetResultID()))
		require.Equal(t, progName, result.GetProgramName())

		// taskID2 (resultID2, resultID3)
		rsp, err = local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{},
			Filter: &ypb.SyntaxFlowResultFilter{
				TaskIDs: []string{taskID2},
			},
		})
		require.NoError(t, err)
		spew.Dump(rsp)

		require.Equal(t, 2, len(rsp.GetResults()))
		require.Equal(t, resultID2, uint(rsp.GetResults()[0].GetResultID()))
		require.Equal(t, resultID3, uint(rsp.GetResults()[1].GetResultID()))
		// check content
		require.Equal(t, syntaxFlowCode, rsp.GetResults()[0].GetRuleContent())
		require.Equal(t, syntaxFlowCode, rsp.GetResults()[1].GetRuleContent())
	})

	t.Run("test query result by program", func(t *testing.T) {
		// progName (resultID1, resultID2, resultID3)
		rsp, err := local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{},
			Filter: &ypb.SyntaxFlowResultFilter{
				ProgramNames: []string{progName},
			},
		})
		require.NoError(t, err)
		spew.Dump(rsp)

		require.Equal(t, 3, len(rsp.GetResults()))
		require.Equal(t, resultID1, uint(rsp.GetResults()[0].GetResultID()))
		require.Equal(t, resultID2, uint(rsp.GetResults()[1].GetResultID()))
		require.Equal(t, resultID3, uint(rsp.GetResults()[2].GetResultID()))
	})

}

func TestGRPCMUSTPASS_Syntaxflow_Notify(t *testing.T) {
	yakit.InitialDatabase()

	local, err := NewLocalClient(true)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	stream, err := local.DuplexConnection(ctx)
	require.NoError(t, err)

	taskID1 := ""
	{
		progName := uuid.NewString()
		prog, err := ssaapi.Parse(`println("araa")`,
			ssaapi.WithProgramName(progName), ssaapi.WithLanguage(ssaapi.Yak),
		)
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
		require.NoError(t, err)

		taskID1 = uuid.NewString()
		res := prog.SyntaxFlow(`println(* as $para); alert $para`)
		resultID1, err := res.Save(taskID1)
		defer ssadb.DeleteResultByID(resultID1)
		defer yakit.DeleteRisk(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{
			RuntimeId: taskID1,
		})
		require.NoError(t, err)
		_ = resultID1
	}

	passCheck := 0

	var res *ypb.DuplexConnectionResponse
	for {
		res, err = stream.Recv()
		log.Info(res)
		log.Info(err)
		if res.MessageType == "syntaxflow_result" {
			var tmp map[string]string
			err = json.Unmarshal(res.GetData(), &tmp)
			require.NoError(t, err)
			require.Equal(t, tmp["task_id"], taskID1)
			passCheck++
		}
		if res.MessageType == "risk" {
			_, risks, err := yakit.QueryRisks(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{
				RuntimeId: taskID1,
			})
			require.NoError(t, err)
			require.Equal(t, 1, len(risks))
			require.Equal(t, taskID1, risks[0].RuntimeId)
			passCheck++
		}

		if passCheck == 2 {
			break
		}
	}
	cancel()
}
