package coreplugin

import (
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var heuristicSQLPlugin *yakit.YakScript
var basicSSTIPlugin *yakit.YakScript
var shiroKeyPlugin *yakit.YakScript
var basicXSSPlugin *yakit.YakScript
var swaggerJSONPlugin *yakit.YakScript

func init() {
	codeBytes := GetCorePluginData("启发式SQL注入检测")
	heuristicSQLPlugin = &yakit.YakScript{
		ScriptName:           "启发式SQL注入检测",
		Type:                 "mitm",
		Content:              string(codeBytes),
		Params:               "\"[{\\\"Field\\\":\\\"target\\\",\\\"TypeVerbose\\\":\\\"string\\\",\\\"FieldVerbose\\\":\\\"目标(URL/IP:Port)\\\",\\\"Help\\\":\\\"输入插件的测试目标，进行基础爬虫（最多10个请求）\\\",\\\"Required\\\":true}]\"",
		Help:                 "对url中的参数进行sql注入检测",
		Author:               "雨过天晴&伞落人离",
		IsGeneralModule:      true,
		GeneralModuleVerbose: "启发式SQL注入检测",
		GeneralModuleKey:     "启发式SQL注入检测",
		IsBatchScript:        true,
		OnlineId:             4766,
		OnlineScriptName:     "启发式SQL注入检测",
		UserId:               1020,
		Uuid:                 uuid.NewV4().String(),
		HeadImg:              "https://thirdwx.qlogo.cn/mmopen/vi_32/DYAIOgq83erZ7f4SaZmr05aSgPtHx2UN0acplnFbibWictadibibQR88yMow1LyG8ZmmpI2SbV4GF43iaVWVMARKXvA/132",
		OnlineBaseUrl:        "https://www.yaklang.com",
		OnlineOfficial:       true,
	}
	codeBytes = GetCorePluginData("SSTI Expr 服务器模版表达式注入")
	basicSSTIPlugin = &yakit.YakScript{
		ScriptName:       "SSTI Expr 服务器模版表达式注入",
		Type:             "mitm",
		Content:          string(codeBytes),
		Params:           "\"null\"",
		Help:             "SSTI 服务器模版表达式注入漏洞（通用漏洞检测）",
		Tags:             "ssti,expr,injection,general",
		Author:           "V1ll4n",
		OnlineId:         18658,
		OnlineScriptName: "SSTI Expr 服务器模版表达式注入",
		UserId:           6,
		Uuid:             uuid.NewV4().String(),
		HeadImg:          "https://thirdwx.qlogo.cn/mmopen/vi_32/VXssGw0QDiaytOYmU0kTk95CEaFKd0ytlUAYLm26kwJkSVztZAnZBI72f4WwMqMORZP3ib4czXNIyIrKpnEqLPEA/132",
		OnlineBaseUrl:    "https://www.yaklang.com",
		OnlineOfficial:   true,
	}
	codeBytes = GetCorePluginData("Shiro 指纹识别 + 弱密码检测")
	shiroKeyPlugin = &yakit.YakScript{
		ScriptName:       "Shiro 指纹识别 + 弱密码检测",
		Type:             "mitm",
		Content:          string(codeBytes),
		Params:           "\"[{\\\"Field\\\":\\\"target\\\",\\\"TypeVerbose\\\":\\\"string\\\",\\\"FieldVerbose\\\":\\\"目标(URL/IP:Port)\\\",\\\"Help\\\":\\\"输入插件的测试目标，进行基础爬虫（最多10个请求）\\\",\\\"Required\\\":true}]\"",
		Help:             "识别应用是否是 Shiro 应用，尝试检测默认 KEY (CBC/GCM 模式均支持)，当发现默认KEY之后进行一次利用链探测",
		Author:           "z3",
		Tags:             "shiro",
		OnlineId:         4146,
		OnlineScriptName: "Shiro 指纹识别 + 弱密码检测",
		IsBatchScript:    true,
		UserId:           11,
		Uuid:             uuid.NewV4().String(),
		HeadImg:          "https://thirdwx.qlogo.cn/mmopen/vi_32/ag7nfjFEdqcF2OsROrmibCjC3PkdSlErXia1iaSicd5MkkBIpOlXIfQoDgNDuzF0bG3bqCsSuVGiaqGQVIeZ8x2E0sw/132",
		OnlineBaseUrl:    "https://www.yaklang.com",
		OnlineOfficial:   true,
	}
	codeBytes = GetCorePluginData("基础 XSS 检测")
	basicXSSPlugin = &yakit.YakScript{
		ScriptName:       "基础 XSS 检测",
		Type:             "mitm",
		Content:          string(codeBytes),
		Params:           "\"[{\\\"Field\\\":\\\"target\\\",\\\"TypeVerbose\\\":\\\"string\\\",\\\"FieldVerbose\\\":\\\"目标(URL/IP:Port)\\\",\\\"Help\\\":\\\"输入插件的测试目标，进行基础爬虫（最多10个请求）\\\",\\\"Required\\\":true}]\"",
		Help:             "反射型 XSS 检测",
		Author:           "WaY",
		Tags:             "xss",
		OnlineId:         4152,
		OnlineScriptName: "基础 XSS 检测",
		IsBatchScript:    true,
		UserId:           9,
		Uuid:             uuid.NewV4().String(),
		HeadImg:          "https://thirdwx.qlogo.cn/mmopen/vi_32/08picvWzDibBXdgHtxeRfo00atwUrJXmyadRd2icfq66V4KrMvOKH44Bl7rvEqEJkHTByiaybGkUqtKTI0XGc52tCA/132",
		OnlineBaseUrl:    "https://www.yaklang.com",
		OnlineOfficial:   true,
	}

	codeBytes = GetCorePluginData("Swagger JSON 泄漏")
	swaggerJSONPlugin = &yakit.YakScript{
		ScriptName:     "Swagger JSON 泄漏",
		Type:           "mitm",
		Content:        string(codeBytes),
		Params:         "\"[{\\\"Field\\\":\\\"target\\\",\\\"TypeVerbose\\\":\\\"string\\\",\\\"FieldVerbose\\\":\\\"目标(URL/IP:Port)\\\",\\\"Help\\\":\\\"输入插件的测试目标，进行基础爬虫（最多10个请求）\\\",\\\"Required\\\":true}]\"",
		Help:           "检查网站是否开放 Swagger JSON 的 API 信息",
		Author:         "V1ll4n",
		Tags:           "swagger",
		UserId:         6,
		Uuid:           uuid.NewV4().String(),
		HeadImg:        "https://thirdwx.qlogo.cn/mmopen/vi_32/VXssGw0QDiaytOYmU0kTk95CEaFKd0ytlUAYLm26kwJkSVztZAnZBI72f4WwMqMORZP3ib4czXNIyIrKpnEqLPEA/132",
		OnlineBaseUrl:  "https://www.yaklang.com",
		OnlineOfficial: true,
	}
}

func OverWriteCorePluginToLocal() {
	OverWriteYakPlugin("启发式SQL注入检测", heuristicSQLPlugin)
	OverWriteYakPlugin("SSTI Expr 服务器模版表达式注入", basicSSTIPlugin)
	OverWriteYakPlugin("Shiro 指纹识别 + 弱密码检测", shiroKeyPlugin)
	OverWriteYakPlugin("基础 XSS 检测", basicXSSPlugin)
	OverWriteYakPlugin("Swagger JSON 泄漏", swaggerJSONPlugin)
}

func OverWriteYakPlugin(name string, scriptData *yakit.YakScript) {
	log.Infof("start to override plugin: %v", name)
	codeBytes := GetCorePluginData(name)
	if codeBytes == nil {
		log.Errorf("fetch buildin-plugin: %v failed", name)
		return
	}
	backendSha1 := utils.CalcSha1(string(codeBytes))
	databasePlugins := yakit.QueryYakScriptByNames(consts.GetGormProfileDatabase(), name)
	if len(databasePlugins) == 0 {
		log.Infof("no-existed plugin: %v, insert plugin instance", name)
		// 添加核心插件字段
		scriptData.IsCorePlugin = true
		err := yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, scriptData)
		if err != nil {
			log.Errorf("create/update yak script[%v] failed: %s", name, err)
		}
		return
	}
	databasePlugin := databasePlugins[0]
	if databasePlugin.Content != "" && utils.CalcSha1(databasePlugin.Content) == backendSha1 && databasePlugin.IsCorePlugin {
		log.Debugf("existed plugin's code is not changed, skip: %v", name)
		return
	} else {
		databasePlugin.Content = string(codeBytes)
		databasePlugin.IsCorePlugin = true
		err := yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, databasePlugin)
		if err != nil {
			log.Errorf("override %v failed: %s", name, err)
			return
		}
		log.Debugf("override buildin-plugin %v success", name)
	}
}
