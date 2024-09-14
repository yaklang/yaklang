package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func (s *Server) QueryFingerprint(ctx context.Context, req *ypb.QueryFingerprintRequest) (*ypb.QueryFingerprintResponse, error) {
	paging, data, err := yakit.QueryGeneralRule(s.GetProfileDatabase(), req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	start := time.Now()
	var res []*ypb.FingerprintRule
	for _, r := range data {
		m := yakit.SchemaGeneralRuleToGRPCGeneralRule(r)
		if m == nil {
			log.Errorf("failed to convert schema.GeneralRule to ypb.FingerprintRule: %v", r)
		} else {
			res = append(res, m)
		}
	}
	cost := time.Now().Sub(start)
	if cost.Milliseconds() > 200 {
		log.Infof("finished converting httpflow(%v) cost: %s", len(res), cost)
	}

	return &ypb.QueryFingerprintResponse{
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.GetPagination().GetOrderBy(),
			Order:   req.GetPagination().GetOrder(),
		},
		Total: int64(paging.TotalRecord),
		Data:  res,
	}, nil
}

func (s *Server) DeleteFingerprint(ctx context.Context, req *ypb.DeleteFingerprintRequest) (*ypb.DbOperateMessage, error) {
	count, err := yakit.DeleteGeneralRuleByFilter(s.GetProfileDatabase(), req.GetFilter())
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "general_rule",
		Operation:  "delete",
		EffectRows: count,
	}, nil
}

func (s *Server) UpdateFingerprint(ctx context.Context, req *ypb.UpdateFingerprintRequest) (*ypb.DbOperateMessage, error) {
	var err error
	var effectCount int

	rule := req.GetRule()
	if req.GetId() > 0 {
		rule.Id = req.GetId()
		effectCount, err = yakit.UpdateGeneralRule(s.GetProfileDatabase(), yakit.GRPCGeneralRuleToSchemaGeneralRule(rule))
	} else if req.GetRuleName() != "" {
		effectCount, err = yakit.UpdateGeneralRuleByRuleName(s.GetProfileDatabase(), req.GetRuleName(), yakit.GRPCGeneralRuleToSchemaGeneralRule(rule))
	} else {
		return nil, utils.Errorf("id or rule_name must be set at least one")
	}

	if err != nil {
		return nil, err
	}

	if effectCount == 0 {
		return nil, utils.Errorf("no record updated, not found id(%d) or rule_name(%s)", req.GetId(), req.GetRuleName())
	}
	updateMessage := &ypb.DbOperateMessage{
		TableName:  "general_rule",
		Operation:  "update",
		EffectRows: int64(effectCount),
	}
	return updateMessage, nil
}

func (s *Server) CreateFingerprint(ctx context.Context, req *ypb.CreateFingerprintRequest) (*ypb.DbOperateMessage, error) {
	rule := req.GetRule()
	if rule == nil {
		return nil, utils.Errorf("rule is nil")
	}
	err := yakit.CreateGeneralRule(s.GetProfileDatabase(), yakit.GRPCGeneralRuleToSchemaGeneralRule(rule))
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "general_rule",
		Operation:  "create",
		EffectRows: 1,
	}, nil
}

func (s *Server) RecoverBuiltinFingerprint(ctx context.Context, _ *ypb.Empty) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	yakit.ClearGeneralRule(db)
	err := yakit.InsertBuiltinGeneralRules(db)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:    "general_rule",
		Operation:    "recover_builtin",
		ExtraMessage: "recover builtin general rule success",
	}, nil
}
