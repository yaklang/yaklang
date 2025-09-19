package yakit

import (
	"github.com/yaklang/yaklang/common/schema"
	"strconv"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	BARE_REQUEST_GROUP  = "FLOW_ID_TO_BARE_REQUEST"
	BARE_RESPONSE_GROUP = "FLOW_ID_TO_BARE_RESPONSE"
)

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("get post init database function")
			}
		}()
		if GetProjectKey(consts.GetGormProjectDatabase(), "fuzzer-list-cache") == "" {
			SetProjectKey(consts.GetGormProjectDatabase(), `fuzzer-list-cache`, GetKey(consts.GetGormProfileDatabase(), "fuzzer-list-cache"))
			DelKey(consts.GetGormProfileDatabase(), "fuzzer-list-cache")
		}
		return nil
	}, "check-yakit-fuzzer-list-cache")
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

func GetProjectKeyWithError(db *gorm.DB, key interface{}) (string, error) {
	kv, err := GetProjectKeyModel(db, key)
	if err != nil {
		return "", err
	}
	if kv.Value == "" {
		return "", utils.Errorf("value is empty")
	}
	v, err := strconv.Unquote(kv.Value)
	if err != nil {
		log.Errorf("unquote(general storage) value failed: %s", err)
		return kv.Value, nil
	}
	return v, nil
}

func SetProjectKeyWithGroup(db *gorm.DB, key interface{}, value interface{}, group string) error {
	if db == nil {
		return utils.Error("no set database")
	}

	keyStr := strconv.Quote(utils.InterfaceToString(key))
	valueStr := ""
	if value != "" {
		valueStr = strconv.Quote(utils.InterfaceToString(value))
	}
	if db := db.Model(&schema.ProjectGeneralStorage{}).Where(`key = ?`, keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr, "group": group,
	}).FirstOrCreate(&schema.ProjectGeneralStorage{}); db.Error != nil {
		return utils.Errorf("create project storage kv failed: %s", db.Error)
	}
	return nil
}

func SetProjectKey(db *gorm.DB, key interface{}, value interface{}) error {
	if db == nil {
		return utils.Error("no set database")
	}

	keyStr := strconv.Quote(utils.InterfaceToString(key))
	valueStr := ""
	if value != "" {
		valueStr = strconv.Quote(utils.InterfaceToString(value))
	}
	if db := db.Model(&schema.ProjectGeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr,
	}).FirstOrCreate(&schema.ProjectGeneralStorage{}); db.Error != nil {
		return utils.Errorf("create project storage kv failed: %s", db.Error)
	}
	return nil
}

func GetProjectKeyModel(db *gorm.DB, key interface{}) (*schema.ProjectGeneralStorage, error) {
	if db == nil {
		return nil, utils.Error("no database set")
	}
	keyStr := strconv.Quote(utils.InterfaceToString(key))

	var kv schema.ProjectGeneralStorage
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
	if db := db.Model(&schema.ProjectGeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr, "expired_at": time.Now().Add(time.Duration(seconds) * time.Second),
	}).FirstOrCreate(&schema.ProjectGeneralStorage{}); db.Error != nil {
		return utils.Errorf("create project storage kv failed: %s", db.Error)
	}
	return nil
}

func GetProjectKeyByWhere(db *gorm.DB, key []string) ([]*schema.ProjectGeneralStorage, error) {
	if db == nil {
		return nil, utils.Error("no database set")
	}
	var kv []*schema.ProjectGeneralStorage
	if db := db.Where("key in (?)", key).Where(
		"(expired_at IS NULL) OR (expired_at <= ?) OR (expired_at >= ?)",
		yakitZeroTime,
		time.Now(),
	).Find(&kv); db.Error != nil {
		return nil, db.Error
	}
	return kv, nil
}

func DeleteProjectKeyBareRequestAndResponse(db *gorm.DB) error {
	if db == nil {
		return utils.Error("no set database")
	}

	if db := db.Where("key LIKE ? or key LIKE ?", `%_request"`, `%_response"`).Unscoped().Delete(&schema.ProjectGeneralStorage{}); db.Error != nil {
		return utils.Errorf("delete project storage kv bare request and bare response failed: %s", db.Error)
	}
	return nil
}
