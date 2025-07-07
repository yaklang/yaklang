package yakgrpc

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

type SSARiskDiffRequestStream interface {
	Send(response *ypb.SSARiskDiffResponse) error
	Context() context.Context
}

type wrapperSSARiskDiffStream struct {
	ctx            context.Context
	root           ypb.Yak_SSARiskDiffServer
	RequestHandler func(request *ypb.SSARiskDiffRequest) bool
	sendMutex      *sync.Mutex
}

func newWrapperSSARiskDiffStream(ctx context.Context, stream ypb.Yak_SSARiskDiffServer) *wrapperSSARiskDiffStream {
	return &wrapperSSARiskDiffStream{
		root: stream, ctx: ctx,
		sendMutex: new(sync.Mutex),
	}
}

func (w *wrapperSSARiskDiffStream) Send(r *ypb.SSARiskDiffResponse) error {
	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()
	return w.root.Send(r)
}

func (w *wrapperSSARiskDiffStream) Context() context.Context {
	return w.ctx
}

func (s *Server) SSARiskDiff(req *ypb.SSARiskDiffRequest, server ypb.Yak_SSARiskDiffServer) error {
	stream := newWrapperSSARiskDiffStream(server.Context(), server)
	context := stream.Context()
	if req.GetBaseLine() == nil || req.GetCompare() == nil {
		return utils.Error("base and compare are required")
	}

	if req.GetBaseLine().GetProgramName() == "" && req.GetBaseLine().GetRiskRuntimeId() == "" {
		return utils.Error("base and compare are required")
	}

	base := req.GetBaseLine()
	compare := req.GetCompare()

	kind := schema.Program
	if base.GetProgramName() == "" {
		kind = schema.RuntimeId
	}

	switch req.Type {
	case "custom":
		// 自定义对比
		return utils.Error("custom diff type not supported")
	case "risk":
		// 对比Risk

		// 使用baseLine项目的risk作为对比的基础
		baseRiskItem, err := ssaapi.NewSSARiskComparisonItem(
			ssaapi.DiffWithVariableName(base.GetVariable()),
			ssaapi.DiffWithRuleName(base.GetRuleName()),
			ssaapi.DiffWithProgram(base.GetProgramName()),
		)
		if err != nil {
			return err
		}

		// 创建比较器
		resultComparator := ssaapi.NewSSAComparator[*schema.SSARisk](baseRiskItem)
		// 使用compare项目的risk进行对比
		compareRiskItem, err := ssaapi.NewSSARiskComparisonItem(
			ssaapi.DiffWithVariableName(compare.GetVariable()),
			ssaapi.DiffWithRuleName(compare.GetRuleName()),
			ssaapi.DiffWithProgram(compare.GetProgramName()))
		if err != nil {
			return err
		}
		// 执行对比
		res := resultComparator.Compare(context, compareRiskItem,
			// 对比结果保存到数据库
			ssaapi.WithComparatorSaveResultHandler(func(risks []*ssaapi.ComparisonResult[*schema.SSARisk]) {
				utils.GormTransactionReturnDb(consts.GetGormDefaultSSADataBase(), func(tx *gorm.DB) {
					for _, risk := range risks {
						result := &schema.SSADiffResult{
							BaseLineProgName: base.GetProgramName(),
							CompareProgName:  compare.GetProgramName(),
							RuleName:         risk.FromRule,
							BaseLineRiskHash: risk.BaseValHash,
							CompareRiskHash:  risk.NewValHash,
							Status:           string(risk.Status),
							CompareType:      req.GetType(),
							DiffResultKind:   string(kind),
						}
						if kind == schema.RuntimeId {
							result.BaseLineProgName = base.RiskRuntimeId
							result.CompareProgName = req.Compare.RiskRuntimeId
						}
						tx.Save(result)
					}
				})
			}),
			// 设置回调函数，返回一些信息作为对比的依据
			ssaapi.WithComparatorGetBasisInfo[*schema.SSARisk](func(risk *schema.SSARisk) (
				rule string,
				originHash string,
				diffHash string,
			) {
				// 使用规则、代码片段和变量名作为对比的依据
				return risk.FromRule, risk.Hash, utils.CalcMd5(
					risk.FromRule,
					risk.CodeFragment,
					risk.Variable,
				)
			}),
		)
		for re := range res {
			stream.Send(&ypb.SSARiskDiffResponse{
				BaseRisk:    re.BaseValue.ToGRPCModel(),
				CompareRisk: re.NewValue.ToGRPCModel(),
				RuleName:    re.FromRule,
				Status:      string(re.Status),
			})
		}
		return nil
	default:
		return utils.Error("unknown diff type")
	}
}
