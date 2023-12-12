package utils

import "github.com/jinzhu/gorm"

func GormTransaction(db *gorm.DB, callback func(tx *gorm.DB) error) (err error) {
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if err != nil {
			tx.Rollback()
		} else {
			db := tx.Commit()
			if db != nil {
				err = db.Error
			}
		}
	}()

	err = callback(tx)
	return
}
