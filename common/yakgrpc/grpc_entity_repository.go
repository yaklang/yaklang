package yakgrpc

import (
	"context"
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
	paging, i, err := yakit.QueryEntitiesPaging(db, req.GetFilter(), req.GetPagination())
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

// QueryRelationship 查询关系
func (s *Server) QueryRelationship(ctx context.Context, req *ypb.QueryRelationshipRequest) (*ypb.QueryRelationshipResponse, error) {
	db := consts.GetGormProfileDatabase()
	paging, i, err := yakit.QueryRelationshipPaging(db, req.GetFilter(), req.GetPagination())
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
