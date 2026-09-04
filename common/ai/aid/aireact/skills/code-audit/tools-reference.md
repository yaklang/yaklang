# Code Audit Tools Reference (multi-language)

> Generic Go-native AI tools registered in the `codeaudittools` package.
> Every tool takes a required `language` param (java | python | go | php | node)
> and dispatches through the per-language catalog content packs.

## project_probe

探测项目构建系统、框架和 CMS 产品，输出推荐工具列表。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | 项目根目录绝对路径 |
| language | string | 是 | - | java / python / go / php / node |
| detection-mode | string | 否 | balanced | permissive / balanced / strict |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| cms-products | string | 否 | - | 强制 CMS id |
| dedupe-findings | string | 否 | true | true / false |

输出 artifacts: build_system, detected_frameworks, detected_cms_products, recommended_tools

构建系统识别：java→maven/gradle；python→pip/pipenv/poetry/setuptools；go→go-modules；php→composer；node→npm/yarn/pnpm。

## dependencies_scan

提取依赖清单并进行 SCA 分析（java: pom.xml/build.gradle；python: requirements.txt/pyproject.toml；
go: go.mod/go.sum；php: composer.json/composer.lock；node: package.json 及 lockfile）。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | 项目根目录绝对路径 |
| language | string | 是 | - | java / python / go / php / node |
| risky-mode | string | 否 | name | name / off |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| dedupe-findings | string | 否 | true | true / false |

输出 artifacts: dependencies, risky_components

## secrets_scan

扫描硬编码密码、API Key、数据库连接串内嵌凭据、dotenv 凭据、JWT 与私钥（CWE-798）。
规则为语言无关的通用规则集。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | 项目根目录绝对路径 |
| language | string | 是 | - | java / python / go / php / node |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |

## framework_arch_info

提取指定框架的架构基线（入口点、配置文件、模块结构）。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | 项目根目录绝对路径 |
| language | string | 是 | - | java / python / go / php / node |
| framework | string | 是 | - | 框架 id（见 language 枚举说明） |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| dedupe-findings | string | 否 | true | true / false |

## framework_config_audit

审计指定框架/语言的配置安全风险。config-scope=all 时审计该语言全部规则。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | 项目根目录绝对路径 |
| language | string | 是 | - | java / python / go / php / node |
| framework | string | 是 | - | 框架 id 或语言级 id（python/php/node/go） |
| config-scope | string | 否 | framework | framework / all |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| dedupe-findings | string | 否 | true | true / false |

典型规则 id 前缀：py.*（python）、go.*、php.*、node.*，java 沿用 spring.*/shiro.*/struts2.* 等。

## cms_product_audit

识别 CMS 产品并执行专项配置规则（java: RuoYi/MCMS/Halo 等；php: WordPress）。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| target | string | 是 | - | 项目根目录绝对路径 |
| language | string | 是 | - | java / python / go / php / node |
| cms-products | string | 否 | - | 强制 CMS id |
| scope-modules | string | 否 | - | 逗号分隔子模块名 |
| scope-exclude | string | 否 | - | 排除路径片段 |
| dedupe-findings | string | 否 | true | true / false |
