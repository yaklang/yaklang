# pcapx 性能与验证

完整抓包、TCP 重组、协议分帧和字段解析结果统一保留在
[最终性能与边界](../../bin-parser/PERFORMANCE.md)，操作见 [抓包分析指南](BIN_PARSER.md)。
仅读取/重组字节的基准不能代替完整协议解析带宽。

默认单 worker 同步处理；多 worker 按双向会话分片，流内有序，慢回调会产生背压。
只比较 1/2/4 worker，并检查成功字节、CPU、队列、缺口和原生丢包。
单会话不能靠增加 worker 并行解析其 TCP 状态。

回归、边界、并发与基准代码保留在本包，过程日志不提交到 testdata。
`pcap-load` 生成本机 HTTP/HTTPS/MQTT，`pcap-inspect` 捕获、保存与回放。
同步写盘仍可能影响读取，原生缓冲只能吸收短时突发。
