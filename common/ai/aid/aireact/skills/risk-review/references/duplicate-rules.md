# Risk 重复判定规则

## 候选分组

首选分组键：

```text
normalized_target | normalized_risk_type | parameter
```

规范化目标时至少处理：

- scheme 和 host 大小写
- 默认端口 80/443
- URL 末尾 `/`
- query 参数顺序
- 空参数和重复参数

候选分组只是降低审查量，不能直接写回。

## 判定重复

同时满足以下条件才可判重复：

1. 目标指向同一业务对象或同一端点。
2. RiskType 表达同一类安全问题。
3. 输入点或触发条件基本相同。
4. 根因相同，而不只是现象相似。
5. 证据没有显示两个独立业务实例。
6. 修复其中一个根因时，另一个也会随之失效。

## 常见反例

- `/api/users/1` 与 `/api/users/2` 的越权：同一模式，但不同业务对象实例。除非用户要求按根因归并，否则保留为独立风险。
- 同页面的反射 XSS 与存储 XSS：类型接近但触发和影响不同。
- 同一参数上的 SQL 注入与命令注入：目标和参数相同，但漏洞类型和影响不同。
- 相同标题但不同插件、不同证据：不能仅按标题归并。
- 同一 URL 的登录后与未登录问题：身份条件不同，影响可能不同。

## Canonical 选择

依次比较：

1. 仍为 active 且未被标记误报。
2. 有完整请求/响应或 PacketPairs。
3. 描述、复现步骤和修复建议更完整。
4. 严重程度更高。
5. CreatedAt 更早。
6. ID 更小。

canonical 不应再标记为 duplicate。禁止重复链和自引用。

## 写回要求

`reason` 必须说明：

- 共同目标
- 共同类型
- 共同参数或输入
- 共同根因
- 使用了哪些证据

示例：

```text
same normalized target /api/orders, type IDOR, parameter orderId,
and both requests returned order 1002 for the same test identity.
Canonical 101 contains the complete request/response pair.
```
