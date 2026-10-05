package aicommon

import (
	"fmt"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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

	// Save the complete snapshot. Persistence must never retire history.
	m.mu.RLock()
	tlstr, err := marshalTimelineUnlocked(m)
	m.mu.RUnlock()
	if err != nil {
		return err
	}

	result := strconv.Quote(tlstr)
	update := db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", persistentId).Update("quoted_timeline", result)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected == 0 {
		return fmt.Errorf("timeline checkpoint has no runtime for persistent session %q", persistentId)
	}
	return nil
}
