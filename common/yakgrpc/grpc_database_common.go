package yakgrpc

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	DatabaseNameProject = "Project"
	DatabaseNameProfile = "Profile"
	DatabaseNameSSA     = "SSA"
)

func (s *Server) GroupTableColumn(ctx context.Context, req *ypb.GroupTableColumnRequest) (*ypb.GroupTableColumnResponse, error) {
	var db *gorm.DB
	switch req.DatabaseName {
	case DatabaseNameProfile:
		db = s.GetProfileDatabase()
	case DatabaseNameSSA:
		db = s.GetSSADatabase()
	case DatabaseNameProject:
		db = s.GetProjectDatabase()
	default:
		return nil, utils.Error("database name not found")
	}
	data, err := bizhelper.GroupColumn(db, req.TableName, req.ColumnName)
	if err != nil {
		return nil, err
	}
	return &ypb.GroupTableColumnResponse{Data: lo.Map(data, func(item any, _ int) string {
		return utils.InterfaceToString(item)
	})}, nil
}
