# HTTPS + keylog 调查任务

## 调查目标

比较同一份 HTTPS 捕获在没有 keylog 和提供授权 keylog 时的可分析范围，并验证解密后流量是否进入下游 HTTP 解析。

## 任务 A：无 keylog 基线

记录 TLS 会话数、SNI/证书/ALPN/协商版本、HTTP 可见性、Application Data 可见性和可推断范围。

预期：应用数据标记为 `encrypted`，不能读取 URL、HTTP Header、Body 或对象。

## 任务 B：keylog 解密

加入 `capture.keys` 后记录：

1. Application Data 是否变为 `authenticated-plaintext`。
2. 是否出现解密后的 HTTP 请求和响应。
3. `GET /invest_20.dll` 的 Host、状态和响应大小。
4. `POST /docs.php` 的 Host、次数和响应状态。
5. 其他办公软件正常 HTTPS 流量如何与恶意活动区分。

## 任务 C：覆盖率与限制

统计 keylog 条目、成功解密、未匹配、unsupported suite、认证失败和无法解析的应用数据。

必须明确：记录认证不等于证书可信，也不等于用户或服务身份已经验证。

## 任务 D：导出与对象验证

识别下载对象、检查完整性和声明长度、计算 SHA256、在隔离目录做类型识别。不要执行下载文件。

公开教程给出的 DLL SHA256 只用于验证提取结果：

```text
31cf42b2a7c5c558f44cfc67684cc344c17d4946d3a1e0b2cecb8eb58173cb2f
```

该值不能由工具预先返回。

## 任务 E：AI 导入链路

检查 AI PCAP 工具能否接收 keylog、将解密后的 HTTP 存入 `pcapdb.protocol_messages`、通过 `query_pcapdb` 查询 URL/Host/Body，并在报告中标明 keylog 使用和解密覆盖率。

预期：当前 `load_pcapfile` 不支持 keylog，完整 AI 持久化分析链路应标记为未实现。

## 命令

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/https-keylog-dridex/capture.pcap -workers 1 -protocol http -rows 20
```

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/https-keylog-dridex/capture.pcap -tls-keylog testdata/enterprise-traffic-analysis/https-keylog-dridex/capture.keys -workers 1 -protocol http -rows 20
```

## 禁止的捷径

- 不复制 keylog 到不受控位置，不在日志打印密钥。
- 不把“解密成功”写成“所有 TLS 都可解密”。
- 不把记录认证写成证书已验证。
- 不预置公开样本的 DLL 哈希作为查询结果。

## 证据要求

- 保留 DatasetID、MessageID、SessionID、StreamID 和 PacketID。
- 对每个解密会话记录 keylog 条目、认证状态和覆盖率。
- 对导出对象记录来源、大小、类型、SHA256 和完整性。
- 对无法解密或不支持的会话明确列出原因。
