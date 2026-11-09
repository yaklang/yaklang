package ssadb

import (
	"time"

	"github.com/yaklang/gorm"
)

// HasAuditEdgesWithoutResult reports whether the database holds legacy edges
// that are not addressable through result_id. Old imports stored edges without
// stamping result_id; a result-scoped query would silently drop them, so
// callers must keep the per-node reader for such a database.
//
// The check is database-wide on purpose: the caller's program name may differ
// from the name stamped on a historical edge, and a false negative would drop
// real report paths. audit_edges is small compared with ir_codes, and callers
// cache the answer once per Program.
func HasAuditEdgesWithoutResult(db *gorm.DB) bool {
	if db == nil {
		db = GetDB()
	}
	if db == nil {
		return false
	}
	var rows []struct{ ID uint }
	started := time.Now()
	err := db.Table(TableAuditEdges).
		Select("id").
		Where("result_id IS NULL OR result_id = 0").
		Limit(1).
		Find(&rows).Error
	RecordDBRead(time.Since(started), err != nil)
	return err == nil && len(rows) > 0
}
