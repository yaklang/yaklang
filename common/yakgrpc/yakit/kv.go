package yakit

import (
	"context"
	"github.com/jinzhu/copier"
	"github.com/jinzhu/gorm"
	"os"
	"strconv"
	"sync"
	"time"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func MigrateLegacyDatabase() error {
	// 自动迁移数据库，主要在于个人配置数据与插件数据自动迁移
	projectDB := consts.GetGormProjectDatabase()
	profileDB := consts.GetGormProfileDatabase()

	log.Info("Start migrate general storage")
	if projectDB.HasTable(&GeneralStorage{}) {
		var count int64
		projectDB.Model(&GeneralStorage{}).Count(&count)
		if count > 0 {
			log.Infof("start auto migrate kv user cache: %v", count)
			for i := range YieldGeneralStorages(projectDB.Model(&GeneralStorage{}), context.Background()) {
				var kv = &GeneralStorage{}
				err := copier.Copy(kv, i)
				if err != nil {
					log.Errorf("copier.Copy for kv failed: %s", err)
					continue
				}
				kv.Model = gorm.Model{}
				profileDB.Save(kv)
			}
			// 迁移之后移除缓存
			projectDB.Where("true").Delete(&GeneralStorage{})
		}
	}

	log.Info("start to migrate yakscript")
	if projectDB.HasTable(&YakScript{}) {
		var count int64
		projectDB.Model(&YakScript{}).Count(&count)
		if count > 0 {
			log.Infof("start auto migrate yakscript cache: %v", count)
			for i := range YieldYakScripts(projectDB.Model(&YakScript{}), context.Background()) {
				var s YakScript
				err := copier.Copy(&s, i)
				if err != nil {
					log.Errorf("copier.Copy error: %s", err)
					continue
				}
				sp := &s
				sp.Model = gorm.Model{}
				err = CreateOrUpdateYakScriptByName(profileDB, sp.ScriptName, sp)
				if err != nil {
					log.Errorf("save yakscript failed: %s", err)
					continue
				}
			}
			// 迁移之后移除缓存
			projectDB.Where("true").Delete(&YakScript{})
		}
	}

	log.Info("start to migrate payload")
	if projectDB.HasTable(`payloads`) {
		for _, group := range PayloadGroups(projectDB) {
			switch group {
			case "user_top10", "pass_top25":
				log.Info("skip build-in group " + group)
				continue
			}
			log.Infof("start to migrate group: %v", group)
			var payloads []string
			for p := range YieldPayloads(projectDB.Where("`group` = ?", group).Model(&Payload{}), context.Background()) {
				pStr, _ := strconv.Unquote(p.Content)
				if pStr == "" {
					pStr = p.Content
				}
				payloads = append(payloads, pStr)
			}
			SavePayloadGroup(profileDB, group, payloads)
			projectDB.Where("`group` = ?", group).Unscoped().Delete(&Payload{})
		}
	}
	return nil
}

func init() {
	// RegisterPostInitDatabaseFunction(MigrateLegacyDatabase)
}

type GeneralStorage struct {
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

func (s *GeneralStorage) ToGRPCModel() *ypb.GeneralStorage {
	keyStr, _ := strconv.Unquote(s.Key)
	if keyStr == "" {
		keyStr = s.Key
	}
	valueStr, _ := strconv.Unquote(s.Value)
	if valueStr == "" {
		valueStr = s.Value
	}
	var expiredAt int64 = 0
	if !s.ExpiredAt.IsZero() {
		expiredAt = s.ExpiredAt.Unix()
	}

	if valueStr == `""` {
		valueStr = ""
	}
	return &ypb.GeneralStorage{
		Key:        utils.EscapeInvalidUTF8Byte([]byte(keyStr)),
		Value:      utils.EscapeInvalidUTF8Byte([]byte(valueStr)),
		ExpiredAt:  expiredAt,
		ProcessEnv: s.ProcessEnv,
		Verbose:    s.Verbose,
		Group:      "",
	}
}

func GetProcessEnvKey(db *gorm.DB) []*GeneralStorage {
	var keys []*GeneralStorage

	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&GeneralStorage{}).Where("process_env = true").Where(
		"(expired_at IS NULL) OR (expired_at <= ?) OR (expired_at >= ?)",
		yakitZeroTime,
		time.Now(),
	)
	if db.Find(&keys); db.Error != nil {
		log.Errorf("fetch process_env from general_storage failed: %s", db.Error)
	}
	return keys
}

func (s *GeneralStorage) EnableProcessEnv() {
	if s == nil {
		return
	}
	if !s.ProcessEnv {
		return
	}
	key := s
	keyStr, _ := strconv.Unquote(key.Key)
	if keyStr == "" {
		keyStr = key.Key
	}

	valueStr, _ := strconv.Unquote(key.Value)
	if valueStr == "" {
		valueStr = key.Value
	}
	err := os.Setenv(keyStr, valueStr)
	if err != nil {
		log.Errorf("set env[%s] failed: %s", keyStr, err)
	}
}

var refreshLock = new(sync.Mutex)

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		RefreshProcessEnv(consts.GetGormProfileDatabase())
		return nil
	})
}

// RefreshProcessEnv 在数据库初始化的时候执行这个，可以快速更新本进程的环境变量
func RefreshProcessEnv(db *gorm.DB) {
	refreshLock.Lock()
	defer refreshLock.Unlock()

	db = UserDataAndPluginDatabaseScope(db)

	TidyGeneralStorage(db)
	for _, key := range GetProcessEnvKey(db) {
		key.EnableProcessEnv()
	}

	// 刷新 DNS 服务器
	consts.RefreshDNSServer()
}

func SetKey(db *gorm.DB, key interface{}, value interface{}) error {
	db = UserDataAndPluginDatabaseScope(db)

	if db == nil {
		return utils.Error("no set database")
	}

	var keyStr = strconv.Quote(utils.InterfaceToString(key))
	var valueStr = strconv.Quote(utils.InterfaceToString(value))
	if valueStr == `""` {
		valueStr = ""
	}
	if db := db.Model(&GeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr,
	}).FirstOrCreate(&GeneralStorage{}); db.Error != nil {
		return utils.Errorf("create storage kv failed: %s", db.Error)
	}
	return nil
}

func InitKey(db *gorm.DB, key interface{}, verbose interface{}, env bool) error {
	db = UserDataAndPluginDatabaseScope(db)

	if db == nil {
		return utils.Error("no set database")
	}

	var keyStr = strconv.Quote(utils.InterfaceToString(key))
	var valueStr = strconv.Quote(utils.InterfaceToString(verbose))
	if db := db.Model(&GeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "verbose": valueStr, "process_env": env,
	}).FirstOrCreate(&GeneralStorage{}); db.Error != nil {
		return utils.Errorf("create storage kv failed: %s", db.Error)
	}
	return nil
}

func SetKeyWithTTL(db *gorm.DB, key interface{}, value interface{}, seconds int) error {
	db = UserDataAndPluginDatabaseScope(db)

	if db == nil {
		return utils.Error("no set database")
	}

	var keyStr = strconv.Quote(utils.InterfaceToString(key))
	var valueStr = strconv.Quote(utils.InterfaceToString(value))
	if db := db.Model(&GeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr, "expired_at": time.Now().Add(time.Duration(seconds) * time.Second),
	}).FirstOrCreate(&GeneralStorage{}); db.Error != nil {
		return utils.Errorf("create storage kv failed: %s", db.Error)
	}
	return nil
}

func SetKeyProcessEnv(db *gorm.DB, key interface{}, processEnv bool) {
	db = UserDataAndPluginDatabaseScope(db)

	var keyStr = strconv.Quote(utils.InterfaceToString(key))
	if db := db.Model(&GeneralStorage{}).Where("key = ?", keyStr).Updates(map[string]interface{}{
		"process_env": processEnv,
	}); db.Error != nil {
		log.Errorf("update process env failed: %s", db.Error)
	}
}

func DelKey(db *gorm.DB, key interface{}) {
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Where(`key = ?`, strconv.Quote(utils.InterfaceToString(key))).Unscoped().Delete(&GeneralStorage{}); db.Error != nil {
		log.Errorf("del general storage failed: %s", db.Error)
	}
}

func GetKeyModel(db *gorm.DB, key interface{}) (*GeneralStorage, error) {
	db = UserDataAndPluginDatabaseScope(db)

	if db == nil {
		return nil, utils.Error("no database set")
	}

	keyStr := strconv.Quote(utils.InterfaceToString(key))

	var kv GeneralStorage
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

// yaklang was born in 2019
var yakitZeroTime = time.Date(2018, 1, 1, 1, 1, 1, 0, time.UTC)

func GetKey(db *gorm.DB, key interface{}) string {
	db = UserDataAndPluginDatabaseScope(db)

	kv, err := GetKeyModel(db, key)
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

func TidyGeneralStorage(db *gorm.DB) {
	db = UserDataAndPluginDatabaseScope(db)

	if db == nil {
		return
	}
	if db := db.Model(&GeneralStorage{}).Where(
		"(expired_at < ?) AND (expired_at > ?)",
		time.Now().Add(-(time.Hour * 24)),
		yakitZeroTime,
	).Unscoped().Delete(&GeneralStorage{}); db.Error != nil {
		return
	}
}

func YieldGeneralStorages(db *gorm.DB, ctx context.Context) chan *GeneralStorage {
	outC := make(chan *GeneralStorage)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*GeneralStorage
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}
