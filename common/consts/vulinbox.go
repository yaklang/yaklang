package consts

import "github.com/jinzhu/gorm"

func CreateVulinboxDatabase(path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path, SQLiteExtend)
	if err != nil {
		return nil, err
	}
	AutoMigrate(db, KEY_SCHEMA_VULINBOX_DATABASE)
	return db, nil
}
