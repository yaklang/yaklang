# 主机与用户识别任务

## 调查目标

建立主机与用户候选，并说明每个候选的来源、时间、关联方式和置信度。重点不是解出一个字符串，而是判断它能否归属于某个 IP 和时间窗口。

## 文件用途

| 文件 | 主要用途 |
| --- | --- |
| `...1-of-5.pcap` | DHCP 和 NBNS 关联 MAC、IP、主机名 |
| `...2-of-5.pcap` | NBNS 识别 Windows 主机名 |
| `...3-of-5.pcap` | Kerberos、LDAP 用户线索和浏览器 User-Agent |
| `...4-of-5.pcap` | Android 浏览器 User-Agent |
| `...5-of-5.pcap` | iOS/Safari User-Agent |

## 任务 A：DHCP 实体

1. 找出 DHCP Request/Reply。
2. 提取 MAC、Requested IP、Hostname、Client Identifier 和租约。
3. 判断 Hostname 是否已经解码成可展示字符串。
4. 如果字段是 base64，指出这是当前解析契约缺口，不直接把编码值当作主机名。

## 任务 B：NBNS/mDNS

1. 找出主机名声明。
2. 说明主机名出现的源 IP 和方向。
3. 判断能否与 DHCP 交叉验证。
4. 说明重名、伪造和广播缓存导致的错误可能。

## 任务 C：Windows 用户

1. 在 Kerberos 和 LDAP 中寻找用户主体、域和认证时间。
2. 判断当前工具是否有稳定的 `user_principal` 字段。
3. 如果没有，说明原始 DER/ASN.1 中是否包含线索，以及需要补什么实体提取。
4. 不把机器账户、服务账户和交互用户混淆。

## 任务 D：User-Agent 推断

1. 提取平台和浏览器线索。
2. 标注这是弱证据。
3. 不把 User-Agent 作为资产或用户权威身份。

## 任务 E：实体归一

候选实体表至少包含：

- EntityType、原始值和规范化值。
- IP、MAC、域名和域环境。
- FirstSeen、LastSeen。
- 来源协议和 MessageID/PacketID。
- Confidence、冲突和未知。

## 当前实现预期

- DHCP 可解析。
- DHCP Hostname 当前输出可能是 base64。
- mDNS/NBNS 的文本线索可能可读。
- Kerberos 输出复杂 DER 字段树。
- 普通摘要不保证提供稳定用户名实体。
- LDAP SASL 可能标记为 `context-required`。
- HTTP User-Agent 可读取，但只是弱线索。

## 禁止的捷径

- 不根据文件名或公开答案填入结果。
- 不把 MAC、主机名或 User-Agent 当作个人或权威资产身份。
- 不把多个时间段的同一 IP 当成同一主机。
- 不把 Kerberos 任意字符串都当作当前登录用户。

## 验收标准

同时返回原始证据、规范化实体、时间有效性、来源、置信度和未解析状态。只有 `hostname=...` 字符串不算完成。
