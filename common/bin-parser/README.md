# bin-parser：规则驱动的协议解析

YAML 描述字段、长度、类型分发和必要的 Yak operator。结果可以是 Node 树，
也可以是可独立持有的结构化对象；规则与执行计划缓存复用，解析期间不要求 JSON 编码。
pcapx 提供实时抓包/文件回放、TCP 重组、有限协议探测、消息分帧和会话内计划复用。

| 文档 | 内容 |
|---|---|
| [抓包、初步解析与回放](../pcapx/pcaputil/BIN_PARSER.md) | 类 Wireshark 基本工作流、CLI、Yak/Go API、当前自动协议范围 |
| [消息解析 API](API.md) | ParseStructured / PrepareStructured、完整消息和上下文合同 |
| [协议实现 TODO](PROTOCOL_TODO.md) | 路线图、未完成模型和实时协议接入任务 |
| [最终性能与边界](PERFORMANCE.md) | 当前实测结果、失败边界和复现入口 |
| [协议交付标准](PROTOCOL_DELIVERY.md) | 字段、样本、负例、完整帧与评分验收 |
| [P0 消息合同](P0_SCOPE.md)、[P0 评分](P0_SCORES.md)、[P1 评分](P1_SCORES.md) | 被测试约束的交付范围与评分证据 |
| [样本目录](testdata/protocol-corpus/README.md) | 捕获、字段合同、来源、许可和维护工具 |

当前实时准入为 HTTP/1.x、TLS、MQTT 3.1/3.1.1、DNS、Kerberos，以及 Memcached /
Cassandra 的限定阶段。YAML、显式字段入口和完整实时协议支持是不同层次；其余协议
从 TODO 和 `protocol_catalog.go` 选择明确入口，不把已识别端口当作完整解析。

## 规则与结果

大写键描述字段/子节点，小写键描述设置。常用设置包括 `endian`、`list`、
`length` 和 `length-from-field`。复杂控制流可用 operator，但应优先使用声明式规则；
可审核的轻量计划会被复用，其余规则保留原解释路径。

```yaml
Package:
  Message:
    endian: big
    operator: |
      kind = this.Kind.Process()
      if kind == 1 {
          this.Value.Process()
      }
    Kind: uint8
    Value: uint32
```

输入必须满足所选入口的完整消息边界。TCP chunk 不保证是一条完整消息；实时流量
由 pcapx framer 先拆包/合包，再复用计划。版本、方向、能力和事务阶段来自实际协商
或显式配置，不能仅由端口猜测。

## 压缩规则与懒加载

`rules/**/*.yaml` 是源文件；运行时只嵌入 `rules/rules.tar.zst`。生成器固定路径顺序、
LF 换行和 tar 元数据。修改 YAML 后执行：

```sh
go generate ./common/bin-parser/rules
```

`RuleFS` 首次访问时解压并建立只读索引，随后并发复用；构造时不解压、不创建
goroutine。结构化入口索引及规则指纹校验也延迟到首次使用。解压上限为 32 MiB。
ReadFile 返回独立副本；内部文件视图共享不可变归档，避免逐文件内容缓冲的重复分配。
规则正文的 LF 字节、字段语义和计划校验一致，使用现有纯 Go zstd，无新增 CGO。
测试逐文件检查源码与归档相同，CI 校验生成结果没有漂移。

## 开发与验证

```sh
go generate ./common/bin-parser
go generate ./common/bin-parser/rules
go test ./common/utils/embeddedfs ./common/bin-parser/... -count=1 -timeout=5m
go test ./common/pcapx/pcaputil ./common/pcapx/cmd/... -count=1 -timeout=5m
go test -race ./common/utils/embeddedfs ./common/bin-parser/rules ./common/bin-parser/parser/... -count=1 -timeout=5m
```

后续优先逐协议补齐字段、协商状态和实时入口。保留全部回归正负样本和来源；
性能只比较 1/2/4 worker、同等完整输出及 CPU 时间。实验日志、profile、过程报告、
临时 EXE 和压测大文件放在仓库外。
