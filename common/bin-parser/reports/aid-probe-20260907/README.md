# 现有 aid PCAP 脚本的离线行为评估

2026-09-07。**这是现有工具脚本的受控运行，不是模型正确率或完整 aid 集成测试。** 本轮只增加可复现评估与报告，没有修改脚本、PCAP 后端、客户端、协议实现、依赖或 P0 门槛。14 项延期范围保持不变。

## 结论

8 个用例各重复 3 次，全部实际执行。每次均有 5 个用例违反预期合同，另 3 个基础用例通过。非日志结果逐用例、逐轮完全一致，包括统计、回调数、输出字节数与 SHA。**这些有意选取的对照不是随机样本，不能把通过比例当作 AI 正确率或产品总体可靠率。**

| 用例 | 独立核对 / 预期 | 原脚本实际行为（三轮一致） |
| --- | --- | --- |
| Cassandra 原捕获 | 20 条记录，应用样本为 CQL / internode | 20 条 TCP，但误报 1 次 DNS 查询 |
| Memcached 文本原捕获 | 10 条记录 | 10 条 TCP，HTTP / DNS 均为 0；基础计数通过，不等于应用字段已接入 |
| 旧 / PR #5023 两代二进制 GET | 每份 4 条记录，文件身份独立 | 每份 4 条 TCP，HTTP / DNS 均为 0；两份基础计数通过 |
| Memcached，`max-packets=1` | 输出并报告 1 条；检查是否形成硬处理上限 | 输出 1 条详情，却报告总数 2；全部 10 条仍进入包处理回调 |
| Memcached，`tcp port 1` | 独立 libpcap BPF 匹配结果为 0 | 仍报告、回调、输出全部 10 条；输出 SHA 与无过滤相同 |
| Memcached，`tcp and (` | 独立 BPF 编译器返回语法错误 | API 不返回错误，仍处理 10 条并报告成功 |
| 无效文件 | 13 字节 `not-a-capture\n`，不是 PCAP | 后端记录 `unknown file format`，但 API 返回 nil；脚本报告 0 条且成功 |

原始结果见 [三轮完整日志](contracts-three-runs.txt)，含 24 条 `PROBE_JSON`，退出码为 **1**。失败保留为失败，没有用“期望当前错误”替换正确性断言。

## 误计数的具体来源

独立离线扫描在 `ndpi-cassandra.pcap` **frame 14、包内零基偏移 51** 找到 `01 00 00 01`；此记录的 TCP 端口为 `9042 → 37892`。该标记是脚本使用的 DNS 查询启发式之一，不能单独识别 DNS。对应文件 SHA 为 `5992c6bbbd1f84fafb84052520e710adec4144fbf5f71b75a8a6d74bad57d155`。

原有 [逐记录 Cassandra 字段检查](../../protocol_corpus_cassandra_fields_test.go) 把 frame 14 验证为 `CQLSupported5InitialFields`，逐项核对协议版本列表、压缩列表与 CQL_VERSION。结论不是仅凭端口或文件名得出。本轮再次独立运行该检查，结果见 [字段复验日志](cassandra-field-check.txt)。

相关源码路径：

- [分析脚本](../../../ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai/pcap/analyze_pcap.yak) 对整个 `packet.Data()` 做 DNS 字节子串判断；上限则先递增、再比较，仅让该回调提前返回。
- [离线 Start 分支](../../../pcapx/pcaputil/capture.go) 调用 OpenFile 后没有设置 BPF；打开失败只记日志，未把错误返回给调用者。[OpenFile](../../../pcapx/pcaputil/open_device.go) 自身确实返回了打开错误。
- [配置与回调](../../../pcapx/pcaputil/config.go) 的 WithBPFFilter 只保存表达式；TCP 处理在延迟执行的 everyPacket 回调前发生。本轮计数证明达到脚本上限后仍调用后续包处理；它不是磁盘 syscall 次数或 TCP 重组成功次数。

## 方法与复现边界

[评估代码](aid_probe_test.go) 带 `binparser_aid_probe` 构建标签，不进入默认 bin-parser 包测试列表。复现命令从仓库根执行：

```sh
go test -tags=binparser_aid_probe ./common/bin-parser/reports/aid-probe-20260907 -run '^TestAIDPCAPProbe$' -count=3 -v -timeout=3m
go test ./common/bin-parser -run '^TestProtocolCorpusCassandraFieldsOriginalRecords$' -count=1 -timeout=1m
```

第一条在当前版本**应当失败**，用于复现上表问题，而不是标准全量通过门槛。若以后修复相应行为，应按正确合同重新验收，不应保留错误来维持本次观察值。

补充检查：`go vet -tags=binparser_aid_probe ./common/bin-parser/reports/aid-probe-20260907` 通过；Cassandra 原记录字段复验 0.856 s，通过。默认 `go list ./common/bin-parser/...` 不包含此探针包。24 条 JSON 的重复一致性、12 个源文件 / 捕获 SHA、三份说明的 48 个本地链接均核对通过。本轮未重复完整 bin-parser 回归；上一轮结果与 P0 剩余失败继续保留。

执行环境为 Go 1.22.12、darwin/arm64、libpcap 1.10.1；模块版本与源文件 SHA 见 [源码身份](source-identity.txt)。使用当前工作树编译，非已安装 Yak 可执行文件。工作树含既有未提交变更，HEAD 本身不能代表完整构建身份；本轮未冻结完整工具二进制，所列 SHA 也不是整个传递依赖闭包。

执行的是未经修改的 `analyze_pcap.yak`，使用仓库 Yak VM、原有 append / len、真实 `pcaputil.OpenPcapFile`、真实过滤配置与每包回调。没有关闭默认 TCP assembly。额外加入独立计数回调和每用例 20 秒 context，限制评估失控；三轮均正常结束，未触发超时。PCAP 输入限定为选中的本地文件，不开放网卡捕获。

隔离项必须计入限制：

- CLI 参数通过固定值适配，不评估真实命令行解析、默认值应用或 aid 参数转换。
- yakit 日志由内存收集器接收，AutoInitYakit 不执行；不连接 UI / webhook。
- `file.IsExisted` 使用本地存在检查；`file.Save` 只接收文本，不真实写入脚本的临时路径。日志中的“已保存”是原脚本发出的消息，不能证明文件落盘。
- Join 适配器只接受该脚本实际追加的字符串，再使用 strings.Join；没有修改统计、过滤或上限逻辑。文件持久化成本和其他类型的 Join 行为未测。
- 独立 PCAP reader 核对文件记录，独立编译 / 执行 BPF 判断期望结果；两者使用与后端相同的现有 libpcap 绑定，不新增依赖。
- 没有调用 aid Tool.Callback、模型、RPC 或客户端。没有验证 ToolExecutionResult.Success 的实际传播、模型是否会采信错误统计、用户界面显示、模型时延、token 用量或回答正确率。

4 份未经重复计算的输入共 38 条原始记录；无过滤时生成详情文本共 **31,495 字节**，不是结构化应用字段。该值只说明这批文本大小，不是 token 计数、峰值内存、吞吐或客户端体验指标。三轮命令的包测试耗时为 1.320 s（包含测试进程开销等），**不用它估计解析吞吐或带宽**；性能结论继续使用 [冻结程序的对照报告](../config-store-20260907/README.md)。

首次探针构建同时引入两个 libpcap Go 绑定，发生重复符号错误，见 [构建失败记录](initial-build.txt)。随后只使用库已经采用的 `github.com/yaklang/pcap`，未改 go.mod / go.sum。修正后的 [第一次行为观察](first-observation.txt) 已复现相同 5 类用例失败；再加入独立字节位置取证后执行本报告三轮。没有把构建失败当作产品缺陷，也没有隐去它。

## 对 AI 产品评估的影响

当前字段解析组件能为这些样本提供可追溯证据，但现有 PCAP 脚本尚未调用这些字段入口。工具层已经能够产生错误计数或错误成功信息，因此不能仅凭“模型接到了工具结果”宣称可用的 AI 版 Wireshark。

后续若获准修改工具，应先修复离线过滤、错误传播和资源上限，再接入带 capture SHA、frame、字节范围及 decoded / partial / raw / deferred 状态的有界字段结果。随后用 [已有 12 道样本题](../../PERFORMANCE_ASSESSMENT.md) 测事实、证据定位和未知情况处理，分别归因工具错误与模型错误。这里的顺序是评估建议，不代表本轮已获准实施新的 aid / 客户端集成。

历史 P0 对 LDAP、MySQL、PostgreSQL、SMB3 的整体状态要求与本轮样本范围冲突仍独立存在；没有更改目录 partial、评分、门槛或验收范围，也不据此次工具评估宣布整体 goal 完成。
