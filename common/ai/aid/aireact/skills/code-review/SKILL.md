---
name: code-review
description: >
  基于 Yaklang SyntaxFlow 引擎的代码审计技能。结合 SSA（静态单赋值）中间表示
  和内置的 312 条 SyntaxFlow 规则，对 Java、Golang、PHP、C 项目进行安全审计
  和代码质量分析。覆盖 30+ CWE 漏洞类型，支持污点追踪、数据流分析和组件安全检查。
---

# 代码审计技能 (Code Review)

基于 Yaklang 的 SyntaxFlow 静态分析引擎，对代码进行系统性安全审计。
SyntaxFlow 通过 SSA 中间表示实现跨过程的污点追踪和数据流分析，
内置 312 条规则覆盖 Java / Golang / PHP / C 四种语言。

---

## 1. 审计流程

### 1.1 项目编译为 SSA IR

使用 `ssa.Parse` 或 `yak code-scan` 将源代码编译为 SSA 中间表示：

```
ssa.Parse(code, ssa.withLanguage(ssa.Java))
ssa.ParseProjectFromPath("/path/to/project", ssa.withLanguage(ssa.Golang))
```

命令行方式：

```
yak code-scan --target /path/to/project --language java
yak code-scan --target /path/to/project --language golang
```

### 1.2 执行 SyntaxFlow 规则扫描

通过 `syntaxflow.ExecRule` 执行规则，或使用 `prog.SyntaxFlowWithError` 直接查询：

```
prog = ssa.Parse(code, ssa.withLanguage(ssa.Java))
result, err = prog.SyntaxFlowWithError(ruleContent)
```

批量执行内置规则：

```
for rule := range syntaxflow.QuerySyntaxFlowRules("java-sca") {
    res, err := syntaxflow.ExecRule(rule, prog, syntaxflow.withSave())
}
```

### 1.3 结果分析与报告

使用 `sfreport` 生成审计报告，支持 irify 和 SARIF 格式。
通过 `risk` 库管理和查询发现的安全风险。

---

## 2. 安全漏洞审计（SyntaxFlow 内置规则覆盖）

以下为 SyntaxFlow 已内置规则覆盖的漏洞类型，审计时应重点关注。

### 2.1 注入类漏洞

#### SQL 注入 (CWE-89)

SyntaxFlow 内置规则覆盖多种 ORM 和数据库框架：

| 语言 | 覆盖框架 | 规则示例 |
|------|---------|---------|
| Java | Hibernate, MyBatis, JDBC, JPA | `java-hibernate-create-query.sf` |
| Golang | GORM, sqlx, database/sql | `golang-gorm-sql.sf`, `golang-database-net-sql.sf` |
| PHP | PDO, MySQLi, ThinkPHP | `php-mysql-inject.sf`, `php-thinkphp-sql-injection.sf` |
| C | 原生 SQL 拼接 | `c-sql-injection.sf` |

审计要点：
- 检查 SQL 语句是否使用参数化查询
- `StringBuilder.append` 拼接 SQL 是常见的 Java 注入模式
- GORM 的 `Where`/`Raw` 使用字符串拼接时存在风险
- MyBatis 的 `${}` 占位符不做参数化，需使用 `#{}`

#### 命令注入 (CWE-77 / CWE-78)

| 语言 | 关键 Sink | 规则 |
|------|----------|------|
| Golang | `exec.Command`, `exec.CommandContext` | `golang-command-injection.sf` |
| PHP | `exec`, `system`, `passthru`, `shell_exec` | `php-cmd-injection.sf` |
| C | `system()`, `popen()`, `execvp()` | `c-command-injection.sf` |
| Java | `Runtime.exec`, `ProcessBuilder` | `java-runtime-exec.sf` |

审计要点：
- 用户输入是否经过过滤后传入命令执行函数
- SyntaxFlow 使用 `<include('golang-user-input')>` 追踪用户输入源
- 通过 `until: "* & $source"` 进行反向污点追踪定位数据流

#### LDAP 注入 (CWE-90)

覆盖 `InitialDirContext.search` 等 LDAP 查询接口，追踪用户输入到查询参数的数据流。

#### CRLF 注入 (CWE-93)

覆盖 Golang 的 Beego 框架等 HTTP 头注入场景。

### 2.2 跨站脚本 (XSS) (CWE-79)

| 语言 | 覆盖场景 | 规则 |
|------|---------|------|
| Golang | `template.HTML` 直接渲染、Gin Context 输出 | `golang-reflected-xss-template.sf` |
| PHP | `echo`/`print` 输出未转义内容 | `php-xss.sf` |
| C | CGI 输出 | `c-xss-vulnerability.sf` |

审计要点：
- `template.HTML` 类型转换会绕过 Go 模板的自动转义
- 关注 `c.Writer.WriteString` 和 `c.String` 等 Gin 直接输出

### 2.3 服务端请求伪造 (SSRF) (CWE-918)

覆盖 `http.Get`、`http.Post`、`http.NewRequest` 等 HTTP 客户端调用，
追踪用户输入是否流入 URL 参数。

### 2.4 XML 外部实体注入 (XXE) (CWE-611)

覆盖 Golang 和 PHP 的 XML 解析器，检查是否禁用了外部实体解析。

### 2.5 路径穿越与文件操作 (CWE-22 / CWE-73 / CWE-434)

| 类型 | 覆盖 |
|------|------|
| 路径穿越 | 文件读写操作中的用户输入未经 `filepath.Clean` 等安全处理 |
| 任意文件上传 | 上传接口缺少文件类型校验 |
| 未过滤路径 | 检查 `strings.HasPrefix` / `filepath.Clean` 等防护措施 |

### 2.6 服务端模板注入 (SSTI) (CWE-1336)

覆盖 Golang 的 `text/template`、`sprig` 以及 Java 的 Thymeleaf 模板引擎。

### 2.7 反序列化漏洞 (CWE-502)

| 语言 | 覆盖 |
|------|------|
| PHP | `unserialize()` 对不可信数据的使用 |
| Java | Fastjson、Jackson、XStream、Shiro 等组件 |

### 2.8 开放重定向 (CWE-601)

检查 HTTP 重定向目标是否来自用户可控输入。

### 2.9 CSRF (CWE-352)

覆盖 Golang Gin 框架等场景，检查 CSRF Token 防护。

---

## 3. 配置与加密安全

### 3.1 硬编码凭据 (CWE-259 / CWE-798)

SyntaxFlow 检查以下模式：
- 数据库连接中的硬编码密码（PHP `mysql_connect` 等）
- 代码中的常量密码、API Key
- 通用硬编码凭据检测

### 3.2 弱加密算法 (CWE-327)

覆盖 Golang 的 `crypto/cipher` 包：
- 检测 `NewCBCEncrypter` / `NewCBCDecrypter` 等 CBC 模式使用
- 标记使用 MD5、SHA-1 做完整性校验的场景

### 3.3 证书验证不当 (CWE-295)

检测 `InsecureSkipVerify: true` 等跳过 TLS 证书验证的配置。

### 3.4 明文传输 (CWE-319)

检查是否使用 HTTP 而非 HTTPS 传输敏感数据。

### 3.5 Cookie 安全 (CWE-614 / CWE-1004)

- 敏感 Cookie 缺少 `Secure` 标志
- 敏感 Cookie 缺少 `HttpOnly` 标志

### 3.6 CORS 配置 (CWE-942)

检测 `Access-Control-Allow-Origin: *` 或使用用户输入设置 CORS 头。

---

## 4. 内存与资源安全 (C 语言)

### 4.1 缓冲区溢出 (CWE-119 / CWE-120)

检测 `strcpy`、`strcat`、`sprintf` 等不安全函数，
追踪用户输入是否未经长度校验直接用于缓冲区操作。

### 4.2 格式化字符串漏洞 (CWE-134)

检查 `printf` 系列函数的格式化字符串是否来自用户输入。

### 4.3 内存泄漏 (CWE-401)

检查 `malloc`/`calloc` 分配的内存是否在所有路径上被正确释放。

### 4.4 信息泄露 (CWE-200)

检查错误信息中是否暴露内部路径、堆栈等敏感信息。

---

## 5. 组件安全 (SCA)

SyntaxFlow 内置的 SCA 规则覆盖以下 Java 组件：

| 组件 | 风险类型 | 规则目录 |
|------|---------|---------|
| Fastjson | 反序列化 RCE | `java/sca/` |
| Jackson | 反序列化 | `java/sca/` |
| Shiro | 认证绕过、反序列化 | `java/components/shiro/` |
| Log4j | Log4Shell (JNDI 注入) | `java/components/log4j/` |
| Spring Actuator | 信息泄露、RCE | `java/components/actuator/` |
| Thymeleaf | SSTI | `java/components/mytheleaf/` |
| Quartz | 反序列化 | `java/components/quartz/` |
| JWT | 算法混淆、弱密钥 | `java/components/jwt/` |

---

## 6. 代码质量与最佳实践

### 6.1 异常处理 (CWE-396)

- 检查是否捕获了过于宽泛的异常类型（如 Java 的 `catch(Exception e)`）
- 检查返回值是否被正确处理 (CWE-690)

### 6.2 资源释放 (CWE-772)

- 检查网络连接、文件句柄是否在所有路径上被关闭
- Java 的 `Socket` 未关闭等场景

### 6.3 授权检查 (CWE-863)

- Golang 的文件操作权限检查是否缺失
- 敏感操作前是否有权限校验

### 6.4 J2EE 代码规范

- 检查 EJB、Servlet 中的编码规范问题
- 检查日志输出规范

---

## 7. SyntaxFlow 规则编写参考

### 7.1 规则结构

```
desc(
    title: "规则标题"
    title_zh: "中文标题"
    type: audit
    level: high
    risk: "rce"
    desc: <<<DESC
    漏洞描述
    DESC
)

<include('java-servlet-param')> as $source;
dangerous.Function(* as $sink);

$sink #{
    until: `* & $source`
}-> as $result

alert $result for {
    message: "发现安全风险"
}
```

### 7.2 关键语法

| 语法 | 含义 |
|------|------|
| `#->` | 反向数据流追踪（TopDef，追溯到定义源头） |
| `-->` | 正向深度追踪（DeepNext） |
| `?{opcode: const}` | 过滤 SSA 操作码（常量、调用等） |
| `?{have: 'string'}` | 字符串包含检查 |
| `<include('rule')>` | 引用共享规则库（source/sink 定义） |
| `<slice(index=N)>` | 提取函数第 N 个参数 |
| `$a & $b` | 交集（两个集合都匹配） |
| `$a - $b` | 差集（排除安全情况） |
| `<dataflow(exclude:...)>` | 带排除条件的数据流分析 |
| `<typeName>` / `<fullTypeName>` | 类型名匹配 |

### 7.3 共享规则库 (lib/)

每种语言都提供可复用的 source 和 sink 定义：

- `golang-user-input` -- HTTP 请求参数、Gin/Beego 框架输入
- `golang-os-exec` -- 命令执行 Sink
- `golang-file-read-sink` / `golang-file-write-sink` -- 文件操作 Sink
- `java-servlet-param` / `java-spring-mvc-param` -- Java 用户输入源
- `java-command-exec-sink` / `java-sql-operator` -- Java 危险 Sink
- `c-user-input` -- C 语言用户输入 (`argv`, `getenv`, `fgets` 等)
- `php-user-input` -- PHP 超全局变量 (`$_GET`, `$_POST` 等)

---

## 8. 审计输出格式

按严重程度对发现进行分级：**严重 > 高危 > 中危 > 低危 > 信息**

每条发现包含：
- 漏洞类型与 CWE 编号
- 受影响的文件路径和行号
- 数据流路径（从 Source 到 Sink）
- 修复建议
- 相关 SyntaxFlow 规则名称

```
### [高危] CWE-89 SQL 注入

文件: src/main/java/UserDao.java:42
数据流: HttpServletRequest.getParameter("id") -> StringBuilder.append -> createQuery
规则: java-hibernate-create-query
建议: 使用参数化查询替代字符串拼接，将 createQuery 改为 createNamedQuery 并使用绑定参数。
```
