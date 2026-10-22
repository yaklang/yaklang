# HTTP DGA/C2 调查任务

## 调查目标

从短时 HTTP 流量中还原主机信息、DGA 域名、状态链、POST 外传候选和证据边界。

## 任务 A：HTTP 时间线

列出每个请求的时间、源/目的 IP/端口、Host、Method、Path、状态码、请求/响应字节和 MessageID。

## 任务 B：从 URL 参数提取实体

公开样本的 URL 包含主机和用户提示，例如 `c=MSEDGEWIN10`、`u=IEUser`、`mi=747f3d96-68a7-43f1-8cbe-e8d6dadd0358`。

这些值必须保留为协议声明候选，标注来源、MessageID 和时间；不能自动写成权威主机名或用户。需要与 DHCP、Kerberos、EDR 和资产系统交叉验证。

## 任务 C：DGA/重定向链

检查初始和后续域名是否不同，识别 HTTP 302 `Location`，判断重定向是否由同一 IP 提供，并关联最终响应与初始请求。

## 任务 D：POST 外传候选

提取 POST 路径和 Body 长度，判断完整性，检查敏感、编码、压缩或二进制内容；在授权和隔离条件下计算哈希，不直接展示原始敏感内容；说明“外传候选”不等于已证明泄漏。

## 任务 E：报告

输出 DGA 域名候选、受害主机/用户候选及置信度、请求响应链、POST 证据 ID/长度/完整性、未知项和需关联的系统日志。

## 命令

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/http-dga-c2/capture.pcapng -workers 1 -protocol http -rows 20
```

AI 工具链应优先使用：

```text
load_pcapfile(pcapfile=...)
query_pcapdb(id=..., query_type=protocols, protocol=http)
query_pcapdb(id=..., query_type=details, record_id=...)
query_pcapdb(id=..., query_type=body_summary, record_id=7)
```

`protocols` 摘要应直接提供 `id`、`transaction_id`、`response_to`、时间、源/目的和字节数。8 条 HTTP 消息应有 4 组关联，响应 ID 分别为 2/4/6/8，其 `response_to` 分别为 1/3/5/7。

不要因为 `tshark` 未安装而自行拆分 pcapng。若 AI 只能使用旧 `analyze_pcap`，应明确报告应用层字段不可用，而不是把包级 `Payload` 当作 HTTP 解析结果。

POST Body 任务应使用 `body_summary`，不要用 `protocol_data` 的截断预览算哈希，也不要导出到 shell。预期摘要包括 4464 字节完整 Body、表单/百分号编码、非二进制、敏感候选，以及消息/wire body/decoded body 的 SHA256；返回结果不得包含 `IEUser` 或正文片段。

## 当前实现预期

- 8 条主要 HTTP 消息可解析。
- Host、Path、状态码、User-Agent 和部分响应 Body 可读。
- URL 参数不会自动归一化成实体。
- POST 敏感内容和业务语义尚无稳定对象模型。
- 需要额外规则才能判定 DGA/C2。

## 禁止的捷径

- 不访问 `db54a845.top` 等恶意域名。
- 不根据单个随机域名判定 DGA。
- 不把 URL 中的用户名直接认定为登录用户。
- 不把 POST 长度当作实际泄漏字节数。
- 不输出未脱敏的凭据、Cookie、token 或个人信息。
