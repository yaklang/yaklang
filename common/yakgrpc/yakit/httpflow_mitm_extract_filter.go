package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

// HTTPFlowRequestHasNonEmptyFilter is true when r has any filter field other than pagination.
func HTTPFlowRequestHasNonEmptyFilter(r *ypb.QueryHTTPFlowRequest) bool {
	if r == nil {
		return false
	}
	c := proto.Clone(r).(*ypb.QueryHTTPFlowRequest)
	c.Pagination = nil
	return !proto.Equal(c, &ypb.QueryHTTPFlowRequest{})
}

// filterHTTPFlowByMITMExtractAggregateRows keeps flows that have extracted_data matching
// any row (OR on rule_verbose; data optional, exact match when set).
func filterHTTPFlowByMITMExtractAggregateRows(db *gorm.DB, rows []*ypb.MITMExtractAggregateFlowFilterRow) *gorm.DB {
	if len(rows) == 0 {
		return db
	}
	var ors []string
	var args []interface{}
	for _, row := range rows {
		rv := strings.TrimSpace(row.GetRuleVerbose())
		if rv == "" {
			continue
		}
		if dd := strings.TrimSpace(row.GetDisplayData()); dd == "" {
			ors = append(ors, "ed.rule_verbose = ?")
			args = append(args, rv)
		} else {
			ors = append(ors, "(ed.rule_verbose = ? AND ed.data = ?)")
			args = append(args, rv, dd)
		}
	}
	if len(ors) == 0 {
		return db
	}
	edTable := db.NewScope(&schema.ExtractedData{}).TableName()
	hfTable := db.NewScope(&schema.HTTPFlow{}).TableName()
	cond := strings.Join(ors, " OR ")
	sub := db.New().Table(hfTable+" AS hf").
		Select("DISTINCT hf.id").
		Joins("INNER JOIN "+edTable+" AS ed ON ed.trace_id = hf.hidden_index").
		Where("ed.trace_id != ?", "").
		Where("hf.hidden_index != ?", "").
		Where(cond, args...)
	return db.Where("id IN (?)", sub.QueryExpr())
}
