package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"time"
)

type ProjectGeneralStorage struct {
	gorm.Model

	Key string `json:"key" gorm:"unique_index"`

	// 经过 JSON + Strconv
	Value string `json:"value"`

	// 过期时间
	ExpiredAt time.Time

	// YAKIT SUBPROC_ENV
	ProcessEnv bool

	// 帮助信息，描述这个变量是干嘛的
	Verbose string

	// 描述变量所在的组是啥
	Group string
}

func GetProjectKey(db *gorm.DB, key interface{}) string {
	kv, err := GetProjectKeyModel(db, key)
	if err != nil {
		return ""
	}
	if kv.Value == "" {
		return ""
	}
	v, err := strconv.Unquote(kv.Value)
	if err != nil {
		log.Errorf("unquote(general storage) value failed: %s", err)
		return kv.Value
	}
	return v
}

func SetProjectKey(db *gorm.DB, key interface{}, value interface{}) error {
	//db = UserDataAndPluginDatabaseScope(db)

	if db == nil {
		return utils.Error("no set database")
	}

	var keyStr = strconv.Quote(utils.InterfaceToString(key))
	var valueStr = strconv.Quote(utils.InterfaceToString(value))
	if valueStr == `""` {
		valueStr = ""
	}
	if db := db.Model(&ProjectGeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr,
	}).FirstOrCreate(&ProjectGeneralStorage{}); db.Error != nil {
		return utils.Errorf("create project storage kv failed: %s", db.Error)
	}
	return nil
}

func GetProjectKeyModel(db *gorm.DB, key interface{}) (*ProjectGeneralStorage, error) {
	if db == nil {
		return nil, utils.Error("no database set")
	}
	keyStr := strconv.Quote(utils.InterfaceToString(key))

	var kv ProjectGeneralStorage
	if db := db.Where("key = ?", keyStr).Where(
		"(expired_at IS NULL) OR (expired_at <= ?) OR (expired_at >= ?)",
		yakitZeroTime,
		time.Now(),
	).First(&kv); db.Error != nil {
		//log.Errorf("error for query[%v] general storage: %s", keyStr, db.Error)
		return nil, db.Error
	}
	return &kv, nil
}


func SetProjectKeyWithTTL(db *gorm.DB, key interface{}, value interface{}, seconds int) error {
	if db == nil {
		return utils.Error("no set database")
	}
	var keyStr = strconv.Quote(utils.InterfaceToString(key))
	var valueStr = strconv.Quote(utils.InterfaceToString(value))
	if db := db.Model(&ProjectGeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr, "expired_at": time.Now().Add(time.Duration(seconds) * time.Second),
	}).FirstOrCreate(&ProjectGeneralStorage{}); db.Error != nil {
		return utils.Errorf("create project storage kv failed: %s", db.Error)
	}
	return nil
}