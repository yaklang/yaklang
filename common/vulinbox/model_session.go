package vulinbox

import (
	uuid "github.com/google/uuid"
	"github.com/jinzhu/gorm"
)

type Session struct {
	gorm.Model
	Uuid     string
	Username string
	Role     string
}

func (v *VulinUser) CreateSession(dbm *dbm) (session Session, err error) {
	session = Session{
		Uuid:     uuid.New().String(),
		Username: v.Username,
		Role:     v.Role,
	}
	// 插入会话记录到数据库
	if err := dbm.db.Create(&session).Error; err != nil {
		return session, err
	}

	return session, nil
}
