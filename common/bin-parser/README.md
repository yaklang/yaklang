# 流解析工具

rule文件以yaml格式编写，支持解析多种格式的数据流，二进制文件、链路层、网络层、应用层协议等。

逐协议交付与打分验收见 [PROTOCOL_DELIVERY.md](PROTOCOL_DELIVERY.md)。P0 打分结果见 [P0_SCORES.md](P0_SCORES.md)，P1 打分结果见 [P1_SCORES.md](P1_SCORES.md)。P0 / P1 标 `done` 前必须过硬门槛并达到 **B 级及以上（总分 ≥ 75）**。A 级（≥ 90）留给 Wireshark 级主 PDU，不是清掉 leftover 之后的默认分。

## 1. rule组成
rule的根节点为Package，其下包含多个子节点，用来描述数据的结构、属性。
如
```yaml
Package:
  Rule1: xxx
  Rule2: xxx
```
## 2. rule节点
每个节点由属性和子节点组成，属性用小写字母开头的key表示，子节点用大写字母开头的key表示。
如一个Package节点
```yaml
Package:
  endian: big
  list: true
  Rule1: xxx
```
其中endian、isList为Package的属性，Rule1为Package的子节点。
## 3. 属性
默认解析器内置了一些属性，包括：
- endian: 字节序，big或little，默认为big
- isList: 是否为列表，true或false，默认为false
- length: 长度，用于描述固定长度的数据，如uint32、ipv4等
## 4. operator
operator是一个特殊的属性，可以编写yak代码，控制解析流程
如
```yaml
Package:
  endian: big
  list: true
  length: 10
  operator: |
    d = this.Rule1.Process()
    if d == 1 {
      this.Rule2.Process()
    }
  Rule1: raw,10
  Rule2: raw,10
```
## 5. Context
operator可以使用context传输上下文数据，如
```yaml
Package:
  endian: big
  list: true
  length: 10
  operator: |
    this.SetCtx("rule1-length", 1)
  Rule1: 
    operator: |
      this.GetCtx("rule1-length")
```
## 6. 默认解析器
可以通过属性、上下文控制默认解析器的解析行为，上面提到了属性，下面介绍上下文
- stopArray: 停止数组解析，true或false，默认为false

## 7. 0907 性能快照（2026-09-07）

完整应用解析路径为 `ParseBinary → NodeToMap → encoding/json`，包含完整字段和 `additionInfo` 元数据。固定工作量是 Memcached / Cassandra 的 **11 条消息、1,421 字节/批**，每批创建独立读取器并保留原 worker 调度；样本加载与规则预热不计时。

基线为 `5b6cbe1f8f7e7e505f8f3aaa542d4ac12f0d3741`，0907 最终实测版本为 `35607643c484b3d13c8e6fb08320c6c527b2b896`。环境：Apple M1 Max、10 个逻辑 CPU、64 GiB 内存、macOS 14.1.2、Go 1.22.12、darwin/arm64、CGO 开启。桌面环境未独占 CPU。

基线与优化版交替、串行执行独立进程，每个版本在 1 / 10 worker 下各测 **5 个独立的 3 秒窗口**；工作目录与 `PWD` 一致，计时期间不编译、不跑回归或 profile。下表为五次中位数，耗时与分配均以一批为单位：

| worker | 基线耗时 | 优化后耗时 | 吞吐倍数 | 基线分配字节 | 优化后分配字节 | 基线分配次数 | 优化后分配次数 |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1.693982 ms | 0.843256 ms | **2.008859×** | 1,397,293 B | 721,059 B | 14,376 | 6,481 |
| 10 | 0.882223 ms | 0.466153 ms | **1.892561×** | 1,419,958 B | 739,941 B | 14,404 | 6,507 |

单 worker 约 **6,494 → 13,045 条消息/秒**，分配字节减少约 **48.4%**，分配次数减少约 **54.9%**。单 worker 的五次耗时范围为基线 1.690730–1.702709 ms、优化后 0.840191–0.847291 ms；10 worker 为 0.876314–0.889323 ms、0.463226–0.484146 ms。单次窗口有波动，不能保证每次都达到两倍。

主要优化包括紧凑配置及历史存储、共享不可变默认配置、嵌入 YAML 构树计划、批量准备节点、直接读取字段字节范围、预分配导出集合，以及十个精确匹配的 Memcached / Cassandra 原生桥。每次解析独立持有可变节点、上下文与事务；公开结果不池化，原 Yak 错误及兼容路径保留。

20 种代表负载在 `GOMAXPROCS=1/10` 下共 40 个组合，最终均未出现超过 5% 的耗时回退，最大增加 0.96%。此前 AllJoyn 大计数负载的持续回退已通过减少重复配置读取与加锁修复，独立复测分别为 +0.56% / +0.69%。这些数字来自同一冻结版本的历史测量，**不是清理数据或 rebase 后重新取得的性能成绩**。

保留的 [性能核心数据](reports/performance-20260907.json) 包含环境、版本、主基准全部 20 个窗口及代表负载原值和汇总，便于重算。中间日志、profile、失败实验补丁和阶段性报告已清理；完整历史记录仍可在 [清理前的报告提交](https://github.com/yaklang/yaklang/tree/6d56809935c8b8446bc9a323e9411112db084f8e/common/bin-parser/reports/hotpath-20260907) 查看。

这些是指定小样本、显式入口的离线解析结果，不包含自动协议识别、TCP 重组、实时捕获、长期队列、客户端渲染或 AI 调用，不能换算为持续无丢包带宽或完整协议支持率。字段解析范围见 [协议目录](protocol_catalog.go)、[P0 消息范围](P0_SCOPE.md) 和 [当前范围](ACTIVE_SCOPE.md)。

### 复现与回归

从仓库根目录执行：

```sh
go test ./common/bin-parser/... -count=1 -timeout=5m
go test -race ./common/bin-parser/parser/... -count=1 -timeout=5m

# 每个命令调用是一个独立窗口；正式对照请先编译两版程序，再交替运行各五次。
(cd common/bin-parser && GOMAXPROCS=1 go test -run '^$' -bench '^BenchmarkCurrentCorpusApplicationJSON$' -benchtime=3s -benchmem -count=1)
(cd common/bin-parser && GOMAXPROCS=10 go test -run '^$' -bench '^BenchmarkCurrentCorpusApplicationJSON$' -benchtime=3s -benchmem -count=1)
```

0907 冻结版本的 bin-parser 全包回归、相关 race、58,533 条封装记录及 11 条应用消息的输出摘要对照均通过；这是历史验证记录。当前提交的构建结果以 [PR #5013 的 Checks](https://github.com/yaklang/yaklang/pull/5013/checks) 为准。

## 8. 数据维护

保留协议实现、YAML 规则、回归/基准代码、被测试实际引用的 PCAP、必要的派生流、来源/许可和说明。`testdata/protocol-corpus` 的捕获完整性、字段、边界与负例仍参与正常测试；代表帧按位置、长度和 SHA-256 核对，不再保存重复 hex 文件。实验产物应写入仓库外的临时目录。
