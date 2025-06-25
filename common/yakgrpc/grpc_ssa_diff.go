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
	if req.Base.Program == "" && req.Base.RiskRuntimeId == "" {
		return utils.Error("base and compare are required")
	}
	kind := schema.Prog
	if req.Base.Program == "" {
		kind = schema.RuntimeId
	}
	switch req.Type {
	case int64(Custom):
		return utils.Error("custom diff type not supported")
	case int64(Risk):
		baseItem, err := ssaapi.NewCompareRiskItem(
			ssaapi.DiffWithVariableName(req.GetBase().GetVariable()),
			ssaapi.DiffWithRuleName(req.GetBase().GetRuleName()),
			ssaapi.DiffWithProgram(req.GetBase().GetProgram()),
		)
		if err != nil {
			return err
		}
		compare := ssaapi.NewSsaCompare[*schema.SSARisk](baseItem)
		compareItem, err := ssaapi.NewCompareRiskItem(
			ssaapi.DiffWithVariableName(req.GetCompare().GetVariable()),
			ssaapi.DiffWithRuleName(req.GetCompare().GetRuleName()),
			ssaapi.DiffWithProgram(req.GetCompare().GetProgram()))
		if err != nil {
			return err
		}
		res := compare.Compare(context, compareItem,
			ssaapi.WithSaveValueFunc(func(risks []*ssaapi.CompareResult[*schema.SSARisk]) {
				utils.GormTransactionReturnDb(consts.GetGormDefaultSSADataBase(), func(tx *gorm.DB) {
					for _, risk := range risks {
						result := &schema.SSADiffResult{
							BaseItem:        req.Base.Program,
							CompareItem:     req.Compare.Program,
							RuleName:        risk.FromRule,
							BaseRiskHash:    risk.BaseValHash,
							CompareRiskHash: risk.NewValHash,
							Status:          int(risk.Status),
							CompareType:     int(Risk),
							DiffResultKind:  kind,
						}
						if kind == schema.RuntimeId {
							result.BaseItem = req.Base.RiskRuntimeId
							result.CompareItem = req.Compare.RiskRuntimeId
						}
						tx.Save(result)
					}
				})
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
		//close(taskChannel)
		//processor.Wait()
		return nil
	default:
		return utils.Error("unknown diff type")
	}
}
