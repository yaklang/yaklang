package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) EvaluateMultiExpression(ctx context.Context, req *ypb.EvaluateMultiExpressionRequest) (*ypb.EvaluateMultiExpressionResponse, error) {
	defaultSandbox := yak.NewSandbox(yak.WithYaklang_Libs(req.GetImportYaklangLibs()))
	var deps map[string]any
	if len(req.GetVariables()) > 0 {
		deps = make(map[string]any)
		for _, item := range req.GetVariables() {
			deps[item.Key] = utils.StringLiteralToAny(item.Value)
		}
	}
	exprs := req.GetExpressions()
	results := make([]*ypb.EvaluateExpressionResponse, 0, len(exprs))

	for _, expr := range exprs {
		value, err := defaultSandbox.ExecuteAsExpressionRaw(expr, deps)
		if err != nil {
			return nil, err
		}
		boolResult := true
		if v, ok := value.(bool); ok {
			boolResult = v
		} else if funk.IsEmpty(value) || funk.IsZero(value) {
			boolResult = false
		}
		results = append(results, &ypb.EvaluateExpressionResponse{
			Result:     utils.InterfaceToJsonString(value),
			BoolResult: boolResult,
		})

	}

	return &ypb.EvaluateMultiExpressionResponse{
		Results: results,
	}, nil
}

func (s *Server) EvaluateExpression(ctx context.Context, req *ypb.EvaluateExpressionRequest) (*ypb.EvaluateExpressionResponse, error) {
	defaultSandbox := yak.NewSandbox(yak.WithYaklang_Libs(req.GetImportYaklangLibs()))
	var deps map[string]any
	if len(req.GetVariables()) > 0 {
		deps = make(map[string]any)
		for _, item := range req.GetVariables() {
			deps[item.Key] = utils.StringLiteralToAny(item.Value)
		}
	}

	value, err := defaultSandbox.ExecuteAsExpressionRaw(req.GetExpression(), deps)
	if err != nil {
		return nil, err
	}

	boolResult := true
	if v, ok := value.(bool); ok {
		boolResult = v
	} else if funk.IsEmpty(value) || funk.IsZero(value) {
		boolResult = false
	}

	return &ypb.EvaluateExpressionResponse{
		Result:     utils.InterfaceToJsonString(value),
		BoolResult: boolResult,
	}, nil
}
