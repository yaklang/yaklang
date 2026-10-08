---
name: fuzztag
metadata:
  display_name_zh-CN: FuzzTag 文本生成
  auto_load: "true"
description: 使用 Yaklang/Yakit FuzzTag 模板生成文本、HTTP 数据包、字典及编码解码变体；用于编写、渲染或调试模板。
---

# FuzzTag 文本生成

FuzzTag 是多结果模板引擎：固定文本保留，标签生成或加工字符串。语法 `{{标签(参数)}}`，无参可写 `{{uuid}}`；参数是文本，各标签自行解析分隔符。

`{{list(A|B)}}-{{int(1-2)}}` 默认取笛卡尔积，产生四项；`{{int(1-3|3)}}` 得 001–003。同层相同 `::row` 标签同步配对：`{{list::row(A|B)}}={{int::row(1-2)}}` 得 A=1、B=2，列表保持等长。raw 写法 `{{=原文=}}` 保留字面内容。

## 执行与交付

加载 `exec_fuzztag` 的工具说明后传 `template`；默认 stdout，`output-file` 保存文件（建议绝对路径），`stdout:true` 同时输出，`force:true` 覆盖。`format:json` 保留每项的多行边界；默认 lines 每项后加换行。外部值传 `variables:{"name":["小王","小李"]}`，模板仍用 `{{params(name)}}`。

编码解码可直接作为 template，嵌套按从内到外执行；每层保留完整 `{{…}}`，变量也写成标签，如 `{{hexdec({{base64dec({{params(encoded)}})}})}}`。

- `{{base64(hello)}}` → aGVsbG8=；`{{base64dec(aGVsbG8=)}}` → hello。
- `{{hex(abc)}}` → 616263；`{{hexdec(616263)}}` → abc。
- `{{base64({{hex(abc)}})}}` → NjE2MjYz；逆序解码 `{{hexdec({{base64dec(NjE2MjYz)}})}}` → abc。
- `{{base64({{list(hello|world)}})}}` 每项分别编码，得两项。
- `{{urlescape({{base64({{={"a":1}=}})}})}}` 将 JSON 编码成 Base64，再转义 URL 特殊字符；urlenc 则编码全部字符。

构建 HTTP 数据包尤其适合这种模板，raw 包住 JSON，外层加工字段：

```http
GET /query?id={{base64({{={"a":1}=}})}} HTTP/1.1
Host: example.com

```

渲染后 id 为 eyJhIjoxfQ==。查询值含 +、/、= 时可再套 urlescape；实际报文用 CRLF 行尾（可插入 `{{crlf}}`），空行结束头部。exec_fuzztag 生成报文文本；HTTP 工具也能直接渲染并发送。

保留重复项；limit 默认 1000，范围 1–100000。检查 success、count、truncated、output_file、bytes 和 preview，截断时 count 只是已输出数。`fuzz.Strings` 会去重。未知标签可能变空串，须检查样本；文件词表需 enable-file-tags，插件/热加载需对应宿主。

## 直接辅助 HTTP 测试

少量编码或随机字段直接用 `do_http_request`，设置 `fuzztag:true`，在 `packet` 或 URL 模式的 `url`、`headers`、`body`、`query`/`form` 值中写标签。它可直接展开并发送多份请求：单包显示完整报文与执行结果，多包默认显示逐包摘要和统计，RESULT.items 按模板顺序保存执行状态与报文预览。普通请求默认原样发送，模板里的局部字面文本用 raw。

同一请求模板的范围、字典与编码组合可直接用 `do_http_request`，例如 `url:"https://target.example/users/{{int(1-10)}}"`、`query:{"token":"{{base64({{hex(abc)}})}}"}`、`fuzztag:true`、`max-requests:10`。精确 packet 的查询编码可写 `{{urlescape({{base64({{params(payload)}})}})}}`；URL 模式结构化 query/form 不再套 URL 编码。`concurrent` 默认 5，设为 1 顺序发送；`max-requests` 默认 100、最多 500，超限整批不发送。多包不自动做 JSON→form 重试，便于比较原始测试；`verbose:true` 展示报文，`save-packet:true` 保存完整包。

多个路径通过 `url:"https://target.example{{params(path)}}"` 与 `variables:{"path":["/health","/api/users","/admin"]}` 发送；完整目标 URL 数组用 `url:"{{params(target)}}"`。路径 × 载荷是笛卡尔积，例如 3 个路径配 `form:{"input":"{{params(payload)}}"}`、`variables.payload:["normal","a & b"]` 得 6 包。需要配对时给同层标签相同 `::row`：URL 的 `{{int::row(1-2)}}` 与 query 的 `{{list::row(reader|admin)}}` 只发 2 包。精确 packet 中直接使用 params/int/list 和嵌套编码，所有字段一起展开，无需特殊路径占位符或中间文件。

多包输出含逐包 request/Response 预览、匹配上下文、响应文件、状态分布、汇总表与 output_file；完整响应自动保存。`include-code`、`exclude-code`、`exclude-size` 只影响展示，filtered_count 与 RESULT.items 保留过滤信息；`max-body-size` 控制响应预览。`delay-seconds` 设置请求间隔并强制顺序发送。重复项保留；未知标签、缺失变量、未闭合模板与超限在发包前报错，整批不发送。HTTP 入口不启用文件/插件标签；`fuzztag:false` 全部原样发送。

以实际 request packet 和结构化 request/response/status/transport_error 为准：先保留正常基线，一次改变一个变量，比较状态、内容、响应头和耗时；一次未命中不能代表漏洞不存在。

## 使用已有 Payload 字典

先用 `query_payloads` 查看运行时字典的组名、样本和 usage；需要维护时用 `manage_payloads` 的 add/change/delete。将返回的 `{{payload(组名)}}` 放入 do_http_request 的 query/form/body/packet，并设置 fuzztag:true 和 max-requests。原生标签是单数 payload；`payload:full` 保留每条完整多行载荷。文件型字典只读。自动加载的 `engine-data` Skill 提供字典管理、历史流量和风险追溯的连续任务示例。

## 查看全集

需要更多标签、别名与运行条件，用 `loading_skills` 加载 `skill_name:"fuzztag-reference"`；选好标签后用 exec_fuzztag 验证。
