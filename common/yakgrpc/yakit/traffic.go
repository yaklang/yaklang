package yakit

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func SaveTrafficSession(db *gorm.DB, session *schema.TrafficSession) error {
	return db.Save(session).Error
}

func SaveTrafficPacket(db *gorm.DB, packet *schema.TrafficPacket) error {
	return db.Save(packet).Error
}
