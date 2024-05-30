package coreplugin

import (
	"github.com/yaklang/yaklang/common/schema"
	"strings"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var (
	buildInPlugin = make(map[string]*schema.YakScript)
)

type pluginConfig struct {
	Help     string
	Author   []string
	ParamRaw string
}

type pluginOption func(*pluginConfig)

func withPluginHelp(pluginHelp string) pluginOption {
	return func(config *pluginConfig) {
		config.Help = pluginHelp
	}
}

func withPluginAuthors(authors ...string) pluginOption {
	return func(config *pluginConfig) {
		config.Author = authors
	}
}

func withPluginParamRaw(s string) pluginOption {
	return func(config *pluginConfig) {
		config.ParamRaw = s
	}
}

func registerBuildInPlugin(pluginType string, name string, opt ...pluginOption) {
	var codes = string(GetCorePluginData(name))
	if len(codes) <= 0 {
		return
	}

	config := &pluginConfig{}
	for _, o := range opt {
		o(config)
	}
	var plugin = &schema.YakScript{
		ScriptName:         name,
		Type:               pluginType,
		Content:            codes,
		Help:               config.Help,
		Author:             "yaklang.io",
		Params:             config.ParamRaw,
		OnlineContributors: strings.Join(config.Author, ","),
		Uuid:               uuid.New().String(),
		OnlineOfficial:     true,
		IsCorePlugin:       true,
		HeadImg:            `https://yaklang.oss-cn-beijing.aliyuncs.com/yaklang-avator-logo.png`,
	}
	buildInPlugin[name] = plugin
	OverWriteYakPlugin(plugin.ScriptName, plugin)
}

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		log.Debug("start to load core plugin")
		registerBuildInPlugin(
			"mitm",
			"HTTP请求走私",
			withPluginAuthors("V1ll4n"),
			withPluginHelp("HTTP请求走私漏洞检测，通过设置畸形的 Content-Length(CL) 和 Transfer-Encoding(TE) 来检测服务器是否会对畸形数据包产生不安全的反应。"),
		)
		registerBuildInPlugin(
			"mitm", "CSRF 表单保护与 CORS 配置不当检测",
			withPluginHelp("检测应用是否存在 CSRF 表单保护，以及 CORS 配置不当"),
			withPluginAuthors("Rookie"),
		)
		registerBuildInPlugin(
			"mitm", "Fastjson 综合检测",
			withPluginHelp("综合 FastJSON 反序列化漏洞检测"),
			withPluginAuthors("z3"),
		)
		registerBuildInPlugin(
			"mitm", "Shiro 指纹识别 + 弱密码检测",
			withPluginHelp("识别应用是否是 Shiro 应用，尝试检测默认 KEY (CBC/GCM 模式均支持)，当发现默认KEY之后进行一次利用链检测"),
			withPluginAuthors("z3", "go0p"),
		)
		registerBuildInPlugin(
			"mitm", "SSRF HTTP Public",
			withPluginHelp("检测参数中的 SSRF 漏洞"),
		)
		registerBuildInPlugin(
			"mitm", "SQL注入-UNION注入-MD5函数",
			withPluginHelp("Union 注入，使用 md5 函数检测特征输出（mysql/postgres）"),
			withPluginAuthors("V1ll4n"),
		)
		registerBuildInPlugin(
			"mitm", "SQL注入-MySQL-ErrorBased",
			withPluginHelp("MySQL 报错注入（使用 MySQL 十六进制字符串特征检测）"),
			withPluginAuthors("V1ll4n"),
		)
		registerBuildInPlugin(
			"mitm",
			"SSTI Expr 服务器模版表达式注入",
			withPluginHelp("SSTI 服务器模版表达式注入漏洞（通用漏洞检测）"),
			withPluginAuthors("V1ll4n"),
		)
		registerBuildInPlugin(
			"mitm", "Swagger JSON 泄漏",
			withPluginHelp("检查网站是否开放 Swagger JSON 的 API 信息"),
			withPluginAuthors("V1ll4n"),
		)
		registerBuildInPlugin(
			"mitm", "启发式SQL注入检测",
			withPluginHelp("请求包中各种情况参数进行sql注入检测"),
			withPluginAuthors("雨过天晴&伞落人离"),
		)
		registerBuildInPlugin(
			"mitm", "基础 XSS 检测",
			withPluginHelp("一个检测参数中的 XSS 算法，支持各种被编码或 JSON 中的 XSS 检测"),
			withPluginAuthors("WaY"),
		)
		registerBuildInPlugin(
			"mitm", "文件包含",
			withPluginHelp(`利用PHP伪协议特性和base64收敛特性测试文件包含`),
			withPluginAuthors("V1ll4n"),
		)
		registerBuildInPlugin(
			"mitm", "开放 URL 重定向漏洞",
			withPluginHelp("检测开放 URL 重定向漏洞，可检查 meta / js / location 中的内容"),
			withPluginAuthors("Rookie"),
		)
		registerBuildInPlugin(
			"mitm", "回显命令注入",
			withPluginHelp("检测回显型命令注入漏洞（不检测 Cookie 中的命令注入）"),
			withPluginAuthors("V1ll4n"),
		)
		registerBuildInPlugin(
			"yak", "核心引擎性能采样",
			withPluginHelp("动态开启PPROF采样，用于性能调优"),
			withPluginAuthors("V1ll4n,Q16G"),
			withPluginParamRaw(`"[{\"Field\":\"memProfile\",\"TypeVerbose\":\"string\",\"FieldVerbose\":\"内存文件路径\",\"Help\":\"设置默认内存的profile文件路径\",\"MethodType\":\"string\"},{\"Field\":\"cpuProfileFile\",\"TypeVerbose\":\"string\",\"FieldVerbose\":\"cpu文件路径\",\"Help\":\"设置默认cpu的profile文件路径\",\"MethodType\":\"string\"},{\"Field\":\"timeout\",\"DefaultValue\":\"10\",\"TypeVerbose\":\"float\",\"FieldVerbose\":\"检测时间\",\"Help\":\"检测 timeout 时间\",\"Required\":true,\"MethodType\":\"float\"},{\"Field\":\"startMemory\",\"DefaultValue\":\"true\",\"TypeVerbose\":\"boolean\",\"FieldVerbose\":\"是否检测内存\",\"Help\":\"开始检测内存\",\"Required\":true,\"MethodType\":\"boolean\"},{\"Field\":\"startCpu\",\"DefaultValue\":\"true\",\"TypeVerbose\":\"boolean\",\"FieldVerbose\":\"是否检测cpu\",\"Help\":\"开始检测cpu\",\"Required\":true,\"MethodType\":\"boolean\"}]"`),
		)
		return nil
	})
}

func OverWriteCorePluginToLocal() {
	for pluginName, instance := range buildInPlugin {
		OverWriteYakPlugin(pluginName, instance)
	}
}

func OverWriteYakPlugin(name string, scriptData *schema.YakScript) {
	codeBytes := GetCorePluginData(name)
	if codeBytes == nil {
		log.Errorf("fetch buildin-plugin: %v failed", name)
		return
	}
	backendSha1 := utils.CalcSha1(string(codeBytes), scriptData.HeadImg, strings.Trim(scriptData.Params, `"`))
	databasePlugins := yakit.QueryYakScriptByNames(consts.GetGormProfileDatabase(), name)
	if len(databasePlugins) == 0 {
		log.Infof("add core plugin %v to plugin database", name)
		// 添加核心插件字段
		scriptData.IsCorePlugin = true
		err := yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, scriptData)
		if err != nil {
			log.Errorf("create/update yak script[%v] failed: %s", name, err)
		}
		return
	}
	databasePlugin := databasePlugins[0]
	if databasePlugin.Content != "" && utils.CalcSha1(databasePlugin.Content, databasePlugin.HeadImg, strings.Trim(databasePlugin.Params, `"`)) == backendSha1 && databasePlugin.IsCorePlugin {
		log.Debugf("existed plugin's code is not changed, skip: %v", name)
		return
	} else {
		err := yakit.DeleteYakScriptByID(consts.GetGormProfileDatabase(), int64(databasePlugin.ID))
		if err != nil {
			log.Warnf("delete legacy script reason: overrid")
		}
		log.Infof("start to override existed plugin: %v", name)
		err = yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, scriptData)
		if err != nil {
			log.Errorf("override %v failed: %s", name, err)
			return
		}
		log.Debugf("override buildin-plugin %v success", name)
	}
}
