package yakgrpc

import (
	"context"
	"errors"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ListEntityRepository 列出所有实体仓库
func (s *Server) ListEntityRepository(ctx context.Context, req *ypb.Empty) (*ypb.ListEntityRepositoryResponse, error) {
	db := consts.GetGormProfileDatabase()
	var repos []*schema.EntityRepository
	if err := db.Find(&repos).Error; err != nil {
		return nil, err
	}

	return &ypb.ListEntityRepositoryResponse{
		EntityRepositories: lo.Map(repos, func(repo *schema.EntityRepository, _ int) *ypb.EntityRepository {
			return repo.ToGRPC()
		}),
	}, nil
}

// QueryEntity 查询实体
func (s *Server) QueryEntity(ctx context.Context, req *ypb.QueryEntityRequest) (*ypb.QueryEntityResponse, error) {
	db := consts.GetGormProfileDatabase()
	pagination := req.GetPagination()
	if pagination == nil {
		pagination = &ypb.Paging{
			Page:    1,
			Limit:   20,
			OrderBy: "id",
		}
	}
	paging, i, err := yakit.QueryEntitiesPaging(db, req.GetFilter(), pagination)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryEntityResponse{
		Entities: lo.Map(i, func(e *schema.ERModelEntity, _ int) *ypb.Entity {
			return e.ToGRPC()
		}),
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.GetPagination().GetOrderBy(),
			Order:   req.GetPagination().GetOrder(),
		},
		Total: uint64(paging.TotalRecord),
	}, nil
}

func (s *Server) DeleteEntity(ctx context.Context, req *ypb.DeleteEntityRequest) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	affectRaw, err := yakit.DeleteEntities(db, req.GetFilter())
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		Operation:  DbOperationDelete,
		EffectRows: affectRaw,
	}, nil
}

// QueryRelationship 查询关系
func (s *Server) QueryRelationship(ctx context.Context, req *ypb.QueryRelationshipRequest) (*ypb.QueryRelationshipResponse, error) {
	db := consts.GetGormProfileDatabase()
	pagination := req.GetPagination()
	if pagination == nil {
		pagination = &ypb.Paging{
			Page:    1,
			Limit:   20,
			OrderBy: "id",
		}
	}

	paging, i, err := yakit.QueryRelationshipPaging(db, req.GetFilter(), pagination)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryRelationshipResponse{
		Relationships: lo.Map(i, func(e *schema.ERModelRelationship, _ int) *ypb.Relationship {
			return e.ToGRPC()
		}),
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.GetPagination().GetOrderBy(),
			Order:   req.GetPagination().GetOrder(),
		},
		Total: uint64(paging.TotalRecord),
	}, nil
}

// GenerateERMDot 生成 ER 图 DOT 格式
func (s *Server) GenerateERMDot(ctx context.Context, req *ypb.GenerateERMDotRequest) (*ypb.GenerateERMDotResponse, error) {
	db := consts.GetGormProfileDatabase()
	ERM, err := yakit.QueryEntityWithDepth(db, req.GetFilter(), int(req.GetDepth()))
	if err != nil {
		return nil, err
	}

	return &ypb.GenerateERMDotResponse{
		Dot: ERM.Dot().GenerateDOTString(),
	}, nil
}

func (s *Server) QuerySubERM(ctx context.Context, req *ypb.QuerySubERMRequest) (*ypb.QuerySubERMResponse, error) {
	db := consts.GetGormProfileDatabase()
	ERM, err := yakit.QueryEntityWithDepth(db, req.GetFilter(), int(req.GetDepth()))
	if err != nil {
		return nil, err
	}
	return &ypb.QuerySubERMResponse{
		Relationships: lo.Map(ERM.Relationships, func(r *schema.ERModelRelationship, _ int) *ypb.Relationship {
			return r.ToGRPC()
		}),
		Entities: lo.Map(ERM.Entities, func(e *schema.ERModelEntity, _ int) *ypb.Entity {
			return e.ToGRPC()
		}),
	}, nil
}

// UpdateEntity 更新或创建实体（根据 yakit 实现）
func (s *Server) UpdateEntity(ctx context.Context, req *ypb.Entity) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()

	if req.ID > 0 {
		err := yakit.UpdateEntity(db, uint(req.GetID()), schema.EntityGRPCToModel(req))
		if err != nil {
			return nil, err
		}
	} else {
		err := yakit.UpdateEntityByUUID(db, req.GetHiddenIndex(), schema.EntityGRPCToModel(req))
		if err != nil {
			return nil, err
		}
	}

	return &ypb.DbOperateMessage{
		Operation:  DbOperationUpdate,
		EffectRows: 1,
	}, nil
}

// CreateEntity 创建实体，如果 req.ID 提供则会尝试更新
func (s *Server) CreateEntity(ctx context.Context, req *ypb.Entity) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	model := schema.EntityGRPCToModel(req)

	// If client provided ID, treat as update
	if req.GetID() > 0 {
		if err := yakit.UpdateEntity(db, uint(req.GetID()), model); err != nil {
			return nil, err
		}
		return &ypb.DbOperateMessage{
			Operation:  DbOperationUpdate,
			EffectRows: 1,
			CreateID:   int64(req.GetID()),
		}, nil
	}
	// create new entity
	err := yakit.CreateEntity(db, model)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		Operation:  DbOperationCreate,
		EffectRows: 1,
		CreateID:   int64(model.ID),
	}, nil
}

// CreateRelationship 创建关系，如果 req.ID 提供则会尝试更新
func (s *Server) CreateRelationship(ctx context.Context, req *ypb.Relationship) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	model := schema.RelationshipGRPCToModel(req)

	// If client provided ID, treat as update
	if req.GetID() > 0 {
		if err := yakit.UpdateRelationship(db, uint(req.GetID()), model); err != nil {
			return nil, err
		}
		return &ypb.DbOperateMessage{
			Operation:  DbOperationUpdate,
			EffectRows: 1,
			CreateID:   int64(req.GetID()),
		}, nil
	}

	// create new relationship
	err := yakit.CreateRelationship(db, model)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		Operation:  DbOperationCreate,
		EffectRows: 1,
		CreateID:   int64(model.ID),
	}, nil
}

// UpdateRelationship 更新或创建关系（根据 yakit 实现）
// 使用 profile 数据库，调用 yakit.UpdateRelationshipByUUID 来执行具体的 DB 操作
func (s *Server) UpdateRelationship(ctx context.Context, req *ypb.Relationship) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	model := schema.RelationshipGRPCToModel(req)

	if req.ID > 0 {
		if err := yakit.UpdateRelationship(db, uint(req.GetID()), model); err != nil {
			return nil, err
		}
	} else if req.GetUUID() != "" {
		if err := yakit.UpdateRelationshipByUUID(db, req.GetUUID(), model); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("missing relationship identifier (ID or UUID)")
	}

	return &ypb.DbOperateMessage{
		Operation:  DbOperationUpdate,
		EffectRows: 1,
	}, nil
}

// DeleteRelationship 删除关系
func (s *Server) DeleteRelationship(ctx context.Context, req *ypb.DeleteRelationshipRequest) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	err := yakit.DeleteRelationships(db, req.GetFilter())
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		Operation: DbOperationDelete,
	}, nil
}
