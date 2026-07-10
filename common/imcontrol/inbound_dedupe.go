package imcontrol

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/notify"
)

type inboundMessageRecord struct {
	ID        int64     `gorm:"primary_key;auto_increment"`
	DedupeKey string    `gorm:"unique_index;size:64;not null"`
	Platform  string    `gorm:"index;size:32"`
	ChatID    string    `gorm:"type:text"`
	MessageID string    `gorm:"type:text"`
	EventTime time.Time `gorm:"index"`
	SeenAt    time.Time `gorm:"index"`
	ExpiresAt time.Time `gorm:"index"`
}

func (inboundMessageRecord) TableName() string {
	return "im_control_inbound_messages"
}

func (e *Engine) markPersistentInboundSeen(msg *notify.InboundMessage, messageID string, seenAt time.Time) (bool, error) {
	if msg == nil || msg.IsCardAction {
		return false, nil
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false, nil
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return false, nil
	}
	if err := db.AutoMigrate(&inboundMessageRecord{}).Error; err != nil {
		return false, err
	}
	_ = db.Where("expires_at < ?", seenAt).Delete(&inboundMessageRecord{}).Error

	key := inboundPersistentDedupeKey(string(msg.Platform), msg.ChatID, messageID)
	var existing inboundMessageRecord
	err := db.Where("dedupe_key = ?", key).First(&existing).Error
	if err == nil {
		return true, nil
	}
	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return false, err
	}

	record := inboundMessageRecord{
		DedupeKey: key,
		Platform:  string(msg.Platform),
		ChatID:    msg.ChatID,
		MessageID: messageID,
		EventTime: msg.EventTime,
		SeenAt:    seenAt,
		ExpiresAt: seenAt.Add(inboundPersistentDedupeTTL),
	}
	if err := db.Create(&record).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func inboundPersistentDedupeKey(platform, chatID, messageID string) string {
	sum := sha256.Sum256([]byte(platform + "\x00" + chatID + "\x00" + messageID))
	return hex.EncodeToString(sum[:])
}
