package yakgrpc

import (
	"context"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func queryResourcePaging(paginator *bizhelper.Paginator, request *ypb.Paging) *ypb.Paging {
	if paginator == nil {
		return nil
	}
	return &ypb.Paging{
		Page:    int64(paginator.Page),
		Limit:   int64(paginator.Limit),
		OrderBy: request.GetOrderBy(),
		Order:   request.GetOrder(),
	}
}

func (s *Server) QueryAITool(ctx context.Context, req *ypb.QueryAIToolRequest) (*ypb.QueryAIToolResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		req = &ypb.QueryAIToolRequest{}
	}
	db := s.GetProfileDatabase()
	if db == nil {
		return nil, utils.Error("database not initialized")
	}

	paginator, tools, err := yakit.QueryAIYakTools(db, req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	data := make([]*ypb.AITool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		data = append(data, tool.ToGRPC())
	}

	return &ypb.QueryAIToolResponse{
		Data:       data,
		Pagination: queryResourcePaging(paginator, req.GetPagination()),
		Total:      int64(paginator.TotalRecord),
	}, nil
}

func knowledgeBaseInfoToGRPC(db *gorm.DB, kb *schema.KnowledgeBaseInfo) *ypb.KnowledgeBaseInfo {
	if kb == nil {
		return nil
	}

	isImported := kb.SerialVersionUID != ""
	if !isImported {
		collectionInfo, _ := yakit.GetRAGCollectionInfoByName(db, kb.KnowledgeBaseName)
		isImported = collectionInfo != nil && collectionInfo.SerialVersionUID != ""
	}

	return &ypb.KnowledgeBaseInfo{
		ID:                       int64(kb.ID),
		KnowledgeBaseName:        kb.KnowledgeBaseName,
		KnowledgeBaseDescription: kb.KnowledgeBaseDescription,
		KnowledgeBaseType:        kb.KnowledgeBaseType,
		Tags:                     utils.StringSplitAndStrip(kb.Tags, ","),
		IsImported:               isImported,
		CreatedFromUI:            kb.CreatedFromUI,
		IsDefault:                kb.IsDefault,
		SerialVersionID:          kb.SerialVersionUID,
	}
}

func (s *Server) QueryKnowledgeBase(ctx context.Context, req *ypb.QueryKnowledgeBaseRequest) (*ypb.QueryKnowledgeBaseResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		req = &ypb.QueryKnowledgeBaseRequest{}
	}
	db := s.GetProfileDatabase()
	if db == nil {
		return nil, utils.Error("database not initialized")
	}

	paginator, knowledgeBases, err := yakit.QueryKnowledgeBaseByFilter(db, req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	data := make([]*ypb.KnowledgeBaseInfo, 0, len(knowledgeBases))
	for _, kb := range knowledgeBases {
		if item := knowledgeBaseInfoToGRPC(db, kb); item != nil {
			data = append(data, item)
		}
	}

	return &ypb.QueryKnowledgeBaseResponse{
		Data:       data,
		Pagination: queryResourcePaging(paginator, req.GetPagination()),
		Total:      int64(paginator.TotalRecord),
	}, nil
}
