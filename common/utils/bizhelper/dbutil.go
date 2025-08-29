package bizhelper

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	escapeRegexp = regexp.MustCompile(`[%_\[\]^\\]`)
)

func createTempTestDatabase() (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		return nil, err
	}
	return db, nil
}

type Range struct {
	Min, Max uint64
}

func QueryBySpecificPorts(db *gorm.DB, field string, ports string) *gorm.DB {
	if ports == "" {
		return db
	}

	var query []string
	var items []interface{}

	for _, raw := range strings.Split(ports, ",") {
		portRaw := strings.TrimSpace(raw)
		if portRaw == "" {
			continue
		}

		if strings.Contains(portRaw, "-") {
			result := strings.SplitN(portRaw, "-", 2)
			if len(result) < 2 {
				continue
			}
			startR := strings.TrimSpace(result[0])
			endR := strings.TrimSpace(result[1])

			start, err := strconv.ParseInt(startR, 10, 64)
			if err != nil {
				continue
			}

			end, err := strconv.ParseInt(endR, 10, 64)
			if err != nil {
				continue
			}

			if start > end {
				continue
			}

			query = append(query, fmt.Sprintf("(%v >= ? AND %v <= ?)", field, field))
			items = append(items, start, end)
		} else {
			p, err := strconv.ParseInt(portRaw, 10, 64)
			if err != nil {
				continue
			}

			query = append(query, fmt.Sprintf("(%v = ?)", field))
			items = append(items, p)
		}
	}

	if len(query) > 0 {
		db = db.Where(strings.Join(query, " OR "), items...)
	}

	return db
}

func QueryBySpecificAddressP(db *gorm.DB, field string, targets *string) *gorm.DB {
	if targets != nil {
		return QueryBySpecificAddress(db, field, *targets)
	}
	return db
}

func QueryBySpecificAddress(db *gorm.DB, field string, targets string) *gorm.DB {
	if targets == "" {
		return db
	}

	var query []string
	var items []interface{}

	for _, raw := range strings.Split(targets, ",") {
		netBlock := strings.TrimSpace(raw)

		log.Debugf("current filter netblock: %s", netBlock)

		if strings.Contains(netBlock, "/") {
			_, netBlock, err := net.ParseCIDR(netBlock)
			if err != nil {
				continue
			}

			start, end, err := utils.ParseIPNetToRange(netBlock)
			if err != nil {
				continue
			}

			query = append(query, fmt.Sprintf("(%v >= ? AND %v <= ?)", field, field))
			items = append(items, start, end)
		} else {
			ip := net.ParseIP(netBlock)
			if ip == nil {
				continue
			}
			ipInt, err := utils.IPv4ToUint32(ip.To4())
			if err != nil {
				continue
			}

			query = append(query, fmt.Sprintf("(%v = ?)", field))
			items = append(items, ipInt)
		}
	}

	if len(query) > 0 {
		db = db.Where(strings.Join(query, " OR "), items...)
	}
	return db
}

func FuzzQueryArrayOr(db *gorm.DB, field string, s []interface{}) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, sub := range s {
		querys = append(querys, fmt.Sprintf("( %v LIKE ? )", field))
		items = append(items, fmt.Sprintf("%%%v%%", sub))
	}

	return db.Where(strings.Join(querys, " OR "), items...)
}

func FuzzQueryArrayStringOrLike(db *gorm.DB, field string, s []string) *gorm.DB {
	s = utils.StringArrayFilterEmpty(s)
	if len(s) <= 0 {
		return db
	}

	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return FuzzQueryArrayOrLike(db, field, raw, false)
}

// FuzzQueryArrayOrLike
func FuzzQueryArrayOrLike(db *gorm.DB, field string, s []interface{}, escape bool) *gorm.DB {
	return FuzzQueryOrEx(db, []string{field}, s, escape)
}

// FuzzQueryArrayOrLike
func FuzzQueryOrEx(db *gorm.DB, fields []string, s []interface{}, escape bool) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, field := range fields {
		for _, sub := range s {
			pattern := fmt.Sprintf("%v", sub) // 将 'sub' 转换为类似于 '%sub%' 的形式
			if escape {
				pattern = escapeRegexp.ReplaceAllString(pattern, `\$0`) // 转义特殊字符
			} else {
				pattern = strings.ReplaceAll(pattern, "*", "%") // 将 '*' 替换为 SQL 通配符 '%'
			}
			pattern = fmt.Sprintf("%%%s%%", pattern)
			if escape {
				querys = append(querys, fmt.Sprintf(`( %v LIKE ? ESCAPE '\' )`, field))
			} else {
				querys = append(querys, fmt.Sprintf("( %v LIKE ? )", field))
			}

			items = append(items, pattern)
		}
	}

	return db.Where(strings.Join(querys, " OR "), items...) //.Debug()
}

// FuzzQueryArrayOrLike
func FuzzQueryArrayOrLikeExclude(db *gorm.DB, field string, s []interface{}) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, sub := range s {
		pattern := fmt.Sprintf("%%%v%%", sub)           // 将 'sub' 转换为类似于 '%sub%' 的形式
		pattern = strings.ReplaceAll(pattern, "*", "%") // 将 '*' 替换为 SQL 通配符 '%'
		querys = append(querys, fmt.Sprintf("( %v LIKE ? )", field))
		items = append(items, pattern)
	}

	return db.Where(`(not (`+strings.Join(querys, " OR ")+`))`, items...)
}

func FuzzQueryArrayOrPrefixLike(db *gorm.DB, field string, s []interface{}) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, sub := range s {
		querys = append(querys, fmt.Sprintf("( %v LIKE ? )", field))
		items = append(items, fmt.Sprintf("%v%%", sub))
	}

	return db.Where(strings.Join(querys, " OR "), items...)
}

func FuzzQueryIntArrayOr(db *gorm.DB, field string, s []int) *gorm.DB {
	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return FuzzQueryArrayOr(db, field, raw)
}

func FuzzQueryInt64ArrayOr(db *gorm.DB, field string, s []int64) *gorm.DB {
	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return FuzzQueryArrayOr(db, field, raw)
}

func FuzzQueryStringArrayOr(db *gorm.DB, field string, s []string) *gorm.DB {
	s = utils.StringArrayFilterEmpty(s)
	if len(s) <= 0 {
		return db
	}

	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return FuzzQueryArrayOr(db, field, raw)
}

func FuzzQueryStringArrayOrLike(db *gorm.DB, field string, s []string) *gorm.DB {
	s = utils.StringArrayFilterEmpty(s)
	if len(s) <= 0 {
		return db
	}

	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return FuzzQueryArrayOrLike(db, field, raw, false)
}

func FuzzQueryStringArrayOrLikeExclude(db *gorm.DB, field string, s []string) *gorm.DB {
	s = utils.StringArrayFilterEmpty(s)
	if len(s) <= 0 {
		return db
	}

	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return FuzzQueryArrayOrLikeExclude(db, field, raw)
}

func FuzzQueryStringArrayOrPrefixLike(db *gorm.DB, field string, s []string) *gorm.DB {
	s = utils.StringArrayFilterEmpty(s)
	if len(s) <= 0 {
		return db
	}

	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	// return FuzzQueryArrayOrLike(db, field, raw)
	return FuzzQueryArrayOrPrefixLike(db, field, raw)
}

func FuzzQueryStringByFieldsOr(db *gorm.DB, fields []string, keyword string) *gorm.DB {
	fields = utils.StringArrayFilterEmpty(fields)
	if len(fields) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, field := range fields {
		querys = append(querys, fmt.Sprintf("( %v LIKE ? )", field))
		items = append(items, fmt.Sprintf("%%%v%%", keyword))
	}

	return db.Where(strings.Join(querys, " OR "), items...)
}

func FuzzQueryStringByFieldsOrP(db *gorm.DB, fields []string, keyword *string) *gorm.DB {
	if keyword == nil {
		return db
	}
	return FuzzQueryStringByFieldsOr(db, fields, *keyword)
}

func ExactQueryArrayOr(db *gorm.DB, field string, s []interface{}) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, sub := range s {
		querys = append(querys, fmt.Sprintf("( %v = ? )", field))
		items = append(items, sub)
	}

	return db.Where(strings.Join(querys, " OR "), items...)
}

func ExactOrQueryArrayOr(db *gorm.DB, field string, s []interface{}) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, sub := range s {
		querys = append(querys, fmt.Sprintf("( %v = ? )", field))
		items = append(items, sub)
	}

	return db.Where(strings.Join(querys, " OR "), items...)
}

func ExactQueryExcludeArrayOr(db *gorm.DB, field string, s []interface{}) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, sub := range s {
		querys = append(querys, fmt.Sprintf("( %v != ? )", field))
		items = append(items, sub)
	}

	return db.Where(strings.Join(querys, " AND "), items...)
}

func ExactQueryUint64ArrayOr(db *gorm.DB, field string, s []uint64) *gorm.DB {
	raw := make([]int64, len(s))
	for index, sub := range s {
		raw[index] = int64(sub)
	}
	return ExactQueryInt64ArrayOr(db, field, raw)
}

func ExactQueryIntArrayOr(db *gorm.DB, field string, s []int) *gorm.DB {
	raw := make([]uint64, len(s))
	for index, sub := range s {
		raw[index] = uint64(sub)
	}
	return ExactQueryUInt64ArrayOr(db, field, raw)
}

func ExactExcludeQueryInt64Array(db *gorm.DB, field string, s []int64) *gorm.DB {
	raw := make([]uint64, len(s))
	for index, sub := range s {
		raw[index] = uint64(sub)
	}
	return ExactExcludeQueryUInt64Array(db, field, raw)
}

func ExactExcludeQueryUInt64Array(db *gorm.DB, field string, s []uint64) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	sort.Slice(s, func(i, j int) bool {
		return s[i] < s[j]
	})

	ranges := make([]Range, 0)
	querys := make([]string, 0)
	items := make([]any, 0)

	for _, val := range s {
		if len(ranges) == 0 || val-1 > ranges[len(ranges)-1].Max {
			ranges = append(ranges, Range{
				Min: val,
				Max: val,
			})
		} else {
			ranges[len(ranges)-1].Max = val
		}
	}

	for _, r := range ranges {
		if r.Min == r.Max {
			querys = append(querys, fmt.Sprintf("(%v <> ?)", field))
			items = append(items, r.Min)
		} else {
			querys = append(querys, fmt.Sprintf("(%v < ? OR %v > ?)", field, field))
			items = append(items, r.Min, r.Max)
		}
	}

	return db.Where(strings.Join(querys, " AND "), items...)
}

func ExactQueryInt64ArrayOr(db *gorm.DB, field string, s []int64) *gorm.DB {
	raw := make([]uint64, len(s))
	for index, sub := range s {
		raw[index] = uint64(sub)
	}
	return ExactQueryUInt64ArrayOr(db, field, raw)
}

func ExactQueryUIntArrayOr(db *gorm.DB, field string, s []uint) *gorm.DB {
	raw := make([]uint64, len(s))
	for index, sub := range s {
		raw[index] = uint64(sub)
	}
	return ExactQueryUInt64ArrayOr(db, field, raw)
}

func ExactQueryMultipleUInt64ArrayOr(db *gorm.DB, field []string, s []uint64) *gorm.DB {
	if len(s) <= 0 || len(field) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, f := range field {
		querys = append(querys, fmt.Sprintf("( %v IN (?) )", f))
		items = append(items, s)
	}

	return db.Where(strings.Join(querys, " OR "), items...)
}

func ExactQueryUInt64ArrayOr(db *gorm.DB, field string, s []uint64) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	sort.Slice(s, func(i, j int) bool {
		return s[i] < s[j]
	})

	ranges := make([]Range, 0)
	querys := make([]string, 0)
	items := make([]any, 0)
	//如果where数量大于100，使用in查询
	if len(s) > 100 {
		return db.Where(fmt.Sprintf("%v IN (?)", field), s)
	}
	for _, val := range s {
		if len(ranges) == 0 || val-1 > ranges[len(ranges)-1].Max {
			ranges = append(ranges, Range{
				Min: val,
				Max: val,
			})
		} else {
			ranges[len(ranges)-1].Max = val
		}
	}

	for _, r := range ranges {
		if r.Min == r.Max {
			querys = append(querys, fmt.Sprintf("(%v = ?)", field))
			items = append(items, r.Min)
		} else {
			querys = append(querys, fmt.Sprintf("(%v >= ? AND %v <= ?)", field, field))
			items = append(items, r.Min, r.Max)
		}
	}

	return db.Where(strings.Join(querys, " OR "), items...)
}

func ExactQueryStringArrayOr(db *gorm.DB, field string, s []string) *gorm.DB {
	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return ExactQueryArrayOr(db, field, raw)
}

func ExactOrQueryStringArrayOr(db *gorm.DB, field string, s []string) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return ExactOrQueryArrayOr(db, field, raw)
}

func ExactQueryExcludeStringArrayOr(db *gorm.DB, field string, s []string) *gorm.DB {
	if s == nil {
		return db
	}
	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return ExactQueryExcludeArrayOr(db, field, raw)
}

func FuzzQuery(db *gorm.DB, field string, value string) *gorm.DB {
	if value == "" {
		return db
	}
	return db.Where(fmt.Sprintf("%s LIKE ?", field), "%"+value+"%")
}

func FuzzQueryLike(db *gorm.DB, field string, value string) *gorm.DB {
	if value == "" {
		return db
	}
	return db.Where(fmt.Sprintf("%s LIKE ?", field), "%"+value+"%")
}

func FuzzQueryPrefixLike(db *gorm.DB, field string, value string) *gorm.DB {
	if value == "" {
		return db
	}
	return db.Where(fmt.Sprintf("%s LIKE ?", field), value+"%")
}

func FuzzQueryStrP(db *gorm.DB, field string, value *string) *gorm.DB {
	if value == nil {
		return db
	}

	return FuzzQuery(db, field, *value)
}

func FuzzQueryP(db *gorm.DB, field string, valueP *string) *gorm.DB {
	if valueP == nil {
		return db
	}

	return FuzzQuery(db, field, *valueP)
}

func ExactQueryStringP(db *gorm.DB, field string, valueP *string) *gorm.DB {
	if valueP == nil {
		return db
	}

	return ExactQueryString(db, field, *valueP)
}

func PrefixQueryStringP(db *gorm.DB, field string, valueP *string) *gorm.DB {
	if valueP == nil {
		return db
	}

	return PrefixQueryString(db, field, *valueP)
}

func PrefixQueryString(db *gorm.DB, field string, value string) *gorm.DB {
	if value == "" {
		return db
	}
	return db.Where(fmt.Sprintf("%s LIKE ?", field), fmt.Sprintf("%s%%", value))
}

func StartswithStringP(db *gorm.DB, field string, valueP *string) *gorm.DB {
	if valueP == nil {
		return db
	}
	return StartswithString(db, field, *valueP)
}

func StartswithString(db *gorm.DB, field string, value string) *gorm.DB {
	if value == "" {
		return db
	}
	return db.Where(fmt.Sprintf("%s LIKE ?", field), value+"%")
}

func StartswithStringLike(db *gorm.DB, field string, value string) *gorm.DB {
	if value == "" {
		return db
	}
	return db.Where(fmt.Sprintf("%s LIKE ?", field), value+"%")
}

func StartswithStringArrayOr(db *gorm.DB, field string, value []string) *gorm.DB {
	if len(value) <= 0 {
		return db
	}

	var (
		cond  []string
		items []interface{}
	)

	for _, v := range value {
		cond = append(cond, fmt.Sprintf("( %v LIKE ? )", field))
		items = append(items, v+"%")
	}
	return db.Where(strings.Join(cond, " OR "), items...)
}

func ExactQueryString(db *gorm.DB, field string, value string) *gorm.DB {
	if value == "" {
		return db
	}
	return db.Where(fmt.Sprintf("%s = ?", field), value)
}

func ExactQueryInt64P(db *gorm.DB, field string, value *int64) *gorm.DB {
	if value == nil {
		return db
	}
	return ExactQueryInt64(db, field, *value)
}

func ExactQueryInt64(db *gorm.DB, field string, value int64) *gorm.DB {
	return db.Where(fmt.Sprintf("%s = ?", field), value)
}

func QueryLargerThanFloatOr_AboveZero(db *gorm.DB, field string, value float64) *gorm.DB {
	if value <= 0 {
		return db
	}
	return db.Where(fmt.Sprintf("%v >= ?", field), value)
}

func QueryLargerOrEqualThanIntOr_AboveZero(db *gorm.DB, field string, value int64) *gorm.DB {
	if value <= 0 {
		return db
	}
	return db.Where(fmt.Sprintf("%v >= ?", field), value)
}

func QueryLargerThanIntOr_AboveZero(db *gorm.DB, field string, value int64) *gorm.DB {
	if value <= 0 {
		return db
	}
	return db.Where(fmt.Sprintf("%v > ?", field), value)
}

func QueryLargerOrEqualThanInt(db *gorm.DB, field string, value *int64) *gorm.DB {
	if value == nil {
		return db
	}
	return db.Where(fmt.Sprintf("%v >= ?", field), *value)
}

func QueryLessThanIntOr_AboveZero(db *gorm.DB, field string, value int64) *gorm.DB {
	if value <= 0 {
		return db
	}
	return db.Where(fmt.Sprintf("%v < ?", field), value)
}

func QueryLessOrEqualThanInt(db *gorm.DB, field string, value *int64) *gorm.DB {
	if value == nil {
		return db
	}
	return db.Where(fmt.Sprintf("%v <= ?", field), *value)
}

func QueryDateTimeAfterTimestampOr(db *gorm.DB, field string, timestamp int64) *gorm.DB {
	t := time.Unix(timestamp, 0)
	if t.IsZero() || t.Year() < 1975 {
		return db
	}
	return db.Where(fmt.Sprintf("%v >= ?", field), t)
}

func QueryDateTimeBeforeTimestampOr(db *gorm.DB, field string, timestamp int64) *gorm.DB {
	t := time.Unix(timestamp, 0)
	if t.IsZero() {
		return db
	}
	return db.Where(fmt.Sprintf("%v <= ?", field), t)
}

func QueryOrder(db *gorm.DB, byField, order string) *gorm.DB {
	if byField == "" {
		byField = "updated_at"
		order = "desc"
	}
	return db.Order(fmt.Sprintf("%v %v", byField, order))
}

func FuzzQueryJsonText(db *gorm.DB, jsonField string, search string) *gorm.DB {
	search = fmt.Sprintf("%%%v%%", search)
	db = db.Where(
		fmt.Sprintf("(%v::text LIKE ?)", jsonField),
		search,
	)
	return db
}

func FuzzQueryJsonTextP(db *gorm.DB, jsonField string, search *string) *gorm.DB {
	if search == nil {
		return db
	}

	return FuzzQueryJsonText(db, jsonField, Str(search))
}

func QueryOrderP(db *gorm.DB, orderBy, order *string) *gorm.DB {
	orderByS, orderS := "created_at", "desc"
	if orderBy != nil {
		orderByS = *orderBy
	}

	if order != nil {
		orderS = *order
	}

	return QueryOrder(db, orderByS, orderS)
}

func OrderByPaging(db *gorm.DB, p *ypb.Paging) *gorm.DB {
	if p.GetRawOrder() != "" {
		return db.Order(p.GetRawOrder())
	} else if p.GetOrderBy() != "" {
		return QueryOrder(db, p.GetOrderBy(), p.GetOrder())
	}
	return db
}

func Paging(db *gorm.DB, page int, limit int, data interface{}) (*Paginator, *gorm.DB) {
	p, db := NewPagination(&Param{
		DB:    db,
		Page:  page,
		Limit: limit,
		// ShowSQL: true,
	}, data)
	return p, db
}

func PagingByPagination(db *gorm.DB, p *ypb.Paging, data any) (*Paginator, *gorm.DB) {
	return NewPagination(&Param{
		DB:    db,
		Page:  int(p.GetPage()),
		Limit: int(p.GetLimit()),
	}, data)
}

func PagingP(db *gorm.DB, page *int64, limit *int64, data interface{}) (*Paginator, *gorm.DB) {
	pageInt, limitInt := 1, 10
	if page != nil {
		pageInt = int(Int64(page))
	}
	if limit != nil {
		limitInt = int(Int64(limit))
	}
	return Paging(db, pageInt, limitInt, data)
}

func QueryByBoolP(db *gorm.DB, byField string, v *bool) *gorm.DB {
	if v == nil {
		return db
	}

	return QueryByBool(db, byField, *v)
}

func QueryByBool(db *gorm.DB, field string, v bool) *gorm.DB {
	return db.Where(fmt.Sprintf("%v = ?", field), v)
}

func QueryByTimestampRange(db *gorm.DB, field string, start, end int64) *gorm.DB {
	if start > 0 {
		db = db.Where(fmt.Sprintf("%v >= ?", field), start)
	}

	if end > start {
		db = db.Where(fmt.Sprintf("%v <= ?", field), end)
	}

	return db
}

func QueryByTimestampRangeP(db *gorm.DB, field string, start, end *int64) *gorm.DB {
	var startTs, endTs int64
	if start != nil {
		startTs = *start
	}
	if end != nil {
		endTs = *end
	}

	return QueryByTimestampRange(db, field, startTs, endTs)
}

func QueryByTimeRange(db *gorm.DB, field string, start, end time.Time) *gorm.DB {
	if !start.IsZero() {
		db = db.Where(fmt.Sprintf("%v >= ?", field), start)
	}

	if end.After(start) {
		db = db.Where(fmt.Sprintf("%v <= ?", field), end)
	}

	return db
}

func QueryByTimeRangeWithTimestampP(db *gorm.DB, field string, startTs, endTs *int64) *gorm.DB {
	var start, end int64
	if startTs != nil {
		start = *startTs
	}

	if endTs != nil {
		end = *endTs
	}

	return QueryByTimeRangeWithTimestamp(db, field, start, end)
}

func QueryByTimeRangeWithTimestamp(db *gorm.DB, field string, startTs, endTs int64) *gorm.DB {
	start := time.Unix(startTs, 0)
	end := time.Unix(endTs, 0)

	if !start.IsZero() {
		db = db.Where(fmt.Sprintf("%v >= ?", field), start)
	}

	if end.After(start) {
		db = db.Where(fmt.Sprintf("%v <= ?", field), end)
	}

	return db
}

func QueryPostgresArrayCommonElements(db *gorm.DB, field string, arrayType string, t []string) *gorm.DB {
	if t == nil {
		return db
	}

	t = utils.StringArrayFilterEmpty(t)
	if len(t) <= 0 {
		return db
	}

	return db.Where(fmt.Sprintf("%v && ARRAY[?]::%v", field, arrayType), t)
}

func FuzzQueryPostgresStringArray(
	db *gorm.DB,
	tableName string,
	subQueryField string,
	subQueryFieldArrayType string,
	data []string,
) *gorm.DB {
	var (
		conds []string
		items []interface{}
	)
	for _, h := range data {
		if h == "" {
			continue
		}
		conds = append(conds, "(fuzzquery LIKE ?)")
		items = append(items, "%"+h+"%")
	}

	if len(conds) <= 0 {
		return db
	}

	subQuery := db.Table(tableName).Select(
		fmt.Sprintf("unnest(%v) as fuzzquery", subQueryField),
	).QueryExpr()

	items = append([]interface{}{subQuery}, items...)
	db = db.Joins(
		fmt.Sprintf(
			"JOIN (?) as t on ((%v) and (%v.%v && ARRAY[fuzzquery]::%v))",
			strings.Join(conds, " OR "),
			tableName, subQueryField, subQueryFieldArrayType,
		), items...,
	)

	return db
}

type GormWhereBlock struct {
	Cond  string
	Items []interface{}
}

func MergeAndGormWhereBlocks(blocks []*GormWhereBlock) *GormWhereBlock {
	var (
		cond  []string
		items []interface{}
	)

	for _, b := range blocks {
		cond = append(cond, fmt.Sprintf("(%v)", b.Cond))
		items = append(items, b.Items...)
	}

	if len(cond) > 0 {
		return &GormWhereBlock{
			Cond:  strings.Join(cond, " AND "),
			Items: items,
		} // db.Where(, items...)
	}
	return nil
}

func MergeOrGormWhereBlocks(blocks []*GormWhereBlock) *GormWhereBlock {
	var (
		cond  []string
		items []interface{}
	)

	for _, b := range blocks {
		cond = append(cond, fmt.Sprintf("(%v)", b.Cond))
		items = append(items, b.Items...)
	}

	if len(cond) > 0 {
		return &GormWhereBlock{
			Cond:  strings.Join(cond, " OR "),
			Items: items,
		} // db.Where(, items...)
	}
	return nil
}

func QueryCount(db *gorm.DB, m interface{}, items *GormWhereBlock) int {
	var count int
	if m != nil {
		db = db.Model(m)
	}

	if items != nil {
		db = db.Where(items.Cond, items.Items...)
	}
	if db := db.Count(&count); db.Error != nil {
		log.Errorf("query count failed: %s", db.Error)
		return 0
	}
	return count
}

func QueryByJsonKey(db *gorm.DB, field string, filter map[string]interface{}) *gorm.DB {
	jsonStrByte, _ := json.Marshal(filter)
	return db.Where(" ? ::jsonb @> ? ::jsonb  ", field, string(jsonStrByte[:]))
}

func QueryByJsonKeyList(db *gorm.DB, field string, keys *string, values *string, sep string) *gorm.DB {
	if keys == nil || values == nil {
		return db
	}

	keyList := strings.Split(*keys, sep)
	valueList := strings.Split(*values, sep)

	if len(keyList) != len(valueList) {
		return db
	}

	jsonKeyMap := make(map[string]interface{})
	for k, v := range keyList {
		jsonKeyMap[v] = valueList[k]
	}
	return QueryByJsonKey(db, field, jsonKeyMap)
}

func CalcTags(origin []string, op string, now []string) []string {
	switch op {
	case "set":
		return utils.RemoveRepeatStringSlice(now)
	default:
		return utils.RemoveRepeatStringSlice(append(origin, now...))
	}
}

func CalcTagsP(origin []string, op *string, now []string) []string {
	switch Str(op) {
	case "set":
		return utils.RemoveRepeatStringSlice(now)
	default:
		return utils.RemoveRepeatStringSlice(append(origin, now...))
	}
}

func FuzzSearchP(db *gorm.DB, fields []string, target *string) *gorm.DB {
	if target == nil {
		return db
	}
	return FuzzSearch(db, fields, Str(target))
}

func FuzzSearch(db *gorm.DB, fields []string, target string) *gorm.DB {
	return FuzzSearchEx(db, fields, target, true)
}

// ilike sqliter not support
func FuzzSearchEx(db *gorm.DB, fields []string, target string, ilike bool) *gorm.DB {
	if target == "" || len(fields) <= 0 {
		return db
	}

	target = fmt.Sprintf("%%%s%%", target)

	var conds []string
	var items []interface{}

	for _, field := range fields {
		if ilike {
			conds = append(conds, fmt.Sprintf("( %v ILIKE ?)", field))
		} else {
			conds = append(conds, fmt.Sprintf("( %v LIKE ?)", field))
		}
		items = append(items, target)
	}

	return db.Where(strings.Join(conds, " OR "), items...)
}

func FuzzSearchWithStringArrayOr(db *gorm.DB, fields []string, targets []string) *gorm.DB {
	return FuzzSearchWithStringArrayOrEx(db, fields, targets, true)
}

// ilike sqliter not support
func FuzzSearchWithStringArrayOrEx(db *gorm.DB, fields []string, targets []string, ilike bool) *gorm.DB {
	if len(targets) <= 0 {
		return db
	}

	if len(fields) <= 0 {
		return db
	}

	var conds []string
	var items []interface{}

	for _, field := range fields {
		for _, target := range targets {
			if len(target) == 0 {
				continue
			}
			if ilike {
				conds = append(conds, fmt.Sprintf("( %v ILIKE ? )", field))
			} else {
				conds = append(conds, fmt.Sprintf("( %v LIKE ? )", field))
			}
			target := fmt.Sprintf("%%%s%%", target)
			items = append(items, target)
		}
	}

	if len(conds) <= 0 {
		return db
	}

	return db.Where(strings.Join(conds, " OR "), items...)
}

// ilike sqliter not support
func FuzzSearchNotEx(db *gorm.DB, fields []string, target string, ilike bool) *gorm.DB {
	if target == "" || len(fields) <= 0 {
		return db
	}

	target = fmt.Sprintf("%%%s%%", target)

	var conds []string
	var items []interface{}

	for _, field := range fields {
		if ilike {
			conds = append(conds, fmt.Sprintf("( %v NOT ILIKE ?)", field))
		} else {
			conds = append(conds, fmt.Sprintf("( %v NOT LIKE ?)", field))
		}
		items = append(items, target)
	}

	return db.Where(strings.Join(conds, " AND "), items...)
}

func QueryIntegerInArrayInt64(db *gorm.DB, field string, targets []int64) *gorm.DB {
	if len(targets) > 0 {
		return db.Where(
			fmt.Sprintf(
				"%v = any(array[?]::int[])", field,
			), targets)
	}
	return db
}

func QueryStringInStringSlice(db *gorm.DB, field string, targets []string) *gorm.DB {
	if len(targets) > 0 {
		return db.Where(
			fmt.Sprintf(
				"%v = any(array[?]::text[])", field,
			), targets)
	}
	return db
}

func OrFuzzQueryStringArrayOrLike(db *gorm.DB, field string, s []string) *gorm.DB {
	s = utils.StringArrayFilterEmpty(s)
	if len(s) <= 0 {
		return db
	}

	raw := make([]interface{}, len(s))
	for index, sub := range s {
		raw[index] = sub
	}
	return OrFuzzQueryArrayOrLike(db, field, raw)
}

func OrFuzzQueryArrayOrLike(db *gorm.DB, field string, s []interface{}) *gorm.DB {
	if len(s) <= 0 {
		return db
	}

	var (
		querys []string
		items  []interface{}
	)

	for _, sub := range s {
		querys = append(querys, fmt.Sprintf("( %v LIKE ? )", field))
		items = append(items, fmt.Sprintf("%%%v%%", sub))
	}

	return db.Where(strings.Join(querys, " or "), items...)
}

// ilike sqliter not support
func FuzzSearchWithStringArrayOrAf(db *gorm.DB, fields []string, targets []string, ilike bool) *gorm.DB {
	if len(targets) <= 0 {
		return db
	}

	if len(fields) <= 0 {
		return db
	}

	var conds []string
	var items []interface{}

	for _, field := range fields {
		for _, target := range targets {
			if ilike {
				conds = append(conds, fmt.Sprintf("( %v ILIKE ? )", field))
			} else {
				conds = append(conds, fmt.Sprintf("( %v LIKE ? )", field))
			}
			target := fmt.Sprintf("%%%s", target)
			items = append(items, target)
		}
	}

	if len(conds) <= 0 {
		return db
	}

	return db.Where(strings.Join(conds, " OR "), items...)
}

func GetTableCurrentId(db *gorm.DB, tableName string) (int64, error) {
	var result struct {
		Count int64
	}
	if db = db.Raw(`select seq as count from SQLITE_SEQUENCE where name = ?`, tableName).Find(&result); db.Error != nil {
		return 0, db.Error
	}
	return result.Count, nil
}

func YakitPagingQuery(db *gorm.DB, p *ypb.Paging, data any) (*Paginator, *gorm.DB) {
	db = QueryOrder(db, p.OrderBy, p.Order)          // set order by
	if p.GetBeforeId() <= 0 && p.GetAfterId() <= 0 { // if not set after_id and before_id, use page and limit
		return NewPagination(&Param{
			DB:    db,
			Page:  int(p.GetPage()),
			Limit: int(p.GetLimit()),
		}, data)
	}

	var count int
	if db.Model(data).Count(&count); db.Error != nil { // get total count
		log.Errorf("query count failed: %s", db.Error)
	}

	// Incremental query
	db = QueryLargerThanIntOr_AboveZero(db, "id", p.GetAfterId()) // if set after_id, use after_id, not page data ,just query
	db = QueryLessThanIntOr_AboveZero(db, "id", p.GetBeforeId())
	var paginator = &Paginator{}
	if p.Limit == -1 {
		db.Find(data)
		paginator.Limit = count
	} else {
		db.Limit(p.Limit).Find(data)
	}
	paginator.TotalRecord = count
	paginator.Records = data
	return paginator, db
}

func GroupCount(db *gorm.DB, tableName string, column string) []*ypb.FieldGroup {
	var res []*ypb.FieldGroup
	db.Table(tableName).Select(fmt.Sprintf("%v as name , count(*) as total", column)).Group(column).Scan(&res)
	return res
}

func GroupColumn(db *gorm.DB, tableName string, column string) ([]any, error) {
	var res []any
	if db := db.Table(tableName).Select(column).Group(column).Pluck(column, &res); db.Error != nil {
		return nil, db.Error
	}
	return res, nil
}
