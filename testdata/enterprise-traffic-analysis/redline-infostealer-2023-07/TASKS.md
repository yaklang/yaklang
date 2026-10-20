# RedLine Stealer 调查任务

## 调查背景

企业收到一台 Windows 主机疑似下载恶意软件的报告，并取得该主机所在网段的短时 PCAP。目标是确认受害对象、还原网络时间线、定位可疑基础设施并说明证据边界。

样本时间使用 PCAP 显示时区。公开答案使用 UTC，作答时必须明确采用哪一个。

## 任务 A：捕获质量

1. 统计包数、捕获字节和起止时间。
2. 列出内网主机和外网端点。
3. 说明 DHCP、DNS、mDNS、Kerberos、HTTP、TLS 和未知 TCP 是否存在。
4. 找出截断、解析失败或上下文不足的记录。

产物：捕获范围和证据质量说明。

## 任务 B：确认受害对象

1. 从 DHCP、mDNS、NBNS、Kerberos、LDAP 和原始包寻找 IP、MAC、主机名和用户候选。
2. 对每个候选标注来源协议、时间、消息 ID 和置信度。
3. 不得只凭单个 DHCP Hostname、User-Agent 或 MAC 认定资产身份。
4. 说明流量证据不能直接证明什么。

## 任务 C：还原下载链

至少回答：

1. 哪个主机先访问了哪个域名？
2. 明文 HTTP 请求的方法、Host、Path 和状态码是什么？
3. 是否发生 HTTP 301/302 跳转，`Location` 指向哪里？
4. 跳转后建立了哪个 TLS 会话，SNI 是什么？
5. 服务器向客户端传输了多少应用数据，方向如何？
6. 能否提取并验证文件？如果不能，缺什么？

## 任务 D：定位 C2 候选

1. 找出非常用端口的对外 TCP 连接。
2. 统计双向包数和应用载荷字节。
3. 检查持续时间、周期和会话是否完整。
4. 说明“C2 候选”与“已证实 C2”的区别。

## 任务 E：影响判断

1. 公开资料说恶意软件尝试窃取哪些类别？
2. 这些类别是否都能从 PCAP 直接证明？
3. 哪些主机侧检查和凭据处置不能由 PCAP 代替？

## 任务 F：交付报告

报告至少包含：

- 执行摘要。
- 受害主机和用户候选，以及置信度。
- 时间线。
- URL、域名、IP、SNI、端口和 User-Agent。
- 每个结论引用 MessageID、SessionID、StreamID 或 PacketID。
- TLS 正文不可见、文件未验证、公开答案补充等限制。
- 建议的隔离、封禁、凭据重置、扩展搜索和检测规则。

## 当前实现预期

可稳定观察到：

- DHCP 请求和响应。
- mDNS 中的 `DESKTOP-9PEA63H.local`。
- DNS 查询和响应。
- `623start.site` 的两个 HTTP GET。
- `92.118.151.9` 返回 HTTP 301 到 `guiatelefonos.com`。
- TLS ClientHello 的 `SNI=guiatelefonos.com`。
- 主机向 `194.26.135.119:12432` 发起 TCP 连接。
- 解密前的 TLS 应用数据只显示密文字节。

当前不能仅凭普通摘要稳定回答：

- `victim.hostname`、`victim.user` 的规范化实体。
- HTTP 下载对象、MIME 和 SHA256。
- 未知端口载荷的协议语义。
- TLS 正文中的实际文件内容。

## 禁止的捷径

- 不要仅凭公开 README 宣称工具完成了 RedLine 识别。
- 不要把 `194.26.135.119:12432` 写成“已确认 C2”，除非引用检测规则或外部情报。
- 不要把 DNS 查询成功等同于恶意软件下载成功。
- 不要把约 550 KB 双向流量直接称为“已泄露 550 KB 数据”。
- 不要执行样本中的任何二进制或脚本。

## 证据查询提示

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/redline-infostealer-2023-07/capture.pcap -workers 1 -protocol http -rows 20
```

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/redline-infostealer-2023-07/capture.pcap -workers 1 -protocol tls -rows 40
```

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/redline-infostealer-2023-07/capture.pcap -workers 1 -flow 68 -rows 20
```

## 可接受答案示例

```text
调查范围：
  2023-07-11 06:39:22 至 06:39:58，2,497 个包，单网段短时捕获。

事实：
  10.7.10.47 查询 guiatelefonos.com；
  随后与 92.118.151.9:443 建立 TLS 会话，SNI 为 guiatelefonos.com；
  服务器到客户端出现约 354 KB 应用数据，正文加密不可见。

推断：
  该会话与公开分析所述恶意载荷下载相符，但文件类型、哈希和内容未由 PCAP 验证。

未知：
  缺少 keylog 和文件落盘证据，无法仅凭网络流量确认执行结果。
```
