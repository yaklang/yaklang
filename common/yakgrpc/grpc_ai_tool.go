package yakgrpc

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetAIToolList(ctx context.Context, req *ypb.GetAIToolListRequest) (*ypb.GetAIToolListResponse, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	var (
		pagination *bizhelper.Paginator
		tools      []*schema.AIYakTool
		err        error
	)

	// Setup pagination parameters
	page := int(req.GetPagination().GetPage())
	limit := int(req.GetPagination().GetLimit())
	orderBy := req.GetPagination().GetOrderBy()
	order := req.GetPagination().GetOrder()

	// If ToolName is provided, search by exact name
	if req.GetToolName() != "" {
		tool, err := schema.GetAIYakTool(db, req.GetToolName())
		if err != nil {
			return &ypb.GetAIToolListResponse{
				Tools: []*ypb.AITool{},
			}, nil
		}

		// Convert single tool to response format
		return &ypb.GetAIToolListResponse{
			Tools: []*ypb.AITool{
				{
					Name:        tool.Name,
					Description: tool.Description,
					Content:     tool.Content,
					ToolPath:    tool.Path,
					Keywords:    strings.Split(tool.Keywords, ","),
				},
			},
		}, nil
	}

	// Otherwise use Query for fuzzy search with pagination
	pagination, tools, err = schema.SearchAIYakToolWithPagination(db, req.GetQuery(), page, limit, orderBy, order)
	if err != nil {
		log.Errorf("failed to search AI tools: %s", err)
		return &ypb.GetAIToolListResponse{
			Tools: []*ypb.AITool{},
		}, nil
	}

	// Convert tools to response format
	var result []*ypb.AITool
	for _, tool := range tools {
		result = append(result, &ypb.AITool{
			Name:        tool.Name,
			Description: tool.Description,
			Content:     tool.Content,
			ToolPath:    tool.Path,
			Keywords:    strings.Split(tool.Keywords, ","),
		})
	}

	// Prepare response with pagination info
	return &ypb.GetAIToolListResponse{
		Tools: result,
		Pagination: &ypb.Paging{
			Page:    int64(pagination.Page),
			Limit:   int64(pagination.Limit),
			OrderBy: orderBy,
			Order:   order,
		},
	}, nil
}
