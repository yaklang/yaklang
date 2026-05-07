package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// scopeThinkingChunks narrows to one logical scope: persistent session + loop, or
// (when session id is empty) runtime + loop with no persistent_session_id set.
func scopeThinkingChunks(db *gorm.DB, loopName, persistentSessionID, runtimeID string) *gorm.DB {
	q := db.Model(&schema.AIReActThinkingChunk{}).Where("loop_name = ?", loopName)
	ps := strings.TrimSpace(persistentSessionID)
	if ps != "" {
		return q.Where("persistent_session_id = ?", ps)
	}
	rt := strings.TrimSpace(runtimeID)
	return q.Where("runtime_id = ?", rt).Where("(persistent_session_id IS NULL OR persistent_session_id = '')")
}

// SaveAIReActThinkingChunk persists one thinking/reason stream chunk for auditing and replay.
func SaveAIReActThinkingChunk(db *gorm.DB, row *schema.AIReActThinkingChunk) error {
	if db == nil || row == nil {
		return nil
	}
	if err := db.Create(row).Error; err != nil {
		log.Errorf("SaveAIReActThinkingChunk failed: %v", err)
		return err
	}
	return nil
}

// LoadAIReActThinkingAggregated returns merged thinking text (chunk contents concatenated
// in created_at order, then id). When persistentSessionID is non-empty it selects by session;
// otherwise by runtimeID (non-persistent scope).
func LoadAIReActThinkingAggregated(db *gorm.DB, loopName, persistentSessionID, runtimeID string) (merged string, err error) {
	if db == nil {
		return "", nil
	}
	ps := strings.TrimSpace(persistentSessionID)
	rt := strings.TrimSpace(runtimeID)
	if ps == "" && rt == "" {
		return "", nil
	}
	var rows []schema.AIReActThinkingChunk
	q := scopeThinkingChunks(db, loopName, ps, rt).Order("created_at asc, id asc")
	if err := q.Find(&rows).Error; err != nil {
		return "", err
	}
	var b strings.Builder
	for _, row := range rows {
		b.WriteString(row.Content)
	}
	return b.String(), nil
}

// LoadAIReActThinkingAggregatedForSession is LoadAIReActThinkingAggregated for a persistent session id.
func LoadAIReActThinkingAggregatedForSession(db *gorm.DB, persistentSessionID, loopName string) (merged string, err error) {
	return LoadAIReActThinkingAggregated(db, loopName, strings.TrimSpace(persistentSessionID), "")
}
