package mitmextractdb

import (
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

const mitmAggregateTraceConcatMaxLen = 4096

// mitmExtractRuleGroupSep separates 「规则组」与「规则名」并写入 rule_verbose，与 Yakit 展示约定一致。
const mitmExtractRuleGroupSep = " / "

// Use bound parameter for separator to avoid manual SQL literal composition.
const mitmExtractRuleGroupInstrExpr = `instr(trim(ed.rule_verbose), ?)`

func mitmExtractRuleGroupSQLExpr() string {
	return `trim(substr(trim(ed.rule_verbose), 1, ` + mitmExtractRuleGroupInstrExpr + ` - 1))`
}

func mitmAggregateReqForDistinctRuleGroups(req *ypb.QueryMITMExtractedAggregateRequest) *ypb.QueryMITMExtractedAggregateRequest {
	if req == nil {
		return &ypb.QueryMITMExtractedAggregateRequest{}
	}
	c := proto.Clone(req).(*ypb.QueryMITMExtractedAggregateRequest)
	c.RuleGroup = ""
	c.OnlyUncategorizedRules = false
	c.RuleVerbose = nil
	c.IncludeDistinctRuleGroups = false
	return c
}

func queryMITMExtractDistinctRuleGroups(db *gorm.DB, req *ypb.QueryMITMExtractedAggregateRequest) ([]string, error) {
	tabReq := mitmAggregateReqForDistinctRuleGroups(req)
	sep := mitmExtractRuleGroupSep
	expr := mitmExtractRuleGroupSQLExpr()
	type row struct {
		RuleGroup string `gorm:"column:rule_group"`
	}
	var rows []row
	q := mitmExtractAggregateBaseDB(db, tabReq).
		Where(mitmExtractRuleGroupInstrExpr+` > 0`, sep).
		Select(expr+` AS rule_group`, sep).
		Group("1").
		Order("1")
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for i := range rows {
		if g := strings.TrimSpace(rows[i].RuleGroup); g != "" {
			out = append(out, g)
		}
	}
	return out, nil
}

type mitmExtractAggregateScan struct {
	RuleVerbose string `gorm:"column:rule_verbose"`
	Data        string `gorm:"column:data"`
	HitCount    int64  `gorm:"column:hit_count"`
	LatestUnix  int64  `gorm:"column:latest_unix"`
	TraceConcat string `gorm:"column:trace_concat"`
}

func mitmExtractAggregateBaseDB(db *gorm.DB, req *ypb.QueryMITMExtractedAggregateRequest) *gorm.DB {
	q := JoinExtractedDataWithHTTPFlow(db)
	if req == nil {
		return q
	}
	if h := strings.TrimSpace(req.GetHostContains()); h != "" {
		q = bizhelper.FuzzSearchEx(q, []string{"hf.host"}, h, false)
	}
	if r := strings.TrimSpace(req.GetRuntimeId()); r != "" {
		q = q.Where("hf.runtime_id = ?", r)
	}
	if rv := req.GetRuleVerbose(); len(rv) > 0 {
		q = q.Where("ed.rule_verbose IN (?)", rv)
	}
	if kw := strings.TrimSpace(req.GetRuleVerboseKeyword()); kw != "" {
		q = bizhelper.FuzzSearchEx(q, []string{"ed.rule_verbose"}, kw, false)
	}
	if req.GetUpdatedAtSince() > 0 {
		q = q.Where("ed.updated_at >= ?", time.Unix(req.GetUpdatedAtSince(), 0))
	}
	if req.GetOnlyUncategorizedRules() {
		q = q.Where(mitmExtractRuleGroupInstrExpr+` = 0`, mitmExtractRuleGroupSep)
	} else if g := strings.TrimSpace(req.GetRuleGroup()); g != "" {
		expr := mitmExtractRuleGroupSQLExpr()
		q = q.Where(mitmExtractRuleGroupInstrExpr+` > 0`, mitmExtractRuleGroupSep).
			Where(expr+` = ?`, mitmExtractRuleGroupSep, g)
	}
	return q
}

// mitmAggregatePagingParams 归一化分页与排序字段（与 grpc 默认一致）。
func mitmAggregatePagingParams(req *ypb.QueryMITMExtractedAggregateRequest) (page, limit, offset int, orderCol, orderDir string) {
	params := req.GetPagination()
	if params == nil {
		params = &ypb.Paging{Page: 1, Limit: 30, OrderBy: "hit_count", Order: "desc"}
	}
	page = int(params.GetPage())
	if page < 1 {
		page = 1
	}
	limit = int(params.GetLimit())
	if limit <= 0 {
		limit = 30
	}
	offset = (page - 1) * limit

	orderCol = "hit_count"
	switch strings.ToLower(strings.TrimSpace(params.GetOrderBy())) {
	case "latest_updated_at", "latest_unix":
		orderCol = "latest_unix"
	case "hit_count", "":
		orderCol = "hit_count"
	default:
		orderCol = "hit_count"
	}
	orderDir = "DESC"
	if strings.EqualFold(strings.TrimSpace(params.GetOrder()), "asc") {
		orderDir = "ASC"
	}
	return page, limit, offset, orderCol, orderDir
}

func splitTraceConcat(s string, maxN int) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, id := range strings.Split(s, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) >= maxN {
			break
		}
	}
	return out
}

// QueryMITMExtractedAggregate groups extracted_data by (rule_verbose, data) with http_flows join.
// When req.IncludeDistinctRuleGroups is set, the third return value lists distinct rule group names (same scope as tabs: Host/Runtime/Keyword/Time only).
func QueryMITMExtractedAggregate(db *gorm.DB, req *ypb.QueryMITMExtractedAggregateRequest) (*bizhelper.Paginator, []*ypb.MITMExtractedAggregateRow, []string, error) {
	if req == nil {
		req = &ypb.QueryMITMExtractedAggregateRequest{}
	}
	_, limit, offset, orderCol, orderDir := mitmAggregatePagingParams(req)

	keySub := mitmExtractAggregateBaseDB(db, req).
		Select("ed.rule_verbose, ed.data").
		Group("ed.rule_verbose, ed.data")

	var total int64
	if err := db.Raw("SELECT COUNT(*) FROM (?) AS _mitm_agg", keySub.QueryExpr()).Row().Scan(&total); err != nil {
		return nil, nil, nil, utils.Errorf("aggregate count: %v", err)
	}

	dataSub := mitmExtractAggregateBaseDB(db, req).
		Select(`ed.rule_verbose AS rule_verbose, ed.data AS data, COUNT(*) AS hit_count,
			CAST(strftime('%s', MAX(ed.updated_at)) AS INTEGER) AS latest_unix,
			SUBSTR(GROUP_CONCAT(DISTINCT ed.trace_id), 1, ?) AS trace_concat`, mitmAggregateTraceConcatMaxLen).
		Group("ed.rule_verbose, ed.data")

	dataSub = dataSub.Order(orderCol + " " + orderDir).Offset(offset).Limit(limit)

	var scans []mitmExtractAggregateScan
	if err := dataSub.Scan(&scans).Error; err != nil {
		return nil, nil, nil, utils.Errorf("aggregate query: %v", err)
	}

	out := make([]*ypb.MITMExtractedAggregateRow, 0, len(scans))
	for i := range scans {
		row := &scans[i]
		out = append(out, &ypb.MITMExtractedAggregateRow{
			RuleVerbose:     row.RuleVerbose,
			DisplayData:     row.Data,
			HitCount:        row.HitCount,
			LatestUpdatedAt: row.LatestUnix,
			SampleTraceIds:  splitTraceConcat(row.TraceConcat, 20),
		})
	}

	var distinctGroups []string
	if req.GetIncludeDistinctRuleGroups() {
		groups, err := queryMITMExtractDistinctRuleGroups(db, req)
		if err != nil {
			return nil, nil, nil, utils.Errorf("distinct rule groups: %v", err)
		}
		distinctGroups = groups
	}

	return &bizhelper.Paginator{TotalRecord: int(total)}, out, distinctGroups, nil
}
