package consts

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func CreateVulinboxDatabase(path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path, SQLiteExtend)
	if err != nil {
		return nil, err
	}
	schema.AutoMigrate(db, schema.KEY_SCHEMA_VULINBOX_DATABASE)
	return db, nil
}
