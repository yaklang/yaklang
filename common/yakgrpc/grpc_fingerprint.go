package yakgrpc

import (
	"context"
	"github.com/samber/lo"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryFingerprint(ctx context.Context, req *ypb.QueryFingerprintRequest) (*ypb.QueryFingerprintResponse, error) {
	paging, data, err := yakit.QueryGeneralRule(s.GetProfileDatabase(), req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	start := time.Now()
	var res []*ypb.FingerprintRule
	for _, r := range data {
		m := r.ToGRPCModel()
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
	var effectCount int64

	rule := req.GetRule()
	if req.GetId() > 0 {
		rule.Id = req.GetId()
		effectCount, err = yakit.UpdateGeneralRule(s.GetProfileDatabase(), schema.GRPCGeneralRuleToSchemaGeneralRule(rule))
	} else if req.GetRuleName() != "" {
		effectCount, err = yakit.UpdateGeneralRuleByRuleName(s.GetProfileDatabase(), req.GetRuleName(), schema.GRPCGeneralRuleToSchemaGeneralRule(rule))
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
		EffectRows: effectCount,
	}
	return updateMessage, nil
}

func (s *Server) CreateFingerprint(ctx context.Context, req *ypb.CreateFingerprintRequest) (*ypb.DbOperateMessage, error) {
	rule := req.GetRule()
	if rule == nil {
		return nil, utils.Errorf("rule is nil")
	}
	err := yakit.CreateGeneralRule(s.GetProfileDatabase(), schema.GRPCGeneralRuleToSchemaGeneralRule(rule))
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

func (s *Server) CreateFingerprintGroup(ctx context.Context, req *ypb.FingerprintGroup) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	err := yakit.CreateGeneralRuleGroup(db, schema.GRPCFingerprintGroupToSchemaGeneralRuleGroup(req))
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "general_rule_group",
		Operation:  "create",
		EffectRows: 1,
	}, nil
}

func (s *Server) GetAllFingerprintGroup(ctx context.Context, req *ypb.Empty) (*ypb.FingerprintGroups, error) {
	db := s.GetProfileDatabase()
	group, err := yakit.GetAllGeneralRuleGroup(db)
	if err != nil {
		return nil, err
	}
	return &ypb.FingerprintGroups{
		Data: lo.Map(group, func(g *schema.GeneralRuleGroup, _ int) *ypb.FingerprintGroup {
			return g.ToGRPCModel()
		}),
	}, nil
}

func (s *Server) RenameFingerprintGroup(ctx context.Context, req *ypb.RenameFingerprintGroupRequest) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	err := yakit.RenameGeneralRuleGroupName(db, req.GetGroupName(), req.GetNewGroupName())
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "general_rule_group",
		Operation:  "update",
		EffectRows: 1,
	}, nil
}

func (s *Server) DeleteFingerprintGroup(ctx context.Context, req *ypb.DeleteFingerprintGroupRequest) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	effectRow, err := yakit.DeleteGeneralRuleGroupByName(db, req.GetGroupNames())
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "general_rule_group",
		Operation:  "delete",
		EffectRows: effectRow,
	}, nil
}

func (s *Server) BatchAppendFingerprintToGroup(ctx context.Context, req *ypb.BatchAppendFingerprintToGroupRequest) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	effectRow, err := yakit.AppendMultipleGeneralRuleToGroup(db, req.GetFilter(), req.GetGroupName())
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "general_rule_group",
		Operation:  "delete",
		EffectRows: effectRow,
	}, nil
}
