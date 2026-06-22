# Java Audit 工具参数参考

## java_project_probe

| 参数 | 默认 | 说明 |
|------|------|------|
| target | (required) | 项目根或子模块路径 |
| detection-mode | balanced | permissive / balanced / strict |
| scope-modules | "" | 模块目录名，逗号分隔 |
| scope-exclude | "" | 排除路径片段 |
| include-frameworks | "" | 强制包含框架名 |
| exclude-frameworks | "" | 强制排除框架名 |
| tool-profile | full | full / minimal / config-only / deps-secrets |
| resolve-monorepo-root | false | 一般保持 false；RuoYi 靠 scope 扩展即可 |
| cms-products | "" | ruoyi, ruoyi-cloud, mcms, halo 等 |
| config-scope | framework | framework / all |
| risky-mode | name | SCA risky 匹配：name / off |
| dedupe-findings | true | 同规则 dedupe |
| audit-options | "" | JSON 覆盖 |
| output-md | "" | 可选 markdown 输出路径 |

**输出关键字段**：`artifacts.scan_root`、`artifacts.detected_frameworks`、`artifacts.detected_cms_products`、`artifacts.recommended_tools`、`meta.files_scanned`

## java_maven_gradle_dependencies

同 probe 的 scope/detection/audit-options。输出 `artifacts.dependencies`、`findings`（risky 组件）。

## java_hardcoded_secrets_scan

同 probe 的 scope 参数。扫描 `.java`、`.properties`、`.yml`、`.xml` 等。

## java_cms_product_audit

| 参数 | 说明 |
|------|------|
| cms-products | 必填或依赖 probe 传递；如 `ruoyi-cloud` |
| cms-min-confidence | 覆盖 CMS 检测阈值 |

已知 CMS id：`ruoyi`、`ruoyi-cloud`、`mcms`、`halo`、`publiccms`、`mall` 等（以 cms_catalog 为准）。

## 框架 arch_info / config_audit

工具名模式：`{framework}_arch_info`、`{framework}_config_audit`。

framework 取值：spring_boot, spring_cloud, spring_security, servlet, mybatis, shiro, struts2, jpa, dubbo, jfinal, vertx, play

共用参数：target, detection-mode, scope-modules, scope-exclude, dedupe-findings, audit-options, config-scope（config 类）, output-md

## probe → 下游映射

| probe detected_framework.name | arch_info | config_audit |
|------------------------------|-----------|--------------|
| spring_boot | spring_boot_arch_info | spring_boot_config_audit |
| spring_cloud | spring_cloud_arch_info | spring_cloud_config_audit |
| spring_security | spring_security_arch_info | spring_security_config_audit |
| servlet | servlet_arch_info | servlet_config_audit |
| mybatis | mybatis_arch_info | mybatis_config_audit |
| shiro | shiro_arch_info | shiro_config_audit |
| struts2 | struts2_arch_info | struts2_config_audit |
| jpa | jpa_arch_info | jpa_config_audit |
| dubbo | dubbo_arch_info | dubbo_config_audit |
| jfinal | jfinal_arch_info | jfinal_config_audit |
| vertx | vertx_arch_info | vertx_config_audit |
| play | play_arch_info | play_config_audit |

若 `detected_cms_products` 非空，额外执行 `java_cms_product_audit`。
