---
name: web-crawler
metadata:
  display_name_zh-CN: Web 爬虫与目标发现
description: >
  Web 目标发现与按需爬取技能。定义何时直接请求 URL、何时进行有界爬取，覆盖爬取策略、
  URL/参数/表单/API 端点的发现与提取、JavaScript 渲染页面处理、爬取结果分析，
  以及与后续漏洞测试的衔接。是侦查(Recon)阶段的核心技能之一。
---

# Web 爬虫与目标发现

在渗透测试的侦查阶段，Web 爬虫是发现攻击面的观测手段之一，不是每个 URL 任务的固定第一步或必做项。
通过系统化的页面爬取和内容分析，提取 URL、参数、表单、API 端点等信息，
为后续的漏洞测试提供完整的目标列表。

## 0. URL 快速路径与调用预算

| 用户目标 | 首选行动 | 默认调用预算 |
|----------|----------|--------------|
| 查看一个已知 URL 的状态、响应头、正文、跳转或是否可访问 | `do_http_request` | 1 次；结果已回答问题就停止 |
| 快速测试一个未知 Web 目标，既要基线响应也要服务端可见入口 | 一次 `do_http_request` + 一次有界 `simple_crawler` | 各 1 次；互不依赖时可并发 |
| 构建多页站点地图、发现链接/表单/参数及有限前端请求候选 | `simple_crawler`（默认 `ai-js=auto`） | 先用较小 `reqs-max`/`max-depth`；只在新 URL 会影响测试时扩展 |
| 判断 JS 执行后的 DOM、表单或交互 | `use_browser` | HTTP/爬虫证据显示 SPA 或交互依赖后，先 `op=open` 1 次 |
| 从前端资产发现路由/API/WebSocket/source map | `crawl_js_collector` → `js_static_extract_ai` | SPA 或重型 JS 证据出现后按需使用 |

执行规则：

1. URL 只是种子，不自动等于“必须爬取”。单 URL 精确响应问题走直接请求快速路径。
2. 用户要求“测试这个未知网站”且基线响应与多页覆盖都会影响后续决策时，直接请求和轻量爬虫各调用一次即可；不要重复相同观测。
3. 任一结果已经回答用户问题或足以开始定向测试时，停止扩大侦查。每次追加调用都必须说明要降低的新不确定性。
4. 普通爬虫不执行 JavaScript；`simple_crawler` 的 `ai-js=auto` 只会下载资产并对有限证据窗口做静态分析。若仍缺少运行态证据，不要机械提高深度或请求数；改看一次渲染后 DOM，或使用完整、可落盘审计的 JS 资产链路。
5. 浏览器渲染态不能替代原始 HTTP 状态、响应头和响应体；直接请求也不能替代 JS 执行后的 DOM。

---

## 1. 爬虫在渗透测试中的位置

```
侦查阶段
├── 被动侦查
│   ├── 子域名枚举
│   ├── DNS 记录收集
│   └── 搜索引擎 Dork
├── 主动侦查
│   ├── 端口扫描 ← 确定 Web 服务端口
│   ├── Web 爬虫 ← 发现页面和参数 (本技能)
│   ├── 目录扫描 ← 发现隐藏路径
│   └── 指纹识别 ← 确定技术栈
└── 产出: 攻击面地图
```

爬虫产出直接驱动后续测试：
- URL + 参数 → 注入测试（SQL/XSS/命令注入等）
- 表单 → CSRF 测试、文件上传测试
- API 端点 → 认证绕过、越权测试
- JavaScript 文件 → 敏感信息泄露、DOM XSS 分析

---

## 2. 爬取策略

### 2.1 广度优先 vs 深度优先

**广度优先（推荐用于初始侦查）**
- 先爬取所有顶层页面，再逐层深入
- 快速覆盖整个站点结构
- 适合发现不同功能模块

**深度优先（用于深入分析）**
- 沿一个路径深入到底，再回溯
- 适合发现深层嵌套的功能
- 可能长时间停留在某个功能模块

**推荐按证据升级**
1. 基线波：根据用户目标使用一次直接 HTTP、一次有界爬虫，或并发取得两类独立证据
2. 覆盖波：仅当服务端链接确实需要覆盖时，做 depth=2-3 的有界广度爬取
3. 升级波：对高价值模块定向深入；疑似 SPA 则切换浏览器/JS 资产路径，不重复加深普通爬虫
4. 只有任务需要并具备授权与凭据时，才做认证后或多角色覆盖

### 2.2 爬取范围控制

**域名范围**
- 同域爬取：只爬取目标域名下的页面
- 子域爬取：包含目标域名的所有子域
- 注意排除第三方服务（CDN、统计、社交平台）

**路径范围**
- 避免爬取登出链接（`/logout`, `/signout`）
- 避免爬取删除操作（`/delete`, `/remove`）
- 排除静态资源目录（`/static/`, `/assets/`, `/images/`）
- 排除无限循环路径（日历、分页等）

**频率控制**
- 设置请求间隔，避免触发 WAF 或被封禁
- 建议初始间隔 100-500ms
- 对生产环境目标适当降低频率

### 2.3 认证处理

**未认证爬取**
- 首先进行未认证爬取，发现公开页面
- 记录登录表单位置和参数

**认证后爬取**
- 使用有效凭据登录后爬取
- 通过 Cookie 或 Token 维持会话
- 对比认证前后的页面差异，发现需要认证的功能

**多角色爬取**
- 使用不同权限的账户分别爬取
- 对比各角色可访问的页面
- 发现越权访问的候选测试点

---

## 3. 信息提取

### 3.1 URL 与参数提取

从爬取结果中提取：
- 完整的 URL 列表（去重后）
- 每个 URL 的查询参数名和示例值
- URL 路径中的动态段（如 `/user/123/profile` 中的 `123`）
- RESTful API 的资源路径模式

**参数分类**
| 参数类型 | 示例 | 测试重点 |
|----------|------|---------|
| ID 类 | `id=123`, `uid=456` | IDOR、SQL 注入 |
| 搜索/过滤 | `q=keyword`, `filter=xxx` | SQL 注入、XSS |
| 文件路径 | `file=report.pdf`, `path=/data` | 路径穿越、文件包含 |
| URL/重定向 | `url=http://...`, `redirect=...` | SSRF、开放重定向 |
| 模版/渲染 | `template=xxx`, `view=xxx` | SSTI |
| 命令/操作 | `cmd=ping`, `action=export` | 命令注入 |
| 排序/列名 | `sort=name`, `order=desc` | SQL 注入（ORDER BY） |

### 3.2 表单发现

提取所有表单信息：
- 表单 `action` URL 和 `method`
- 所有 `input` 字段的 `name`、`type`、`value`
- 隐藏字段（可能包含 CSRF Token、内部参数）
- 文件上传字段（`type="file"`）
- `enctype` 属性（`multipart/form-data` 表示可上传文件）

**高价值表单**
- 登录表单 → 暴力破解、SQL 注入
- 注册表单 → SQL 注入、XSS（存储型）
- 搜索表单 → SQL 注入、XSS（反射型）
- 评论/反馈表单 → 存储型 XSS
- 文件上传表单 → 文件上传漏洞
- 密码修改表单 → CSRF
- 管理功能表单 → 越权、命令注入

### 3.3 API 端点发现

**从 HTML/JS 中提取**
- JavaScript 文件中的 API URL
- AJAX 请求的端点
- 前端路由配置

**常见 API 模式**
```
/api/v1/users
/api/v1/users/{id}
/api/v1/users/{id}/profile
/graphql
/graphql/playground
/.well-known/openapi.json
/swagger.json
/swagger-ui.html
/api-docs
```

**API 文档发现**
- Swagger/OpenAPI: `/swagger.json`, `/v2/api-docs`, `/openapi.yaml`
- GraphQL: `/graphql` (introspection query)
- WADL: `/application.wadl`

### 3.4 敏感信息发现

在爬取过程中关注：
- 注释中的敏感信息（`<!-- TODO: remove before production -->`）
- JavaScript 中硬编码的 API Key、Token
- 错误页面泄露的技术栈信息
- robots.txt 和 sitemap.xml 中的隐藏路径
- .env、.git、.svn 等配置文件暴露
- 备份文件（.bak, .old, .swp, ~）

---

## 4. JavaScript 分析

### 4.1 JS 文件收集

爬虫应收集所有引用的 JavaScript 文件：
- `<script src="...">` 引用的外部 JS
- 内联 `<script>` 块中的代码
- 动态加载的 JS（通过 XHR/Fetch）

### 4.2 JS 中的信息提取

从 JavaScript 文件中提取：
- API 端点 URL（`fetch('/api/...')`, `axios.get('/api/...')`）
- 路由定义（前端路由配置）
- 硬编码的凭据或密钥
- WebSocket 端点
- 第三方服务集成信息

**正则提取模式**
```
# API 端点
("|')(/api/[a-zA-Z0-9/_-]+)("|')
("|')(https?://[^"']+)("|')

# 密钥
(api[_-]?key|apikey|secret|token|password)\s*[:=]\s*["'][^"']+["']

# AWS Key
AKIA[0-9A-Z]{16}
```

### 4.3 JavaScript 渲染页面

SPA 判断是启发式分类，不应由单个特征直接定论。高价值组合信号包括：
- HTML 主要是 `#root` / `#app` / `#__next` / `#__nuxt` / `<app-root>` 挂载壳
- 服务端可见正文、`a[href]` 和表单很少，却存在多个 bundle 或 `type="module"` 脚本
- `__NEXT_DATA__`、`/_next/static/`、`__NUXT__`、`/_nuxt/`、`data-reactroot`、`ng-version`、webpack/Vite/SvelteKit 标记
- 普通爬虫请求成功却只有入口页和静态资产，增加深度仍没有新的服务端导航入口

对于疑似 SPA (Single Page Application)：
- 明确记录普通爬虫不执行 JavaScript；`ai-js=auto` 的结果仍是静态候选，没有发现 URL 不等于没有前端路由或 API
- 先用一次浏览器 `open` 获取渲染后 DOM、表单、链接、脚本和可见文本；没有交互需求时不要继续点击
- 收集入口 HTML 引用的 JS，静态提取路由、API、WebSocket、source map 线索，再用直接 HTTP 请求验证候选
- 只有必须触发懒加载或路由状态时才点击、滚动或等待，每个交互对应一个明确假设
- 只有工具实际提供网络观察时才能声称看到 XHR/Fetch；否则应通过 HTTP 流量或代理能力验证

---

## 5. 爬取结果分析

### 5.1 站点地图构建

将爬取结果组织为树形结构：

```
target.com
├── / (首页)
├── /login (登录)
├── /register (注册)
├── /user/
│   ├── /user/profile (个人资料)
│   └── /user/settings (设置)
├── /api/
│   ├── /api/v1/users
│   ├── /api/v1/posts
│   └── /api/v1/upload
├── /admin/ (管理后台)
│   ├── /admin/dashboard
│   └── /admin/users
└── /search (搜索)
```

### 5.2 攻击面评估

对每个发现的功能点评估潜在风险：

| 功能点 | 输入参数 | 潜在漏洞 | 测试优先级 |
|--------|---------|----------|-----------|
| /search?q= | q (搜索词) | SQL 注入, XSS | 高 |
| /api/v1/users/{id} | id (用户ID) | IDOR, SQL 注入 | 高 |
| /upload | file (文件) | 文件上传漏洞 | 高 |
| /user/profile | name, bio | 存储型 XSS | 中 |
| /admin/* | - | 未授权访问 | 高 |

### 5.3 与漏洞测试的衔接

爬取完成后，将结果按漏洞类型分组，传递给对应的测试技能：

1. **注入测试组**：所有带参数的 URL → sql-injection, command-injection, template-injection
2. **XSS 测试组**：所有参数在响应中有反射的 URL → xss-testing
3. **认证测试组**：登录/注册/密码功能 → 暴力破解、Session 测试
4. **越权测试组**：带 ID 参数的 API → IDOR 测试
5. **文件操作组**：上传/下载功能 → 文件上传、路径穿越
6. **SSRF 测试组**：接受 URL 参数的端点 → SSRF 测试

---

## 6. 目录扫描补充

爬虫可能遗漏的路径通过字典扫描补充：

### 6.1 常见敏感路径

```
/.git/config
/.svn/entries
/.env
/.env.production
/.env.local
/backup/
/backup.sql
/database.sql
/wp-config.php.bak
/config.yml
/config.json
/.htaccess
/server-status
/server-info
/phpinfo.php
/info.php
/test.php
/debug/
/console/
/actuator/
/actuator/env
/actuator/health
/metrics
/trace
/heapdump
```

### 6.2 管理后台路径

```
/admin/
/administrator/
/manage/
/management/
/backend/
/dashboard/
/portal/
/cp/
/controlpanel/
/webmaster/
```

---

## 7. 目标发现检查清单

- [ ] 已根据用户目标选择精确 HTTP、服务端覆盖、渲染态或 JS 资产路径，没有因输入 URL 就自动爬取
- [ ] 快速测试未知网站时，直接请求和有界爬虫未分别重复调用
- [ ] 疑似 SPA 时已停止机械加深普通爬虫，并给出浏览器或 JS 资产行动建议
- [ ] 仅在任务需要多页面覆盖时完成未认证爬取
- [ ] 仅在范围需要且有凭据/授权时完成认证后爬取
- [ ] URL 列表提取并去重
- [ ] 参数列表提取并分类
- [ ] 表单发现并记录
- [ ] API 端点发现并记录
- [ ] JavaScript 文件分析完成
- [ ] 敏感信息收集完成
- [ ] robots.txt / sitemap.xml 检查完成
- [ ] 目录扫描补充完成
- [ ] 站点地图构建完成
- [ ] 攻击面评估完成
- [ ] 测试目标分组完成，准备交接给漏洞测试
