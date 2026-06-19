package antlr4nasl

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/script_core"
)

// QueryAllScripts 根据可选的查询条件从本地数据库中检索已导入的 NASL 脚本信息
// 在 yak 中通过 nasl.QueryAllScripts 调用，支持按 origin_file_name、cve、script_name、category、family 过滤
// 参数:
//   - script: 可选的查询条件 map
//
// 返回值:
//   - 匹配的 NASL 脚本信息列表
//
// Example:
// ```
// // 该示例为示意性用法：依赖本地已导入的 NASL 脚本库
// scripts = nasl.QueryAllScripts({"family": "Web Servers"})
// println("scripts:", len(scripts))
// ```
func QueryAllScripts(script ...any) []*script_core.NaslScriptInfo {
	queryCondition := map[string]any{}
	if len(script) > 0 {
		for k, v := range utils.InterfaceToMapInterface(script[0]) {
			if utils.StringArrayContains([]string{"origin_file_name", "cve", "script_name", "category", "family"}, k) {
				queryCondition[k] = v
			} else {
				log.Warnf("not allow query field %s", k)
			}
		}
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}

	var scripts []*schema.NaslScript
	if db := db.Where(queryCondition).Find(&scripts); db.Error != nil {
		log.Errorf("cannot query script: %s", db.Error.Error())
		return nil
	}
	var ret []*script_core.NaslScriptInfo
	for _, s := range scripts {
		ret = append(ret, script_core.NewNaslScriptObjectFromNaslScript(s))
	}
	return ret
}

// RemoveDatabase 清空本地数据库中已导入的全部 NASL 脚本
// 在 yak 中通过 nasl.RemoveDatabase 调用
// 返回值:
//   - 错误信息，操作失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：清空 NASL 脚本库
// err = nasl.RemoveDatabase()
// ```
func RemoveDatabase() error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("cannot fetch database: %s", db.Error)
	}
	if db := db.Model(&schema.NaslScript{}).Unscoped().Delete(&schema.NaslScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

// UpdateDatabase 从指定文件或目录加载 NASL 脚本(.nasl/.inc)并导入到本地数据库
// 在 yak 中通过 nasl.UpdateDatabase 调用，传入目录时会递归加载
// 参数:
//   - p: NASL 脚本文件或目录路径
//
// Example:
// ```
// // 该示例为示意性用法：从目录批量导入 NASL 脚本
// nasl.UpdateDatabase("/path/to/nasl-scripts")
// ```
func UpdateDatabase(p string) {
	saveScript := func(path string) {
		if !strings.HasSuffix(path, ".nasl") {
			log.Errorf("Error load script %s: not a nasl file", path)
			return
		}
		engine := script_core.NewScriptEngine()
		engine.AddScriptLoadedHook(func(scriptIns *script_core.NaslScriptInfo) {
			err := scriptIns.Save()
			if err != nil {
				log.Errorf("Error save script %s: %s", path, err.Error())
			}
		})
		err := engine.LoadScript(path)
		if err != nil {
			log.Errorf("Error load script %s: %s", path, err.Error())
			return
		}
	}
	if utils.IsDir(p) {
		swg := utils.NewSizedWaitGroup(20)
		raw, err := utils.ReadFilesRecursively(p)
		if err == nil {
			for _, r := range raw {
				if !strings.HasSuffix(r.Path, ".nasl") && !strings.HasSuffix(r.Path, ".inc") {
					continue
				}
				swg.Add()
				go func(path string) {
					defer swg.Done()
					saveScript(path)
				}(r.Path)
			}
		}
		swg.Wait()
	} else if utils.IsFile(p) {
		saveScript(p)
	}
}

// ScanTarget 对单个目标(host:port)运行 NASL 脚本进行扫描，以 channel 形式返回扫描得到的知识库(KB)结果
// 在 yak 中通过 nasl.ScanTarget 调用，依赖网络环境与已导入的 NASL 脚本
// 参数:
//   - target: 扫描目标，形如 "192.168.1.1:80"
//   - opts: 可选配置项，如 nasl.plugin、nasl.family、nasl.proxy 等
//
// 返回值:
//   - 一个只读 channel，逐条产出扫描结果 NaslKBs
//   - 错误信息，目标解析失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：对目标运行指定 NASL 插件
// res = nasl.ScanTarget("192.168.1.1:80", nasl.family("Web Servers"))~
//
//	for kb = range res {
//	    println(kb)
//	}
//
// ```
func ScanTarget(target string, opts ...script_core.NaslScriptConfigOptFunc) (chan *script_core.NaslKBs, error) {
	host, port, err := utils.ParseStringToHostPort(target)
	if err != nil {
		return nil, err
	}
	return script_core.NaslScan(host, fmt.Sprint(port), opts...), nil
}

// Scan 对指定主机与端口运行 NASL 脚本进行扫描，以 channel 形式返回扫描得到的知识库(KB)结果
// 在 yak 中通过 nasl.Scan 调用，依赖网络环境与已导入的 NASL 脚本
// 参数:
//   - hosts: 目标主机(支持多种主机表达式)
//   - ports: 目标端口(支持端口表达式)
//   - opts: 可选配置项，如 nasl.plugin、nasl.family 等
//
// 返回值:
//   - 一个只读 channel，逐条产出扫描结果 NaslKBs
//
// Example:
// ```
// // 该示例为示意性用法：对主机端口运行 NASL 扫描
// res = nasl.Scan("192.168.1.1", "80,443", nasl.family("Web Servers"))
//
//	for kb = range res {
//	    println(kb)
//	}
//
// ```
func Scan(hosts, ports string, opts ...script_core.NaslScriptConfigOptFunc) chan *script_core.NaslKBs {
	return script_core.NaslScan(hosts, ports, opts...)
}

// WithPlugins 指定本次 NASL 扫描要运行的插件(脚本文件名)列表
// 在 yak 中通过 nasl.plugin 调用
// 参数:
//   - plugins: 一个或多个 NASL 插件名
//
// 返回值:
//   - 一个 nasl.Scan/nasl.ScanTarget 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定运行的 NASL 插件
// res = nasl.ScanTarget("192.168.1.1:80", nasl.plugin("http_version.nasl"))~
// ```
func WithPlugins(plugins ...string) script_core.NaslScriptConfigOptFunc {
	return script_core.WithPlugins(plugins...)
}

// WithFamily 指定本次 NASL 扫描要运行的脚本家族(family)
// 在 yak 中通过 nasl.family 调用
// 参数:
//   - family: NASL 脚本家族名称
//
// 返回值:
//   - 一个 nasl.Scan/nasl.ScanTarget 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：按家族选择脚本
// res = nasl.ScanTarget("192.168.1.1:80", nasl.family("Web Servers"))~
// ```
func WithFamily(family string) script_core.NaslScriptConfigOptFunc {
	return script_core.WithFamily(family)
}

// WithRiskHandle 设置 NASL 扫描发现风险时触发的回调函数
// 在 yak 中通过 nasl.riskHandle 调用
// 参数:
//   - f: 接收风险对象的回调函数
//
// 返回值:
//   - 一个 nasl.Scan/nasl.ScanTarget 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：处理扫描发现的风险
//
//	res = nasl.ScanTarget("192.168.1.1:80", nasl.riskHandle(func(r) {
//	    println("risk:", r)
//	}))~
//
// ```
func WithRiskHandle(f func(any)) script_core.NaslScriptConfigOptFunc {
	return script_core.WithRiskHandle(f)
}

// WithProxy 设置 NASL 扫描使用的代理地址列表
// 在 yak 中通过 nasl.proxy 调用
// 参数:
//   - proxies: 一个或多个代理地址
//
// 返回值:
//   - 一个 nasl.Scan/nasl.ScanTarget 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：通过代理扫描
// res = nasl.ScanTarget("192.168.1.1:80", nasl.proxy("socks5://127.0.0.1:1080"))~
// ```
func WithProxy(proxies ...string) script_core.NaslScriptConfigOptFunc {
	return script_core.WithProxy(proxies...)
}

// WithConditions 按条件筛选要运行的 NASL 脚本
// 在 yak 中通过 nasl.conditions 调用
// 参数:
//   - script: 一个或多个筛选条件
//
// 返回值:
//   - 一个 nasl.Scan/nasl.ScanTarget 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：按条件筛选脚本
// res = nasl.ScanTarget("192.168.1.1:80", nasl.conditions({"category": "ACT_GATHER_INFO"}))~
// ```
func WithConditions(script ...any) script_core.NaslScriptConfigOptFunc {
	return script_core.WithConditions(script...)
}

// WithSourcePath 指定额外的 NASL 脚本源码搜索路径
// 在 yak 中通过 nasl.sourcePaths 调用
// 参数:
//   - sourcePath: 一个或多个脚本源码目录
//
// 返回值:
//   - 一个 nasl.Scan/nasl.ScanTarget 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定脚本源码路径
// res = nasl.ScanTarget("192.168.1.1:80", nasl.sourcePaths("/path/to/nasl"))~
// ```
func WithSourcePath(sourcePath ...string) script_core.NaslScriptConfigOptFunc {
	return script_core.WithSourcePath(sourcePath...)
}

var Exports = map[string]any{
	"UpdateDatabase":  UpdateDatabase,
	"RemoveDatabase":  RemoveDatabase,
	"QueryAllScripts": QueryAllScripts,
	"ScanTarget":      ScanTarget,
	"Scan":            Scan,
	"plugin":          WithPlugins,
	"family":          WithFamily,
	"riskHandle":      WithRiskHandle,
	"proxy":           WithProxy,
	"conditions":      WithConditions,
	"sourcePaths":     WithSourcePath,
}
