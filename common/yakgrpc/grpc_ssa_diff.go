package yakgrpc

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type compareType int64

const (
	Unknown compareType = iota
	Custom
	Risk
)

func (s *Server) NewSSADiff(req *ypb.SSADiffRequest, server ypb.Yak_NewSSADiffServer) error {
	context := server.Context()
	if req.Base == nil || req.Compare == nil {
		return utils.Error("base and compare are required")
	}
	taskChannel := make(chan *ssaapi.CompareResult[*schema.SSARisk], 1)
	processor := utils.NewBatchProcessor[*ssaapi.CompareResult[*schema.SSARisk]](context, taskChannel, utils.WithBatchProcessorCallBack(func(risks []*ssaapi.CompareResult[*schema.SSARisk]) {
		utils.GormTransactionReturnDb(consts.GetGormDefaultSSADataBase(), func(tx *gorm.DB) {
			for _, risk := range risks {
				tx.Save(&schema.SSADiffResult{
					BaseProgram:     req.Base.Program,
					CompareProgram:  req.Compare.Program,
					RuleName:        risk.FromRule,
					BaseRiskHash:    risk.BaseValHash,
					CompareRiskHash: risk.NewValHash,
					Status:          int(risk.Status),
					CompareType:     int(Risk),
				})
			}
		})
	}),
	)
	processor.Start()
	switch req.Type {
	case int64(Custom):
		return utils.Error("custom diff type not supported")
	case int64(Risk):
		compare := ssaapi.NewSsaCompare[*schema.SSARisk](ssaapi.NewCompareRiskItem(req.GetBase().GetProgram(),
			ssaapi.WithVariableName(req.GetBase().GetVariable()),
			ssaapi.WithRuleName(req.GetBase().GetRuleName()),
		))
		res := compare.Compare(context, ssaapi.NewCompareRiskItem(req.GetCompare().GetProgram(),
			ssaapi.WithVariableName(req.GetCompare().GetVariable()),
			ssaapi.WithRuleName(req.GetCompare().GetRuleName()),
		),
			ssaapi.WithCompareResultCallback(func(re *ssaapi.CompareResult[*schema.SSARisk]) {
				taskChannel <- re
			}),
			ssaapi.WithCompareResultGetValueInfo[*schema.SSARisk](func(value *schema.SSARisk) (rule string, originHash string, diffHash string) {
				return value.FromRule, value.Hash, utils.CalcMd5(value.FromRule, value.CodeFragment, value.Variable)
			}),
		)
		for re := range res {
			server.Send(&ypb.SSADiffResponse{
				BaseRisk:    re.BaseValue.ToGRPCModel(),
				CompareRisk: re.NewValue.ToGRPCModel(),
				RuleName:    re.FromRule,
				Status:      int64(re.Status),
			})
		}
		close(taskChannel)
		processor.Wait()
		return nil
	default:
		return utils.Error("unknown diff type")
	}
}
