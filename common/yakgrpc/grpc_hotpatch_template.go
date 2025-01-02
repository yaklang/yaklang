package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateHotPatchTemplate(ctx context.Context, req *ypb.HotPatchTemplate) (*ypb.CreateHotPatchTemplateResponse, error) {
	err := yakit.CreateHotPatchTemplate(s.GetProfileDatabase(), req.GetName(), req.GetContent(), req.GetType())
	if err != nil {
		return nil, err
	}
	return &ypb.CreateHotPatchTemplateResponse{
		Message: &ypb.DbOperateMessage{
			TableName: schema.HotPatchTemplateTableName,
			Operation: DbOperationCreate,
		},
	}, nil
}

func (s *Server) DeleteHotPatchTemplate(ctx context.Context, req *ypb.DeleteHotPatchTemplateRequest) (*ypb.DeleteHotPatchTemplateResponse, error) {
	var (
		err          error
		rowsAffected int64
	)
	isAll := req.GetAll()
	if isAll {
		err = yakit.DeleteAllHotPatchTemplate(s.GetProfileDatabase())
	} else {
		condition := req.GetCondition()
		rowsAffected, err = yakit.DeleteHotPatchTemplate(
			s.GetProfileDatabase(),
			yakit.WithHotPatchTemplateIDs(condition.GetId()...),
			yakit.WithHotPatchTemplateNames(condition.GetName()...),
			yakit.WithHotPatchTemplateContentKeyWords(condition.GetContentKeyword()...),
			yakit.WithHotPatchTemplateType(condition.GetType()),
		)
	}
	if err != nil {
		return nil, err
	}
	rsp := &ypb.DeleteHotPatchTemplateResponse{
		Message: &ypb.DbOperateMessage{
			TableName:  schema.HotPatchTemplateTableName,
			Operation:  DbOperationDelete,
			EffectRows: rowsAffected,
		},
	}
	if isAll {
		rsp.Message.ExtraMessage = "delete all hot patch template"
	}
	return rsp, nil

}

func (s *Server) UpdateHotPatchTemplate(ctx context.Context, req *ypb.UpdateHotPatchTemplateRequest) (*ypb.UpdateHotPatchTemplateResponse, error) {
	template := req.GetData()
	condition := req.GetCondition()
	rowAffected, err := yakit.UpdateHotPatchTemplate(
		s.GetProfileDatabase(),
		template.GetName(),
		template.GetContent(),
		template.GetType(),
		yakit.WithHotPatchTemplateIDs(condition.GetId()...),
		yakit.WithHotPatchTemplateNames(condition.GetName()...),
		yakit.WithHotPatchTemplateContentKeyWords(condition.GetContentKeyword()...),
		yakit.WithHotPatchTemplateType(condition.GetType()),
	)

	if err != nil {
		return nil, err
	}
	return &ypb.UpdateHotPatchTemplateResponse{
		Message: &ypb.DbOperateMessage{
			TableName:  schema.HotPatchTemplateTableName,
			Operation:  DbOperationUpdate,
			EffectRows: rowAffected,
		},
	}, nil
}

func (s *Server) QueryHotPatchTemplate(ctx context.Context, req *ypb.HotPatchTemplateRequest) (*ypb.QueryHotPatchTemplateResponse, error) {
	templates, err := yakit.QueryHotPatchTemplate(
		s.GetProfileDatabase(),
		yakit.WithHotPatchTemplateIDs(req.GetId()...),
		yakit.WithHotPatchTemplateNames(req.GetName()...),
		yakit.WithHotPatchTemplateContentKeyWords(req.GetContentKeyword()...),
		yakit.WithHotPatchTemplateType(req.GetType()),
	)
	if err != nil {
		return nil, err
	}
	ypbTemplates := lo.Map(templates, func(t *schema.HotPatchTemplate, _ int) *ypb.HotPatchTemplate {
		return t.ToGRPCModel()
	})

	return &ypb.QueryHotPatchTemplateResponse{
		Message: &ypb.DbOperateMessage{
			TableName: schema.HotPatchTemplateTableName,
			Operation: DbOperationQuery,
		},
		Data: ypbTemplates,
	}, nil
}
