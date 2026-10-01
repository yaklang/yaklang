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

少量编码或随机字段直接用 `do_http_request`，设置 `fuzztag:true`，在 `packet` 或 URL 模式的 `url`、`headers`、`body`、`query`/`form` 值中写标签。它要求恰好一份渲染结果；多结果改用 `batch_do_http_request`。普通请求默认原样发送，模板里的局部字面文本用 raw。

多路径、多载荷优先一次调用 `batch_do_http_request`，无需先生成字典文件或逐个调用单请求工具。默认启用 FuzzTag：`paths:"/users/{{int(1-3)}}"` 配 `form:{"input":"{{params(payload)}}"}` 与 `variables:{"payload":["normal","a & b"]}` 得 3×2=6 份请求。先小样本核对，再设置合适的 `max-requests`、`concurrent`、响应筛选条件。

URL 模式先一起渲染各字段，再对 query/form 的原始值编码一次；不要对结构化值再套 urlenc。`paths:"/users/{{int::row(1-2)}}"` 与 `query:{"role":"{{list::row(reader|admin)}}"}` 同步成两份请求。精确 packet 模式自行编码，如 `{{urlescape({{base64({{params(payload)}})}})}}`；`{{PATH}}` 是当前 paths 行的字面值，路径与 packet 分阶段展开成笛卡尔积，需要同步的标签写在同一份 packet 中。

变量标量/数组用 params 注入，值里的标签不再次执行；批量旧 `{{name}}` 仍兼容。重复项保留，`repeat(3)` 可测试重复请求的稳定性。批量 `max-requests` 默认100、最多500，限制展开后的总数，超限整批不发送。未知标签、缺失变量、未闭合模板在发包前报错；HTTP 入口不启用文件/插件标签。批量 `disable-fuzztag:true` 全部原样发送，包括 PATH。

以实际 request packet 和结构化 request/response/status/transport_error 为准：先保留正常基线，一次改变一个变量，比较状态、内容、响应头和耗时；一次未命中不能代表漏洞不存在。

## 查看全集

需要更多标签、别名与运行条件，用 `loading_skills` 加载 `skill_name:"fuzztag-reference"`；选好标签后用 exec_fuzztag 验证。
