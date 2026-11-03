# M2 PA-010 协议语义摘要

日期：2026-11-03  
分支：`codex/pa010-semantic-summaries`  
初始基线：PR [#5371](https://github.com/yaklang/yaklang/pull/5371) 源分支
`codex/shark-m2-protocol-correctness` / `4ccbc87927660735149efc779896c4328aad5556`。
提 PR 前已普通合入最新源分支 `7335a24a63a3bcae35391c8590074a3fb29bd8bf`
（merge `4fd97d8939`）。

## 有限合同

本项只改消息行摘要，不新增协议识别，不改变完整字段树、原始字节、事务关联或
`decoded` / `deferred` / `incomplete` / `malformed` / `context-required` 状态。
摘要上限为 240 个 Unicode 字符，控制字符替换为 `?`，所有捕获文本都截断，
不把完整字段树复制到摘要。

### Kerberos

- 从已通过 exact DER 校验的 AS/TGS/AP/KRB-ERROR 消息派生稳定 facts。
- 行摘要表达消息类型、观察到的 realm/principal、请求或已观察的加密类型、
  KRB-ERROR 错误码。
- 嵌套 PA-TGS-REQ 不覆盖外层消息类型；其 Ticket 身份作为候选上下文单独保留。
- Ticket、cipher、checksum 和未知 PA-DATA 保持 opaque；摘要固定写明
  `decrypt=no identity=unverified`，不表示票据有效、认证成功或交换成功。

### LDAP

- SearchRequest 行摘要表达 message ID、base、scope 和请求属性数。
- Bind/搜索结果行摘要表达结果码、请求关联和认证观察，但不导出 simple/SASL
  凭据或搜索结果中的敏感属性值。
- SearchResultDone 保留 entries/references；SASL protected record 保持
  `payload=opaque authentication=unverified`。
- CLDAP/UDP 389 的专用 datagram 摘要已经由增量 PR
  [#5390](https://github.com/yaklang/yaklang/pull/5390) 实现，本分支不重复修改
  `protocol_cldap.go`。

### SMB2

- 摘要明确区分 request/response、compound 数和 MessageId。
- 在可观察时表达文件/管道目标、READ/WRITE 长度或返回 count、NT status。
- 请求使用 `result=request-pending`；响应使用
  `result=response-status-observed`，STATUS_PENDING 单独保留
  `result=pending`。
- SMB3 transform 只显示 `ciphertext=opaque decrypt=no`，不把加密载荷误标为
  request/response 业务消息。
- 摘要中的 status 是线上响应事实，不证明业务操作、权限或数据内容成功。

## 实现位置

- `common/bin-parser/parser/stream_parser/kerberos_fields.go`
- `common/pcapx/pcaputil/protocol_semantic_summary.go`
- `common/pcapx/pcaputil/protocol_summary.go`
- `common/pcapx/pcaputil/bin_parser_session.go`

Kerberos decoder 新增有界 `Semantic Summary` metadata；该 metadata 只包含摘要
所需事实。非延迟事件直接复用，延迟事件通过
`DecodeKerberosSummaryFacts` 做一次 same-message bounded projection，不保留完整
字段树。

## 验证

- 合成契约测试覆盖 Kerberos request/reply/error、嵌套 PA-TGS-REQ、LDAP
  Search/Bind/敏感值、SMB2 request/response/STATUS_PENDING 和 240 字符上限。
- 原 Kerberos 捕获继续使用
  `ndpi-kerberos-login.pcap`（SHA-256
  `bccc9bf683c262c7dacc455c73133980db6233066f8f24305213da85103496c2`），
  覆盖 26 个 UDP/TCP 消息、deferred/non-deferred 和 1/2/4 worker。
- 原 LDAP StartTLS 捕获继续使用
  `ldap-openldap-starttls-live.pcap`（SHA-256
  `37d18bd8d943f09c1bbc73b9b5e479996cf557703cb31903d94001c21b6c9065`）。
- 原 SMB2 Samba 捕获继续使用 `smb2-samba-live.pcap`（SHA-256
  `647ee8d3b64a4e70d2b6964481c95fc26d17e8952944f1786b5005781ca1f503`）。
- 回归运行 `common/bin-parser/parser/stream_parser`、
  `common/bin-parser`、`common/pcapx/pcaputil`，并执行 `go vet` 与
  `git diff --check`。

本地最终结果（2026-11-03）：

- 合入最新 #5371 源分支后
  `go test ./common/pcapx/pcaputil -count=1`：PASS，250.331s。
- `go test ./common/bin-parser -count=1`：PASS，79.744s。
- `go test ./common/bin-parser/parser/stream_parser -count=1`：PASS，4.228s。
- Kerberos facts、摘要合同、真实三协议捕获和 SMB2 session 定向 `-race`：
  PASS。macOS linker 输出仓库既有的 `LC_DYSYMTAB` warning，不影响测试结果。
- `FuzzProtocolSemanticSummaryText` 5s：51,463 次执行，28 个新 interesting
  inputs，无崩溃。
- `go vet`（三个目标包）：PASS；`git diff --check`：PASS。

## 仍开放

- 本项不扩展 Kerberos 解密、PAC/票据内容、KDC 结果校验或跨消息交换验证。
- 本项不扩展 LDAP 目录语义、过滤器求值、SASL 层解开或认证证明。
- 本项不扩展 SMB2 加密 transform、业务结果或完整协议状态机。
- PR #5371 的其他非 SMB、TLS/QUIC/HTTP3、IPv4 和 SMB 任务不在本增量内。
