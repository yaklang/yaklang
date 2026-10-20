# 待纳入的公开语料

本目录记录尚未直接加入稳定测试集或尚未验证当前解析能力的公开资源。不要在这里放未经校验的下载链接和预期答案。

## 优先级

| 优先级 | 场景 | 来源 | 暂不直接纳入的原因 |
| --- | --- | --- | --- |
| P0 | 对象导出：HTTP/SMB/SMTP/FTP | Unit 42 extracting-objects | 需逐协议验证对象完整性和隔离导出 |
| P0 | 业务故障和 RTT/重传 | 自有合成或公开正常业务捕获 | 需建立确定性性能断言 |
| P1 | Kerberos/密码喷洒 | PCAP-ATTACK CredAccess | 需验证账号字段和误报边界 |
| P1 | 多主机感染扩展 | Malware-Traffic-Analysis | 文件较大，需明确体积策略 |
| P1 | QUIC/TLS 1.3 keylog | 仓库内部 sealed fixtures | 需补公开可再分发的独立样本 |
| P2 | 实时抓包和丢包 | 自有环境 | 不适合静态 PCAP 回归 |

## 纳入标准

1. 可以稳定下载，或已保存在仓库并记录 SHA256。
2. 来源和再分发条件清楚。
3. 场景能对应当前企业调查任务。
4. 已验证样本不会触发外部网络访问。
5. 任务有可复现事实、允许推断和未知。
6. 大小和运行时间适合 CI。

## Unit 42 对象导出资源

- 教程：<https://unit42.paloaltonetworks.com/using-wireshark-exporting-objects-from-a-pcap/>
- 下载：<https://raw.githubusercontent.com/PaloAltoNetworks/Unit42-Wireshark-tutorials/main/Wireshark-tutorial-extracting-objects-5-pcaps.zip>
- ZIP 密码：`infected`

接入时分别验证 HTTP 对象名称/MIME/长度/SHA256、SMB 对象完成度、SMTP/IMF 消息和附件、FTP 下载与上传方向，以及导出文件隔离、权限和审计。

## Malware-Traffic-Analysis 资源

该站点会更新 ZIP 密码方案。下载前查看 `about` 页面；不要把旧密码写入长期自动化脚本。

候选：

- 2025-01-22 fake Google Authenticator：恶意广告、主机和用户身份、C2。
- 2024-09-04 Big Fish in a Little Pond：告警研判和多协议关联。
- 2024-07-30 You dirty rat：RAT 感染报告。

只有在确认大小、来源页面、密码和 SHA256 后才加入正式案例目录。

## 不在本目录做的事

- 不存放真实企业捕获或生产 keylog。
- 不存放未经授权提取的凭据。
- 不把公开 PDF 答案作为工具输入。
- 不把“样本中存在某域名”写成“当前域名仍恶意或仍在线”。
