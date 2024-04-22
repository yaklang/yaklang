package consts

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/permutil"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	YAK_BRIDGE_REMOTE_REVERSE_ADDR = "YAK_BRIDGE_REMOTE_REVERSE_ADDR"
	YAK_BRIDGE_LOCAL_REVERSE_ADDR  = "YAK_BRIDGE_LOCAL_REVERSE_ADDR"
	YAK_BRIDGE_ADDR                = "YAK_BRIDGE_ADDR"
	YAK_BRIDGE_SECRET              = "YAK_BRIDGE_SECRET"
	YAK_DNSLOG_BRIDGE_ADDR         = "YAK_DNSLOG_BRIDGE_ADDR"
	YAK_DNSLOG_BRIDGE_PASSWORD     = "YAK_DNSLOG_BRIDGE_PASSWORD"
	// 这个是用于绑定 runtime id 到 Risk 上的方式
	YAK_RUNTIME_ID             = "YAK_RUNTIME_ID"
	YAKIT_PLUGIN_ID            = "YAKIT_PLUGIN_ID"
	DefaultUserAgent           = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36"
	defaultPublicReverseServer = "ns1.cybertunnel.run:64333"
	YAK_PROJECT_DATA_DB_NAME   = "default-yakit.db"
	YAK_PROFILE_PLUGIN_DB_NAME = "yakit-profile-plugin.db"
	YAK_VERSION                = "dev"
	YAK_ONLINE_BASEURL         = "https://www.yaklang.com"
	YAK_ONLINE_BASEURL_PROXY   = ""

	CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME = "YAK_DEFAULT_PROJECT_DATABASE_NAME"
	CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME = "YAK_DEFAULT_PROFILE_DATABASE_NAME"
	CONST_YAK_EXTRA_DNS_SERVERS             = "YAK_EXTRA_DNS_SERVERS"
	CONST_YAK_OVERRIDE_DNS_SERVERS          = "YAK_OVERRIDE_DNS_SERVERS"
	CONST_YAK_SAVE_HTTPFLOW                 = "YAK_SAVE_HTTPFLOW"

	// 全局网络配置
	GLOBAL_NETWORK_CONFIG      = "GLOBAL_NETWORK_CONFIG"
	GLOBAL_NETWORK_CONFIG_INIT = "GLOBAL_NETWORK_CONFIG_INIT"

	// default  http flow save config
	GLOBAL_HTTP_FLOW_SAVE = utils.NewBool(true)

	AuthInfoMutex         = new(sync.Mutex)
	GLOBAL_HTTP_AUTH_INFO []*ypb.AuthInfo

	OnceYakitHome = new(sync.Once)
)

func SetGlobalHTTPAuthInfo(info []*ypb.AuthInfo) {
	AuthInfoMutex.Lock()
	defer AuthInfoMutex.Unlock()
	GLOBAL_HTTP_AUTH_INFO = info
}

func GetAuthTypeList(authType string) []string {
	switch strings.ToLower(authType) {
	case "negotiate":
		return []string{"negotiate", "ntlm", "kerberos"}
	default:
		return []string{strings.ToLower(authType)}
	}
}

func GetGlobalHTTPAuthInfo(host, authType string) *ypb.AuthInfo {
	AuthInfoMutex.Lock()
	defer AuthInfoMutex.Unlock()
	anyAuthInfo := new(ypb.AuthInfo)
	gotAnyTypeAuth := false
	for _, info := range GLOBAL_HTTP_AUTH_INFO {
		if !info.Forbidden && utils.HostContains(info.Host, host) {
			if utils.StringSliceContain(GetAuthTypeList(authType), info.AuthType) {
				return info
			}
			if info.AuthType == "any" && !gotAnyTypeAuth { // if got any type auth, save it, just first
				anyAuthInfo = info
				anyAuthInfo.AuthType = authType
				gotAnyTypeAuth = true
			}
		}
	}
	if gotAnyTypeAuth { // if got any type auth, return it
		return anyAuthInfo
	}
	return nil
}

func GetCurrentYakitPluginID() string {
	return utils.EscapeInvalidUTF8Byte([]byte(os.Getenv(YAKIT_PLUGIN_ID)))
}

func GetDefaultSaveHTTPFlowFromEnv() bool {
	ok, _ := strconv.ParseBool(os.Getenv(CONST_YAK_SAVE_HTTPFLOW))
	return ok
}

func GetProjectDatabaseNameFromEnv() string {
	return os.Getenv(CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME)
}

func GetProfileDatabaseNameFromEnv() string {
	return os.Getenv(CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME)
}

func GetExtraDNSServers() []string {
	return utils.RemoveRepeatStringSlice(utils.PrettifyListFromStringSplited(os.Getenv(CONST_YAK_EXTRA_DNS_SERVERS), ","))
}

const (
	YAK_PROJECT_DATA_DB_NAME_RECOVERED   = "default-yakit.db"
	YAK_PROFILE_PLUGIN_DB_NAME_RECOVERED = "yakit-profile-plugin.db"
)

func GetOnlineBaseUrl() string {
	return YAK_ONLINE_BASEURL
}

func SetOnlineBaseUrl(u string) {
	YAK_ONLINE_BASEURL = u
}

func GetOnlineBaseUrlProxy() string {
	return YAK_ONLINE_BASEURL_PROXY
}

func SetOnlineBaseUrlProxy(u string) {
	YAK_ONLINE_BASEURL_PROXY = u
}

func GetDefaultPublicReverseServer() string {
	addr := os.Getenv(YAK_DNSLOG_BRIDGE_ADDR)
	if addr == "" {
		return defaultPublicReverseServer
	}
	return addr
}

func GetYakVersion() string {
	return YAK_VERSION
}

func SetYakVersion(v string) {
	YAK_VERSION = v
}

func GetDefaultPublicReverseServerPassword() string {
	secret := os.Getenv(YAK_DNSLOG_BRIDGE_PASSWORD)
	if secret == "" {
		return ""
	}
	return secret
}

func SetDefaultPublicReverseServer(addr string) {
	os.Setenv(YAK_DNSLOG_BRIDGE_ADDR, addr)
}

func SetDefaultPublicReverseServerPassword(addr string) {
	os.Setenv(YAK_DNSLOG_BRIDGE_PASSWORD, addr)
}

func GetDefaultYakitProjectDatabase(base string) string {
	if filepath.IsAbs(YAK_PROJECT_DATA_DB_NAME) {
		return YAK_PROJECT_DATA_DB_NAME
	}

	blocks := filepath.SplitList(YAK_PROJECT_DATA_DB_NAME)
	paths := make([]string, 1+len(blocks))
	paths[0] = base
	for i := 0; i < len(blocks); i++ {
		paths[i+1] = blocks[i]
	}
	return filepath.Join(paths...)
}

func GetDefaultYakitPluginDatabase(base string) string {
	if filepath.IsAbs(YAK_PROFILE_PLUGIN_DB_NAME) {
		return YAK_PROFILE_PLUGIN_DB_NAME
	}

	blocks := filepath.SplitList(YAK_PROFILE_PLUGIN_DB_NAME)
	paths := make([]string, 1+len(blocks))
	paths[0] = base
	for i := 0; i < len(blocks); i++ {
		paths[i+1] = blocks[i]
	}
	return filepath.Join(paths...)
}

func SetDefaultYakitProjectDatabaseName(i string) {
	if i == "" {
		YAK_PROJECT_DATA_DB_NAME = YAK_PROJECT_DATA_DB_NAME_RECOVERED
		return
	}
	YAK_PROJECT_DATA_DB_NAME = i
}

func SetDefaultYakitProfileDatabaseName(i string) {
	if i == "" {
		YAK_PROFILE_PLUGIN_DB_NAME = YAK_PROFILE_PLUGIN_DB_NAME_RECOVERED
		return
	}
	YAK_PROFILE_PLUGIN_DB_NAME = i
}

func GetDefaultYakitBaseDir() string {
	OnceYakitHome.Do(GetRegistryYakitHome)
	// 这个检测默认数据库
	if os.Getenv("YAKIT_HOME") != "" {
		return os.Getenv("YAKIT_HOME")
	}

	return filepath.Join(utils.GetHomeDirDefault("."), "yakit-projects")
}

func TempFile(pattern string) (*os.File, error) {
	return ioutil.TempFile(GetDefaultYakitBaseTempDir(), pattern)
}

func TempFileFast(datas ...any) string {
	f, err := TempFile("yakit-*.tmp")
	if err != nil {
		log.Errorf("create temp file error: %v", err)
		return ""
	}
	defer f.Close()
	data := bytes.Join(
		lo.Map(datas, func(item any, _ int) []byte {
			return codec.AnyToBytes(item)
		}),
		[]byte("\r\n"),
	)
	f.Write(data)
	return f.Name()
}

func GetDefaultYakitBaseTempDir() string {
	OnceYakitHome.Do(GetRegistryYakitHome)

	if os.Getenv("YAKIT_HOME") != "" {
		dirName := filepath.Join(os.Getenv("YAKIT_HOME"), "temp")
		if b, _ := utils.PathExists(dirName); !b {
			os.MkdirAll(dirName, 0o777)
		}
		return dirName
	}

	a := filepath.Join(utils.GetHomeDirDefault("."), "yakit-projects", "temp")
	if utils.GetFirstExistedPath(a) == "" {
		_ = os.MkdirAll(a, 0o777)
	}
	return a
}

func GetDefaultBaseHomeDir() string {
	yHome := GetDefaultYakitBaseDir()
	return filepath.Dir(yHome)
}

func GetDefaultYakitPayloadsDir() string {
	pt := filepath.Join(GetDefaultYakitBaseDir(), "payloads")
	if !utils.IsDir(pt) {
		os.MkdirAll(pt, 0o777)
	}
	return pt
}

func GetDefaultYakitProjectsDir() string {
	pt := filepath.Join(GetDefaultYakitBaseDir(), "projects")
	if !utils.IsDir(pt) {
		os.MkdirAll(pt, 0o777)
	}
	return pt
}

var (
	gormDatabase        *gorm.DB
	initOnce            = new(sync.Once)
	gormPluginDatabase  *gorm.DB
	gormCVEDatabase     *gorm.DB
	gormCVEDescDatabase *gorm.DB
)

func SetGormProjectDatabase(d *gorm.DB) {
	log.Info("load gorm database connection")
	gormDatabase = d
}

func GetGormProfileDatabase() *gorm.DB {
	return gormPluginDatabase
}

func InitilizeDatabase(projectDatabase string, profileDBName string) {
	projectName := GetProjectDatabaseNameFromEnv()
	if projectDatabase != "" {
		projectName = projectDatabase
	}
	profileDatabase := GetProfileDatabaseNameFromEnv()
	if profileDBName != "" {
		profileDatabase = profileDBName
	}
	SetDefaultYakitProjectDatabaseName(projectName)
	SetDefaultYakitProfileDatabaseName(profileDatabase)
	GetGormProjectDatabase()
}

func GetCVEDatabasePath() string {
	return filepath.Join(GetDefaultYakitBaseDir(), "default-cve.db")
}

func GetCVEDescriptionDatabasePath() string {
	return filepath.Join(GetDefaultYakitBaseDir(), "default-cve-description.db")
}

func GetCVEDescriptionDatabaseGzipPath() string {
	return filepath.Join(GetDefaultYakitBaseDir(), "default-cve-description.db.gzip")
}

func GetCVEDatabaseGzipPath() string {
	return filepath.Join(GetDefaultYakitBaseDir(), "default-cve.db.gzip")
}

func SetGormCVEDatabase(db *gorm.DB) {
	if gormCVEDatabase == nil {
		gormCVEDatabase = db
		return
	}
	gormCVEDatabase.Close()
	gormCVEDatabase = db
	return
}

func GetFfmpegPath() string {
	defaultPath := GetDefaultYakitProjectsDir()
	var paths []string
	if runtime.GOOS == "darwin" {
		paths = append(paths, filepath.Join(defaultPath, "libs", "ffmpeg"))
		paths = append(paths, filepath.Join(defaultPath, "base", "ffmpeg"))
		paths = append(paths, filepath.Join(defaultPath, "engine", "ffmpeg"))
		paths = append(paths, filepath.Join(defaultPath, "ffmpeg"))
		paths = append(paths, "ffmpeg")
		paths = append(paths, filepath.Join("/", "usr", "local", "bin", "ffmpeg"))
		paths = append(paths, filepath.Join("/", "bin", "ffmpeg"))
		paths = append(paths, filepath.Join("/", "usr", "bin", "ffmpeg"))
	}

	if runtime.GOOS == "windows" {
		paths = append(paths, filepath.Join(defaultPath, "base", "ffmpeg.exe"))
		paths = append(paths, filepath.Join(defaultPath, "libs", "ffmpeg.exe"))
		paths = append(paths, filepath.Join(defaultPath, "engine", "ffmpeg.exe"))
		paths = append(paths, filepath.Join(defaultPath, "ffmpeg.exe"))
		paths = append(paths, "ffmpeg.exe")
	}
	return utils.GetFirstExistedFile(paths...)
}

func GetVulinboxPath() string {
	defaultPath := GetDefaultYakitProjectsDir()
	var paths []string
	if runtime.GOOS == "darwin" {
		paths = append(paths, filepath.Join(defaultPath, "libs", "vulinbox"))
		paths = append(paths, filepath.Join(defaultPath, "base", "vulinbox"))
		paths = append(paths, filepath.Join(defaultPath, "engine", "vulinbox"))
		paths = append(paths, filepath.Join(defaultPath, "vulinbox"))
		paths = append(paths, "vulinbox")
		paths = append(paths, filepath.Join("/", "usr", "local", "bin", "vulinbox"))
		paths = append(paths, filepath.Join("/", "bin", "vulinbox"))
		paths = append(paths, filepath.Join("/", "usr", "bin", "vulinbox"))
	}

	if runtime.GOOS == "windows" {
		paths = append(paths, filepath.Join(defaultPath, "base", "vulinbox.exe"))
		paths = append(paths, filepath.Join(defaultPath, "libs", "vulinbox.exe"))
		paths = append(paths, filepath.Join(defaultPath, "engine", "vulinbox.exe"))
		paths = append(paths, filepath.Join(defaultPath, "vulinbox.exe"))
		paths = append(paths, "vulinbox.exe")
	}
	return utils.GetFirstExistedFile(paths...)
}

func GetGormCVEDatabase() *gorm.DB {
	if gormCVEDatabase == nil {
		var err error
		gormCVEDatabase, err = InitializeCVEDatabase()
		if err != nil {
			log.Debugf("initialize cve db failed: %s", err)
		}
	}
	return gormCVEDatabase
}

func GetGormCVEDescriptionDatabase() *gorm.DB {
	if gormCVEDescDatabase == nil {
		var err error
		gormCVEDescDatabase, err = InitializeCVEDescriptionDatabase()
		if err != nil {
			log.Debugf("initialize cve db failed: %s", err)
		}
	}
	return gormCVEDescDatabase
}

func InitializeCVEDescriptionDatabase() (*gorm.DB, error) {
	log.Info("start to initialize cve desc db")
	cveDescDb := GetCVEDescriptionDatabasePath()
	cveDescGzip := GetCVEDescriptionDatabaseGzipPath()
	ret := utils.GetFirstExistedFile(cveDescDb, cveDescGzip)
	log.Infof("found first existed file: %s", ret)
	if ret == cveDescGzip {
		log.Infof("start to un-gzip %v", cveDescGzip)
		fp, err := os.Open(cveDescGzip)
		if err != nil {
			return nil, err
		}
		defer fp.Close()
		dbFp, err := os.OpenFile(cveDescDb, os.O_RDWR|os.O_CREATE, 0o666)
		if err != nil {
			return nil, err
		}
		defer dbFp.Close()

		gr, err := gzip.NewReader(fp)
		if err != nil {
			return nil, utils.Errorf("un-gzip for %v failed: %s", cveDescDb, err)
		}
		io.Copy(dbFp, gr)
		log.Infof("finished to create: %s", cveDescGzip)
	}
	if ret == "" {
		return nil, utils.Error("no cve description db found")
	}
	return gorm.Open("sqlite3", cveDescDb)
}

func InitializeCVEDatabase() (*gorm.DB, error) {
	cveDatabase := GetCVEDatabasePath()
	cveDatabaseEncrypted := GetCVEDatabaseGzipPath()

	// 自动化解压
	if utils.GetFirstExistedFile(cveDatabase) == "" && utils.GetFirstExistedFile(cveDatabaseEncrypted) != "" {
		fp, err := os.Open(cveDatabaseEncrypted)
		if err != nil {
			return nil, err
		}
		dbFp, err := os.OpenFile(cveDatabase, os.O_RDWR|os.O_CREATE, 0o666)
		if err != nil {
			fp.Close()
			return nil, err
		}
		gr, err := gzip.NewReader(fp)
		if err != nil {
			fp.Close()
			dbFp.Close()
			return nil, utils.Errorf("un-gzip for %v failed: %s", cveDatabase, err)
		}
		io.Copy(dbFp, gr)
		dbFp.Close()
		fp.Close()
	}

	var err error
	gormCVEDatabase, err = gorm.Open("sqlite3", cveDatabase)
	if err != nil {
		return nil, utils.Errorf(`cve database[%v] failed: %s`, cveDatabase, err)
	}
	// issue #725 这一步要在添加索引之前，否则会从添加索引的 return 语句中返回
	// 如果没有表就删除 open 产生的文件
	if !gormCVEDatabase.HasTable(&cveresources.CVE{}) {
		gormCVEDatabase.Close()
		gormCVEDatabase = nil
		err := os.RemoveAll(cveDatabase)
		if err != nil {
			return nil, utils.Errorf("remove [%s] failed: %v", cveDatabase, err)
		}
		return nil, utils.Errorf("cve database failed: %s", "empty")
	}
	var count int
	_ = gormCVEDatabase.DB().QueryRow("PRAGMA index_info(idx_cves_cve)").Scan(&count)
	// 如果没有索引就添加
	if count == 0 {
		err = gormCVEDatabase.Model(&cveresources.CVE{}).AddIndex("idx_cves_cve", "CVE").Error
		if err != nil {
			return nil, utils.Errorf(`add index  failed: %s`, err)
		}
	}
	var cweCount int
	err = gormCVEDatabase.Model(&cveresources.CWE{}).AddIndex("idx_cwes_id_str", "IdStr").Error
	if cweCount == 0 {
		if err != nil {
			return nil, utils.Errorf(`cwe database[%v] failed: %s`, cveDatabase, err)
		}
	}
	return gormCVEDatabase, nil
}

func doDBPatch() {
	err := gormDatabase.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_source"
ON "http_flows" (
  "source_type" ASC
);`).Error
	if err != nil {
		log.Warnf("failed to add index on http_flows.source_type: %v", err)
	}

	err = gormDatabase.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_tags"
ON "http_flows" (
  "tags" ASC
);`).Error
	if err != nil {
		log.Warnf("failed to add index on table: http_flows.tags: %v", err)
	}
}

func doDBRiskPatch() {
	err := gormDatabase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_id ON risks(id);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.id: %v", err)
	}
	err = gormDatabase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_is_read ON risks(is_read);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.is_read: %v", err)
	}

	err = gormDatabase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_risk_type ON risks(risk_type);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.risk_type: %v", err)
	}

	err = gormDatabase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_ip ON risks(ip);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.ip: %v", err)
	}
}

func GetGormProjectDatabase() *gorm.DB {
	initOnce.Do(func() {
		log.Debug("start to loading gorm project/profile database")
		var (
			exist               bool
			err                 error
			baseDir             = GetDefaultYakitBaseDir()
			projectDatabaseName = GetDefaultYakitProjectDatabase(baseDir)
			profileDatabaseName = GetDefaultYakitPluginDatabase(baseDir)
		)

		if exist, err = utils.PathExists(baseDir); err != nil {
			log.Errorf("check dir[%v] if exist failed: %s", baseDir, err)
		} else if !exist {
			err = os.MkdirAll(baseDir, 0o777)
			if err != nil {
				log.Errorf("make dir[%v] failed: %s", baseDir, err)
			}
		}

		if runtime.GOOS == "darwin" {
			if permutil.IsFileUnreadAndUnWritable(projectDatabaseName) {
				log.Info("打开项目数据库遇到权限问题，尝试自主修复数据库权限错误")
				if err := permutil.DarwinSudo(
					"chmod +rw "+strconv.Quote(projectDatabaseName),
					permutil.WithVerbose("修复 Yakit 项目数据库权限"),
				); err != nil {
					log.Errorf("sudo chmod +rw %v failed: %v", strconv.Quote(projectDatabaseName), err)
				}
				if permutil.IsFileUnreadAndUnWritable(projectDatabaseName) {
					log.Errorf("No Permission for %v", projectDatabaseName)
				}
			}

			/*修复profile db*/
			if permutil.IsFileUnreadAndUnWritable(profileDatabaseName) {
				log.Info("打开用户插件数据库遇到权限问题，尝试自主修复")
				if err := permutil.DarwinSudo(
					"chmod +rw "+strconv.Quote(profileDatabaseName),
					permutil.WithVerbose("修复 Yakit 用户数据库权限"),
				); err != nil {
					log.Errorf("sudo chmod +rw %v failed: %v", strconv.Quote(profileDatabaseName), err)
				}
				if permutil.IsFileUnreadAndUnWritable(profileDatabaseName) {
					log.Errorf("No Permission for %v", profileDatabaseName)
				}
			}
		}

		/* 先创建插件数据库 */
		if utils.IsDir(profileDatabaseName) {
			os.RemoveAll(profileDatabaseName)
		}
		gormPluginDatabase, err = gorm.Open("sqlite3", profileDatabaseName)
		if err != nil {
			log.Errorf("init plugin-db[%v] failed: %s", profileDatabaseName, err)
		} else {
			configureAndOptimizeDB(gormPluginDatabase)
			err := os.Chmod(profileDatabaseName, 0o666)
			if err != nil {
				log.Errorf("chmod +rw failed: %s", err)
			}
		}

		/* 再创建项目数据库 */
		if utils.IsDir(projectDatabaseName) {
			os.RemoveAll(projectDatabaseName)
		}
		gormDatabase, err = gorm.Open("sqlite3", projectDatabaseName)
		if err != nil {
			log.Errorf("init db[%v] failed: %s", projectDatabaseName, err)
		} else {
			configureAndOptimizeDB(gormDatabase)
			err := os.Chmod(projectDatabaseName, 0o666)
			if err != nil {
				log.Errorf("chmod +rw failed: %s", err)
			}
		}

		doDBPatch()
		doDBRiskPatch()
	})
	return gormDatabase
}

func configureAndOptimizeDB(db *gorm.DB) {
	db.DB().SetConnMaxLifetime(time.Hour)
	db.DB().SetMaxIdleConns(10)
	db.DB().SetMaxOpenConns(100)

	db.Exec("PRAGMA synchronous = OFF;")
	// db.Exec("PRAGMA locking_mode = EXCLUSIVE;")
	db.Exec("PRAGMA journal_mode = OFF;")
	db.Exec("PRAGMA temp_store = MEMORY;")
	db.Exec("PRAGMA cache_size = 8000;")
	db.Exec("PRAGMA busy_timeout = 10000;")
}
