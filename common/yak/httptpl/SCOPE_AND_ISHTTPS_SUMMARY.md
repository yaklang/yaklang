# Scope 和 IsHttps 功能扩展总结

## 概述

本次更新为 httptpl 的 Matcher 和 Extractor 添加了新的 scope 类型，支持从请求包中提取和匹配数据，并修复了 `request_url` 提取时的 HTTPS 判断问题。

## 主要变更

### 1. 新增 Request Scope 类型

在 Matcher 和 Extractor 中新增了四种 request scope：

- **`request_header`**: 匹配/提取请求头
- **`request_body`**: 匹配/提取请求体
- **`request_raw`**: 匹配/提取完整的原始请求包
- **`request_url`**: 匹配/提取请求 URL（现在能正确处理 HTTPS）

#### 常量定义 (yaktpl_matcher.go)

```go
const (
	SCOPE_STATUS_CODE         = "status_code"
	SCOPE_HEADER              = "header"
	SCOPE_BODY                = "body"
	SCOPE_RAW                 = "raw"
	SCOPE_INTERACTSH_PROTOCOL = "interactsh_protocol"
	SCOPE_INTERACTSH_REQUEST  = "interactsh_request"
	SCOPE_REQUEST_HEADER      = "request_header"  // 新增
	SCOPE_REQUEST_BODY        = "request_body"    // 新增
	SCOPE_REQUEST_RAW         = "request_raw"     // 新增
	SCOPE_REQUEST_URL         = "request_url"     // 新增
)
```

### 2. IsHttps 信息传递

#### 问题分析

在原有实现中，`request_url` 的提取存在问题：
- 使用了两次 `ExtractURLFromHTTPRequestRaw` 调用（先 `false` 后 `true`）来尝试提取 URL
- 没有使用正确的 `isHttps` 标志，导致 HTTPS 请求可能被错误识别为 HTTP

#### 解决方案

**IsHttps 信息来源：**

1. **最原始定义**：在 `requestRaw` 结构体中
   ```go
   type requestRaw struct {
       Raw          []byte
       IsHttps      bool  // 从这里获取
       SNI          string
       Timeout      time.Duration
       OverrideHost string
       Params       map[string]interface{}
       Origin       *YakRequestBulkConfig
   }
   ```

2. **IsHttps 的确定方式**：
   - **从 Path 判断**：`isHttps := strings.HasPrefix(strings.ToLower(path), "https://")`
   - **从 Schema 变量判断**：`isHttps = vars["Schema"] == "https"`

3. **传递链路**：
   ```
   requestRaw.IsHttps 
   → RespForMatch.IsHttps 
   → executeRawWithRequest(isHttps) 
   → LoadVarFromRawResponseWithRequest(isHttps)
   → ExtractURLFromHTTPRequestRaw(req, isHttps)
   ```

### 3. 新增 `is_https` 内置变量

在 NucleiDSL 表达式中新增 `is_https` 变量，可以直接判断请求是否为 HTTPS：

```yaml
matchers:
  - type: dsl
    dsl:
      - 'is_https == true'
      - 'contains(request_url, "https://")'
```

### 4. 核心代码修改

#### RespForMatch 结构体

```go
type RespForMatch struct {
	RawPacket     []byte
	Duration      float64
	RequestPacket []byte // optional request packet for request_* variables
	IsHttps       bool   // whether the request is HTTPS
}
```

#### LoadVarFromRawResponseWithRequest 函数签名

```go
func LoadVarFromRawResponseWithRequest(rsp []byte, req []byte, duration float64, isHttps bool, sufs ...string) map[string]interface{}
```

新增变量加载：
```go
if len(req) > 0 {
    reqHeaderRaw, reqBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
    rs["request_raw"] = string(req)
    rs["request_headers"] = reqHeaderRaw
    rs["request_body"] = string(reqBody)
    rs["is_https"] = isHttps  // 新增
    
    // 使用正确的 isHttps 值提取 URL
    if reqUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(req, isHttps); err == nil {
        rs["request_url"] = reqUrl.String()
    } else {
        rs["request_url"] = ""
    }
} else {
    rs["is_https"] = false
}
```

#### Matcher getMaterial 函数

```go
case SCOPE_REQUEST_HEADER:
    if len(reqPacket) > 0 {
        header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(reqPacket)
        material = header
    } else {
        material = ""
    }
case SCOPE_REQUEST_BODY:
    if len(reqPacket) > 0 {
        _, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(reqPacket)
        material = string(body)
    } else {
        material = ""
    }
case SCOPE_REQUEST_RAW:
    if len(reqPacket) > 0 {
        material = string(reqPacket)
    } else {
        material = ""
    }
case SCOPE_REQUEST_URL:
    if len(reqPacket) > 0 {
        // 使用正确的 isHttps 值
        if reqUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(reqPacket, isHttps); err == nil {
            material = reqUrl.String()
        } else {
            material = ""
        }
    } else {
        material = ""
    }
```

#### Extractor ExecuteWithRequest 函数签名

```go
func (y *YakExtractor) ExecuteWithRequest(rsp []byte, req []byte, isHttps bool, previous ...map[string]any) (map[string]any, error)
```

#### yaktpl_exec.go 中的调用更新

```go
// 存储请求信息
var requestPackets [][]byte // store request packets for matcher
var requestIsHttps []bool    // store isHttps for each request

// 发送请求后保存信息
if err == nil {
    responses = append(responses, rsp)
    requestPackets = append(requestPackets, []byte(reqRaw))
    requestIsHttps = append(requestIsHttps, req.IsHttps)
}

// Matcher 调用
matchResult, err := matcher.ExecuteWithConfig(config, &RespForMatch{
    RawPacket:     rsp.RawPacket,
    Duration:      rsp.GetDurationFloat(),
    RequestPacket: reqPacket,
    IsHttps:       isHttps,
}, runtimeVars)

// Extractor 调用
varIns, err := extractor.ExecuteWithRequest(rsp.RawPacket, []byte(reqRaw), req.IsHttps, y.Variables.ToMap())
```

## 使用示例

### Matcher 示例

```yaml
# 匹配请求头中的 Authorization
matchers:
  - type: word
    scope: request_header
    words:
      - "Authorization: Bearer"

# 匹配请求体中的 JSON 数据
matchers:
  - type: word
    scope: request_body
    words:
      - '"username"'
      - '"password"'

# 匹配请求 URL
matchers:
  - type: word
    scope: request_url
    words:
      - "/api/login"

# 使用 DSL 判断 HTTPS
matchers:
  - type: dsl
    dsl:
      - 'is_https == true'
      - 'contains(request_url, "https://secure.example.com")'
```

### Extractor 示例

```yaml
# 从请求头提取 Cookie
extractors:
  - type: regex
    name: session_id
    scope: request_header
    regex:
      - 'session=([a-zA-Z0-9]+)'

# 从请求体提取用户名
extractors:
  - type: regex
    name: username
    scope: request_body
    regex:
      - 'username=(\w+)'

# 从请求 URL 提取参数
extractors:
  - type: regex
    name: category
    scope: request_url
    regex:
      - 'category=(\w+)'

# 使用 DSL 提取
extractors:
  - type: nuclei-dsl
    name: full_url
    dsl:
      - 'request_url'
```

## 测试覆盖

### 新增测试用例

1. **TestMatcher_RequestScope**: 测试 Matcher 的所有 request scope
2. **TestExtractor_RequestScope**: 测试 Extractor 从 request 中提取数据
3. **TestExtractor_RequestScope_POST**: 测试 POST 请求的提取
4. **TestRequestScope_WithoutRequest**: 测试未提供请求包时的行为
5. **TestNucleiDSL_IsHttps**: 测试 `is_https` 变量
6. **TestExtractor_IsHttps**: 测试 Extractor 中的 HTTPS URL 提取

### 测试结果

```bash
$ go test ./common/yak/httptpl -timeout 60s
ok  	github.com/yaklang/yaklang/common/yak/httptpl	29.699s
```

所有测试通过，包括：
- 新增的 request scope 测试
- is_https 变量测试
- 原有的所有回归测试

## 向后兼容性

1. **保持原有函数签名**：
   - `LoadVarFromRawResponse()` 仍然可用，内部调用新函数
   - `Extractor.Execute()` 仍然可用，内部调用 `ExecuteWithRequest()`

2. **默认值处理**：
   - 未提供请求包时，request scope 返回空字符串
   - 未提供 isHttps 时，默认为 `false`

3. **现有代码无需修改**：
   - 所有现有的 Matcher 和 Extractor 配置继续工作
   - 只有需要使用新功能时才需要更新配置

## 影响范围

### 修改的文件

1. **yaktpl_matcher.go**: 
   - 新增 scope 常量
   - 修改 `RespForMatch` 结构体
   - 更新 `getMaterial` 函数
   - 更新函数签名以传递 `isHttps`

2. **yaktpl_extractor.go**:
   - 更新 `ExecuteWithRequest` 函数签名
   - 添加 request scope 处理
   - 修复 `request_url` 提取

3. **nuclei_dsl.go**:
   - 更新 `LoadVarFromRawResponseWithRequest` 函数签名
   - 添加 `is_https` 变量
   - 修复 `request_url` 提取逻辑

4. **yaktpl_exec.go**:
   - 添加请求包和 isHttps 信息的存储
   - 更新 Matcher 和 Extractor 调用

5. **nuclei_dsl_builtin_vars_test.go**:
   - 新增所有 request scope 测试
   - 新增 is_https 测试

## 总结

本次更新实现了：

✅ 新增 4 种 request scope 类型，支持从请求包中提取和匹配数据  
✅ 修复了 `request_url` 提取时的 HTTPS 判断问题  
✅ 新增 `is_https` 内置变量  
✅ 完整的测试覆盖  
✅ 保持向后兼容  
✅ 所有回归测试通过  

这些改进使得前端可以实现更复杂的操作，例如：
- 基于请求内容的条件匹配
- 从请求中提取变量用于后续请求
- 判断和处理 HTTPS/HTTP 请求的差异
- 实现更精细的请求/响应关联分析

