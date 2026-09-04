package codeaudittools

import (
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/codeaudit"
	"github.com/yaklang/yaklang/common/log"
)

// frameworkEnumByLanguage lists the framework ids the arch/config tools accept
// per language (mirrors the catalog content packs).
var frameworkEnumByLanguage = map[string]string{
	"java":   "spring_boot|spring_cloud|spring_security|servlet|mybatis|shiro|struts2|jpa|dubbo|jfinal|vertx|play",
	"python": "django|flask|fastapi|tornado|sqlalchemy",
	"go":     "gin|echo|beego|fiber|gorm",
	"php":    "laravel|thinkphp|wordpress",
	"node":   "express|koa|nestjs",
}

// languageParam registers the shared required `language` parameter.
func languageParam() aitool.ToolOption {
	return aitool.WithStringParam("language",
		aitool.WithParam_Required(true),
		aitool.WithParam_Description("目标语言: java | python | go | php | node"),
		aitool.WithParam_Enum("java", "python", "go", "php", "node"),
	)
}

// CreateGenericCodeAuditTools builds the language-generic AI tools for static
// security auditing. Unlike CreateCodeAuditTools (java_* names, fixed java),
// every tool here takes a required `language` parameter and dispatches through
// the per-language catalog content packs.
func CreateGenericCodeAuditTools() []*aitool.Tool {
	factory := aitool.NewFactory()

	register := func(name string, opts ...aitool.ToolOption) {
		if err := factory.RegisterTool(name, opts...); err != nil {
			log.Errorf("register %s tool: %v", name, err)
		}
	}

	// 1. project_probe
	register("project_probe",
		aitool.WithVerboseName("Project Probe"),
		aitool.WithVerboseNameZh("多语言项目探测"),
		aitool.WithDescription("探测目标语言项目的构建系统、主流框架与 CMS 产品，并推荐后续审计工具。java 识别 Maven/Gradle 与 Spring 全家桶等；python 识别 pip/poetry 与 Django/Flask/FastAPI 等；go 识别 go-modules 与 gin/echo 等；php 识别 composer 与 Laravel/WordPress 等；node 识别 npm/yarn/pnpm 与 express/nestjs 等。"),
		aitool.WithKeywords([]string{"audit", "probe", "framework detection", "project", "探测", "框架探测", "多语言"}),
		languageParam(),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("项目根目录绝对路径"),
		),
		aitool.WithStringParam("detection-mode",
			aitool.WithParam_Default("balanced"),
			aitool.WithParam_Description("检测模式: permissive | balanced | strict"),
			aitool.WithParam_Enum("permissive", "balanced", "strict"),
		),
		aitool.WithStringParam("scope-modules",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("逗号分隔子模块名，用于 monorepo 过滤"),
		),
		aitool.WithStringParam("scope-exclude",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("排除路径片段，逗号分隔"),
		),
		aitool.WithStringParam("cms-products",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("强制 CMS id，逗号分隔"),
		),
		aitool.WithStringParam("dedupe-findings",
			aitool.WithParam_Default("true"),
			aitool.WithParam_Description("是否去重发现: true / false"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			target := params.GetString("target")
			opts := buildGenericOptions(params)

			report := codeaudit.ProbeProject(target, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 2. dependencies_scan
	register("dependencies_scan",
		aitool.WithVerboseName("Dependency SCA"),
		aitool.WithVerboseNameZh("多语言依赖SCA分析"),
		aitool.WithDescription("提取第三方依赖清单并进行 SCA 分析。java 解析 pom.xml/build.gradle；python 解析 requirements.txt/pyproject.toml/Poetry；go 解析 go.mod/go.sum；php 解析 composer.json/composer.lock；node 解析 package.json 及各类 lockfile。已知高危组件按语言标记（如 java 的 fastjson/shiro/log4j、node 的 node-serialize）。"),
		aitool.WithKeywords([]string{"sca", "dependency", "component", "依赖", "组件", "多语言"}),
		languageParam(),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("项目根目录绝对路径"),
		),
		aitool.WithStringParam("risky-mode",
			aitool.WithParam_Default("name"),
			aitool.WithParam_Description("高危组件匹配模式: name | off"),
			aitool.WithParam_Enum("name", "off"),
		),
		aitool.WithStringParam("scope-modules",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("逗号分隔子模块名"),
		),
		aitool.WithStringParam("scope-exclude",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("排除路径片段，逗号分隔"),
		),
		aitool.WithStringParam("dedupe-findings",
			aitool.WithParam_Default("true"),
			aitool.WithParam_Description("是否去重发现: true / false"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			target := params.GetString("target")
			opts := buildGenericOptions(params)

			report := codeaudit.ScanDependencies(target, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 3. secrets_scan
	register("secrets_scan",
		aitool.WithVerboseName("Hardcoded Secrets Scan"),
		aitool.WithVerboseNameZh("多语言硬编码密钥扫描"),
		aitool.WithDescription("扫描源码与配置文件中的硬编码密码、API Key、数据库连接串内嵌凭据、dotenv 凭据、JWT 与私钥（CWE-798）。规则为语言无关的通用规则集，适用于全部支持语言。"),
		aitool.WithKeywords([]string{"hardcoded", "secret", "password", "api key", "credential", "CWE-798", "硬编码", "密钥"}),
		languageParam(),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("项目根目录绝对路径"),
		),
		aitool.WithStringParam("scope-modules",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("逗号分隔子模块名"),
		),
		aitool.WithStringParam("scope-exclude",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("排除路径片段，逗号分隔"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			target := params.GetString("target")
			opts := buildGenericOptions(params)

			report := codeaudit.ScanSecrets(target, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 4. framework_arch_info
	register("framework_arch_info",
		aitool.WithVerboseName("Framework Architecture"),
		aitool.WithVerboseNameZh("框架架构基线"),
		aitool.WithDescription("提取指定框架的架构基线（入口点、配置文件、模块结构）。框架按 language 取值: java=spring_boot|spring_cloud|spring_security|servlet|mybatis|shiro|struts2|jpa|dubbo|jfinal|vertx|play; python=django|flask|fastapi|tornado|sqlalchemy; go=gin|echo|beego|fiber|gorm; php=laravel|thinkphp|wordpress; node=express|koa|nestjs。"),
		aitool.WithKeywords([]string{"architecture", "baseline", "entrypoint", "架构", "入口"}),
		languageParam(),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("项目根目录绝对路径"),
		),
		aitool.WithStringParam("framework",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("框架 id，取值见 language 说明（如 python 的 django/flask、php 的 laravel/wordpress）"),
		),
		aitool.WithStringParam("scope-modules",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("逗号分隔子模块名"),
		),
		aitool.WithStringParam("scope-exclude",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("排除路径片段，逗号分隔"),
		),
		aitool.WithStringParam("dedupe-findings",
			aitool.WithParam_Default("true"),
			aitool.WithParam_Description("是否去重发现: true / false"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			target := params.GetString("target")
			framework := params.GetString("framework")
			opts := buildGenericOptions(params)

			report := codeaudit.RunFrameworkAudit(target, framework, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 5. framework_config_audit
	register("framework_config_audit",
		aitool.WithVerboseName("Framework Config Audit"),
		aitool.WithVerboseNameZh("框架配置审计"),
		aitool.WithDescription("审计指定框架/语言的配置安全风险（如 Django DEBUG、Flask debug 模式、Laravel APP_KEY 泄露、Go InsecureSkipVerify、Node rejectUnauthorized:false、java 各框架的 actuator 暴露/默认密钥等）。框架按 language 取值: java=spring_boot|...|play; python=django|flask|fastapi|tornado|sqlalchemy|python(语言级); go=go; php=laravel|wordpress|php(语言级); node=node。config-scope=all 时审计该语言全部规则。"),
		aitool.WithKeywords([]string{"config", "security", "misconfiguration", "配置审计", "安全配置"}),
		languageParam(),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("项目根目录绝对路径"),
		),
		aitool.WithStringParam("framework",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("框架 id（或语言级 id 如 python/php/node/go），取值见 language 说明"),
		),
		aitool.WithStringParam("scope-modules",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("逗号分隔子模块名"),
		),
		aitool.WithStringParam("scope-exclude",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("排除路径片段，逗号分隔"),
		),
		aitool.WithStringParam("config-scope",
			aitool.WithParam_Default("framework"),
			aitool.WithParam_Description("配置审计范围: framework | all"),
			aitool.WithParam_Enum("framework", "all"),
		),
		aitool.WithStringParam("dedupe-findings",
			aitool.WithParam_Default("true"),
			aitool.WithParam_Description("是否去重发现: true / false"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			target := params.GetString("target")
			framework := params.GetString("framework")
			opts := buildGenericOptions(params)

			report := codeaudit.AuditConfig(target, framework, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 6. cms_product_audit
	register("cms_product_audit",
		aitool.WithVerboseName("CMS Product Audit"),
		aitool.WithVerboseNameZh("CMS产品审计"),
		aitool.WithDescription("识别 CMS 产品并执行专项配置加固检查。java 支持 RuoYi/MCMS/Halo 等；php 支持 WordPress。自动检测 CMS 类型后应用对应规则集（如 WordPress 的 wp-config.php 数据库口令检查）。"),
		aitool.WithKeywords([]string{"cms", "ruoyi", "wordpress", "mcms", "halo", "CMS", "若依"}),
		languageParam(),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("项目根目录绝对路径"),
		),
		aitool.WithStringParam("cms-products",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("强制 CMS id，逗号分隔"),
		),
		aitool.WithStringParam("scope-modules",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("逗号分隔子模块名"),
		),
		aitool.WithStringParam("scope-exclude",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("排除路径片段，逗号分隔"),
		),
		aitool.WithStringParam("dedupe-findings",
			aitool.WithParam_Default("true"),
			aitool.WithParam_Description("是否去重发现: true / false"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			target := params.GetString("target")
			opts := buildGenericOptions(params)

			report := codeaudit.AuditCmsProduct(target, opts...)
			return marshalReport(report, stdout)
		}),
	)

	return factory.Tools()
}

// buildGenericOptions converts generic tool invoke params into codeaudit
// ProbeOption slice. Unlike buildOptions it reads the language from params;
// an omitted language falls back to java (the historical default).
func buildGenericOptions(params aitool.InvokeParams) []codeaudit.ProbeOption {
	language := params.GetString("language")
	if language == "" {
		language = "java"
	}
	opts := []codeaudit.ProbeOption{
		codeaudit.WithLanguage(language),
	}

	if v := params.GetString("detection-mode"); v != "" {
		opts = append(opts, codeaudit.WithDetectionMode(v))
	}
	if v := params.GetString("risky-mode"); v != "" {
		opts = append(opts, codeaudit.WithRiskyMode(v))
	}
	if v := params.GetString("scope-modules"); v != "" {
		opts = append(opts, codeaudit.WithScopeModules(v))
	}
	if v := params.GetString("scope-exclude"); v != "" {
		opts = append(opts, codeaudit.WithScopeExclude(v))
	}
	if v := params.GetString("cms-products"); v != "" {
		opts = append(opts, codeaudit.WithCmsProducts(v))
	}
	if v := params.GetString("config-scope"); v != "" {
		opts = append(opts, codeaudit.WithConfigScope(v))
	}
	if v := params.GetString("dedupe-findings"); v != "" {
		opts = append(opts, codeaudit.WithDedupeFindings(v != "false"))
	}

	return opts
}
