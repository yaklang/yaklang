# SMB/DCE-RPC 横向移动调查任务

## 调查背景

EDR 告警显示一台工作站可能与域内目标发生远程服务创建。PCAP 只覆盖很短的 SMB 和 DCE/RPC 阶段。目标是判断网络侧能看到哪些步骤，以及能否形成可复核的横向移动候选。

## 任务 A：连接关系

列出 SMB/RPC 的源、目的 IP/端口和发起方，统计时间、方向和字节，标记会话是否完整。

## 任务 B：SMB 行为

1. 找出 SMB2 协商、认证、树连接、文件读写和服务控制消息。
2. 提取共享、路径、文件名及请求/响应关联。
3. 判断能否恢复远程执行命令行。
4. 若不能，说明缺协议字段、上下文还是数据完整性。

## 任务 C：DCE/RPC 行为

识别 Endpoint Mapper、目标接口、版本和服务控制操作，追溯到原始包，并标记解析错误、未匹配消息和上下文缺失。

## 任务 D：横向移动判定

输出：

- 事实：实际出现的连接、消息和文件操作。
- 推断：是否与 PsExec/SMBExec/远程服务创建一致。
- 未知：凭据是否成功、进程是否启动、命令结果和持久化。

## 任务 E：处置和扩展

搜索其他主机是否连接相同目标服务，检查共享写入、服务创建、账户和时间相关性，关联域控、EDR、Windows 事件日志和文件哈希。

网络流量中的账号字符串不能直接认定为攻击者身份。

## 当前实现预期

- SMB2 请求/响应摘要可解析。
- DCE/RPC Bind 可解析出接口和上下文。
- Endpoint Mapper 和 svcctl 的完整业务语义仍可能缺失。
- 可能出现 `malformed`、`incomplete` 或 unmatched MessageId。
- 单凭 PCAP 不足以证明横向移动成功。

## 命令

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/lateral-movement-psexec/capture.pcapng -workers 1 -protocol smb2 -rows 30
```

```sh
/tmp/pcap-inspect -read testdata/enterprise-traffic-analysis/lateral-movement-psexec/capture.pcapng -workers 1 -protocol dcerpc -rows 30
```

## 禁止的捷径

- 不因端口 445 就判定横向移动。
- 不因 DCE/RPC Bind 就判定服务创建成功。
- 不因 `ADMIN$`、`IPC$` 或 `svcctl` 字符串就认定攻击者身份。
- 不把流量候选取代 Windows 事件、EDR 和身份日志。

## 证据要求

- 保留 SMB/RPC 消息的 MessageID、SessionID 和 PacketID。
- 证据必须包含源/目的、方向、时间、请求/响应关联和完整性。
- 区分协议事实、横向移动推断和未确认的执行结果。
- 导出或引用原始包时记录捕获来源和 SHA256。
