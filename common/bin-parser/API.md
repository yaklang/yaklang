# 完整消息执行计划

`PrepareStructured` 将“选择一个明确的规则入口”与“重复解析完整消息”分开。计划只保存规则及原生解码函数，不保存样本输出、可变 Node、VM 或会话状态；可以由多个 worker 共享。

```go
// MQTT 3.1.1 已由握手/调用方确认，且上游已经完成消息分帧。
plan, err := bin_parser.PrepareStructured(
    "application-layer.mqtt_fields", "MQTT311PacketFields",
)
if err != nil {
    return err
}

// 对同一协议上下文的后续完整 PDU 重复调用；每次完整校验并生成新对象。
result, err := plan.Parse(completePDU)
if err != nil {
    // 交给路由层处理分帧错误、状态变化或重新分类；不自动吞掉错误。
    return err
}
fields, metadata := result["fields"], result["metadata"]
```

这里的 `bin_parser` 是 `github.com/yaklang/yaklang/common/bin-parser` 的包名。片段中的 `completePDU` 由调用方提供；示例不是 TCP 重组器。

## 两个接口的用途

| 接口 | 成功路径 | 非法数据 | 不支持的入口 |
|---|---|---|---|
| `ParseStructured(data, rule, entry)` | 已准入入口原生直出；其他走 Node | 保留旧路径 Yak 错误文本与位置 | 使用原解析器 |
| `PrepareStructured(rule, entry)` → `plan.Parse(data)` | 复用已解析的明确入口，原生直出 | 直接返回原生校验错误，不重复调用 VM | 准备阶段报错，由调用方选择原解析器 |

两者都生成完整 `fields` 和 `metadata`，不做 JSON 编码。返回对象拥有自己的数据，允许返回后复用输入、保留输出、并发使用同一个计划。正在解析时仍不能改写输入。修改默认 parser 注册后，已有计划也会执行新注册的解析器。

## 必须由上游确定的信息

计划不推断协议、方向、版本、协商能力或事务阶段。MySQL 的 greeting、command、resultset 等仍是不同入口；MQTT 的 3.1 与 3.1.1 也不能因端口相同就混用。路由层可以缓存一组适合当前阶段的计划，但不能把首次消息入口永远套用到该连接的全部消息。

`Parse` 的输入必须符合所选入口的完整边界。它可能是一条 PDU，也可能是规则明确定义的响应块。TCP 重组回调给出的 chunk 不保证是完整消息；拆包、粘包必须先处理。此 API 没有增加 pcap、TCP 重组、通用分帧器或自动会话迁移。

## 当前准入范围

14 个内置规则文件、82 个入口：Memcached 3、Cassandra 7、TNS 9、TDS 6、LDAP 7、MySQL/MariaDB 12、PostgreSQL 16、IMAP 3、POP3 11、SMTP 2、MQTT 2、Kerberos 2、SMB3 Transform 1、TLS Certificate 1。

规则必须是经过完整构造检查的内置 YAML。根设置、Package、入口和整个 operator 程序都要符合已审核的简单形式。含试探/恢复的 Carrier、子节点、额外长度/输出设置、自定义 operator 和其他复杂规则继续保留原执行语义。

缓存只对内置文件分配槽位，正向及不支持的结果都只编译一次；未知用户名称不会无限增长缓存。增加协议数量不会让已经取得的 `plan.Parse` 开始遍历全部协议，但新增协议的解码复杂度、首次分类与会话生命周期仍需要单独测量。
