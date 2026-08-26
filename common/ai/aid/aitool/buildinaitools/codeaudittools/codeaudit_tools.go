package codeaudittools

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/codeaudit"
	"github.com/yaklang/yaklang/common/log"
)

// CreateCodeAuditTools builds the Go-native AI tools for Java static security auditing.
// These tools wrap the codeaudit library directly — no yak script involvement.
func CreateCodeAuditTools() []*aitool.Tool {
	factory := aitool.NewFactory()

	register := func(name string, opts ...aitool.ToolOption) {
		if err := factory.RegisterTool(name, opts...); err != nil {
			log.Errorf("register %s tool: %v", name, err)
		}
	}

	// 1. java_project_probe
	register("java_project_probe",
		aitool.WithVerboseName("Java Project Probe"),
		aitool.WithVerboseNameZh("Java项目探测"),
		aitool.WithDescription("探测 Java 项目构建系统、主流框架与 CMS，并推荐后续审计工具。输出构建系统(Maven/Gradle)、检测到的框架列表、CMS 产品列表及推荐工具列表。"),
		aitool.WithKeywords([]string{"java", "audit", "probe", "framework detection", "spring boot", "struts", "mybatis", "Java审计", "框架探测"}),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Java 项目根目录绝对路径"),
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
			opts := buildOptions(params)

			report := codeaudit.ProbeProject(target, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 2. java_maven_gradle_dependencies
	register("java_maven_gradle_dependencies",
		aitool.WithVerboseName("Java Dependency SCA"),
		aitool.WithVerboseNameZh("Java依赖SCA分析"),
		aitool.WithDescription("从 Maven/Gradle/JAR 提取第三方依赖与版本，进行 SCA 并标记已知高危组件（如 fastjson、shiro、log4j、commons-collections 等）。"),
		aitool.WithKeywords([]string{"java", "audit", "sca", "dependency", "maven", "gradle", "依赖", "组件"}),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Java 项目根目录绝对路径"),
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
			opts := buildOptions(params)

			report := codeaudit.ScanDependencies(target, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 3. java_hardcoded_secrets_scan
	register("java_hardcoded_secrets_scan",
		aitool.WithVerboseName("Java Hardcoded Secrets Scan"),
		aitool.WithVerboseNameZh("硬编码密钥扫描"),
		aitool.WithDescription("扫描 Java 源码与配置文件中的硬编码密码、API Key、JDBC 凭据、JWT 与私钥。检测 CWE-798 等硬编码凭证问题。"),
		aitool.WithKeywords([]string{"hardcoded", "secret", "password", "api key", "credential", "CWE-798", "硬编码", "密钥"}),
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
			opts := buildOptions(params)

			report := codeaudit.ScanSecrets(target, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 4. java_cms_product_audit
	register("java_cms_product_audit",
		aitool.WithVerboseName("Java CMS Product Audit"),
		aitool.WithVerboseNameZh("Java CMS产品审计"),
		aitool.WithDescription("识别 RuoYi/MCMS/Halo 等 Java CMS 产品并执行专项配置加固检查。自动检测 CMS 类型后应用对应规则集。"),
		aitool.WithKeywords([]string{"java", "cms", "ruoyi", "mcms", "halo", "CMS", "若依"}),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Java 项目根目录绝对路径"),
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
			opts := buildOptions(params)

			report := codeaudit.AuditCmsProduct(target, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 5. java_framework_arch_info
	register("java_framework_arch_info",
		aitool.WithVerboseName("Java Framework Architecture"),
		aitool.WithVerboseNameZh("Java框架架构基线"),
		aitool.WithDescription("提取指定 Java 框架的架构基线（入口点、路由、模块、配置文件）。支持框架: spring_boot, spring_cloud, spring_security, servlet, mybatis, shiro, struts2, jpa, dubbo, jfinal, vertx, play。"),
		aitool.WithKeywords([]string{"java", "audit", "architecture", "baseline", "架构"}),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Java 项目根目录绝对路径"),
		),
		aitool.WithStringParam("framework",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("框架名: spring_boot|shiro|struts2|servlet|mybatis|..."),
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
			opts := buildOptions(params)

			report := codeaudit.RunFrameworkAudit(target, framework, opts...)
			return marshalReport(report, stdout)
		}),
	)

	// 6. java_framework_config_audit
	register("java_framework_config_audit",
		aitool.WithVerboseName("Java Framework Config Audit"),
		aitool.WithVerboseNameZh("Java框架配置审计"),
		aitool.WithDescription("审计指定 Java 框架的配置安全风险。检查配置文件中的不安全设置（如 actuator 暴露、debug 模式、默认密钥等）。支持框架: spring_boot, spring_cloud, spring_security, servlet, mybatis, shiro, struts2, jpa, dubbo, jfinal, vertx, play。"),
		aitool.WithKeywords([]string{"java", "audit", "config", "security", "配置审计"}),
		aitool.WithStringParam("target",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Java 项目根目录绝对路径"),
		),
		aitool.WithStringParam("framework",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("框架名: spring_boot|shiro|struts2|servlet|mybatis|..."),
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
			opts := buildOptions(params)

			report := codeaudit.AuditConfig(target, framework, opts...)
			return marshalReport(report, stdout)
		}),
	)

	return factory.Tools()
}

// buildOptions converts tool invoke params into codeaudit ProbeOption slice.
func buildOptions(params aitool.InvokeParams) []codeaudit.ProbeOption {
	opts := []codeaudit.ProbeOption{
		codeaudit.WithLanguage("java"),
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

// marshalReport serializes a codeaudit.Report to JSON, writes a summary to stdout,
// and returns the JSON string as the tool result.
func marshalReport(report *codeaudit.Report, stdout io.Writer) (any, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	jsonStr := string(data)

	// Write a short summary line to stdout
	summary := report.Summary
	if summary == "" {
		summary = strings.Join([]string{
			"tool:", report.Tool,
			"status:", report.Status,
			"findings:", strconv.Itoa(len(report.Findings)),
		}, " ")
	}
	stdout.Write([]byte(summary + "\n"))

	return jsonStr, nil
}