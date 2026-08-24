---
name: java-audit
metadata:
  display_name_zh-CN: Java 静态审计
description: >
  Java 项目静态安全审计技能。基于 codeaudit 内置工具链：先 java_project_probe 识别构建系统、
  框架与 CMS，再按推荐列表调用依赖 SCA、硬编码密钥、框架配置审计、CMS 专项规则。
  支持 Maven/Gradle 单体与 monorepo（如 RuoYi-Cloud）、scope-modules 模块过滤、
  detection-mode strict/balanced 控制误报。
---

# Java 静态安全审计 (java-audit)

使用 codeaudit 内置 java_audit/* AI 工具对 Java 项目进行确定性静态审计。
所有工具输出 JSON 报告（含 findings、artifacts、meta.files_scanned）。

**执行类任务**：加载本技能后必须调用工具并汇总 findings，不能只描述计划。

---

## 1. 标准流程

1. `java_project_probe` → 识别 build/framework/CMS，读取 recommended_tools
2. `java_maven_gradle_dependencies` → SCA 与 risky 组件
3. `java_hardcoded_secrets_scan` → 硬编码密钥/凭据
4. `java_cms_product_audit` → 若 probe 检测到 CMS
5. probe 推荐的 `java_framework_arch_info` / `java_framework_config_audit` → 按 framework 参数逐项执行
6. 汇总报告（按 severity 排序，附 file/line/evidence）

## 2. 工具注册表

### 2.1 入口与通用
| 工具名 | 用途 |
|--------|------|
| java_project_probe | 框架/CMS 探测，输出 recommended_tools |
| java_maven_gradle_dependencies | Maven/Gradle 依赖与 risky 组件 |
| java_hardcoded_secrets_scan | 硬编码密码/secret/token/JDBC |
| java_cms_product_audit | RuoYi/MCMS/Halo 等产品专项规则 |

### 2.2 框架工具（通过 framework 参数区分）
| 工具名 | 参数 | 用途 |
|--------|------|------|
| java_framework_arch_info | framework=spring_boot | Spring Boot 架构基线 |
| java_framework_config_audit | framework=spring_boot | Spring Boot 配置审计 |
| java_framework_arch_info | framework=shiro | Shiro 架构基线 |
| java_framework_config_audit | framework=shiro | Shiro 配置审计 |
| ... | ... | (支持 12 种框架) |

### 2.3 支持的框架
spring_boot, spring_cloud, spring_security, servlet, mybatis, shiro, struts2, jpa, dubbo, jfinal, vertx, play

## 3. 通用参数
| 参数 | 说明 | 推荐值 |
|------|------|--------|
| target | Java 项目根目录绝对路径 | 必填 |
| detection-mode | permissive / balanced / strict | 默认 balanced |
| scope-modules | 逗号分隔子模块目录名 | monorepo 必配 |
| framework | 框架名 | arch_info/config_audit 必填 |

## 4. 报告格式
每条 finding 包含 id、severity、title、recommendation、evidence[]。
检查 meta.files_scanned：0 表示 scope 配置错误。
