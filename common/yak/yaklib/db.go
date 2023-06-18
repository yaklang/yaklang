package yaklib

import (
	"context"
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

	return yakit.CreateOrUpdateYakScriptByName(db, scriptName, &yakit.YakScript{
		ScriptName: scriptName,
		Type:       typeStr,
		Content:    utils.InterfaceToString(content),
		Level:      "middle",
	})
}

var DatabaseExports = map[string]interface{}{
	// Download IP
	"DownloadGeoIP": DownloadMMDB,
	"QueryIPCity":   QueryIP,
	"QueryIPForIPS": QueryIPForISP,

	// 写入资产
	"SaveHTTPFlowFromNative":         saveCrawler,
	"SaveHTTPFlowFromNativeWithType": saveHTTPFlowWithType,
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
	"QueryHTTPFlowsAll": func() chan *yakit.HTTPFlow {
		return queryHTTPFlowByKeyword("")
	},
	"QueryPortsByUpdatedAt":       queryPortsByUpdatedAt,
	"QueryPortsByTaskName":        queryPortsByTaskName,
	"QueryHTTPFlowsByID":          queryHTTPFlowByID,
	"QueryHostPortByNetwork":      queryHostPortByNetwork,
	"QueryHostPortByKeyword":      queryHostAssetByNetwork,
	"QueryHostsByDomain":          queryHostAssetByDomainKeyword,
	"QueryDomainsByNetwork":       queryDomainAssetByNetwork,
	"QueryDomainsByDomainKeyword": queryDomainAssetByDomainKeyword,
	"QueryDomainsByTitle":         queryDomainAssetByHTMLTitle,
	"QueryPayloadGroups":          getPayloadGroups,
	"DeletePayloadByGroup":        deletePayload,

	"SetKey": func(k, v interface{}) error {
		return yakit.SetKey(consts.GetGormProfileDatabase(), k, v)
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
}

func _deleteYakScriptByName(i string) error {
	return yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), i)
}

func _yieldYakScript() chan *yakit.YakScript {
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

func queryYakitPluginByName(name string) (*yakit.YakScript, error) {
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

func init() {
	//YakitExports["QueryPortAssetByPort"] = queryPortAssetByNetwork
	//YakitExports["QueryPortAssetByKeyword"] = queryPortAssetByNetwork
}
