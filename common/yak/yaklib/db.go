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

// saveYakitPlugin 将插件源码保存到本地插件数据库（导出名为 db.SaveYakitPlugin）
//
// 把一段插件源码以指定类型持久化为本地插件，之后可在 Yakit 中加载、或用 db.GetYakitPluginByName 取回。
// 插件类型用内置常量指定：db.YAKIT_PLUGIN_TYPE_YAK（yak 脚本）、db.YAKIT_PLUGIN_TYPE_MITM、
// db.YAKIT_PLUGIN_TYPE_PORTSCAN、db.YAKIT_PLUGIN_TYPE_NUCLEI、db.YAKIT_PLUGIN_TYPE_CODEC、
// db.YAKIT_PLUGIN_TYPE_PACKET_HACK。注意：scriptName 不可与已有插件重名，否则返回错误。
// 若只是想临时注册一个插件供调用而不长期保存，考虑 db.CreateTemporaryYakScript。
//
// 参数:
//   - scriptName: 插件名称（全局唯一，重名会报错）
//   - typeStr: 插件类型，使用 db.YAKIT_PLUGIN_TYPE_* 常量
//   - content: 插件源码内容
//
// 返回值:
//   - 错误信息（数据库不可用、重名或保存失败时返回）
//
// Example:
// ```
// // 保存 -> 按名取回 -> 清理 的完整生命周期
// name = "doc-demo-plugin-"+str.RandStr(6)
// code = `yakit.Info("hello from saved plugin")`
// db.SaveYakitPlugin(name, db.YAKIT_PLUGIN_TYPE_YAK, code)~
//
// got = db.GetYakitPluginByName(name)~
// println(got.ScriptName, got.Type)
// assert got.ScriptName == name && got.Type == db.YAKIT_PLUGIN_TYPE_YAK, "saved plugin should be retrievable by name"
//
// db.DeleteYakScriptByName(name)~   // 清理演示插件
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
//
// 把一次 HTTP 交互（请求包 + 响应包）持久化为可检索的流量记录，来源类型默认标记为 "basic-crawler"。
// 入库后可用 db.QueryHTTPFlowsByKeyword / db.QueryUrlsByKeyword 等检索。是“发请求 -> 存证据”的常用一环：
// 常与 poc 库（poc.HTTP/poc.Get 返回的原始包）联动，把扫描/爬取过程中的关键请求落库供后续分析。
//
// 参数:
//   - url: 请求 URL（用于建立索引；https/wss 前缀会被识别为 HTTPS 流量）
//   - req: 原始 HTTP 请求字节（完整请求报文）
//   - rsp: 原始 HTTP 响应字节（完整响应报文）
//
// 返回值:
//   - 错误信息（项目数据库不可用或保存失败时返回）
//
// Example:
// ```
// // 联动 poc：真实发起一次请求，再把原始请求/响应落库，最后检索验证
// host = "doc-demo-rawflow.example.com"
// reqRaw = f`GET / HTTP/1.1
// Host: ${host}
// User-Agent: yak-doc-demo
//
// `
// rspRaw = "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 5\r\n\r\nhello"
// db.SaveHTTPFlowFromRaw("http://"+host+"/", []byte(reqRaw), []byte(rspRaw))~
//
// found = 0
// for flow in db.QueryHTTPFlowsByKeyword(host) { found++; break }
// println(found)   // OUT: 1
// assert found == 1, "the saved flow should be retrievable by its host keyword"
// ```
func saveHTTPFlowFromRaw(url string, req, rsp []byte) error {
	return saveHTTPFlowFromRawWithType(url, req, rsp, "basic-crawler")
}

// saveHTTPFlowFromRawWithType 根据原始请求/响应及来源类型保存一条 HTTP 流量记录（导出名为 db.SaveHTTPFlowFromRawWithType）
//
// 与 db.SaveHTTPFlowFromRaw 相同，但可显式指定来源类型（source type），便于按来源分类检索与统计。
// 常见 type 取值："basic-crawler"（爬虫）、"mitm"（中间人代理）、"scan"（扫描器）、自定义业务标记等。
//
// 参数:
//   - url: 请求 URL
//   - req: 原始 HTTP 请求字节
//   - rsp: 原始 HTTP 响应字节
//   - typeStr: 流量来源类型标记
//
// 返回值:
//   - 错误信息（项目数据库不可用或保存失败时返回）
//
// Example:
// ```
// // 同一目标的两次流量打上不同来源类型，便于后续按 type 区分
// host = "doc-demo-typed.example.com"
// req = []byte(f"GET / HTTP/1.1\r\nHost: ${host}\r\n\r\n")
// rsp = []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
// db.SaveHTTPFlowFromRawWithType("http://"+host+"/crawl", req, rsp, "basic-crawler")~
// db.SaveHTTPFlowFromRawWithType("http://"+host+"/scan",  req, rsp, "scan")~
//
// cnt = 0
// for flow in db.QueryHTTPFlowsByKeyword(host) { cnt++ }
// assert cnt >= 2, "both typed flows should be saved and retrievable"
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
//
// 是 HTTP 流量入库最灵活的接口：除请求/响应外，可追加可选项进一步描述这条流量。
// 目前可用的可选项：db.saveHTTPFlowWithTags(tags)，用于给流量打标签（如标记可疑、命中规则等），
// 标签随流量入库后可在 Yakit 历史流量中用于筛选。
//
// 参数:
//   - url: 请求 URL
//   - req: 原始 HTTP 请求字节
//   - rsp: 原始 HTTP 响应字节
//   - exOption: 额外的流量创建选项（可变参数），如 db.saveHTTPFlowWithTags("...")
//
// 返回值:
//   - 错误信息（项目数据库不可用或保存失败时返回）
//
// Example:
// ```
// // 给入库的流量打上标签，便于后续在历史流量中筛选
// host = "doc-demo-tagged.example.com"
// req = []byte(f"GET /admin HTTP/1.1\r\nHost: ${host}\r\n\r\n")
// rsp = []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nadmin")
// db.SaveHTTPFlowFromRawWithOption(
//     "http://"+host+"/admin", req, rsp,
//     db.saveHTTPFlowWithTags("suspicious|admin-panel"),
// )~
//
// found = 0
// for flow in db.QueryHTTPFlowsByKeyword(host) { found++; break }
// assert found == 1, "tagged flow should be saved and retrievable"
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
	"QueryHTTPFlowsAll": dbQueryHTTPFlowsAll,
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
	"GetProjectKey": dbGetProjectKey,
	"SetProjectKey": dbSetProjectKey,
	"SetKey":        dbSetKey,
	"SetKeyWithTTL": dbSetKeyWithTTL,
	"GetKey":        dbGetKey,
	"DelKey":        dbDelKey,

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

	"NewAliveHost":   YakitNewAliveHost,
	"QueryAliveHost": dbQueryAliveHost,

	"saveHTTPFlowWithTags": dbSaveHTTPFlowWithTags,

	// operate origin database
	"OpenDatabase":           OpenDatabase,
	"OpenSqliteDatabase":     OpenSqliteDatabase,
	"OpenTempSqliteDatabase": OpenTempSqliteDatabase,

	"ScanResult": ScanResult,

	"SaveAIYakScript": dbSaveAIYakScript,

	// Yield AI materials
	"YieldAllAITools":    dbYieldAllAITools,
	"YieldAllAIForges":   dbYieldAllAIForges,
	"YieldAllMCPServers": dbYieldAllMCPServers,
}

// SetKey 向 Profile（全局配置）数据库写入一个键值对（导出名为 db.SetKey）
//
// 这是 yaklang 中最常用的跨脚本/跨运行共享数据的方式。键值会持久化保存在用户级 Profile 库中，
// 不随项目切换而丢失，可用 db.GetKey 读取、db.DelKey 删除、db.SetKeyWithTTL 设置带过期时间的版本。
// 典型用途：保存扫描配置、缓存中间结果、记录上次运行状态、在多个插件之间传递数据。
// 注意：value 会被转成字符串存储；如需保存结构化数据（map/list），请先用 json.dumps 序列化，
// 读取时再用 json.loads 反序列化（见下方联动示例）。Profile 库与 db.SetProjectKey 的项目库相互隔离。
//
// 参数:
//   - k: 键名，建议使用有意义且带前缀的命名（如 "myscan-target"）避免与其他脚本冲突
//   - v: 值，任意类型，最终以字符串形式持久化
//
// 返回值:
//   - 错误信息（数据库不可用或写入失败时返回）
//
// Example:
// ```
// // 1) 基础读写：保存与读取扫描目标
// db.SetKey("myscan-target", "192.168.1.0/24")
// db.SetKey("myscan-ports", "22,80,443,3306,8080")
// target = db.GetKey("myscan-target")
// ports = db.GetKey("myscan-ports")
// println(target)   // OUT: 192.168.1.0/24
// assert target == "192.168.1.0/24" && ports == "22,80,443,3306,8080", "SetKey should persist the values"
//
// // 2) 联动 json：用键值存储保存结构化的扫描配置
// scanConfig = {"concurrent": 50, "timeout": 5, "excludes": [22, 3389]}
// db.SetKey("myscan-config", json.dumps(scanConfig))
// loaded = json.loads(db.GetKey("myscan-config"))
// assert loaded["concurrent"] == 50 && len(loaded["excludes"]) == 2, "config should round-trip via json"
//
// // 3) 清理
// db.DelKey("myscan-target"); db.DelKey("myscan-ports"); db.DelKey("myscan-config")
// ```
func dbSetKey(k, v interface{}) error {
	return yakit.SetKey(consts.GetGormProfileDatabase(), k, v)
}

// SetKeyWithTTL 向 Profile 数据库写入一个带过期时间（TTL）的键值对（导出名为 db.SetKeyWithTTL）
//
// 与 db.SetKey 相同，但该键会在 ttl 秒之后自动失效，过期后 db.GetKey 返回空字符串。
// 典型用途：缓存有时效性的数据（如临时 token、会话 ID、限速窗口、一次性验证码），避免脏数据长期残留。
//
// 参数:
//   - k: 键名
//   - v: 值（以字符串形式持久化）
//   - ttl: 过期时间，单位为秒
//
// 返回值:
//   - 错误信息（数据库不可用或写入失败时返回）
//
// Example:
// ```
// // 缓存一个有效期 60 秒的临时凭据，过期前可正常读取
// db.SetKeyWithTTL("session-token", "tok-"+str.RandStr(16), 60)
// tok = db.GetKey("session-token")
// println(tok != "")   // OUT: true
// assert tok != "", "value should be readable before it expires"
//
// // 联动 SetKey：常见的“短期缓存 + 长期配置”组合
// db.SetKey("api-endpoint", "https://api.internal/scan")          // 长期配置
// db.SetKeyWithTTL("api-rate-window", "1", 2)                       // 2 秒限速窗口
// assert db.GetKey("api-rate-window") == "1", "ttl key should exist initially"
// db.DelKey("session-token"); db.DelKey("api-endpoint")
// ```
func dbSetKeyWithTTL(k, v any, ttl int) error {
	return yakit.SetKeyWithTTL(consts.GetGormProfileDatabase(), k, v, ttl)
}

// GetKey 从 Profile（全局配置）数据库读取一个键对应的值（导出名为 db.GetKey）
//
// 读取由 db.SetKey / db.SetKeyWithTTL 写入的值。键不存在或已过期时返回空字符串（""），
// 因此可用 `if db.GetKey(k) == ""` 判断键是否存在。若存储的是 json 字符串，读取后用 json.loads 还原。
//
// 参数:
//   - k: 键名
//
// 返回值:
//   - 键对应的值字符串；键不存在或已过期时返回空字符串
//
// Example:
// ```
// // 读取已存在的值，并对“首次运行”做默认值兜底
// if db.GetKey("scan-round") == "" {
//     db.SetKey("scan-round", "1")            // 首次运行初始化
// }
// round = atoi(db.GetKey("scan-round"))~
// db.SetKey("scan-round", sprint(round + 1))  // 每次运行自增
// println(round >= 1)   // OUT: true
// assert db.GetKey("scan-round") != "", "GetKey should return the persisted counter"
// db.DelKey("scan-round")
// ```
func dbGetKey(k interface{}) string {
	return yakit.GetKey(consts.GetGormProfileDatabase(), k)
}

// DelKey 从 Profile（全局配置）数据库删除一个键（导出名为 db.DelKey）
//
// 删除由 db.SetKey / db.SetKeyWithTTL 写入的键。删除后 db.GetKey 返回空字符串。
// 常用于脚本结束时清理临时数据，或重置某个配置项。删除不存在的键不会报错。
//
// 参数:
//   - k: 要删除的键名
//
// Example:
// ```
// // 完整生命周期：写入 -> 读取 -> 删除 -> 确认已删除
// db.SetKey("temp-flag", "running")
// assert db.GetKey("temp-flag") == "running", "key should exist after SetKey"
// db.DelKey("temp-flag")
// println(db.GetKey("temp-flag"))   // OUT:
// assert db.GetKey("temp-flag") == "", "DelKey should remove the key"
// ```
func dbDelKey(k interface{}) {
	yakit.DelKey(consts.GetGormProfileDatabase(), k)
}

// SetProjectKey 向当前项目数据库写入一个键值对（导出名为 db.SetProjectKey）
//
// 与 db.SetKey 用法完全一致，区别在于作用域：db.SetProjectKey 写入“当前项目”库，
// 切换项目后读不到（实现项目级数据隔离）；db.SetKey 写入全局 Profile 库，所有项目共享。
// 经验法则：与具体扫描项目强相关的数据（项目名、目标范围、本项目的中间结果）用 ProjectKey；
// 跨项目复用的工具配置（API 地址、字典路径、个人偏好）用 db.SetKey。同样建议用 json 存结构化数据。
//
// 参数:
//   - k: 键名
//   - v: 值（以字符串形式持久化）
//
// 返回值:
//   - 错误信息（数据库不可用或写入失败时返回）
//
// Example:
// ```
// // 保存当前项目的元信息，并与全局配置区分开
// db.SetProjectKey("project-name", "WebSec-Assessment-2026")
// db.SetProjectKey("project-scope", json.dumps(["example.com", "192.168.1.0/24"]))
// db.SetKey("global-api-endpoint", "https://api.internal")   // 全局，跨项目共享
//
// name = db.GetProjectKey("project-name")
// scope = json.loads(db.GetProjectKey("project-scope"))
// println(name)   // OUT: WebSec-Assessment-2026
// assert name == "WebSec-Assessment-2026" && len(scope) == 2, "project key should persist project-scoped data"
// ```
func dbSetProjectKey(k, v any) error {
	return yakit.SetProjectKey(consts.GetGormProjectDatabase(), k, v)
}

// GetProjectKey 从当前项目数据库读取一个键对应的值（导出名为 db.GetProjectKey）
//
// 读取由 db.SetProjectKey 写入的项目级值；键不存在时返回空字符串。
// 与 db.GetKey（全局 Profile 库）相互独立：同名键在两个库里互不影响。
//
// 参数:
//   - k: 键名
//
// 返回值:
//   - 键对应的值字符串；键不存在时返回空字符串
//
// Example:
// ```
// // 演示项目库与全局库的隔离：同名键互不干扰
// db.SetProjectKey("env", "project-value")
// db.SetKey("env", "global-value")
// println(db.GetProjectKey("env"))   // OUT: project-value
// assert db.GetProjectKey("env") == "project-value", "GetProjectKey reads from the project DB"
// assert db.GetKey("env") == "global-value", "db.GetKey reads from the global profile DB"
// db.DelKey("env")
// ```
func dbGetProjectKey(k any) string {
	return yakit.GetProjectKey(consts.GetGormProjectDatabase(), k)
}

// QueryHTTPFlowsAll 查询数据库中保存的全部 HTTP 流量记录（导出名为 db.QueryHTTPFlowsAll）
// 以 channel 形式逐条返回，便于流式遍历
//
// 返回值:
//   - 逐条产出 HTTP 流量记录的 channel
//
// Example:
// ```
// // 遍历数据库中已保存的全部 HTTP 流量
// count = 0
// for flow in db.QueryHTTPFlowsAll() {
//     count++
//     if count > 5 { break }
// }
// println("scanned http flows")
// ```
func dbQueryHTTPFlowsAll() chan *schema.HTTPFlow {
	return queryHTTPFlowByKeyword("")
}

// QueryAliveHost 按运行时 ID 查询存活主机记录（导出名为 db.QueryAliveHost）
// 以 channel 形式逐条返回，常配合扫描任务的 runtimeId 使用
//
// 参数:
//   - runtimeId: 扫描任务的运行时 ID
//
// 返回值:
//   - 逐条产出存活主机记录的 channel
//
// Example:
// ```
// // runtimeId 来自某次扫描任务（示意性示例）
// for host in db.QueryAliveHost("example-runtime-id") {
//     println(host.IP)
// }
// ```
func dbQueryAliveHost(runtimeId string) chan *schema.AliveHost {
	return yakit.YieldAliveHostRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId)
}

// saveHTTPFlowWithTags 构造一个为 HTTP 流量附加标签的保存选项（导出名为 db.saveHTTPFlowWithTags）
// 作为保存 HTTP 流量相关接口的可选项使用，用于在入库时打上指定标签
//
// 参数:
//   - tags: 要附加的标签字符串（多个标签可用分隔符拼接）
//
// 返回值:
//   - HTTP 流量保存选项
//
// Example:
// ```
// // 作为保存 HTTP 流量的可选项使用（示意性示例）
// opt = db.saveHTTPFlowWithTags("suspicious")
// ```
func dbSaveHTTPFlowWithTags(tags string) yakit.CreateHTTPFlowOptions {
	return yakit.CreateHTTPFlowWithTags(tags)
}

// SaveAIYakScript 将一个 AI 工具定义保存到 Profile 数据库（导出名为 db.SaveAIYakScript）
//
// 参数:
//   - tool: AI 工具定义对象
//
// 返回值:
//   - 错误信息（数据库不可用或保存失败时返回）
//
// Example:
// ```
// // tool 为已构造的 AIYakTool 对象（示意性示例）
// db.SaveAIYakScript(tool)~
// ```
func dbSaveAIYakScript(tool *schema.AIYakTool) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Error("empty database connection")
	}
	_, err := yakit.SaveAIYakTool(db, tool)
	return err
}

// YieldAllAITools 遍历 Profile 数据库中保存的全部 AI 工具（导出名为 db.YieldAllAITools）
// 以 channel 形式逐条返回
//
// 返回值:
//   - 逐条产出 AI 工具的 channel
//
// Example:
// ```
// for tool in db.YieldAllAITools() {
//     println(tool.Name)
// }
// ```
func dbYieldAllAITools() chan *schema.AIYakTool {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	return yakit.YieldAllAITools(context.Background(), db)
}

// YieldAllAIForges 遍历 Profile 数据库中保存的全部 AI Forge（导出名为 db.YieldAllAIForges）
// 以 channel 形式逐条返回
//
// 返回值:
//   - 逐条产出 AI Forge 的 channel
//
// Example:
// ```
// for forge in db.YieldAllAIForges() {
//     println(forge.ForgeName)
// }
// ```
func dbYieldAllAIForges() chan *schema.AIForge {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	return yakit.YieldAllAIForges(context.Background(), db)
}

// YieldAllMCPServers 遍历 Profile 数据库中保存的全部 MCP Server（导出名为 db.YieldAllMCPServers）
// 以 channel 形式逐条返回
//
// 返回值:
//   - 逐条产出 MCP Server 的 channel
//
// Example:
// ```
// for server in db.YieldAllMCPServers() {
//     println(server.Name)
// }
// ```
func dbYieldAllMCPServers() chan *schema.MCPServer {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	return yakit.YieldAllMCPServers(context.Background(), db)
}

// OpenDatabase 通过指定方言与连接源打开一个任意 gorm 数据库连接（导出名为 db.OpenDatabase）
//
// 这是最底层、最通用的建连接口，用于连接 yaklang 内置库之外的数据库（如目标业务库、外部 MySQL）。
// 返回的连接对象支持 .Exec(sql, args...) 执行写操作、配合 db.ScanResult 执行查询。
// 若只是需要一个临时本地库做数据中转，优先用 db.OpenTempSqliteDatabase；连接 SQLite 文件用 db.OpenSqliteDatabase。
//
// 参数:
//   - dialect: 数据库方言，常见取值 "sqlite3"、"mysql"、"postgres"
//   - source: 数据源连接串。sqlite3 为文件路径；mysql 形如 "user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4"
//
// 返回值:
//   - 数据库连接对象（*gorm.DB）
//   - 错误信息（连接失败时返回）
//
// Example:
// ```
// // sqlite3：打开/创建一个本地库
// conn = db.OpenDatabase("sqlite3", "/tmp/doc-demo-open.db")~
// defer conn.Close()
// conn.Exec("CREATE TABLE IF NOT EXISTS kv(k TEXT, v TEXT)")
// conn.Exec("INSERT INTO kv VALUES (?, ?)", "name", "yak")
// rows = db.ScanResult(conn, "SELECT v FROM kv WHERE k = ?", "name")~
// assert rows[0]["v"] == "yak", "OpenDatabase + Exec + ScanResult should round-trip"
//
// // mysql（依赖目标 MySQL，示意性，不在文档校验中执行）
// // mysqlConn = db.OpenDatabase("mysql", "root:123456@tcp(127.0.0.1:3306)/test?charset=utf8mb4")~
// ```
func OpenDatabase(dialect string, source string) (*gorm.DB, error) {
	db, err := gorm.Open(dialect, source)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// OpenSqliteDatabase 打开（不存在时自动创建）一个 SQLite 数据库文件（导出名为 db.OpenSqliteDatabase）
//
// 适合需要把数据持久化到指定文件、且后续可重复打开的场景（如导出扫描结果、构建自定义数据集）。
// 连接以 shared cache + rwc 模式打开。若不需要持久化文件，用 db.OpenTempSqliteDatabase 更省心。
//
// 参数:
//   - path: SQLite 数据库文件路径（不存在会自动创建空文件）
//
// 返回值:
//   - 数据库连接对象（*gorm.DB）
//   - 错误信息（创建或打开失败时返回）
//
// Example:
// ```
// // 把数据写入指定文件，重新打开后数据仍在（持久化验证）
// path = "/tmp/doc-demo-sqlite.db"
// conn = db.OpenSqliteDatabase(path)~
// conn.Exec("CREATE TABLE IF NOT EXISTS findings(id INTEGER PRIMARY KEY, title TEXT, severity TEXT)")
// conn.Exec("INSERT INTO findings(title, severity) VALUES (?, ?)", "SQL Injection", "high")
// conn.Close()
//
// reopen = db.OpenSqliteDatabase(path)~          // 重新打开同一文件
// defer reopen.Close()
// rows = db.ScanResult(reopen, "SELECT title, severity FROM findings")~
// println(rows[0]["title"])   // OUT: SQL Injection
// assert len(rows) >= 1 && rows[0]["severity"] == "high", "data should persist across reopen"
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
//
// 数据库文件以随机 UUID 命名建于 Yakit 临时目录，适合脚本内部做数据中转、聚合、去重、排序等，
// 用完即弃。是“把内存数据交给 SQL 处理”的最便捷入口：建表 -> 批量写入 -> 用 SQL 聚合/JOIN -> 取结果。
//
// 返回值:
//   - 数据库连接对象（*gorm.DB）
//   - 错误信息（创建失败时返回）
//
// Example:
// ```
// // 完整 CRUD + 聚合：把扫描得到的端口数据交给 SQL 统计每类服务数量
// conn = db.OpenTempSqliteDatabase()~
// defer conn.Close()
// conn.Exec(`CREATE TABLE ports(host TEXT, port INTEGER, service TEXT)`)
// records = [
//     ["10.0.0.1", 80, "http"], ["10.0.0.1", 443, "https"],
//     ["10.0.0.2", 22, "ssh"],  ["10.0.0.2", 80, "http"],
// ]
// for r in records { conn.Exec("INSERT INTO ports VALUES (?, ?, ?)", r[0], r[1], r[2]) }
//
// stats = db.ScanResult(conn, "SELECT service, COUNT(*) AS cnt FROM ports GROUP BY service ORDER BY cnt DESC")~
// for row in stats { println(row["service"], row["cnt"]) }
// assert len(stats) == 3, "should aggregate into 3 service groups"
// assert stats[0]["service"] == "http", "http should be the most frequent service"
// ```
func OpenTempSqliteDatabase() (*gorm.DB, error) {
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	return OpenSqliteDatabase(path)
}

// ScanResult 执行原始 SQL 查询并把每一行转换为一个 map（导出名为 db.ScanResult）
//
// 是从任意 gorm 连接（db.OpenTempSqliteDatabase / db.OpenSqliteDatabase / db.OpenDatabase 返回的对象）
// 读取查询结果的标准方式。返回值是 []map，每个 map 的 key 为列名、value 为该列的值，便于直接用 row["col"] 取值。
// 强烈建议使用 `?` 占位符传参（参数化查询），由驱动负责转义，避免拼接字符串导致的 SQL 注入。
// 写操作（INSERT/UPDATE/DELETE/DDL）用连接对象的 .Exec(sql, args...)，查询用本函数。
//
// 参数:
//   - db: 数据库连接对象（来自 db.Open* 系列函数）
//   - query: 原始 SQL 查询语句，使用 ? 作为占位符
//   - args: 与占位符一一对应的参数
//
// 返回值:
//   - 查询结果，每行一个 map[列名]值
//   - 错误信息（连接为空或 SQL 执行失败时返回）
//
// Example:
// ```
// conn = db.OpenTempSqliteDatabase()~
// defer conn.Close()
// conn.Exec("CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT, role TEXT)")
// conn.Exec("INSERT INTO users(name, role) VALUES (?, ?)", "alice", "admin")
// conn.Exec("INSERT INTO users(name, role) VALUES (?, ?)", "bob", "user")
//
// // 参数化条件查询：只取 admin（即使传入带引号的恶意输入也安全）
// admins = db.ScanResult(conn, "SELECT id, name FROM users WHERE role = ?", "admin")~
// println(admins[0]["name"])   // OUT: alice
// assert len(admins) == 1 && admins[0]["name"] == "alice", "parameterized query should return only admin"
//
// // 聚合查询：统计总行数
// total = db.ScanResult(conn, "SELECT COUNT(*) AS n FROM users")~
// assert int(total[0]["n"]) == 2, "should count 2 users"
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

// _deleteYakScriptByName 按名称删除本地插件（核心插件受保护不会被删，导出名为 db.DeleteYakScriptByName）
//
// 用于清理由 db.SaveYakitPlugin 等保存的本地插件。出于安全考虑，标记为核心插件(is_core_plugin)的插件不会被删除。
// 常作为脚本中保存临时插件后的清理步骤（见 db.SaveYakitPlugin 示例）。
//
// 参数:
//   - i: 要删除的插件名称
//
// 返回值:
//   - 错误信息（数据库不可用或删除失败时返回）
//
// Example:
// ```
// // 保存一个临时插件再删除，验证删除后查不到（保存->删除->确认 联动）
// name = "doc-demo-del-"+str.RandStr(6)
// db.SaveYakitPlugin(name, db.YAKIT_PLUGIN_TYPE_YAK, `yakit.Info("temp")`)~
// db.DeleteYakScriptByName(name)~
// _, err = db.GetYakitPluginByName(name)
// assert err != nil, "the plugin should no longer exist after deletion"
// ```
func _deleteYakScriptByName(i string) error {
	db := consts.GetGormProfileDatabase()
	db = db.Where("is_core_plugin = ?", false)
	return yakit.DeleteYakScriptByName(db, i)
}

// _yieldYakScript 以 channel 形式流式遍历本地数据库中的全部插件（导出名为 db.YieldYakScriptAll）
//
// 枚举所有本地插件，常用于按类型/作者/标签做统计或批量处理。返回 YakScript 对象 channel，
// 可读取 .ScriptName / .Type / .Author / .Tags 等字段。
//
// 返回值:
//   - 插件对象的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 统计本地插件按类型的分布，并用饼图展示（遍历->统计->可视化 联动）
// byType = {}
// for script in db.YieldYakScriptAll() {
//     t = script.Type
//     if t in byType { byType[t] = byType[t] + 1 } else { byType[t] = 1 }
// }
// pie = yakit.NewPieGraph("plugin types")
// for t, c in byType { pie.Add(t, c) }
// yakit.Output(pie)
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
//
// 取回某个已存在插件的完整对象，可读取 .ScriptName / .Type / .Content / .Help / .Author 等字段。
// 常用于在调用前检查插件是否存在、读取插件源码做二次处理。配套有按 ID / UUID 查询的版本。
//
// 参数:
//   - name: 插件名称
//
// 返回值:
//   - 插件对象（*schema.YakScript）
//   - 错误信息（数据库不可用或插件不存在时返回）
//
// Example:
// ```
// // 与 db.SaveYakitPlugin 配对：保存后按名取回并读取源码长度
// name = "doc-demo-getbyname-"+str.RandStr(6)
// db.SaveYakitPlugin(name, db.YAKIT_PLUGIN_TYPE_YAK, `yakit.Info("x")`)~
//
// script = db.GetYakitPluginByName(name)~
// println(script.Type, len(script.Content) > 0)
// assert script.ScriptName == name, "GetYakitPluginByName should return the matching plugin"
// db.DeleteYakScriptByName(name)~
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
