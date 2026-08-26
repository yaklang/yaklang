package ssadb

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
)

// DBOpKind classifies SSA IR database work by SQL verb / GORM callback.
// "搜索 / 按 ID 加载 / Count" all fall under query — they are SELECT traffic.
type DBOpKind string

const (
	DBOpQuery  DBOpKind = "query"  // SELECT: First/Find/Pluck/Count/native read/search
	DBOpCreate DBOpKind = "create" // INSERT
	DBOpUpdate DBOpKind = "update" // UPDATE / Save-as-update
	DBOpDelete DBOpKind = "delete" // DELETE
)

// DBOpKinds is the stable display/order for UI and JSON.
var DBOpKinds = []DBOpKind{DBOpQuery, DBOpCreate, DBOpUpdate, DBOpDelete}

// DBOpBucket is timing for one operation kind.
type DBOpBucket struct {
	Count      int64 `json:"count"`
	TotalMs    int64 `json:"total_ms"`
	MinMs      int64 `json:"min_ms,omitempty"`
	MaxMs      int64 `json:"max_ms,omitempty"`
	AvgMs      int64 `json:"avg_ms,omitempty"`
	ErrorCount int64 `json:"error_count,omitempty"`
}

// DBOpStats is a snapshot of SSA IR database operation timings, split by kind.
type DBOpStats struct {
	Dialect    string                  `json:"dialect,omitempty"`
	Ops        map[DBOpKind]DBOpBucket `json:"ops,omitempty"`
	TotalCount int64                   `json:"total_count"`
	TotalMs    int64                   `json:"total_ms"`
	ErrorCount int64                   `json:"error_count,omitempty"`
}

type dbOpAccumulator struct {
	count   atomic.Int64
	totalNs atomic.Int64
	minNs   atomic.Int64
	maxNs   atomic.Int64
	errors  atomic.Int64
}

func (a *dbOpAccumulator) record(d time.Duration, failed bool) {
	ns := d.Nanoseconds()
	if ns < 0 {
		ns = 0
	}
	a.count.Add(1)
	a.totalNs.Add(ns)
	if failed {
		a.errors.Add(1)
	}
	for {
		cur := a.minNs.Load()
		if cur != 0 && cur <= ns {
			break
		}
		if a.minNs.CompareAndSwap(cur, ns) {
			break
		}
	}
	for {
		cur := a.maxNs.Load()
		if cur >= ns {
			break
		}
		if a.maxNs.CompareAndSwap(cur, ns) {
			break
		}
	}
}

func (a *dbOpAccumulator) snapshot() DBOpBucket {
	count := a.count.Load()
	totalNs := a.totalNs.Load()
	minNs := a.minNs.Load()
	maxNs := a.maxNs.Load()
	errors := a.errors.Load()
	out := DBOpBucket{
		Count:      count,
		TotalMs:    totalNs / int64(time.Millisecond),
		ErrorCount: errors,
	}
	if count > 0 {
		out.MinMs = minNs / int64(time.Millisecond)
		out.MaxMs = maxNs / int64(time.Millisecond)
		out.AvgMs = (totalNs / count) / int64(time.Millisecond)
	}
	return out
}

var (
	dbOpByKind       = map[DBOpKind]*dbOpAccumulator{}
	dbOpDialect      atomic.Value // string
	dbOpCallbackOnce sync.Once
	dbOpInitOnce     sync.Once
)

func initDBOpAccumulators() {
	dbOpInitOnce.Do(func() {
		dbOpByKind = map[DBOpKind]*dbOpAccumulator{
			DBOpQuery:  {},
			DBOpCreate: {},
			DBOpUpdate: {},
			DBOpDelete: {},
		}
	})
}

func accumulatorFor(kind DBOpKind) *dbOpAccumulator {
	initDBOpAccumulators()
	if acc, ok := dbOpByKind[kind]; ok && acc != nil {
		return acc
	}
	// Unknown kinds fold into query so callers never drop timings.
	return dbOpByKind[DBOpQuery]
}

// RecordDBOp records one SSA IR database operation of the given kind.
func RecordDBOp(kind DBOpKind, d time.Duration, failed bool) {
	accumulatorFor(kind).record(d, failed)
}

// RecordDBRead is kept for call-site migration; it records a query.
func RecordDBRead(d time.Duration, failed bool) {
	RecordDBOp(DBOpQuery, d, failed)
}

// RecordDBWrite is kept for call-site migration; prefer RecordDBOp with
// create/update/delete. Unknown writes default to update.
func RecordDBWrite(d time.Duration, failed bool) {
	RecordDBOp(DBOpUpdate, d, failed)
}

// SetDBOpDialect records the active IR database dialect for stats snapshots.
func SetDBOpDialect(dialect string) {
	dbOpDialect.Store(strings.TrimSpace(dialect))
}

// SnapshotDBOpStats returns cumulative IR DB op stats since process start
// (or since the last ResetDBOpStats call).
func SnapshotDBOpStats() DBOpStats {
	initDBOpAccumulators()
	stats := DBOpStats{
		Ops: make(map[DBOpKind]DBOpBucket, len(DBOpKinds)),
	}
	if v := dbOpDialect.Load(); v != nil {
		if s, ok := v.(string); ok {
			stats.Dialect = s
		}
	}
	for _, kind := range DBOpKinds {
		bucket := accumulatorFor(kind).snapshot()
		stats.Ops[kind] = bucket
		stats.TotalCount += bucket.Count
		stats.TotalMs += bucket.TotalMs
		stats.ErrorCount += bucket.ErrorCount
	}
	return stats
}

// DeltaDBOpStats returns stats accumulated between previous and current.
func DeltaDBOpStats(previous, current DBOpStats) DBOpStats {
	out := DBOpStats{
		Dialect: current.Dialect,
		Ops:     make(map[DBOpKind]DBOpBucket, len(DBOpKinds)),
	}
	for _, kind := range DBOpKinds {
		prev := previous.Ops[kind]
		cur := current.Ops[kind]
		bucket := DBOpBucket{
			Count:      cur.Count - prev.Count,
			TotalMs:    cur.TotalMs - prev.TotalMs,
			ErrorCount: cur.ErrorCount - prev.ErrorCount,
		}
		if bucket.Count < 0 {
			bucket.Count = 0
		}
		if bucket.TotalMs < 0 {
			bucket.TotalMs = 0
		}
		if bucket.ErrorCount < 0 {
			bucket.ErrorCount = 0
		}
		// Window min/max are not recoverable from cumulative totals alone;
		// keep current extremes as best-effort peak markers for the UI.
		if bucket.Count > 0 {
			bucket.MinMs = cur.MinMs
			bucket.MaxMs = cur.MaxMs
			bucket.AvgMs = bucket.TotalMs / bucket.Count
		}
		out.Ops[kind] = bucket
		out.TotalCount += bucket.Count
		out.TotalMs += bucket.TotalMs
		out.ErrorCount += bucket.ErrorCount
	}
	return out
}

// ResetDBOpStats clears cumulative counters (tests / isolated runs).
func ResetDBOpStats() {
	initDBOpAccumulators()
	for _, kind := range DBOpKinds {
		dbOpByKind[kind] = &dbOpAccumulator{}
	}
}

func recordScopeOp(kind DBOpKind) func(scope *gorm.Scope) {
	return func(scope *gorm.Scope) {
		started, _ := scope.Get("ssadb:stats:started_at")
		start, _ := started.(time.Time)
		if start.IsZero() {
			return
		}
		RecordDBOp(kind, time.Since(start), scope.HasError())
	}
}

func markScopeStart(scope *gorm.Scope) {
	scope.Set("ssadb:stats:started_at", time.Now())
}

// EnsureDBOpCallbacks registers GORM callbacks that time IR DB ops by kind.
// Safe to call repeatedly.
func EnsureDBOpCallbacks(db *gorm.DB) {
	if db == nil {
		return
	}
	if name := strings.TrimSpace(db.Dialect().GetName()); name != "" {
		SetDBOpDialect(name)
	}
	dbOpCallbackOnce.Do(func() {
		registerDBOpCallbacks(db)
	})
	attachSQLLogger(db)
}

func registerDBOpCallbacks(db *gorm.DB) {
	db.Callback().Query().Before("gorm:query").Register("ssadb:stats:before_query", markScopeStart)
	db.Callback().Query().After("gorm:query").Register("ssadb:stats:after_query", recordScopeOp(DBOpQuery))
	db.Callback().Create().Before("gorm:create").Register("ssadb:stats:before_create", markScopeStart)
	db.Callback().Create().After("gorm:create").Register("ssadb:stats:after_create", recordScopeOp(DBOpCreate))
	db.Callback().Update().Before("gorm:update").Register("ssadb:stats:before_update", markScopeStart)
	db.Callback().Update().After("gorm:update").Register("ssadb:stats:after_update", recordScopeOp(DBOpUpdate))
	db.Callback().Delete().Before("gorm:delete").Register("ssadb:stats:before_delete", markScopeStart)
	db.Callback().Delete().After("gorm:delete").Register("ssadb:stats:after_delete", recordScopeOp(DBOpDelete))
	db.Callback().RowQuery().Before("gorm:row_query").Register("ssadb:stats:before_row_query", markScopeStart)
	db.Callback().RowQuery().After("gorm:row_query").Register("ssadb:stats:after_row_query", recordScopeOp(DBOpQuery))
}

// bindSQLPlaceholdersDB rewrites "?" placeholders for the active dialect.
// database/sql + Postgres requires $1/$2; SQLite accepts "?".
func bindSQLPlaceholdersDB(db *gorm.DB, query string) string {
	name := ""
	if db != nil {
		name = db.Dialect().GetName()
	}
	return bindSQLPlaceholders(name, query)
}

func bindSQLPlaceholders(dialect, query string) string {
	if !strings.Contains(query, "?") {
		return query
	}
	name := strings.ToLower(strings.TrimSpace(dialect))
	if name != "postgres" && name != "postgresql" && name != "cloudsqlpostgres" {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}
