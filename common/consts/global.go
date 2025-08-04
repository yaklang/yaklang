package consts

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"

	"go.uber.org/atomic"

	"github.com/yaklang/yaklang/common/utils"
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

	//control response max content-length
	GLOBAL_MAXSIZE_CONTENT_LENGTH = atomic.NewUint64(1024 * 1024 * 10)

	OnceYakitHome = new(sync.Once)

	GLOBAL_DB_SAVE_SYNC = utils.NewBool(false)

	GLOBAL_CALLER_CALL_PLUGIN_TIMEOUT = atomic.NewFloat64(300)

	// tls global config
	GLOBAL_TLS_MIN_VERSION uint16 = gmtls.VersionSSL30
	GLOBAL_TLS_MAX_VERSION uint16 = gmtls.VersionTLS13
)

func SimpleYakGlobalConfig() {
	GLOBAL_DB_SAVE_SYNC.SetTo(true)
}

const (
	YAK_PROJECT_DATA_DB_NAME_RECOVERED   = "default-yakit.db"
	YAK_PROFILE_PLUGIN_DB_NAME_RECOVERED = "yakit-profile-plugin.db"
)

var (
	Global_Tsl_Mutex = sync.Mutex{}
)

func GetGlobalTLSVersion() (uint16, uint16) {
	Global_Tsl_Mutex.Lock()
	defer Global_Tsl_Mutex.Unlock()
	return GLOBAL_TLS_MIN_VERSION, GLOBAL_TLS_MAX_VERSION
}

func SetGlobalTLSMinVersion(min uint16) {
	Global_Tsl_Mutex.Lock()
	defer Global_Tsl_Mutex.Unlock()
	GLOBAL_TLS_MIN_VERSION = min
}

func SetGlobalTLSMaxVersion(max uint16) {
	Global_Tsl_Mutex.Lock()
	defer Global_Tsl_Mutex.Unlock()
	GLOBAL_TLS_MAX_VERSION = max
}

func GetGlobalCallerCallPluginTimeout() float64 {
	return GLOBAL_CALLER_CALL_PLUGIN_TIMEOUT.Load()
}

func SetGlobalCallerCallPluginTimeout(i float64) {
	GLOBAL_CALLER_CALL_PLUGIN_TIMEOUT.Store(i)
}

func GetGlobalMaxContentLength() uint64 {
	return GLOBAL_MAXSIZE_CONTENT_LENGTH.Load()
}
func SetGlobalMaxContentLength(i uint64) {
	if i > uint64(1024*1024*10) {
		GLOBAL_MAXSIZE_CONTENT_LENGTH.Store(1024 * 1024 * 10)
		return
	}
	GLOBAL_MAXSIZE_CONTENT_LENGTH.Store(i)
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

func IsDevMode() bool {
	return YAK_VERSION == "dev"
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
	return filepath.Join(base, YAK_PROJECT_DATA_DB_NAME)
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

func GetDefaultSSAProjectDir() string {
	pt := filepath.Join(GetDefaultYakitBaseDir(), "ssa-projects")
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

func GetDefaultYakitEngineDir() string {
	pt := filepath.Join(GetDefaultYakitBaseDir(), "yak-engine")
	if !utils.IsDir(pt) {
		os.MkdirAll(pt, 0o777)
	}
	return pt
}

func GetDefaultYakitPprofDir() string {
	pt := filepath.Join(GetDefaultYakitBaseDir(), "pprof-log")
	if !utils.IsDir(pt) {
		os.MkdirAll(pt, 0o777)
	}
	return pt
}

func GetDefaultLibsDir() string {
	pt := filepath.Join(GetDefaultYakitProjectsDir(), "libs")
	if !utils.IsDir(pt) {
		os.MkdirAll(pt, 0o777)
	}
	return pt
}

func GetDefaultDownloadTempDir() string {
	pt := filepath.Join(GetDefaultYakitBaseTempDir(), "download")
	if !utils.IsDir(pt) {
		os.MkdirAll(pt, 0o777)
	}
	return pt
}
