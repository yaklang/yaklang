package store

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// AppendFileOperation inserts one file operation audit row.
func (r *Repository) AppendFileOperation(row *DiscoveryFileOperation) error {
	if r == nil || r.db == nil {
		return utils.Error("nil repository")
	}
	if row == nil {
		return utils.Error("nil row")
	}
	return r.db.Create(row).Error
}

// AppendFileOperations batch-inserts audit rows.
func (r *Repository) AppendFileOperations(rows []DiscoveryFileOperation) error {
	if r == nil || r.db == nil {
		return utils.Error("nil repository")
	}
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		if err := r.db.Create(&rows[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListFileOperations returns file operations for a session with optional filters.
func (r *Repository) ListFileOperations(sessionID uint, stage, operation string, limit int) ([]DiscoveryFileOperation, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	var rows []DiscoveryFileOperation
	q := r.db.Where("session_id = ?", sessionID)
	if s := strings.TrimSpace(stage); s != "" {
		q = q.Where("pipeline_stage = ?", s)
	}
	if op := strings.TrimSpace(operation); op != "" {
		q = q.Where("operation = ?", op)
	}
	err := q.Order("id asc").Limit(limit).Find(&rows).Error
	return rows, err
}

// CountFileOperations returns total rows for a session.
func (r *Repository) CountFileOperations(sessionID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, utils.Error("nil repository")
	}
	var n int64
	err := r.db.Model(&DiscoveryFileOperation{}).Where("session_id = ?", sessionID).Count(&n).Error
	return n, err
}
