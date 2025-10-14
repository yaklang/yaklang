package yakit

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/tlsutils"

	"github.com/jinzhu/copier"
	"github.com/jinzhu/gorm"
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
	if projectDB.HasTable(&schema.GeneralStorage{}) {
		var count int64
		projectDB.Model(&schema.GeneralStorage{}).Count(&count)
		if count > 0 {
			log.Infof("start auto migrate kv user cache: %v", count)
			for i := range YieldGeneralStorages(projectDB.Model(&schema.GeneralStorage{}), context.Background()) {
				kv := &schema.GeneralStorage{}
				err := copier.Copy(kv, i)
				if err != nil {
					log.Errorf("copier.Copy for kv failed: %s", err)
					continue
				}
				kv.Model = gorm.Model{}
				profileDB.Save(kv)
			}
			// 迁移之后移除缓存
			projectDB.Where("true").Delete(&schema.GeneralStorage{})
		}
	}

	log.Info("start to migrate yakscript")
	if projectDB.HasTable(&schema.YakScript{}) {
		var count int64
		projectDB.Model(&schema.YakScript{}).Count(&count)
		if count > 0 {
			log.Infof("start auto migrate yakscript cache: %v", count)
			for i := range YieldYakScripts(projectDB.Model(&schema.YakScript{}), context.Background()) {
				var s schema.YakScript
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
			projectDB.Where("true").Delete(&schema.YakScript{})
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
			for p := range YieldPayloads(projectDB.Where("`group` = ?", group).Model(&schema.Payload{}), context.Background()) {
				pStr, _ := strconv.Unquote(*p.Content)
				if pStr == "" {
					pStr = *p.Content
				}
				payloads = append(payloads, pStr)
			}
			SavePayloadGroup(profileDB, group, payloads)
			projectDB.Where("`group` = ?", group).Unscoped().Delete(&schema.Payload{})
		}
	}
	return nil
}

func GetProcessEnvKey(db *gorm.DB) []*schema.GeneralStorage {
	var keys []*schema.GeneralStorage

	db = db.Model(&schema.GeneralStorage{}).Where("process_env = true").Where(
		"(expired_at IS NULL) OR (expired_at <= ?) OR (expired_at >= ?)",
		yakitZeroTime,
		time.Now(),
	)
	if db.Find(&keys); db.Error != nil {
		log.Errorf("fetch process_env from general_storage failed: %s", db.Error)
	}
	return keys
}

var refreshLock = new(sync.Mutex)

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		RefreshProcessEnv(consts.GetGormProfileDatabase())
		return nil
	}, "refresh-process-env")
}

// RefreshProcessEnv 在数据库初始化的时候执行这个，可以快速更新本进程的环境变量
func RefreshProcessEnv(db *gorm.DB) {
	refreshLock.Lock()
	defer refreshLock.Unlock()

	TidyGeneralStorage(db)
	for _, key := range GetProcessEnvKey(db) {
		key.EnableProcessEnv()
	}

	// 刷新 DNS 服务器
	// consts.RefreshDNSServer()
}

func SetKey(db *gorm.DB, key interface{}, value interface{}) error {
	if db == nil {
		return utils.Error("no set database")
	}

	keyStr := strconv.Quote(utils.InterfaceToString(key))
	valueStr := strconv.Quote(utils.InterfaceToString(value))
	if valueStr == `""` {
		valueStr = ""
	}
	if db := db.Model(&schema.GeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr,
	}).FirstOrCreate(&schema.GeneralStorage{}); db.Error != nil {
		return utils.Errorf("create storage kv failed: %s", db.Error)
	}
	return nil
}

func InitKey(db *gorm.DB, key interface{}, verbose interface{}, env bool) error {
	if db == nil {
		return utils.Error("no set database")
	}

	keyStr := strconv.Quote(utils.InterfaceToString(key))
	valueStr := strconv.Quote(utils.InterfaceToString(verbose))
	if db := db.Model(&schema.GeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "verbose": valueStr, "process_env": env,
	}).FirstOrCreate(&schema.GeneralStorage{}); db.Error != nil {
		return utils.Errorf("create storage kv failed: %s", db.Error)
	}
	return nil
}

func SetKeyWithTTL(db *gorm.DB, key interface{}, value interface{}, seconds int) error {
	if db == nil {
		return utils.Error("no set database")
	}

	keyStr := strconv.Quote(utils.InterfaceToString(key))
	valueStr := strconv.Quote(utils.InterfaceToString(value))
	if db := db.Model(&schema.GeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr, "expired_at": time.Now().Add(time.Duration(seconds) * time.Second),
	}).FirstOrCreate(&schema.GeneralStorage{}); db.Error != nil {
		return utils.Errorf("create storage kv failed: %s", db.Error)
	}
	return nil
}

func SetKeyProcessEnv(db *gorm.DB, key interface{}, processEnv bool) {
	keyStr := strconv.Quote(utils.InterfaceToString(key))
	if db := db.Model(&schema.GeneralStorage{}).Where("key = ?", keyStr).Updates(map[string]interface{}{
		"process_env": processEnv,
	}); db.Error != nil {
		log.Errorf("update process env failed: %s", db.Error)
	}
}

func DelKey(db *gorm.DB, key interface{}) {
	if db := db.Where(`key = ?`, strconv.Quote(utils.InterfaceToString(key))).Unscoped().Delete(&schema.GeneralStorage{}); db.Error != nil {
		log.Errorf("del general storage failed: %s", db.Error)
	}
}

func GetKeyModel(db *gorm.DB, key interface{}) (*schema.GeneralStorage, error) {
	if db == nil {
		return nil, utils.Error("no database set")
	}

	keyStr := strconv.Quote(utils.InterfaceToString(key))

	var kv schema.GeneralStorage
	if db := db.Where("key = ?", keyStr).Where(
		"(expired_at IS NULL) OR (expired_at <= ?) OR (expired_at >= ?)",
		yakitZeroTime,
		time.Now(),
	).First(&kv); db.Error != nil {
		// log.Errorf("error for query[%v] general storage: %s", keyStr, db.Error)
		return nil, db.Error
	}
	return &kv, nil
}

// yaklang was born in 2019
var yakitZeroTime = time.Date(2018, 1, 1, 1, 1, 1, 0, time.UTC)

func Get(key interface{}) string {
	return GetKey(consts.GetGormProfileDatabase(), key)
}

func Set(key interface{}, value interface{}) {
	err := SetKey(consts.GetGormProfileDatabase(), key, value)
	if err != nil {
		log.Debugf("yakit.SetKey(consts.GetGormProfileDatabase(), key, value) failed: %s", err)
	}
}

func GetKey(db *gorm.DB, key interface{}) string {
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

func GetKeyFromProjectOrProfile(key interface{}) string {
	projectDB, profileDB := consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()
	// project first
	value := GetProjectKey(projectDB, key)
	if value == "" {
		value = GetKey(profileDB, key)
	}
	return value
}

func TidyGeneralStorage(db *gorm.DB) {
	if db == nil {
		return
	}
	if db := db.Model(&schema.GeneralStorage{}).Where(
		"(expired_at < ?) AND (expired_at > ?)",
		time.Now().Add(-(time.Hour * 24)),
		yakitZeroTime,
	).Unscoped().Delete(&schema.GeneralStorage{}); db.Error != nil {
		return
	}
}

func YieldGeneralStorages(db *gorm.DB, ctx context.Context) chan *schema.GeneralStorage {
	return bizhelper.YieldModel[*schema.GeneralStorage](ctx, db)
}

func GetNetworkConfig() *ypb.GlobalNetworkConfig {
	data := Get(consts.GLOBAL_NETWORK_CONFIG)
	if data == "" {
		return nil
	}
	config := &ypb.GlobalNetworkConfig{}
	err := json.Unmarshal([]byte(data), &config)
	if err != nil {
		log.Errorf("unmarshal global network config failed: %s", err)
		return nil
	}
	InitNetworkConfig(config)
	return config
}

func InitNetworkConfig(config *ypb.GlobalNetworkConfig) { // init some network config, for add new config which not allow be zero value.
	if config.MaxTlsVersion == 0 {
		config.MaxTlsVersion = tls.VersionTLS13
	}
	if config.MinTlsVersion == 0 {
		config.MinTlsVersion = tls.VersionSSL30
	}
	if config.CallPluginTimeout == 0 {
		config.CallPluginTimeout = float32(consts.GLOBAL_CALLER_CALL_PLUGIN_TIMEOUT.Load()) // use global default instead of previous 60s
	}
	if config.MaxContentLength == 0 {
		config.MaxContentLength = 1024 * 1024 * 10 // default 10M
	}
}

// LoadGlobalNetworkConfig load config from yakit config in db
func LoadGlobalNetworkConfig() {
	ConfigureNetWork(GetNetworkConfig())
}

func GetDefaultNetworkConfig() *ypb.GlobalNetworkConfig {
	defaultConfig := &ypb.GlobalNetworkConfig{
		DisableSystemDNS:  false,
		CustomDNSServers:  nil,
		DNSFallbackTCP:    false,
		DNSFallbackDoH:    false,
		CustomDoHServers:  nil,
		SkipSaveHTTPFlow:  false,
		AuthInfos:         make([]*ypb.AuthInfo, 0),
		DbSaveSync:        false,
		CallPluginTimeout: float32(consts.GLOBAL_CALLER_CALL_PLUGIN_TIMEOUT.Load()),
		MaxTlsVersion:     tls.VersionTLS13,
		MinTlsVersion:     tls.VersionSSL30,
		MaxContentLength:  1024 * 1024 * 10,
	}
	config := netx.NewBackupInitilizedReliableDNSConfig()
	defaultConfig.CustomDoHServers = config.SpecificDoH
	defaultConfig.CustomDNSServers = config.SpecificDNSServers
	defaultConfig.DNSFallbackDoH = config.FallbackDoH
	defaultConfig.DNSFallbackTCP = config.FallbackTCP
	defaultConfig.DisableSystemDNS = config.DisableSystemResolver
	defaultConfig.AiApiPriority = aispec.RegisteredAIGateways()
	return defaultConfig
}

// ConfigureNetWork configure network: update memory and database
func ConfigureNetWork(c *ypb.GlobalNetworkConfig) {
	if c == nil {
		return
	}
	defer func() {
		data, err := json.Marshal(c)
		if err != nil {
			log.Errorf("unmarshal global network config failed: %s", err)
		}
		Set(consts.GLOBAL_NETWORK_CONFIG, data)
	}()
	consts.GLOBAL_HTTP_FLOW_SAVE.SetTo(!c.GetSkipSaveHTTPFlow())
	consts.GLOBAL_DB_SAVE_SYNC.SetTo(c.GetDbSaveSync())
	consts.SetGlobalHTTPAuthInfo(c.GetAuthInfos())
	consts.SetGlobalMaxContentLength(c.GetMaxContentLength())
	consts.ClearThirdPartyApplicationConfig()
	for _, r := range c.GetAppConfigs() {
		consts.UpdateThirdPartyApplicationConfig(r)
	}
	consts.SetAIPrimaryType(c.GetPrimaryAIType())

	if c.GetCallPluginTimeout() > 0 {
		consts.SetGlobalCallerCallPluginTimeout(float64(c.GetCallPluginTimeout()))
	}

	netx.SetDefaultDNSOptions(
		netx.WithDNSFallbackDoH(c.DNSFallbackDoH),
		netx.WithDNSFallbackTCP(c.DNSFallbackTCP),
		netx.WithDNSDisableSystemResolver(c.DisableSystemDNS),
		netx.WithDNSSpecificDoH(c.CustomDoHServers...),
		netx.WithDNSServers(c.CustomDNSServers...),
		netx.WithDNSDisabledDomain(c.GetDisallowDomain()...),
	)

	netx.SetDefaultDialXConfig(
		netx.DialX_WithDisallowAddress(c.GetDisallowIPAddress()...),
		netx.DialX_WithProxy(c.GetGlobalProxy()...),
		netx.DialX_WithEnableSystemProxyFromEnv(c.GetEnableSystemProxyFromEnv()),
	)

	// 插件扫描黑白名单
	SetGlobalPluginScanLists(c.IncludePluginScanURIs, c.ExcludePluginScanURIs)

	consts.SetGlobalTLSMaxVersion(uint16(c.GetMaxTlsVersion()))
	consts.SetGlobalTLSMinVersion(uint16(c.GetMinTlsVersion()))
	netx.ResetPresetCertificates()
	for _, certs := range c.GetClientCertificates() {
		if len(certs.GetPkcs12Bytes()) > 0 {
			err := netx.LoadP12Bytes(certs.Pkcs12Bytes, string(certs.GetPkcs12Password()), certs.GetHost())
			if err != nil {
				log.Errorf("load p12 bytes failed: %s", err)
			}
		} else {
			p12bytes, err := tlsutils.BuildP12(certs.GetCrtPem(), certs.GetKeyPem(), "", certs.GetCaCertificates()...)
			if err != nil {
				log.Errorf("build p12 bytes failed: %s", err)
				continue
			}
			err = netx.LoadP12Bytes(p12bytes, "", certs.GetHost())
			if err != nil {
				log.Errorf("load p12 bytes failed: %s", err)
			}
		}
	}
}
