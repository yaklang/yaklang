package aicommon

import (
	"fmt"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strconv"
)

func (m *Timeline) Save(db *gorm.DB, persistentId string) {
	if err := m.saveChecked(db, persistentId); err != nil {
		log.Errorf("save timeline failed: %v", err)
	}
}

func (m *Timeline) saveChecked(db *gorm.DB, persistentId string) error {
	if utils.IsNil(m) {
		log.Warnf("try to save nil timeline for persistentId: %v", persistentId)
		return nil
	}
	if m.IsBranchTimeline() {
		return nil
	}
	m.persistMu.Lock()
	defer m.persistMu.Unlock()

	// The resume checkpoint holds the active tail/summary only. Archive each
	// live original independently before any compression retirement.
	m.mu.RLock()
	tlstr, err := marshalTimelineUnlocked(m)
	var rows []schema.AITimelineHistory
	if err == nil {
		rows, err = m.historySnapshotLocked()
	}
	m.mu.RUnlock()
	if err != nil {
		return err
	}

	if err := yakit.EnsureAITimelineHistory(db, false); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := yakit.ArchiveAITimelineItems(tx, persistentId, rows); err != nil {
			return err
		}
		update := tx.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", persistentId).Update("quoted_timeline", strconv.Quote(tlstr))
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return fmt.Errorf("timeline checkpoint has no runtime for persistent session %q", persistentId)
		}
		return nil
	})
}
