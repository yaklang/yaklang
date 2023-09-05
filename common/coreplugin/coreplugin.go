package coreplugin

import (
	"strings"

	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var (
	buildInPlugin = make(map[string]*yakit.YakScript)
)

type pluginConfig struct {
	Help   string
	Author []string
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

func registerBuildInPlugin(pluginType string, name string, opt ...pluginOption) {
	var codes = string(GetCorePluginData(name))

	config := &pluginConfig{}
	for _, o := range opt {
		o(config)
	}

	var plugin = &yakit.YakScript{
		ScriptName:         name,
		Type:               pluginType,
		Content:            codes,
		Help:               config.Help,
		Author:             "yaklang.io",
		OnlineContributors: strings.Join(config.Author, ","),
		Uuid:               uuid.NewV4().String(),
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
			"mitm", "SQL注入扫描：UNION注入",
			withPluginHelp("朴实无华的 SQL 注入检测，检测依赖输出响应的特征 Token"),
			withPluginAuthors("v1ll4n"),
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
			"mitm", "开放 URL 重定向漏洞",
			withPluginHelp("检测开放 URL 重定向漏洞，可检查 meta / js / location 中的内容"),
			withPluginAuthors("Rookie"),
		)
		return nil
	})
}

func OverWriteCorePluginToLocal() {
	for pluginName, instance := range buildInPlugin {
		OverWriteYakPlugin(pluginName, instance)
	}
	//OverWriteYakPlugin("启发式SQL注入检测", heuristicSQLPlugin)
	//OverWriteYakPlugin("SSTI Expr 服务器模版表达式注入", basicSSTIPlugin)
	//OverWriteYakPlugin("Shiro 指纹识别 + 弱密码检测", shiroKeyPlugin)
	//OverWriteYakPlugin("基础 XSS 检测", basicXSSPlugin)
	//OverWriteYakPlugin("Swagger JSON 泄漏", swaggerJSONPlugin)
}

func OverWriteYakPlugin(name string, scriptData *yakit.YakScript) {
	codeBytes := GetCorePluginData(name)
	if codeBytes == nil {
		log.Errorf("fetch buildin-plugin: %v failed", name)
		return
	}
	backendSha1 := utils.CalcSha1(string(codeBytes), scriptData.HeadImg)
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
	if databasePlugin.Content != "" && utils.CalcSha1(databasePlugin.Content, databasePlugin.HeadImg) == backendSha1 && databasePlugin.IsCorePlugin {
		log.Debugf("existed plugin's code is not changed, skip: %v", name)
		return
	} else {
		log.Infof("start to override existed plugin: %v", name)
		databasePlugin.Content = string(codeBytes)
		databasePlugin.IsCorePlugin = true
		databasePlugin.HeadImg = scriptData.HeadImg
		err := yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, databasePlugin)
		if err != nil {
			log.Errorf("override %v failed: %s", name, err)
			return
		}
		log.Debugf("override buildin-plugin %v success", name)
	}
}
