---
name: code-review
description: >
  基于 grep 文本搜索和文件读写的代码安全审计技能。通过正则表达式模式匹配在项目源码中
  定位危险函数调用、敏感数据流和已知漏洞模式，覆盖 30+ CWE 漏洞类型，
  支持 Java、Golang、PHP、Python、C/C++、JavaScript 等多种语言的安全审计。
---

# 代码审计技能 (Code Review)

基于 grep 文本搜索工具对项目源代码进行系统性安全审计。
通过精心构造的正则表达式模式，在源码中定位危险函数调用、可疑数据流和已知漏洞模式，
并将审计结果写入文件形成完整的安全报告。

---

## 1. 审计流程

### 1.1 项目结构探查

首先使用文件列表工具了解项目的整体结构：
- 确定项目语言和框架
- 识别入口文件（如 `main.go`、`Application.java`、`index.php`）
- 定位路由定义、控制器、数据访问层等关键目录

### 1.2 分阶段 grep 扫描

按漏洞类型分阶段执行 grep 扫描，每次聚焦一个类别：

**阶段一：注入类漏洞扫描**
- SQL 注入 Sink 点
- 命令注入 Sink 点
- LDAP/XPath 注入

**阶段二：跨站与请求伪造**
- XSS 输出点
- SSRF URL 构造
- CSRF 防护缺失

**阶段三：文件与序列化**
- 路径穿越
- 文件上传
- 反序列化

**阶段四：配置与加密**
- 硬编码凭据
- 弱加密算法
- 不安全的 TLS 配置

### 1.3 上下文验证

对 grep 发现的每个可疑点，读取该文件的上下文代码进行人工判断：
- 是否存在输入过滤/转义
- 是否使用参数化查询
- 数据是否来自可信来源

### 1.4 结果汇总与报告

将所有确认的安全风险写入审计报告文件，按严重程度排序。

---

## 2. 各语言危险函数与 grep 模式

### 2.1 SQL 注入 (CWE-89)

| 语言 | 危险模式 (grep regexp) | 说明 |
|------|----------------------|------|
| PHP | `mysql_query\|mysqli_query\|pg_query\|->query\(.*\$\|->exec\(.*\$\|->prepare\(.*\$` | 直接拼接 SQL |
| Java | `createQuery\|createNativeQuery\|StringBuilder.*append.*sql\|Statement.*execute\|\.Raw\(` | JDBC/Hibernate/MyBatis |
| Golang | `\.Where\(.*\+\|\.Raw\(.*\+\|Exec\(.*fmt\.Sprintf\|Query\(.*\+` | GORM/database-sql 拼接 |
| Python | `cursor\.execute\(.*%\|cursor\.execute\(.*format\|cursor\.execute\(.*\+\|\.raw\(` | Django/SQLAlchemy 拼接 |

审计要点：
- 检查 SQL 语句是否使用参数化查询（`?` 占位符或命名参数）
- `StringBuilder.append` 拼接 SQL 是常见的 Java 注入模式
- GORM 的 `Where`/`Raw` 使用字符串拼接时存在风险
- MyBatis 的 `${}` 占位符不做参数化，需使用 `#{}`
- PHP 的 `mysql_*` 系列函数已废弃，本身就是风险

### 2.2 命令注入 (CWE-77 / CWE-78)

| 语言 | 危险模式 (grep regexp) | 说明 |
|------|----------------------|------|
| PHP | `exec\(\|system\(\|passthru\(\|shell_exec\(\|popen\(\|proc_open\(` | 命令执行函数 |
| Java | `Runtime\.exec\|ProcessBuilder\|\.exec\(` | Runtime 命令执行 |
| Golang | `exec\.Command\|exec\.CommandContext\|os\.StartProcess` | os/exec 包 |
| Python | `os\.system\|os\.popen\|subprocess\.call\|subprocess\.Popen\|subprocess\.run` | 系统命令执行 |
| C | `system\(\|popen\(\|execvp\(\|execl\(\|execv\(` | C 标准库命令执行 |

审计要点：
- 用户输入是否经过过滤后传入命令执行函数
- 是否使用了参数数组形式（安全）而非字符串拼接（危险）
- 检查是否存在命令拼接：`cmd + userInput`

### 2.3 跨站脚本 XSS (CWE-79)

| 语言 | 危险模式 (grep regexp) | 说明 |
|------|----------------------|------|
| PHP | `echo\s.*\$_\|print\s.*\$_\|<\?=.*\$_` | 直接输出用户输入 |
| Golang | `template\.HTML\|\.WriteString\(.*\+\|c\.String\(` | 绕过模板转义 |
| Java | `\.getWriter\(\)\.write\|response\.getOutputStream\|out\.println\(` | Servlet 直接输出 |
| JavaScript | `\.innerHTML\s*=\|document\.write\(\|\.outerHTML\s*=` | DOM XSS |

审计要点：
- `template.HTML` 类型转换会绕过 Go 模板的自动转义
- PHP 中 `echo $_GET['xxx']` 是最经典的反射型 XSS
- 检查输出前是否调用了 `htmlspecialchars` / `html.EscapeString` 等转义函数

### 2.4 服务端请求伪造 SSRF (CWE-918)

| 语言 | 危险模式 (grep regexp) | 说明 |
|------|----------------------|------|
| PHP | `file_get_contents\(\$\|curl_setopt.*CURLOPT_URL.*\$\|fopen\(\$` | URL 参数可控 |
| Golang | `http\.Get\(.*\+\|http\.Post\(.*\+\|http\.NewRequest\(.*\+` | HTTP 客户端 |
| Java | `new\s+URL\(.*\+\|HttpClient.*execute\|RestTemplate` | Java HTTP 请求 |
| Python | `requests\.get\(.*\+\|urllib\.request\.urlopen\|urlopen\(` | Python HTTP 请求 |

### 2.5 路径穿越与文件操作 (CWE-22 / CWE-73 / CWE-434)

| 语言 | 危险模式 (grep regexp) | 说明 |
|------|----------------------|------|
| PHP | `file_get_contents\(\$\|fopen\(\$\|include\(\$\|require\(\$\|move_uploaded_file` | 文件包含/读写 |
| Golang | `os\.Open\(.*\+\|os\.ReadFile\(.*\+\|ioutil\.ReadFile\(.*\+` | 文件路径拼接 |
| Java | `new\s+File\(.*\+\|new\s+FileInputStream\|Paths\.get\(.*\+` | 文件路径可控 |

审计要点：
- 检查文件路径是否经过 `filepath.Clean` / `realpath` 等规范化处理
- 是否校验路径不包含 `..` 和绝对路径前缀
- 上传接口是否校验文件类型和大小

### 2.6 反序列化漏洞 (CWE-502)

| 语言 | 危险模式 (grep regexp) | 说明 |
|------|----------------------|------|
| PHP | `unserialize\(` | PHP 对象注入 |
| Java | `ObjectInputStream\|readObject\(\|Fastjson\|JSON\.parse\|fromXML\|XStream` | Java 反序列化 |
| Python | `pickle\.loads\|yaml\.load\(\|yaml\.unsafe_load` | Python 反序列化 |

### 2.7 XML 外部实体注入 XXE (CWE-611)

| 语言 | 危险模式 (grep regexp) | 说明 |
|------|----------------------|------|
| PHP | `simplexml_load\|DOMDocument\|xml_parse` | PHP XML 解析 |
| Java | `DocumentBuilderFactory\|SAXParserFactory\|XMLInputFactory` | Java XML 解析 |
| Golang | `xml\.NewDecoder\|xml\.Unmarshal` | Go XML 解析 |

审计要点：
- 检查是否禁用了外部实体解析（`FEATURE_EXTERNAL_ENTITIES` = false）
- 检查 `libxml_disable_entity_loader(true)` 是否被调用

### 2.8 服务端模板注入 SSTI (CWE-1336)

| 语言 | 危险模式 (grep regexp) | 说明 |
|------|----------------------|------|
| Golang | `template\.New\(.*Parse\(.*\+\|template\.Must\(.*Parse\(` | text/template 拼接 |
| Java | `Thymeleaf.*process\|templateEngine.*process\|velocity.*evaluate` | 模板引擎 |
| Python | `render_template_string\|jinja2\.Template\(.*\+\|Template\(.*\+` | Jinja2/Django |

### 2.9 开放重定向 (CWE-601)

搜索 HTTP 重定向目标来自用户输入：
- `redirect\(.*\$_\|header\(.*Location.*\$_` (PHP)
- `c\.Redirect\(.*\+\|http\.Redirect\(` (Golang)
- `sendRedirect\(.*\+\|response\.sendRedirect` (Java)

---

## 3. 配置与加密安全

### 3.1 硬编码凭据 (CWE-259 / CWE-798)

```
password\s*=\s*["'][^"']+["']
secret\s*=\s*["'][^"']+["']
api_key\s*=\s*["'][^"']+["']
token\s*=\s*["'][^"']+["']
```

审计要点：
- 区分测试用的占位值和真实凭据
- 检查是否从环境变量或配置文件读取
- 数据库连接字符串中的硬编码密码

### 3.2 弱加密算法 (CWE-327)

```
MD5\|md5\|SHA1\|sha1\|DES\|des\|RC4\|rc4
NewCBCEncrypter\|NewCBCDecrypter
ECB
```

### 3.3 不安全的 TLS 配置 (CWE-295)

```
InsecureSkipVerify\s*:\s*true
verify\s*=\s*False
VERIFY_NONE
```

### 3.4 Cookie 安全 (CWE-614 / CWE-1004)

检查 Cookie 设置中是否缺少 `Secure`、`HttpOnly`、`SameSite` 标志。

### 3.5 CORS 配置 (CWE-942)

```
Access-Control-Allow-Origin.*\*
AllowAllOrigins\s*:\s*true
```

---

## 4. 内存与资源安全 (C/C++)

### 4.1 缓冲区溢出 (CWE-119 / CWE-120)

```
strcpy\|strcat\|sprintf\|gets\|scanf
```

检查是否使用了 `strncpy`、`snprintf` 等安全替代函数。

### 4.2 格式化字符串漏洞 (CWE-134)

```
printf\s*\(\s*[a-zA-Z_]\|fprintf\s*\(\s*[^,]+,\s*[a-zA-Z_]
```

检查 `printf` 系列函数的格式化字符串是否来自用户输入。

### 4.3 内存泄漏 (CWE-401)

检查 `malloc`/`calloc` 分配的内存是否在所有路径上被正确释放。

---

## 5. 组件安全

关注已知存在漏洞的组件：

| 组件 | 风险类型 | grep 模式 |
|------|---------|-----------|
| Fastjson | 反序列化 RCE | `com\.alibaba\.fastjson\|JSON\.parseObject\|JSON\.parse` |
| Log4j | JNDI 注入 | `org\.apache\.log4j\|log4j` |
| Shiro | 认证绕过 | `org\.apache\.shiro\|ShiroFilter` |
| Spring Actuator | 信息泄露 | `actuator\|management\.endpoints` |
| jQuery | XSS | `jquery.*\.min\.js\|jQuery\s` |

---

## 6. 审计工具使用指南

### 6.1 grep 工具使用

使用 `grep` 工具进行模式搜索时的最佳实践：

- **pattern-mode 设为 regexp**：使用正则表达式匹配
- **limit 合理设置**：建议 50-200，避免结果过多导致信息过载
- **context-buffer 设为合理值**：建议 50-200 字节，提供足够的上下文用于判断
- **分类搜索**：每次只搜一类漏洞模式，不要把所有模式合并在一起

示例 grep 参数：
```
{
  "path": "/path/to/project",
  "pattern": "exec\\(|system\\(|passthru\\(|shell_exec\\(",
  "pattern-mode": "regexp",
  "limit": 100,
  "context-buffer": 100
}
```

### 6.2 文件读取验证

对 grep 命中的结果，使用文件读取工具查看完整上下文：
- 读取命中行的前后 20-50 行
- 检查是否有输入过滤、参数化查询等防护措施
- 追踪变量来源，判断是否用户可控

### 6.3 结果保存

将审计结果写入文件，格式建议：

```
### [严重程度] CWE-编号 漏洞类型

文件: path/to/file.php:42
匹配: exec($userInput)
上下文: 用户输入 $_GET['cmd'] 未经过滤直接传入 exec()
风险: 攻击者可执行任意系统命令
建议: 使用白名单过滤或 escapeshellarg() 转义参数
```

---

## 7. 审计输出格式

按严重程度对发现进行分级：**严重 > 高危 > 中危 > 低危 > 信息**

每条发现包含：
- 漏洞类型与 CWE 编号
- 受影响的文件路径和行号（或字节偏移）
- 匹配的代码片段
- 风险说明
- 修复建议

最终报告应包含：
- 审计范围（项目路径、语言、文件数量）
- 发现统计（各严重程度的数量）
- 详细发现列表
- 总体安全评估
