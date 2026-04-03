# SSA 编译期 IR Cache 机制

## 目的

当 SSA 编译跑在数据库模式时，IR 对象没必要一直常驻内存直到整个编译结束。

编译期 IR cache 的目标，是在降低峰值内存的同时保持下面这些性质不被破坏：

- 最近刚访问过的 IR 仍然可以直接从内存快速读取
- 被 spill 到数据库的 IR 仍然可以按需 lazy reload
- 只有对应的数据库写入确认成功后，内存里的 IR 才能真正移除
- 运行时淘汰和最终 close 阶段的 delete / writeback 语义保持一致

## 分层归属

这套机制现在分在四层：

- `ssaconfig.SSACompileConfig`
  - 持有编译期 cache 配置
- `ssa.ProgramCache`
  - 编排 instruction / type / source / index 四个专用 store
- `database_cache_instruction*` / `database_cache_type` / `database_cache_source` / `database_cache_index`
  - 负责各自对象的 resident 状态、spill / save / reload 策略，以及与 DB 的桥接
- `dbcache`
  - 在真正需要的地方提供 resident cache、spill queue、batch save 等基础能力
- `ssadb`
  - 持久化 `IrCode`、`IrSource`、`IrIndex`、`IrOffset` 和 reload 依赖的数据

`ssaapi.Config` 不应该再保留一份重复的 compile IR cache 状态。运行期只读取
`ssaconfig.Config` 里的生效值。

## 配置项

当前对外暴露的编译期 IR cache 开关是：

- `CompileIrCacheTTL`
- `CompileIrCacheMax`

它们的含义是：

- `ttl > 0`
  - 常驻 IR 可以按时间过期
- `max > 0`
  - 常驻 IR 可以按容量过期
- `ttl = 0 && max = 0`
  - 禁用运行时淘汰，IR 一直保留到 close

### 自适应默认值

cache 策略会根据编译输入规模自动调整。这个判断使用的运行时输入是
`SSACompileConfig.CompileProjectBytes`。

这个值的定位是：

- 统计进入编译阶段的源码总字节数
- 只用于给小项目 / 大项目调默认 cache 策略
- 不序列化到 JSON
- 不属于长期项目元数据

## 运行时模型

数据库模式下，instruction cache 现在有三个可观察状态：

1. resident
   - IR 还在内存里
2. pending persist
   - 已经开始淘汰，但在异步持久化完成前仍然保留内存副本
3. persisted and removed
   - 数据库写入成功，内存副本可以删除

这样可以避免一种坏窗口：内存已经清掉了，但数据库写入还没完成。

### Save 确认

淘汰并不会立刻把 resident IR 从内存删掉。

当前流程是：

1. 先把 resident 对象标成 pending
2. marshal 成数据库结构
3. 批量写入
4. 只有 save 成功后才删除 resident 副本

如果一个 pending 对象在 save 完成前又被访问到了，运行时仍然可以继续返回内存中的那份对象，而不是提前强制从数据库 reload。

## Function.Finish 保护

编译期 IR 在整个构建过程中不是一视同仁的。

属于未完成函数的 instruction，不能只因为触发了 TTL 或容量阈值就被淘汰。此时函数仍然持有活跃构建状态，比如参数、自由变量、parameter member、block，以及其他函数级 IR 关系。

当前策略是：

- 未完成函数上的 IR 不参与运行时淘汰
- 当 `Function.Finish()` 执行后，再把函数级 IR 重新纳入淘汰追踪
- hot instruction 即使 finish 之后也可以继续常驻

这样可以一边保持构建过程稳定，一边让已经完成的函数参与降内存。

## Hot Instruction

有一部分 instruction 会被刻意保持得比普通 IR 更热，因为它们会被高频回访，或者仍然持有目前无法可靠 reload 的运行时状态。

这份 hot instruction 集合定义在 `common/yak/ssa/database_cache_instruction_policy.go`。当前包括：

- `Function`
- `BasicBlock`
- `ConstInst`
- `Undefined`
- `Make`

这是一处维护点。如果后续 reload 能力变化，这份 hot 集合需要和 reload 保证一起重新审视。

## Source 持久化

`IrCode` 会引用 `IrSource`，所以 source 的持久化也必须遵循与 instruction 一样的确认语义。

source hash 的流程是：

1. 先把 source hash 预留为 pending
2. 把 `IrSource` 放进保存队列
3. save 成功后，才把它从 pending 移到 persisted 集合
4. 如果 save 失败，要把 pending 预留清掉，后续重试才能继续发生

这样可以避免数据库实际没写进去，但运行时误以为这个 source 已经持久化过。

## Close 语义

close 阶段需要遵循与运行时淘汰一致的持久化语义，但现在由各个专用 store 各自完成收尾。

当前 close 流程是：

- `type store` 先落 type
- `index store` 落 `IrIndex` / `IrOffset`
- `instruction store` 完成 instruction spill / writeback / close flush
- `source store` 最后补齐仍未落库的 source payload

这样可以保持语义一致，同时避免在 `ProgramCache` 里塞一层通用中间 cache 抽象。

## 可观测性

现在可以通过 debug 日志观察 cache 行为。常见日志族包括：

- reload
- save
- save skip
- writeback
- saver summary
- cache summary

这些日志是运维和调试辅助，不应该成为理解公开 API 的前提。

## 维护注意点

后续修改这套机制时，至少要重新检查下面这些不变量：

- 编译期 cache 配置必须来自 `ssaconfig`
- 未完成函数上的 IR 不能被过早淘汰
- resident 对象只能在 save 确认之后移除
- `IrSource` save 失败不能污染后续重试
- hot instruction 的假设必须和 reload 保证一致

## 测试

当前主要回归测试在：

- `common/utils/dbcache/cache_test.go`
- `common/yak/ssa/database_cache_test.go`
- `common/yak/ssa/database_search_test.go`

如果机制继续变化，建议优先补或更新下面这些场景：

- `Function.Finish()` 触发后的淘汰
- lazy reload
- dirty writeback
- source hash 的确认与重试
- hot instruction 常驻行为
