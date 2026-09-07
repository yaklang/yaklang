# 2026-09-06 测量证据

结论、环境、工作负载定义、统计方法、限制及复现命令见 [性能与 AI 评估](../../PERFORMANCE_ASSESSMENT.md)。下表首轮文本来自同一冻结 v2 测试二进制的输出；并非新的生成捕获，不修改 corpus 清单。所有新基准使用公开解析入口，报告真实 worker 数。

第二轮独立归档见 [优化实验](OPTIMIZATION_EXPERIMENTS.md) 和 [逐次数据及统计](optimization-summary.json)：`prepared-*` 是已撤回的路径原型及其同程序对照，`single-pass-*` 是保留的单次字段投影优化及其同程序对照。两者分别记录程序摘要，不能与首轮基线混合算收益。`prepared-rule-experiment.patch` 保留撤回代码用于隔离复现；不是待合并实现。`single-pass-verification.txt` 为完整定向验证日志，`single-pass-race.txt` 为相关 race 输出（含链接器警告，退出码 0）；[最终协议联合复验](single-pass-protocol-final.txt) 包含 Memcached/Cassandra 原生、公开入口、合同、TNS/TDS/TLS 相邻桥接及单次投影定向测试，退出码 0。

| 文件 | 内容与用途 |
| --- | --- |
| [summary.json](summary.json) | 主吞吐基准逐次记录、纳入规则及独立计算的中位数/范围；输入 Mbps 不等于实时链路容量 |
| [envelopes-v2.txt](envelopes-v2.txt) | 1/4 CPU 列表的原始全记录输出。首行实际 4 worker 与列表位置不一致，不计入主统计；其他 5 行有效 |
| [envelopes-single.txt](envelopes-single.txt) | 独立进程补齐的第 3 个单 worker 全记录结果 |
| [application-v2.txt](application-v2.txt) | 字段 / JSON，1/4 worker，600 ms、各 3 轮 |
| [cpu8.txt](cpu8.txt) | 8 worker 全记录 3 轮；另保留同次应用 1x 探索行，后者不参与 600 ms 正式比较 |
| [cpu10-envelopes.txt](cpu10-envelopes.txt) | 10 worker 全记录 3 轮 |
| [cpu8-10-application.txt](cpu8-10-application.txt) | 字段 / JSON，8/10 worker，600 ms、各 3 轮 |
| [latency-v2.txt](latency-v2.txt) | 3 轮单 worker 延迟，每个入口每轮 2,200 条；各 entry 的百分位、GC、分配、采样堆及导出字节 |
| [rss-valid.txt](rss-valid.txt) | 单次 4 worker 全记录进程 `/usr/bin/time -l`，包含加载和启动；不是 10 worker 或整个客户端内存 |
| [sv.txt](sv.txt) | 真实 SV 消息，解析及一次 root 结果遍历，300 ms、3 轮 |
| [dicom.txt](dicom.txt) | 既有 4,096 项内存压力向量，3x、3 轮；不是原始捕获中的超长帧 |
| [igrp.txt](igrp.txt) | 既有 104 路由内存压力向量，3x、3 轮 |
| [profile.txt](profile.txt) | 3 秒 JSON 剖析基准输出；与无剖析主吞吐数据分开 |
| [cpu-top.txt](cpu-top.txt)、[cpu-path.txt](cpu-path.txt) | CPU profile 的 top 与路径父子关系，原始 profile 在 `/tmp/binparser-current-cpu-20260906.pprof` |
| [alloc-top.txt](alloc-top.txt) | `alloc_space` 累积分配摘要；不是 RSS 或泄漏。原始 profile 在 `/tmp/binparser-current-mem-20260906.pprof` |
| [verification-summary.txt](verification-summary.txt) | 完整定向验证日志的**投影**：保留全记录统计、所有顶层 PASS 和总体 PASS，不是全部子测试输出。完整本机日志 `/tmp/binparser-current-verification-20260906.log` |
| [protocol-targeted-final.txt](protocol-targeted-final.txt) | 报告整理后的 Memcached/Cassandra 原生与公开入口、合同及相邻桥接联合定向复验，退出码 0 |

补充失败的设置检查：第一次 RSS 启动使用仓库根目录，未找到相对 manifest，在任何工作负载计时前即退出；未纳入数据。随后在 `common/bin-parser` 目录成功运行，见 `rss-valid.txt`。主统计未剔除配置正确的慢值。

上述 `/tmp` 文件是本机临时诊断文件，可能被清理；持久文本证据和重新生成命令保存在仓库。本轮没有上传捕获、调用模型或运行真实网络发送。
