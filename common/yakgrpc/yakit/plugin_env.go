package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func SetPluginEnv(db *gorm.DB, key string, value string) error {
	if db := db.Save(&schema.PluginEnv{Key: key, Value: value}); db.Error != nil {
		return db.Error
	}
	return nil
}

func CreateOrUpdatePluginEnv(db *gorm.DB, key string, value string) error {
	if db := db.Where("key = ?", key).Assign(schema.PluginEnv{Key: key, Value: value}).FirstOrCreate(&schema.PluginEnv{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func GetPluginEnvByKey(db *gorm.DB, key string) (string, error) {
	var env schema.PluginEnv
	if db := db.Select("value").Where("key = ?", key).First(&env); db.Error != nil {
		return "", db.Error
	}
	return env.Value, nil
}

func GetAllPluginEnv(db *gorm.DB) ([]*schema.PluginEnv, error) {
	var env []*schema.PluginEnv
	if db := db.Find(&env); db.Error != nil {
		return nil, db.Error
	}
	return env, nil
}

func DeletePluginEnvByKey(db *gorm.DB, key string) error {
	if db := db.Where("key = ?", key).Unscoped().Delete(&schema.PluginEnv{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteAllPluginEnv(db *gorm.DB, env *schema.PluginEnv) error {
	if db := db.Unscoped().Delete(env); db.Error != nil {
		return db.Error
	}
	return nil
}
