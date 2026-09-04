---
name: code-audit
metadata:
  display_name_zh-CN: 多语言静态审计
description: >
  多语言（java/python/go/php/node）项目静态安全审计技能。基于 codeaudit Go 原生工具链的
  通用工具集：所有工具带必填 language 参数，先 project_probe 识别构建系统、框架与 CMS，
  再按推荐列表调用 dependencies_scan、secrets_scan、framework_config_audit、cms_product_audit。
  java 语言与 java-audit 技能的 java_* 工具等价（language=java 走同一管道）。
---

# 多语言静态安全审计 (code-audit)

使用 codeaudit 通用 AI 工具对 java/python/go/php/node 项目进行确定性静态审计。
所有工具输出 JSON 报告（含 findings、artifacts、meta.files_scanned）。
每个工具都必须传 `language` 参数（java | python | go | php | node）。

**执行类任务**：加载本技能后必须调用工具并汇总 findings，不能只描述计划。

---

## 1. 标准流程

1. `project_probe` (language) → 识别 build/framework/CMS，读取 recommended_tools
2. `dependencies_scan` (language) → 依赖清单与 risky 组件
3. `secrets_scan` (language) → 硬编码密码/密钥/数据库连接串凭据
4. `cms_product_audit` (language) → 若 probe 检测到 CMS（php: WordPress；java: RuoYi 等）
5. probe 推荐的 `framework_arch_info` / `framework_config_audit` (language + framework) → 逐项执行
6. 汇总报告（按 severity 排序，附 file/line/evidence）

## 2. 工具注册表

| 工具名 | 必填参数 | 用途 |
|--------|----------|------|
| project_probe | target, language | 构建/框架/CMS 探测，输出 recommended_tools |
| dependencies_scan | target, language | 依赖 SCA 与 risky 组件 |
| secrets_scan | target, language | 硬编码密钥/凭据 |
| framework_arch_info | target, language, framework | 框架架构基线（入口点/配置文件/模块） |
| framework_config_audit | target, language, framework | 框架与语言级配置规则审计 |
| cms_product_audit | target, language | CMS 产品专项规则 |

## 3. 各语言 framework 取值

| language | framework 枚举 | 典型规则 |
|----------|----------------|----------|
| java | spring_boot, spring_cloud, spring_security, servlet, mybatis, shiro, struts2, jpa, dubbo, jfinal, vertx, play | actuator 暴露、明文密码、shiro cipherKey |
| python | django, flask, fastapi, tornado, sqlalchemy（语言级用 python） | DEBUG=True、SECRET_KEY 硬编码、pickle/yaml 反序列化 |
| go | gin, echo, beego, fiber, gorm（语言级用 go） | InsecureSkipVerify、弱哈希、http.Client 无超时 |
| php | laravel, thinkphp, wordpress（语言级用 php） | APP_KEY 泄露、APP_DEBUG、eval/unserialize |
| node | express, koa, nestjs（语言级用 node） | rejectUnauthorized:false、eval/exec、CORS 通配 |

## 4. 通用参数

| 参数 | 说明 | 推荐值 |
|------|------|--------|
| target | 项目根目录绝对路径 | 必填 |
| language | java / python / go / php / node | 必填 |
| detection-mode | permissive / balanced / strict | 默认 balanced |
| scope-modules | 逗号分隔子模块目录名 | monorepo 必配 |
| framework | 框架 id 或语言级 id | arch_info/config_audit 必填 |
| config-scope | framework / all | 语言级全量审计用 all |

## 5. 报告格式

每条 finding 包含 id、severity、title、recommendation、evidence[]。
检查 meta.files_scanned：0 表示 scope 配置错误。

详细的工具参数表见 [tools-reference.md](tools-reference.md)。
