package bizhelper

import (
	"gorm.io/gorm"
	"math/rand"
	"time"
)

func RandomQuery(db *gorm.DB, limit int, data interface{}) (int, error) {
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, err
	}

	if total <= int64(limit) {
		if err := db.Find(data).Error; err != nil {
			return 0, err
		}
		// return early to avoid computing random offset when total-limit+1 <= 0
		return int(total), nil
	}

	randomSource := rand.New(rand.NewSource(time.Now().UnixNano()))
	offset := randomSource.Intn(int(total) - limit + 1) // 确保 offset + count <= total
	if err := db.Offset(offset).Limit(limit).Find(data).Error; err != nil {
		return 0, err
	}

	return int(total), nil
}
