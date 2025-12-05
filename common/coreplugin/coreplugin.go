package coreplugin

import (
	"strings"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var buildInPlugin = make(map[string]*schema.YakScript)

type pluginConfig struct {
	Help                string
	Author              []string
	Tags                []string
	EnableGenerateParam bool
	Ignore              bool
}

type pluginOption func(*pluginConfig)

// 不希望被本地插件库查看到的插件
func withPluginIgnore(b bool) pluginOption {
	return func(config *pluginConfig) {
		config.Ignore = b
	}
}

func withPluginTags(tags []string) pluginOption {
	return func(config *pluginConfig) {
		config.Tags = tags
	}
}

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

func withPluginEnableGenerateParam(b bool) pluginOption {
	return func(config *pluginConfig) {
		config.EnableGenerateParam = b
	}
}

func registerBuildInPlugin(pluginType string, name string, opt ...pluginOption) {
	codes := string(GetCorePluginData(name))
	if len(codes) <= 0 {
		return
	}

	config := &pluginConfig{}
	for _, o := range opt {
		o(config)
	}

	plugin := &schema.YakScript{
		ScriptName:         name,
		Type:               pluginType,
		Content:            codes,
		Help:               config.Help,
		Author:             "yaklang.io",
		Params:             "",
		Tags:               strings.Join(config.Tags, ","),
		OnlineContributors: strings.Join(config.Author, ","),
		Uuid:               uuid.New().String(),
		OnlineOfficial:     true,
		IsCorePlugin:       true,
		HeadImg:            `https://yaklang.oss-cn-beijing.aliyuncs.com/yaklang-avator-logo.png`,
		Ignored:            config.Ignore,
	}
	buildInPlugin[name] = plugin
	OverWriteYakPlugin(plugin.ScriptName, plugin, config.EnableGenerateParam)
}

var BlackListCorePlugin = []string{
	"启发式SQL注入检测",
}

func ClearBlackListPlugin(blackList []string) {
	err := yakit.DeleteYakScriptByNames(consts.GetGormProfileDatabase(), blackList)
	if err != nil {
		log.Errorf("delete black list plugin failed: %s", err)
	}
}

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {

		ClearBlackListPlugin(BlackListCorePlugin)

		if !consts.IsDevMode() {
			const key = "cd336beba498c97738c275f6771efca3"
			if yakit.Get(key) == consts.ExistedCorePluginEmbedFSHash {
				return nil
			}
			log.Debug("start to load core plugin")
			defer func() {
				hash, _ := CorePluginHash()
				yakit.Set(key, hash)
			}()
		}

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
			"mitm", "Shiro 自定义检测",
			withPluginHelp("识别Shiro应用，用户可以自行指定密钥，gadget，加密模式进行检测"),
			withPluginAuthors("z3", "go0p"),
			withPluginEnableGenerateParam(true),
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
			"mitm", "SQL注入-时间盲注-Sleep",
			withPluginHelp("SQL 时间盲注"),
			withPluginAuthors("WAY"),
		)
		registerBuildInPlugin(
			"mitm", "SQL注入-堆叠注入",
			withPluginHelp("SQL 堆叠注入（带回显），使用 md5 函数检测特征输出（mysql/postgres）"),
			withPluginAuthors("WAY"),
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
		//registerBuildInPlugin(
		//	"mitm", "启发式SQL注入检测",
		//	withPluginHelp("请求包中各种情况参数进行sql注入检测"),
		//	withPluginAuthors("雨过天晴&伞落人离"),
		//)
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
			"mitm", "修改 HTTP 请求 Header",
			withPluginHelp("允许用户加载该插件修改 / 增加一个请求的 Header，可以设置 URL 关键字作为前提条件"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
			withPluginTags([]string{information.FORWARD_HTTP_PACKET}),
		)
		registerBuildInPlugin(
			"mitm", "修改 HTTP 请求 Cookie",
			withPluginHelp("允许用户加载该插件修改 / 增加一个请求的 Cookie，可以设置 URL 关键字作为前提条件"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
			withPluginTags([]string{information.FORWARD_HTTP_PACKET}),
		)
		registerBuildInPlugin(
			"mitm", "修改 HTTP 请求",
			withPluginHelp("允许用户加载该插件修改/增加/删除请求的 Header/Cookie/GetParams/PostParams，可以设置 URL 关键字作为前提条件"),
			withPluginAuthors("WaY"),
			withPluginEnableGenerateParam(true),
			withPluginTags([]string{information.FORWARD_HTTP_PACKET}),
		)
		registerBuildInPlugin(
			"mitm", "多认证综合越权测试",
			withPluginHelp("可以设置 Cookie 和 Header 的多个认证信息进行越权测试，结果包含相似度"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"yak", "简易意图识别",
			withPluginHelp("使用ai对用户的意图进行简易识别"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
			withPluginIgnore(true),
		)

		registerBuildInPlugin(
			"yak", "Tun劫持服务",
			withPluginHelp("通过Tun设备劫持流量并转发到MITM进行处理"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
			withPluginIgnore(true),
		)

		registerBuildInPlugin(
			"yak", "路由表查询",
			withPluginHelp("查询当前tun劫持的路由信息"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
			withPluginIgnore(true),
		)

		registerBuildInPlugin(
			"yak", "路由表增加",
			withPluginHelp("增加tun劫持的路由"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
			withPluginIgnore(true),
		)

		registerBuildInPlugin(
			"yak", "路由表删除",
			withPluginHelp("删除tun劫持的路由"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
			withPluginIgnore(true),
		)

		registerBuildInPlugin(
			"yak", "劫持进程",
			withPluginHelp("劫持指定进程的流量到tun设备"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
			withPluginIgnore(true),
		)

		//registerBuildInPlugin(
		//	"mitm", "MITM 请求修改",
		//	withPluginHelp("允许用户操作请求：增加/删除/替换请求参数，支持请求头，GET参数，POST参数，Cookie，支持匹配到请求再操作，支持多个操作"),
		//	withPluginAuthors("WaY"),
		//	withPluginEnableGenerateParam(true),
		//	withPluginTags([]string{information.FORWARD_HTTP_PACKET}),
		//)
		//registerBuildInPlugin(
		//	"mitm", "MITM 响应修改",
		//	withPluginHelp("允许用户修改响应：支持正则，支持匹配到响应再操作，支持多个操作"),
		//	withPluginAuthors("WaY"),
		//	withPluginEnableGenerateParam(true),
		//	withPluginTags([]string{information.FORWARD_HTTP_PACKET}),
		//)
		registerBuildInPlugin(
			"yak", "核心引擎性能采样",
			withPluginHelp("动态开启PPROF采样，用于性能调优"),
			withPluginAuthors("V1ll4n,Q16G"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"yak", "崩溃日志收集",
			withPluginHelp("收集本地项目日志中的崩溃信息，保存成 zip 以便分析"),
			withPluginAuthors("V1ll4n"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"yak", "SSA 项目编译",
			withPluginHelp("将选择的项目编译到 SSA 数据库内，用于后续的代码查询和分析。"),
			withPluginAuthors("令则"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"yak", "SSA 项目重编译",
			withPluginHelp("将以编译项目使用当前引擎重新编译。用于引擎编译策略更新或代码有变动时使用。"),
			withPluginAuthors("令则"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"yak", "SyntaxFlow 规则执行",
			withPluginHelp("执行 SyntaxFlow 规则"),
			withPluginAuthors("令则"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"yak", "引擎性能采样自动分析",
			withPluginHelp("自动分析PPROF文件，生成可阅读文件。"),
			withPluginAuthors("intSheep"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"yak", "SSA 项目探测",
			withPluginHelp("代码扫描默认规则探测，根据传入的url来提供代码扫描的默认参数"),
			withPluginAuthors("Q16G"),
			withPluginEnableGenerateParam(true))
		registerBuildInPlugin(
			"yak", "SSA 项目更新",
			withPluginHelp("更新SSA项目信息，包括项目名称、描述、语言、配置等"),
			withPluginAuthors("intSheep"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin("yak", "SyntaxFlow Searcher",
			withPluginHelp(`代码审计项目管理中，支持文件名、函数名、变量名、常量等搜索`),
			withPluginAuthors("Q16G"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"yak",
			"SyntaxFlow 查询项目信息",
			withPluginHelp("使用SyntaxFlow查询项目信息，如项目的过滤函数。"),
			withPluginAuthors("intSheep"),
			withPluginEnableGenerateParam(true),
		)
		registerBuildInPlugin(
			"mitm",
			"SQL注入-Path参数注入",
			withPluginAuthors("intSheep"),
			withPluginHelp("SQL注入检测，针对RESTful API风格设计的Path参数进行SQL注入检测"),
		)
		registerBuildInPlugin(
			"mitm",
			"SQL注入-高危Header注入",
			withPluginAuthors("intSheep"),
			withPluginHelp("SQL注入检测，针对高风险的HTTP Header(如X-Forwarded-For、Referer)进行SQL注入检测"),
		)

		registerBuildInPlugin(
			"yak", "构建知识库",
			withPluginHelp("将指定文件构建成知识库，支持多种文件格式"),
			withPluginEnableGenerateParam(true),
		)

		registerBuildInPlugin(
			"yak", "检索知识",
			withPluginHelp("从知识库中语义检索相关知识，返回相关内容"),
			withPluginEnableGenerateParam(true),
		)

		registerBuildInPlugin(
			"yak", "构建知识条目",
			withPluginHelp("从实体库中构建知识条目，支持目的性的知识条目构建"),
			withPluginEnableGenerateParam(true),
		)

		registerBuildInPlugin(
			"yak", "证书生成",
			withPluginHelp("通过各种选项生成所需的证书文件"),
			withPluginEnableGenerateParam(true),
		)
		return nil
	}, "sync-core-plugin-for-yakit")

}

// only use for test
func OverWriteCorePluginToLocal() {
	for pluginName, instance := range buildInPlugin {
		OverWriteYakPlugin(pluginName, instance, true)
	}
}

func OverWriteYakPlugin(name string, scriptData *schema.YakScript, enableGenerateParam bool) {
	var err error
	generateParam := func(code, pluginType string) (string, string, error) {
		if enableGenerateParam {
			oldLevel := log.GetLevel()
			log.SetLevel(log.ErrorLevel)
			defer log.SetLevel(oldLevel)
			prog, err := static_analyzer.SSAParse(code, pluginType)
			if err != nil {
				return "", "", err
			}
			params, envKey, err := information.GenerateParameterFromProgram(prog)
			if err != nil {
				return "", "", utils.Wrapf(err, "generate param for %s failed", name)
			}
			return params, envKey, nil
		}
		return "", "", nil
	}
	pluginHash := func(code string, headImg string, tags string, ignore bool) string {
		return utils.CalcSha1(string(code), headImg, tags, ignore)
	}

	codeBytes := GetCorePluginData(name)
	code := string(codeBytes)
	if codeBytes == nil {
		log.Errorf("fetch buildin-plugin: %v failed", name)
		return
	}
	newestPluginHash := pluginHash(code, scriptData.HeadImg, scriptData.Tags, scriptData.Ignored)

	databasePlugins := yakit.QueryYakScriptByNames(consts.GetGormProfileDatabase(), name)
	if len(databasePlugins) == 0 {
		log.Infof("add core plugin %v to plugin database", name)
		// 添加核心插件字段
		scriptData.IsCorePlugin = true
		// 生成参数
		scriptData.Params, scriptData.PluginEnvKey, err = generateParam(code, scriptData.Type)
		if err != nil {
			log.Error(err)
		}
		err = yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, scriptData)
		if err != nil {
			log.Errorf("create/update yak script[%v] failed: %s", name, err)
		}
		return
	}
	databasePlugin := databasePlugins[0]
	if databasePlugin.Content != "" && newestPluginHash == pluginHash(databasePlugin.Content, databasePlugin.HeadImg, databasePlugin.Tags, databasePlugin.Ignored) && databasePlugin.IsCorePlugin {
		log.Debugf("existed plugin's code is not changed, skip: %v", name)
		return
	} else {
		// 生成参数
		scriptData.Params, scriptData.PluginEnvKey, err = generateParam(string(codeBytes), scriptData.Type)
		if err != nil {
			log.Error(err)
		}
		err := yakit.DeleteYakScriptByIDs(consts.GetGormProfileDatabase(), int64(databasePlugin.ID))
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
