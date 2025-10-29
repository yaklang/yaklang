package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SyntaxFlow_Result(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	syntaxFlowCode := `println(* as $para)`

	progName := uuid.NewString()
	prog, err := ssaapi.Parse(`println("araa")`,
		ssaapi.WithProgramName(progName), ssaapi.WithLanguage(ssaconfig.Yak),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	require.NoError(t, err)
	taskID1 := uuid.NewString()
	res := prog.SyntaxFlow(syntaxFlowCode)
	resultID1, err := res.Save(schema.SFResultKindDebug, taskID1)
	require.NoError(t, err)

	taskID2 := uuid.NewString()
	res = prog.SyntaxFlow(syntaxFlowCode)
	resultID2, err := res.Save(schema.SFResultKindScan, taskID2)
	require.NoError(t, err)
	res = prog.SyntaxFlow(syntaxFlowCode)
	resultID3, err := res.Save(schema.SFResultKindQuery, taskID2)
	require.NoError(t, err)

	taskID3 := uuid.NewString()
	_, err = res.Save(schema.SFResultKindSearch, taskID3)
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

	t.Run("test query kind", func(t *testing.T) {
		// kind Debug (resultID1)
		rsp, err := local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{},
			Filter: &ypb.SyntaxFlowResultFilter{
				ProgramNames: []string{progName},
				Kind:         []string{string(schema.SFResultKindDebug)},
			},
		})
		require.NoError(t, err)
		spew.Dump(rsp)

		require.Equal(t, 1, len(rsp.GetResults()))
		require.Equal(t, resultID1, uint(rsp.GetResults()[0].GetResultID()))

		// kind Scan (resultID2)
		rsp, err = local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{},
			Filter: &ypb.SyntaxFlowResultFilter{
				ProgramNames: []string{progName},
				Kind:         []string{string(schema.SFResultKindScan)},
			},
		})
		require.NoError(t, err)
		spew.Dump(rsp)

		require.Equal(t, 1, len(rsp.GetResults()))
		require.Equal(t, resultID2, uint(rsp.GetResults()[0].GetResultID()))

		// kind Query (resultID3)
		rsp, err = local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{},
			Filter: &ypb.SyntaxFlowResultFilter{
				ProgramNames: []string{progName},
				Kind:         []string{string(schema.SFResultKindQuery)},
			},
		})
		require.NoError(t, err)
		spew.Dump(rsp)

		require.Equal(t, 1, len(rsp.GetResults()))
		require.Equal(t, resultID3, uint(rsp.GetResults()[0].GetResultID()))

		// query resultID1 check kind Debug
		rsp, err = local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{},
			Filter: &ypb.SyntaxFlowResultFilter{
				ResultIDs: []string{fmt.Sprintf("%d", resultID1)},
			},
		})
		require.NoError(t, err)
		spew.Dump(rsp)
		require.Equal(t, 1, len(rsp.GetResults()))
		require.Equal(t, string(schema.SFResultKindDebug), rsp.GetResults()[0].GetKind())

	})

	//
	t.Run("test exclude search kind", func(t *testing.T) {
		result, err2 := local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{},
		})
		require.NoError(t, err2)
		for _, flowResult := range result.Results {
			require.True(t, flowResult.Kind != string(schema.SFResultKindSearch))
		}
	})

}

func TestGRPCMUSTPASS_SyntaxFlow_Notify(t *testing.T) {
	yakit.InitialDatabase()

	local, err := NewLocalClient(true)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	stream, err := local.DuplexConnection(ctx)
	require.NoError(t, err)

	taskID1 := uuid.NewString()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		check_syntaxflow_result := false
		check_ssa_risk := false
		for {
			res, err := stream.Recv()
			log.Info(res)
			if err == context.Canceled {
				break
			}
			log.Info(err)
			if err != nil {
				break
			}
			require.NotNil(t, res)
			if res.MessageType == ssadb.ServerPushType_SyntaxflowResult {
				var tmp map[string]string
				err = json.Unmarshal(res.GetData(), &tmp)
				require.NoError(t, err)
				require.Equal(t, tmp["task_id"], taskID1)
				check_syntaxflow_result = true
			}
			if res.MessageType == schema.ServerPushType_SSARisk {
				var tmp map[string]string
				err = json.Unmarshal(res.GetData(), &tmp)
				require.NoError(t, err)
				require.Equal(t, tmp["task_id"], taskID1)
				check_ssa_risk = true
			}
			if check_syntaxflow_result && check_ssa_risk {
				break
			}
		}
	}()

	{
		progName := uuid.NewString()
		prog, err := ssaapi.Parse(`println("araa")`,
			ssaapi.WithProgramName(progName),
			ssaapi.WithLanguage(ssaconfig.Yak),
		)
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
		require.NoError(t, err)

		res := prog.SyntaxFlow(`println(* as $para); alert $para`)
		resultID1, err := res.Save(schema.SFResultKindDebug, taskID1)
		defer ssadb.DeleteResultByID(resultID1)
		defer yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskID1},
		})
		require.NoError(t, err)

		// check have risk
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskID1},
		}, nil)
		require.NoError(t, err)
		require.Equal(t, 1, len(risks))
		require.Equal(t, taskID1, risks[0].RuntimeId)
		_ = resultID1
	}
	cancel()
	wg.Wait()

}

func TestGRPCMUSTPASS_SyntaxFlow_ResultDelete(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	progName := uuid.NewString()
	prog, err := ssaapi.Parse(`println("araa")`,
		ssaapi.WithProgramName(progName), ssaapi.WithLanguage(ssaconfig.Yak),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	require.NoError(t, err)

	query := func() []uint {
		rsp, err := local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Pagination: &ypb.Paging{
				Page:     0,
				Limit:    0,
				OrderBy:  "id",
				Order:    "",
				RawOrder: "",
			},
			Filter: &ypb.SyntaxFlowResultFilter{
				ProgramNames: []string{progName},
			},
		})
		require.NoError(t, err)
		resultIDs := make([]uint, 0, len(rsp.GetResults()))
		for _, res := range rsp.GetResults() {
			resultIDs = append(resultIDs, uint(res.GetResultID()))
		}
		return resultIDs
	}

	deleteAllResult := func(deleteContainRisk bool) {
		rsp, err := local.DeleteSyntaxFlowResult(context.Background(), &ypb.DeleteSyntaxFlowResultRequest{
			DeleteContainRisk: deleteContainRisk,
			DeleteAll:         true,
			Filter: &ypb.SyntaxFlowResultFilter{
				ProgramNames: []string{progName},
			},
		})
		require.NoError(t, err)
		_ = rsp
	}
	deleteResult := func(deleteContainRisk bool, resultIDs ...string) int {
		rsp, err := local.DeleteSyntaxFlowResult(context.Background(), &ypb.DeleteSyntaxFlowResultRequest{
			DeleteContainRisk: deleteContainRisk,
			Filter: &ypb.SyntaxFlowResultFilter{
				ResultIDs: resultIDs,
			},
		})
		require.NoError(t, err)
		_ = rsp
		return int(rsp.Message.EffectRows)
	}
	_ = deleteResult

	syntaxflowCodeWithRisk := `
	println(* as $para)
	alert $para for {
		"level": "info", 
	}
	`
	syntaxFlowCode := `println(* as $para)`

	t.Run("test delete all result", func(t *testing.T) {
		res := prog.SyntaxFlow(syntaxflowCodeWithRisk)
		resultID1, err := res.Save(schema.SFResultKindDebug) // risk
		require.NoError(t, err)

		res = prog.SyntaxFlow(syntaxFlowCode)
		resultID2, err := res.Save(schema.SFResultKindScan) // no  risk
		_ = resultID2
		require.NoError(t, err)

		res = prog.SyntaxFlow(syntaxFlowCode)
		resultID3, err := res.Save(schema.SFResultKindQuery) // no risk
		_ = resultID3
		require.NoError(t, err)

		// delete normal result, risk result will not deleted
		// now: [risk, no-risk, no-risk]
		deleteAllResult(false)
		require.Equal(t, []uint{resultID1}, query())

		res = prog.SyntaxFlow(syntaxFlowCode)
		_, err = res.Save(schema.SFResultKindScan) // no  risk
		require.NoError(t, err)
		res = prog.SyntaxFlow(syntaxFlowCode)
		_, err = res.Save(schema.SFResultKindQuery) // no risk
		require.NoError(t, err)

		// delete all, risk result will be deleted, and risk will be deleted
		// now [risk, no-risk, no-risk]
		deleteAllResult(true)
		require.Equal(t, len(query()), 0)

		riskCount := 0
		err = consts.GetGormProjectDatabase().Model(&schema.Risk{}).Where("result_id = ?", resultID1).Count(&riskCount).Error
		require.NoError(t, err)
		require.Equal(t, 0, riskCount)
	})

	t.Run("test delete contain risk", func(t *testing.T) {
		res := prog.SyntaxFlow(syntaxflowCodeWithRisk)
		resultID1, err := res.Save(schema.SFResultKindDebug) // risk
		require.NoError(t, err)

		res = prog.SyntaxFlow(syntaxFlowCode)
		resultID2, err := res.Save(schema.SFResultKindScan) // no  risk
		require.NoError(t, err)

		res = prog.SyntaxFlow(syntaxFlowCode)
		resultID3, err := res.Save(schema.SFResultKindQuery) // no risk
		require.NoError(t, err)

		// delete normal result by id
		{
			count := deleteResult(false, fmt.Sprintf("%d", resultID2))
			require.Equal(t, []uint{resultID1, resultID3}, query())
			require.Equal(t, 1, count)
		}

		// delete contain risk result, but false
		{
			count := deleteResult(false, fmt.Sprintf("%d", resultID1))
			require.Equal(t, 0, count)
			require.Equal(t, []uint{resultID1, resultID3}, query())
		}

		// delete contain risk result
		deleteResult(true, fmt.Sprintf("%d", resultID1))
		require.Equal(t, []uint{resultID3}, query())

	})
}
