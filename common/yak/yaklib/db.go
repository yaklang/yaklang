package yaklib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bizhelper"

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

// saveYakitPlugin 将插件内容保存到本地插件数据库（导出名为 db.SaveYakitPlugin）
// 参数:
//   - scriptName: 插件名称（不可与已有插件重名）
//   - typeStr: 插件类型，如 db.YAKIT_PLUGIN_TYPE_YAK
//   - content: 插件源码内容
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地插件数据库（示意性示例）
// db.SaveYakitPlugin("my-plugin", db.YAKIT_PLUGIN_TYPE_YAK, "yakit.Info(\"hello\")")~
// ```
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

// saveHTTPFlowFromRaw 根据原始请求/响应保存一条 HTTP 流量记录到项目数据库（导出名为 db.SaveHTTPFlowFromRaw）
// 参数:
//   - url: 请求 URL
//   - req: 原始 HTTP 请求字节
//   - rsp: 原始 HTTP 响应字节
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// db.SaveHTTPFlowFromRaw("http://example.com", []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"), []byte("HTTP/1.1 200 OK\r\n\r\n"))~
// ```
func saveHTTPFlowFromRaw(url string, req, rsp []byte) error {
	return saveHTTPFlowFromRawWithType(url, req, rsp, "basic-crawler")
}

// saveHTTPFlowFromRawWithType 根据原始请求/响应及来源类型保存一条 HTTP 流量记录（导出名为 db.SaveHTTPFlowFromRawWithType）
// 参数:
//   - url: 请求 URL
//   - req: 原始 HTTP 请求字节
//   - rsp: 原始 HTTP 响应字节
//   - typeStr: 流量来源类型，如 "basic-crawler"
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// db.SaveHTTPFlowFromRawWithType("http://example.com", reqBytes, rspBytes, "mitm")~
// ```
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

// saveHTTPFlowFromRawWithOption 根据原始请求/响应及自定义选项保存一条 HTTP 流量记录（导出名为 db.SaveHTTPFlowFromRawWithOption）
// 参数:
//   - url: 请求 URL
//   - req: 原始 HTTP 请求字节
//   - rsp: 原始 HTTP 响应字节
//   - exOption: 额外的流量创建选项
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// db.SaveHTTPFlowFromRawWithOption("http://example.com", reqBytes, rspBytes)~
// ```
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
	"QueryHTTPFlowsByID":          queryHTTPFlowsByID,
	"QueryHTTPFlowByID":           queryHTTPFlowByID,
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
	"GetYakitPluginByID":   getYakitPluginByID,
	"GetYakitPluginByUUID": getYakitPluginByUUID,

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

	// Yield AI materials
	"YieldAllAITools": func() chan *schema.AIYakTool {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return nil
		}
		return yakit.YieldAllAITools(context.Background(), db)
	},
	"YieldAllAIForges": func() chan *schema.AIForge {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return nil
		}
		return yakit.YieldAllAIForges(context.Background(), db)
	},
	"YieldAllMCPServers": func() chan *schema.MCPServer {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return nil
		}
		return yakit.YieldAllMCPServers(context.Background(), db)
	},
}

// OpenDatabase 通过指定方言与连接源打开一个数据库连接（导出名为 db.OpenDatabase）
// 参数:
//   - dialect: 数据库方言，如 "sqlite3"、"mysql"
//   - source: 数据源连接串
//
// 返回值:
//   - 数据库连接对象
//   - 错误信息
//
// Example:
// ```
// conn = db.OpenDatabase("sqlite3", "/tmp/test.db")~
// defer conn.Close()
// ```
func OpenDatabase(dialect string, source string) (*gorm.DB, error) {
	db, err := gorm.Open(dialect, source)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// OpenSqliteDatabase 打开（不存在时创建）一个 SQLite 数据库（导出名为 db.OpenSqliteDatabase）
// 参数:
//   - path: SQLite 数据库文件路径
//
// 返回值:
//   - 数据库连接对象
//   - 错误信息
//
// Example:
// ```
// conn = db.OpenSqliteDatabase("/tmp/test.db")~
// defer conn.Close()
// ```
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

// OpenTempSqliteDatabase 在临时目录中创建并打开一个临时 SQLite 数据库（导出名为 db.OpenTempSqliteDatabase）
// 参数:
//   - 无
//
// 返回值:
//   - 数据库连接对象
//   - 错误信息
//
// Example:
// ```
// conn = db.OpenTempSqliteDatabase()~
// defer conn.Close()
// ```
func OpenTempSqliteDatabase() (*gorm.DB, error) {
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	return OpenSqliteDatabase(path)
}

// ScanResult 执行原始 SQL 查询并将结果按行转换为 map 列表（导出名为 db.ScanResult）
// 参数:
//   - db: 数据库连接对象
//   - query: 原始 SQL 查询语句
//   - args: SQL 占位符参数
//
// 返回值:
//   - 查询结果（每行一个 map）
//   - 错误信息
//
// Example:
// ```
// conn = db.OpenTempSqliteDatabase()~
// defer conn.Close()
// rows = db.ScanResult(conn, "SELECT 1 AS n")~
// dump(rows)
// ```
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

// _deleteYakScriptByName 按名称删除本地插件（核心插件除外，导出名为 db.DeleteYakScriptByName）
// 参数:
//   - i: 插件名称
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地插件数据库（示意性示例）
// db.DeleteYakScriptByName("my-plugin")~
// ```
func _deleteYakScriptByName(i string) error {
	db := consts.GetGormProfileDatabase()
	db = db.Where("is_core_plugin = ?", false)
	return yakit.DeleteYakScriptByName(db, i)
}

// _yieldYakScript 以 channel 形式遍历本地数据库中的全部插件（导出名为 db.YieldYakScriptAll）
// 参数:
//   - 无
//
// 返回值:
//   - 插件对象的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 依赖本地插件数据库（示意性示例）
//
//	for script := range db.YieldYakScriptAll() {
//	    println(script.ScriptName)
//	}
//
// ```
func _yieldYakScript() chan *schema.YakScript {
	return yakit.YieldYakScripts(consts.GetGormProfileDatabase(), context.Background())
}

// deleteYakitMenuItemAll 删除全部 Yakit 菜单项（导出名为 db.DeleteYakitMenuItemAll）
// 参数:
//   - 无
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地数据库（示意性示例）
// db.DeleteYakitMenuItemAll()~
// ```
func deleteYakitMenuItemAll() error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Error("empty connection for database")
	}

	return yakit.DeleteMenuItemAll(db)
}

// saveYakitMenuItemByBatchExecuteConfig 根据批量执行配置创建并保存 Yakit 菜单项（导出名为 db.SaveYakitMenuItemByBatchExecuteConfig）
// 参数:
//   - raw: 批量执行配置（JSON 或 map）
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地数据库（示意性示例）
// db.SaveYakitMenuItemByBatchExecuteConfig(config)~
// ```
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

// queryYakitPluginByName 按名称查询本地插件（导出名为 db.GetYakitPluginByName）
// 参数:
//   - name: 插件名称
//
// 返回值:
//   - 插件对象
//   - 错误信息
//
// Example:
// ```
// // 依赖本地插件数据库（示意性示例）
// script = db.GetYakitPluginByName("my-plugin")~
// dump(script)
// ```
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

// getYakitPluginByID 按数据库 ID 查询本地插件（导出名为 db.GetYakitPluginByID）
// 参数:
//   - i: 插件 ID
//
// 返回值:
//   - 插件对象
//   - 错误信息
//
// Example:
// ```
// // 依赖本地插件数据库（示意性示例）
// script = db.GetYakitPluginByID(1)~
// dump(script)
// ```
func getYakitPluginByID(i any) (*schema.YakScript, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Error("no database found")
	}

	id := int64(utils.InterfaceToInt(i))
	if id <= 0 {
		return nil, utils.Errorf("invalid plugin id: %v", i)
	}

	script, err := yakit.GetYakScript(db, id)
	if err != nil {
		return nil, utils.Errorf("query yakit plugin(YakScript) by id[%v] failed: %v", id, err)
	}
	log.Infof("query yakit plugin by id[%d] success: %s", id, script.ScriptName)
	return script, nil
}

// getYakitPluginByUUID 按 UUID 查询本地插件（导出名为 db.GetYakitPluginByUUID）
// 参数:
//   - i: 插件 UUID
//
// 返回值:
//   - 插件对象
//   - 错误信息
//
// Example:
// ```
// // 依赖本地插件数据库（示意性示例）
// script = db.GetYakitPluginByUUID("xxxx-uuid")~
// dump(script)
// ```
func getYakitPluginByUUID(i any) (*schema.YakScript, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Error("no database found")
	}

	uuid := utils.InterfaceToString(i)
	if uuid == "" {
		return nil, utils.Errorf("invalid plugin uuid: %v", i)
	}

	script, err := yakit.GetYakScriptByUUID(db, uuid)
	if err != nil {
		return nil, utils.Errorf("query yakit plugin(YakScript) by uuid[%v] failed: %v", uuid, err)
	}
	log.Infof("query yakit plugin by uuid[%s] success: %s", uuid, script.ScriptName)
	return script, nil
}

// YakitNewAliveHost 创建并保存一条存活主机记录并输出（导出名为 db.NewAliveHost）
// 参数:
//   - target: 主机目标（IP 或域名）
//   - opts: 存活主机可选项，如运行时 ID 等
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// db.NewAliveHost("127.0.0.1")
// ```
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
