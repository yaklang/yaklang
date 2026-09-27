package aicommon

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strconv"
)

func (m *Timeline) Save(db *gorm.DB, persistentId string) {
	if utils.IsNil(m) {
		log.Warnf("try to save nil timeline for persistentId: %v", persistentId)
		return
	}
	if m.IsBranchTimeline() {
		return
	}

	// Save the complete snapshot. Persistence must never retire history.
	m.mu.RLock()
	tlstr, err := marshalTimelineUnlocked(m)
	m.mu.RUnlock()
	if err != nil {
		log.Warnf("save(/marshal) timeline failed: %v", err)
		return
	}

	result := strconv.Quote(tlstr)
	if err := yakit.UpdateAIAgentRuntimeTimelineWithPersistentId(db, persistentId, result); err != nil {
		log.Errorf("ReAct: save timeline to db failed: %v", err)
		return
	}
}
