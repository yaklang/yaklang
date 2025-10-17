package yaklib

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	YAKIT_PLUGIN_TYPE_YAK         = "yak"
	YAKIT_PLUGIN_TYPE_NUCLEI      = "nuclei"
	YAKIT_PLUGIN_TYPE_MITM        = "mitm"
	YAKIT_PLUGIN_TYPE_PORTSCAN    = "port-scan"
	YAKIT_PLUGIN_TYPE_CODEC       = "codec"
	YAKIT_PLUGIN_TYPE_PACKET_HACK = "packet-hack"
)

func saveYakitPlugin(scriptName string, typeStr string, content interface{}) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Error("empty database")
	}
	y, _ := yakit.GetYakScriptByName(db, scriptName)
	if y != nil {
		return utils.Errorf("existed plugin name: %s", scriptName)
	}

	return yakit.CreateOrUpdateYakScriptByName(db, scriptName, &schema.YakScript{
		ScriptName: scriptName,
		Type:       typeStr,
		Content:    utils.InterfaceToString(content),
		Level:      "middle",
	})
}

func saveHTTPFlowFromRaw(url string, req, rsp []byte) error {
	return saveHTTPFlowFromRawWithType(url, req, rsp, "basic-crawler")
}

func saveHTTPFlowFromRawWithType(url string, req, rsp []byte, typeStr string) error {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Error("empty database")
	}
	https := false
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "wss://") {
		https = true
	}
	// basic-crawler
	_, err := yakit.SaveFromHTTPFromRaw(db, https, req, rsp, typeStr, url, "")
	return err
}

func saveHTTPFlowFromRawWithOption(url string, req, rsp []byte, exOption ...yakit.CreateHTTPFlowOptions) error {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Error("empty database")
	}
	https := false
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "wss://") {
		https = true
	}
	extOpts := []yakit.CreateHTTPFlowOptions{
		yakit.CreateHTTPFlowWithHTTPS(https), yakit.CreateHTTPFlowWithRequestRaw(req), yakit.CreateHTTPFlowWithResponseRaw(rsp), yakit.CreateHTTPFlowWithURL(url),
	}
	extOpts = append(extOpts, exOption...)
	flow, err := yakit.CreateHTTPFlow(extOpts...)
	if err != nil {
		return err
	}
	err = yakit.CreateOrUpdateHTTPFlow(db, flow.CalcHash(), flow)
	if err != nil {
		return err
	}
	return nil
}

var DatabaseExports = map[string]interface{}{
	// Download IP
	"DownloadGeoIP": DownloadMMDB,
	"QueryIPCity":   QueryIP,
	"QueryIPForIPS": QueryIPForISP,

	// 写入资产
	"SaveHTTPFlowFromRaw":            saveHTTPFlowFromRaw,
	"SaveHTTPFlowFromRawWithType":    saveHTTPFlowFromRawWithType,
	"SaveHTTPFlowFromNative":         saveCrawler,
	"SaveHTTPFlowFromNativeWithType": saveHTTPFlowWithType,
	"SaveHTTPFlowFromRawWithOption":  saveHTTPFlowFromRawWithOption,
	"SaveHTTPFlowInstance":           saveHTTPFlowInstance,
	"SavePortFromResult":             savePortFromObj,
	"SaveDomain":                     saveDomain,
	"SavePayload":                    savePayloads,
	"SavePayloadByFile":              savePayloadByFile,

	// 保存插件内容
	"YAKIT_PLUGIN_TYPE_NUCLEI":      YAKIT_PLUGIN_TYPE_NUCLEI,
	"YAKIT_PLUGIN_TYPE_YAK":         YAKIT_PLUGIN_TYPE_YAK,
	"YAKIT_PLUGIN_TYPE_MITM":        YAKIT_PLUGIN_TYPE_MITM,
	"YAKIT_PLUGIN_TYPE_PORTSCAN":    YAKIT_PLUGIN_TYPE_PORTSCAN,
	"YAKIT_PLUGIN_TYPE_CODEC":       YAKIT_PLUGIN_TYPE_CODEC,
	"YAKIT_PLUGIN_TYPE_PACKET_HACK": YAKIT_PLUGIN_TYPE_PACKET_HACK,
	"SaveYakitPlugin":               saveYakitPlugin,

	// HTTP
	"QueryUrlsByKeyword":      queryUrlsByKeyword,
	"QueryUrlsAll":            queryAllUrls,
	"QueryHTTPFlowsByKeyword": queryHTTPFlowByKeyword,
	"QueryHTTPFlowsAll": func() chan *schema.HTTPFlow {
		return queryHTTPFlowByKeyword("")
	},
	"QueryPortsByUpdatedAt":       queryPortsByUpdatedAt,
	"QueryPortsByTaskName":        queryPortsByTaskName,
	"QueryPortsByRuntimeId":       queryPortsByRuntimeId,
	"QueryHTTPFlowsByID":          queryHTTPFlowByID,
	"QueryHostPortByNetwork":      queryHostPortByNetwork,
	"QueryHostPortByKeyword":      queryHostAssetByNetwork,
	"QueryHostsByDomain":          queryHostAssetByDomainKeyword,
	"QueryDomainsByNetwork":       queryDomainAssetByNetwork,
	"QueryDomainsByDomainKeyword": queryDomainAssetByDomainKeyword,
	"QueryDomainsByTitle":         queryDomainAssetByHTMLTitle,
	"QueryPayloadGroups":          getPayloadGroups,
	"GetAllPayloadGroupsName":     getAllPayloadGroupsName,
	"DeletePayloadByGroup":        deletePayloadByGroup,
	"YieldPayload":                YieldPayload,
	"GetProjectKey": func(k any) string {
		return yakit.GetProjectKey(consts.GetGormProjectDatabase(), k)
	},
	"SetProjectKey": func(k, v any) error {
		return yakit.SetProjectKey(consts.GetGormProjectDatabase(), k, v)
	},
	"SetKey": func(k, v interface{}) error {
		return yakit.SetKey(consts.GetGormProfileDatabase(), k, v)
	},
	"SetKeyWithTTL": func(k, v any, ttl int) error {
		return yakit.SetKeyWithTTL(consts.GetGormProfileDatabase(), k, v, ttl)
	},
	"GetKey": func(k interface{}) string {
		return yakit.GetKey(consts.GetGormProfileDatabase(), k)
	},
	"DelKey": func(k interface{}) {
		yakit.DelKey(consts.GetGormProfileDatabase(), k)
	},

	"GetYakitPluginByName": queryYakitPluginByName,

	// 脚本中导入特定格式菜单栏
	"SaveYakitMenuItemByBatchExecuteConfig": saveYakitMenuItemByBatchExecuteConfig,
	"DeleteYakitMenuItemAll":                deleteYakitMenuItemAll,

	"YieldYakScriptAll":     _yieldYakScript,
	"DeleteYakScriptByName": _deleteYakScriptByName,

	// CreateTemporaryYakScript
	"CreateTemporaryYakScript": yakit.CreateTemporaryYakScript,

	"NewAliveHost": YakitNewAliveHost,
	"QueryAliveHost": func(runtimeId string) chan *schema.AliveHost {
		return yakit.YieldAliveHostRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId)
	},

	"saveHTTPFlowWithTags": yakit.CreateHTTPFlowWithTags,

	// operate origin database
	"OpenDatabase":           OpenDatabase,
	"OpenSqliteDatabase":     OpenSqliteDatabase,
	"OpenTempSqliteDatabase": OpenTempSqliteDatabase,

	"ScanResult": ScanResult,

	"SaveAIYakScript": func(tool *schema.AIYakTool) error {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return utils.Error("empty database connection")
		}
		_, err := yakit.SaveAIYakTool(db, tool)
		return err
	},
}

func OpenDatabase(dialect string, source string) (*gorm.DB, error) {
	db, err := gorm.Open(dialect, source)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func OpenSqliteDatabase(path string) (*gorm.DB, error) {
	if exist, err := utils.PathExists(path); err != nil {
		return nil, err
	} else if !exist {
		_, err := os.Create(path)
		if err != nil {
			return nil, err
		}
	}
	path = fmt.Sprintf("%s?cache=shared&mode=rwc", path)
	return OpenDatabase("sqlite3", path)
}

func OpenTempSqliteDatabase() (*gorm.DB, error) {
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	return OpenSqliteDatabase(path)
}

func ScanResult(db *gorm.DB, query string, args ...interface{}) ([]map[string]interface{}, error) {
	if db == nil {
		return nil, utils.Error("empty database connection")
	}
	var res = make([]map[string]interface{}, 0)
	rows, err := db.Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		m, err := bizhelper.RawToMap(rows, cols, colTypes)
		if err != nil {
			log.Errorf("failed to convert row to map: %s", err)
			continue
		}
		res = append(res, m)
	}
	return res, nil
}

func _deleteYakScriptByName(i string) error {
	db := consts.GetGormProfileDatabase()
	db = db.Where("is_core_plugin = ?", false)
	return yakit.DeleteYakScriptByName(db, i)
}

func _yieldYakScript() chan *schema.YakScript {
	return yakit.YieldYakScripts(consts.GetGormProfileDatabase(), context.Background())
}

func deleteYakitMenuItemAll() error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Error("empty connection for database")
	}

	return yakit.DeleteMenuItemAll(db)
}

func saveYakitMenuItemByBatchExecuteConfig(raw interface{}) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("create database conn failed: %s", "empty config for database")
	}
	item, err := yakit.NewMenuItemByBatchExecuteConfig(raw)
	if err != nil {
		return utils.Errorf("create menu item failed: %s", err)
	}
	return yakit.CreateOrUpdateMenuItem(db, item.CalcHash(), item)
}

func queryYakitPluginByName(name string) (*schema.YakScript, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Error("no database found")
	}
	scripts := yakit.QueryYakScriptByNames(db, name)
	if len(scripts) > 0 {
		return scripts[0], nil
	}
	return nil, utils.Errorf("yakit plugin(YakScript) cannot found by name: %v", name)
}

func YakitNewAliveHost(target string, opts ...yakit.AliveHostParamsOpt) {
	risk, _ := yakit.NewAliveHost(target, opts...)
	if risk != nil {
		// yakitStatusCard("存活主机", fmt.Sprint(addCounter()))
		yakitOutputHelper(risk)
	}
}

func init() {
	// YakitExports["QueryPortAssetByPort"] = queryPortAssetByNetwork
	// YakitExports["QueryPortAssetByKeyword"] = queryPortAssetByNetwork
}
