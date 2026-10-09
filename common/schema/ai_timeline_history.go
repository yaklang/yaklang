package schema

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/yaklang/gorm"
	"time"
)

// AITimelineHistory retains original entries independently of prompt compression.
// HistoryID survives restore/renumbering; ItemID records the original local ID.
type AITimelineHistory struct {
	gorm.Model
	SessionID   string    `gorm:"unique_index:idx_ai_timeline_session_item;not null" json:"session_id"`
	ItemID      int64     `json:"timeline_item_id"`
	HistoryID   string    `gorm:"unique_index:idx_ai_timeline_session_item;not null" json:"history_id"`
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	ValueJSON   string    `gorm:"type:text" json:"value_json"`
	ContentHash string    `json:"-"`
	Content     string    `gorm:"type:text" json:"content"`
}

// AITimelineLegacyHistoryID gives pre-upgrade entries a stable identity even
// when their root timeline ID has been reassigned during session restore.
func AITimelineLegacyHistoryID(kind string, createdAt time.Time, value json.RawMessage) string {
	canonical := value
	var obj map[string]json.RawMessage
	if json.Unmarshal(value, &obj) == nil && obj != nil {
		delete(obj, "id")
		delete(obj, "ID")
		if raw, err := json.Marshal(obj); err == nil {
			canonical = raw
		}
	}
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte(kind+"/"+createdAt.UTC().Format(time.RFC3339Nano)+"/"), canonical...)))
}
