# httptpl 表达式引擎分析

## 概述

`httptpl` 包中的 `extractor` 和 `matcher` 都支持多种表达式类型，其中 `nuclei-dsl` 类型使用同一个引擎（`NucleiDSL`），但两者还各自支持其他不同的表达式类型。

## 表达式引擎类型

### Extractor 支持的表达式类型

Extractor 支持以下表达式类型（定义在 `yaktpl_extractor.go`）：

1. **`regex`** - 正则表达式提取
   - 使用 Go 标准库 `regexp`
   - 支持命名捕获组和索引捕获组

2. **`kv` / `key-value` / `kval`** - 键值对提取
   - 从 HTTP 响应中提取键值对
   - 支持从 JSON、URL 参数、HTTP Header 等提取

3. **`json` / `jq`** - JSON 查询
   - 使用 `gojq` 库（jq 查询语言）
   - 支持复杂的 JSON 路径查询

4. **`xpath`** - XPath 查询
   - 支持 XML 和 HTML 文档
   - 使用 `xmlquery` 和 `htmlquery` 库

5. **`nuclei-dsl` / `nuclei` / `dsl`** - Nuclei DSL 表达式
   - 使用 `NucleiDSL` 引擎（基于 Yaklang 沙箱）
   - **这是与 Matcher 共享的引擎**

### Matcher 支持的表达式类型

Matcher 支持以下表达式类型（定义在 `yaktpl_matcher.go`）：

1. **`status_code` / `status`** - HTTP 状态码匹配
   - 直接匹配状态码

2. **`content_length` / `size`** - 内容长度匹配
   - 匹配响应体长度

3. **`binary`** - 二进制匹配
   - 自动转换为 hex 编码进行匹配

4. **`word` / `contains`** - 字符串包含匹配
   - 支持在表达式中使用 `{{}}` 进行 DSL 变量替换

5. **`regexp` / `re` / `regex`** - 正则表达式匹配
   - 使用 Yaklang 的正则表达式管理器

6. **`suffix`** - 后缀匹配
   - 使用 `strings.HasSuffix`

7. **`glob`** - 通配符匹配
   - 使用 `glob` 库进行模式匹配

8. **`mime`** - MIME 类型匹配
   - 使用 MIME glob 规则检查

9. **`expr` / `dsl` / `cel`** - 表达式匹配
   - 支持 `nuclei-dsl` 表达式类型
   - **这是与 Extractor 共享的引擎**
   - 不支持 `xray-cel`（已明确标记不支持）

## 共享引擎：NucleiDSL

### 引擎实现

`NucleiDSL` 引擎定义在 `nuclei_dsl.go` 中，基于 Yaklang 沙箱实现：

```go
type NucleiDSL struct {
    Functions         map[string]interface{}
    ExternalVarGetter func(string) (any, bool)
}
```

### Extractor 中的使用

在 `YakExtractor.Execute()` 方法中（`yaktpl_extractor.go:210-240`）：

```go
case "nuclei-dsl", "nuclei", "dsl":
    box := NewNucleiDSLYakSandbox()
    header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
    previousMap := make(map[string]any)
    // ... 加载 previous extractor 结果 ...
    previousMap["body"] = body
    previousMap["body_1"] = body
    previousMap["header"] = header
    previousMap["header_1"] = header
    previousMap["raw"] = string(rsp)
    previousMap["raw_1"] = string(rsp)
    previousMap["response"] = string(rsp)
    previousMap["response_1"] = string(rsp)
    // 执行 DSL 表达式
    data, err := box.Execute(group, previousMap)
```

### Matcher 中的使用

在 `YakMatcher.executeRaw()` 方法中（`yaktpl_matcher.go:387-405`）：

```go
case MATCHER_TYPE_EXPR, "dsl", "cel":
    switch y.ExprType {
    case EXPR_TYPE_NUCLEI_DSL, "nuclei":
        dslEngine := NewNucleiDSLYakSandbox()
        matcherFunc = func(fullResponse string, sub string) bool {
            loadVars := LoadVarFromRawResponse(packet, duration, sufs...)
            // 合并外部变量
            for k, v := range vars {
                loadVars[k] = v
            }
            // 执行 DSL 表达式并返回布尔值
            result, err := dslEngine.ExecuteAsBool(sub, loadVars)
            return result
        }
```

## 内置变量

### Matcher 中的内置变量

通过 `LoadVarFromRawResponse()` 函数加载（`nuclei_dsl.go:838-881`）：

**基础变量：**
- `status_code` - HTTP 状态码（int）
- `content_length` - 响应体长度（int）
- `body` - 响应体内容（string）
- `raw` - 完整响应包（[]byte）
- `all_headers` - 完整响应头（string）
- `duration` - 请求耗时（float64）

**HTTP Header 变量：**
- 所有 HTTP Header 都会被转换为小写，`-` 替换为 `_`，作为变量名
- 例如：`Content-Type` → `content_type`，`X-Forwarded-For` → `x_forwarded_for`

**带后缀的变量（多请求场景）：**
- 当传入 `sufs` 参数时（如 `"_1"`, `"_2"`），会生成带后缀的变量
- 例如：`body_1`, `body_2`, `status_code_1`, `status_code_2` 等
- 如果后缀是 `"_1"`，还会同时设置不带后缀的变量（覆盖原值）

### Extractor 中的内置变量

在 Extractor 的 `nuclei-dsl` 类型中（`yaktpl_extractor.go:214-247`）：

**所有 Matcher 中的内置变量（通过 LoadVarFromRawResponseWithRequest 加载）：**
- `status_code` - HTTP 状态码
- `content_length` - 响应体长度
- `body` - 响应体
- `raw` - 完整响应
- `all_headers` - 完整响应头
- `duration` - 请求耗时
- 所有 HTTP Header（转换为小写+下划线格式）
- `request_raw` - 完整请求包（如果提供）
- `request_headers` - 请求头（如果提供）
- `request_body` - 请求体（如果提供）
- `request_url` - 请求 URL（如果提供）

**向后兼容的变量：**
- `body_1` - 响应体（[]byte）
- `header_1` - 响应头（string）
- `raw_1` - 完整响应（string）
- `response` / `response_1` - 完整响应（string）

**Previous Extractor 结果：**
- 所有之前执行的 extractor 结果都会被加载到 `previousMap` 中
- 数组/切片类型会被转换为逗号分隔的字符串
- 可以通过变量名直接访问之前的提取结果

### 运行时变量

在执行过程中，还会添加以下运行时变量（`yaktpl_exec.go:460-486`）：

- 每个请求的响应变量会通过 `LoadVarFromRawResponse()` 加载，并添加后缀 `_1`, `_2` 等
- 所有模板变量（`y.Variables.ToMap()`）会被合并到运行时变量中
- Extractor 提取的结果会更新到模板变量中，后续的 Matcher 和 Extractor 都可以使用

### 特殊变量

**Request 相关（新增）：**
- `request_raw` - 完整请求包（string）
- `request_headers` - 请求头（string）
- `request_body` - 请求体（string）
- `request_url` - 请求 URL（string）

**OOB（Out-of-Band）相关：**
- `interactsh_protocol` - OOB 协议类型（如 "dns"）
- `interactsh_request` - OOB 请求内容
- `reverse_dnslog_token` - DNS log token

**URL 相关（从请求 URL 提取）：**
- `url` - 完整 URL
- `__host__` - 主机名
- `__port__` - 端口号
- `__hostname__` - 主机名:端口
- `__root_url__` - 根 URL
- `__base_url__` - 基础 URL
- `__path__` - 路径
- `__path_trim_end_slash__` - 去除尾部斜杠的路径
- `__file__` - 文件名
- `__schema__` - 协议（http/https）

## NucleiDSL 内置函数

`NucleiDSL` 提供了大量内置函数（定义在 `nuclei_dsl.go:48-800`），主要包括：

### 字符串处理
- `to_upper` / `toupper` - 转大写
- `to_lower` / `tolower` - 转小写
- `trim` / `trim_left` / `trim_right` / `trim_space` - 去除空白
- `replace` / `replace_regex` - 字符串替换
- `reverse` - 反转字符串
- `substr` - 子字符串提取
- `split` / `join` - 分割和连接
- `contains` / `contains_any` - 包含检查
- `starts_with` / `ends_with` - 前缀/后缀检查
- `line_starts_with` / `line_ends_with` - 行级别检查

### 编码解码
- `base64` / `base64_decode` / `base64_py` - Base64 编码
- `hex_encode` / `hex_decode` - 十六进制编码
- `url_encode` / `url_decode` - URL 编码
- `html_escape` / `html_unescape` - HTML 实体编码

### 压缩解压
- `gzip` / `gzip_decode` - Gzip 压缩
- `zlib` / `zlib_decode` - Zlib 压缩
- `deflate` / `infalte` - Deflate 压缩

### 哈希和加密
- `md5` / `sha1` / `sha256` / `sha512` / `sm3` / `mmh3` - 哈希函数
- `hmac` - HMAC 签名
- `aes_cbc` / `aes_gcm` - AES 加密
- `generate_jwt` - JWT 生成

### 数据处理
- `len` - 长度
- `index` - 索引访问
- `sort` / `uniq` - 排序和去重
- `repeat` - 重复字符串
- `dump` - 调试输出

### 正则表达式
- `regex` / `regex_all` / `regex_any` - 正则匹配

### 随机生成
- `rand_char` / `rand_base` - 随机字符
- `rand_text_alphanumeric` / `rand_text_alpha` / `rand_text_numeric` - 随机文本
- `rand_int` - 随机整数
- `rand_ip` - 随机 IP

### 时间处理
- `unix_time` - Unix 时间戳
- `to_unix_time` - 转换为 Unix 时间戳
- `date_time` - 日期时间格式化
- `wait_for` - 等待

### 数值转换
- `to_number` - 转换为数字
- `to_string` - 转换为字符串
- `dec_to_hex` / `hex_to_dec` - 进制转换
- `oct_to_dec` / `bin_to_dec` - 进制转换

### 其他
- `compare_versions` - 版本比较
- `equals_any` - 相等性检查
- `public_ip` - 获取公网 IP
- `generate_java_gadget` - 生成 Java 反序列化 payload
- `unpack` - 二进制解包
- `print_debug` - 调试打印

## 总结

1. **共享引擎**：Extractor 和 Matcher 都支持 `nuclei-dsl` 类型，使用同一个 `NucleiDSL` 引擎
2. **不同用途**：
   - Extractor 的 DSL 表达式用于**提取数据**，返回任意类型的结果
   - Matcher 的 DSL 表达式用于**匹配判断**，返回布尔值
3. **内置变量**：
   - Matcher 中可以使用 `LoadVarFromRawResponseWithRequest()` 加载的所有变量
   - Extractor 中也使用 `LoadVarFromRawResponseWithRequest()` 加载所有变量，包括 request 相关变量
   - 两者都可以访问模板变量和运行时变量
   - **新增 request 变量**：`request_raw`, `request_headers`, `request_body`, `request_url`
4. **内置函数**：两者都可以使用 `NucleiDSL` 提供的所有内置函数

## 更新日志

### 新增功能（本次更新）

1. **Request 变量支持**：
   - 在 `LoadVarFromRawResponseWithRequest()` 函数中添加了 request 相关变量的支持
   - Matcher 和 Extractor 都可以访问 `request_raw`, `request_headers`, `request_body`, `request_url` 变量
   - 通过 `RespForMatch` 结构的 `RequestPacket` 字段传递请求包

2. **向后兼容**：
   - 保留了原有的 `LoadVarFromRawResponse()` 函数，内部调用新函数
   - Extractor 和 Matcher 的旧代码无需修改即可工作
   - 新增了 `ExecuteWithRequest()` 和 `executeRawWithRequest()` 方法

3. **测试覆盖**：
   - 新增 `nuclei_dsl_builtin_vars_test.go` 测试文件
   - 覆盖所有内置变量的测试
   - 覆盖 request 变量的各种使用场景
   - 确保向后兼容性

