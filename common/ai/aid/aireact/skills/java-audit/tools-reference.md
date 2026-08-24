# Java Audit Tools Reference

## java_project_probe

探测 Java 项目的构建系统、框架和 CMS 产品。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | Java 项目根目录绝对路径 |
| detection-mode | string | 否 | balanced | permissive / balanced / strict |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| cms-products | string | 否 | - | 强制 CMS id |
| dedupe-findings | string | 否 | true | true / false |

输出 artifacts: build_system, detected_frameworks, detected_cms_products, recommended_tools

## java_maven_gradle_dependencies

从 Maven/Gradle/JAR 提取依赖并进行 SCA 分析。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | Java 项目根目录绝对路径 |
| risky-mode | string | 否 | name | name / off |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| dedupe-findings | string | 否 | true | true / false |

输出 artifacts: dependencies, risky_components

## java_hardcoded_secrets_scan

扫描硬编码密码、API Key、JDBC 凭据、JWT 与私钥。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | 项目根目录绝对路径 |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |

## java_cms_product_audit

识别 RuoYi/MCMS/Halo 等 CMS 产品并执行专项配置检查。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | Java 项目根目录绝对路径 |
| cms-products | string | 否 | - | 强制 CMS id |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| dedupe-findings | string | 否 | true | true / false |

## java_framework_arch_info

提取指定框架的架构基线（入口点、配置文件、模块结构）。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | Java 项目根目录绝对路径 |
| framework | string | 是 | - | 框架名 |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| dedupe-findings | string | 否 | true | true / false |

支持框架: spring_boot, spring_cloud, spring_security, servlet, mybatis, shiro, struts2, jpa, dubbo, jfinal, vertx, play

## java_framework_config_audit

审计指定框架的配置安全风险。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | Java 项目根目录绝对路径 |
| framework | string | 是 | - | 框架名 |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| config-scope | string | 否 | framework | framework / all |
| dedupe-findings | string | 否 | true | true / false |
