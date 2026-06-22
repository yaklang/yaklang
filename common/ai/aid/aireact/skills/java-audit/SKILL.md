---
name: java-audit
description: >
  Java 项目静态安全审计技能。基于 java_audit 内置工具链：先 java_project_probe 识别构建系统、
  框架与 CMS，再按推荐列表调用依赖 SCA、硬编码密钥、框架配置审计、CMS 专项规则。
  支持 Maven/Gradle 单体与 monorepo（如 RuoYi-Cloud）、scope-modules 模块过滤、
  detection-mode strict/balanced 控制误报。当用户要求 Java 代码审计、Spring Boot/Shiro/RuoYi
  配置审查、Java SCA 或 Java 安全基线时使用。
metadata:
  tool-prefix: java_audit
  primary-tools: java_project_probe,java_maven_gradle_dependencies,java_hardcoded_secrets_scan,java_cms_product_audit
---

# Java 静态安全审计 (java-audit)

使用 Yak 内置 `java_audit/*` AI 工具对 Java 项目进行确定性静态审计。所有工具输出 JSON 报告（含 `findings`、`artifacts`、`meta.files_scanned`）。

**执行类任务**：加载本技能后必须调用工具并汇总 findings，不能只描述计划。

---

## 1. 标准流程

```
1. java_project_probe     → 识别 build/framework/CMS，读取 recommended_tools
2. java_maven_gradle_dependencies → SCA 与 risky 组件
3. java_hardcoded_secrets_scan    → 硬编码密钥/凭据
4. java_cms_product_audit         → 若 probe 检测到 CMS（RuoYi/MCMS/Halo 等）
5. probe 推荐的 *_arch_info / *_config_audit → 按框架逐项执行
6. 汇总报告（按 severity 排序，附 file/line/evidence）
```

**Monorepo / 子模块**（如 RuoYi-Cloud 从 `ruoyi-gateway` 入口扫描）：
- 无需 `--resolve-monorepo-root`；probe 会自动上扩到含 sibling 模块的根目录
- 用 `--scope-modules` 限定模块，例如：`ruoyi-auth,ruoyi-gateway,ruoyi-modules,ruoyi-common,ruoyi-visual,docker`

**降低误报**：日常审计用 `detection-mode=balanced`；对比 benchmark 或压 FP 时用 `strict` + `dedupe-findings=true`。

---

## 2. 工具注册表（require_tool / call-tool 名称）

以下名称与 Yak AI 工具注册名一致（无路径前缀）：

### 2.1 入口与通用

| 工具名 | 用途 |
|--------|------|
| `java_project_probe` | 框架/CMS 探测，输出 `recommended_tools` 与 `scan_root` |
| `java_maven_gradle_dependencies` | Maven/Gradle 依赖与 risky 组件 |
| `java_hardcoded_secrets_scan` | 硬编码 password/secret/token/JDBC 等 |
| `java_cms_product_audit` | RuoYi/MCMS/Halo/PublicCMS 等产品专项规则 |

### 2.2 框架架构基线（arch_info）

| 工具名 | 框架 |
|--------|------|
| `spring_boot_arch_info` | Spring Boot |
| `spring_cloud_arch_info` | Spring Cloud |
| `spring_security_arch_info` | Spring Security |
| `servlet_arch_info` | Servlet / Java EE |
| `mybatis_arch_info` | MyBatis |
| `shiro_arch_info` | Apache Shiro |
| `struts2_arch_info` | Struts2 |
| `jpa_arch_info` | JPA / Hibernate |
| `dubbo_arch_info` | Dubbo |
| `jfinal_arch_info` | JFinal |
| `vertx_arch_info` | Vert.x |
| `play_arch_info` | Play Framework |

### 2.3 框架配置审计（config_audit）

| 工具名 | 框架 |
|--------|------|
| `spring_boot_config_audit` | Actuator、数据源、CORS、devtools |
| `spring_cloud_config_audit` | Nacos/Config Server、inline secret |
| `spring_security_config_audit` | Security 配置风险 |
| `servlet_config_audit` | web.xml |
| `mybatis_config_audit` | Mapper / SQL 配置 |
| `shiro_config_audit` | anon URL、rememberMe |
| `struts2_config_audit` | devMode、DMI |
| `jpa_config_audit` | JPA 配置 |
| `dubbo_config_audit` | Dubbo 配置 |
| `jfinal_config_audit` | JFinal 配置 |
| `vertx_config_audit` | Vert.x 配置 |
| `play_config_audit` | Play 配置 |

完整参数说明见：<!-- include: tools-reference.md -->

---

## 3. 通用参数（多数工具共用）

| 参数 | 说明 | 推荐值 |
|------|------|--------|
| `target` | Java 项目根目录绝对路径 | **必填** |
| `detection-mode` | `permissive` / `balanced` / `strict` | 默认 `balanced`；压 FP 用 `strict` |
| `scope-modules` | 逗号分隔子模块目录名 | monorepo 必配 |
| `scope-exclude` | 排除路径片段 | 可选 |
| `cms-products` | 强制 CMS id，如 `ruoyi-cloud,ruoyi` | 已知 CMS 时 |
| `dedupe-findings` | `true` / `false` | 建议 `true` |
| `audit-options` | JSON 覆盖上述选项 | 批量传参时用 |

`audit-options` 示例：

```json
{
  "detection_mode": "strict",
  "dedupe_findings": true,
  "scope_modules": ["ruoyi-auth", "ruoyi-gateway", "docker"],
  "cms_products": ["ruoyi-cloud"]
}
```

---

## 4. 调用示例

### 4.1 Probe（第一步必做）

```json
{
  "@action": "require_tool",
  "tool": "java_project_probe",
  "identifier": "probe_java_project",
  "params": {
    "target": "/abs/path/to/JavaProject",
    "detection-mode": "balanced",
    "dedupe-findings": "true"
  }
}
```

RuoYi-Cloud 从 gateway 子目录扫描：

```json
{
  "@action": "require_tool",
  "tool": "java_project_probe",
  "identifier": "probe_ruoyi_cloud",
  "params": {
    "target": "/abs/path/to/RuoYi-Cloud/ruoyi-gateway",
    "detection-mode": "strict",
    "scope-modules": "ruoyi-auth,ruoyi-gateway,ruoyi-modules,ruoyi-common,ruoyi-visual,docker",
    "cms-products": "ruoyi-cloud",
    "dedupe-findings": "true"
  }
}
```

### 4.2 按 probe 推荐执行下游工具

从 probe JSON 的 `artifacts.recommended_tools` 逐项 `require_tool`，**同一 `target` 与 scope/detection 参数保持一致**。`scan_root` 以 probe 输出的 `artifacts.scan_root` 为准（可能与传入 target 不同）。

### 4.3 CMS 专项

```json
{
  "@action": "require_tool",
  "tool": "java_cms_product_audit",
  "identifier": "cms_audit",
  "params": {
    "target": "/abs/path/to/RuoYi-Cloud/ruoyi-gateway",
    "scope-modules": "ruoyi-auth,ruoyi-gateway,ruoyi-modules,ruoyi-common,ruoyi-visual,docker",
    "cms-products": "ruoyi-cloud",
    "detection-mode": "strict"
  }
}
```

---

## 5. 报告解读

每条 finding 通常包含：

- `id`：规则 id（如 `ruoyi-cloud.nacos.token.default`、`secret.password_assignment`）
- `severity`：`critical` / `high` / `medium` / `low`
- `title` / `recommendation`
- `evidence[]`：`file`、`line`、`snippet`

检查 `meta.files_scanned`：
- **0** 表示 scope 配置错误或路径不对，先修正 `target` / `scope-modules`，不要继续下游工具
- monorepo 正常应 **> 100**

---

## 6. 输出模板

```markdown
# Java 安全审计报告

## 范围
- 目标：{target}
- scan_root：{probe.scan_root}
- files_scanned：{probe.meta.files_scanned}
- 框架：{detected_frameworks}
- CMS：{detected_cms_products}

## 统计
| 严重程度 | 数量 |
|----------|------|

## 发现详情
### [high] {title}
- 规则：{id}
- 位置：{file}:{line}
- 证据：{snippet}
- 建议：{recommendation}

## 组件风险（SCA）
{dependencies findings}

## 结论与优先级修复项
```

---

## 7. 注意事项

1. **先 probe 再下游**：避免对未识别框架跑错 config 工具
2. **strict 模式**：会过滤弱信号框架（如仅凭 content 命中的 shiro/servlet），减少 FP
3. **RuoYi-Cloud**：配置多在 Nacos/docker，CMS 规则会查 `docker/nacos/conf/application.properties`
4. **不要混用 grep 替代 config 工具**：本技能以 java_audit 工具输出为准；grep 仅作 probe 未覆盖时的补充
5. 确认风险后可用 `cybersecurity-risk` 上报（若会话已启用）
