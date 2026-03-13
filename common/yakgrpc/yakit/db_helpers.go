package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
)

func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}

func countRowsIgnoreMissingTable(db *gorm.DB, model interface{}) (int64, error) {
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		if isMissingTableErr(err) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}
