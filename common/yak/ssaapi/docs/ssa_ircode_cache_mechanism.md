# SSA IRCode Cache 机制说明

## 目标

数据库编译模式下，IR 不再要求“一直常驻内存直到编译结束”。  
这次实现不再通过 `context` 临时塞配置，也不再在 SSA 层额外叠一层中转持久化逻辑。

现在的结构是：

- `ssaconfig` 提供唯一的 compile IR cache 配置
- `ssa.Cache` 只做模式分发
- `dbcache.Cache` 负责内存 / TTL / 异步保存 / save-ack 后删除

## 配置

配置只来自 `ssaconfig.SSACompileConfig`：

- `CompileIrCacheTTL`
- `CompileIrCacheMax`

默认值：

- `CompileIrCacheTTL = 1s`
- `CompileIrCacheMax = 5000`

当前实现是自适应的：

- 小项目（当前用 handler bytes 判断）走 close-only fast path
- 大项目仍走 runtime spill
- 更大的项目会自动把 runtime spill 调整为更激进的 TTL / max

语义：

- `ttl > 0`：开启基于时间的淘汰
- `max > 0`：开启基于数量的淘汰
- `ttl = 0 && max = 0`：退化成旧行为，常驻内存直到 close

`ssa` 内不再保留第二套默认 TTL 常量。

当前热指令集合：

- `Function`
- `BasicBlock`
- `ConstInst`
- `Undefined`
- `Make`

其中 `BasicBlock` 当前必须常驻。
原因是 `BasicBlock` 持有 `ScopeTable`，而这条分支还没有实现 block/scope 的等价 reload。
如果运行时把 `BasicBlock` 清掉，就会在 lazybuild 的参数绑定、CFG 语法块、side-effect 绑定等路径上放大成 nil-scope / nil-variable panic。

## 运行模型

### 内存模式

- 直接使用内存 map
- 不做数据库写回

### 数据库模式

`dbcache.Cache` 内部有三层动作：

1. resident  
   - IR 在内存里，受 TTL / 容量管理
2. pending persist  
   - TTL 或容量命中后，不立即删内存
   - 先进入异步 marshal + batch save 流程
3. save ack  
   - 数据库保存成功后，才真正把这条 IR 从内存里删除

这样保证：

- 不会出现“内存删了，但数据库 saver 还没真正写完”的窗口
- `Get()` 命中 pending 项时，直接返回内存对象，并取消这次删除
- 删除语义优先，`DeleteInstruction()` 不会因为异步落库重新把已删除 IR 写回数据库

## SSA 层实现

### `ssa.NewProgram`

- 改成直接接收 `*ssaconfig.Config`
- compile IR cache 配置不再通过 `context` 传递

### `ssa.Cache`

- 现在只有两种 backend：
  - memory backend
  - dbcache backend

不再是：

- `SafeMapWithKey + 中转层`
- `PersistenceStrategy`
- `SerializingPersistenceStrategy`

也就是说，数据库模式下只有一份权威缓存，不再存在“外层 map 还持有对象，缓存层已经清理但实际对象还悬挂”的问题。

### IR 写库

- `marshalInstruction()` 里不再手动查询旧行然后给 `gorm.Model.ID` 补值
- 改成写库阶段统一按 `(program_name, code_id)` 做 upsert

这部分逻辑放在 `ssadb.UpsertIrCode(...)`。

## 调试日志

数据库编译模式下保留 debug 日志：

- reload：`[ssa-ir-cache] reload`
- save：`[ssa-ir-cache] save`
- save skip：`[ssa-ir-cache] save-skip`
- writeback：`[ssa-ir-cache] writeback`
- 汇总：`[ssa-ir-cache-summary]`
- saver 统计：`[ssa-ir-cache-saver]`

这些日志只用于观察 cache 行为，不改外部接口。

## Close 路径

- `Close()` 不再单独走一套同步大批量直写逻辑
- close 时只是把 resident 项统一标记为 `deleted`
- 然后继续复用同一条 `marshal -> batch save -> save ack delete` 流程
- 这样可以保留运行时和 close 的一致语义，同时避免 close 阶段再维护第二套保存代码

当前 saver 是单写者批量刷盘模型，并输出以下统计：

- `pending` / `pending_max`
- `batch_count`
- `avg_batch` / `max_batch`
- `enqueue_block_total` / `enqueue_block_max`
- `save_loop_time` / `save_loop_max`

## 当前版本选择

当前这条分支上，实际采用的是：

- 保留 `v12` 的 `memedit` / editor 常驻量优化
- `BasicBlock` 恢复为热指令，不参与运行时淘汰
- 以“先保证真实大项目能稳定跑完、没有 panic”为当前优先级

这意味着当前版本不是“峰值内存最低”的版本，而是“在保住 `v12` 内存优化收益的同时，先修正 `BasicBlock` 被淘汰导致的稳定性问题”的版本。

## 当前验证

`2026-03-25` 在 `~/Target/decompiled-code-target` 上重新验证了一次当前版本（`decompiled_code_target_memory_v12_hotblock`）。

结果：

- 无 panic
- 退出状态 `0`
- total: `17:00.04`
- peak RSS: `9,148,200 KB`
- CPU: `707%`
- pre-handler: `4m55.148484446s`
- save: `11m33.961170351s`

对照历史结果：

| profile | total | peak RSS | CPU | pre-handler | save |
| --- | --- | --- | --- | --- | --- |
| `v11` | `15:38.29` | `8,128,116 KB` | `722%` | `4m39.296261115s` | `9m47.598118706s` |
| 旧 `v12` | `21:20.23` | `6,907,112 KB` | `617%` | `5m2.016784189s` | `14m45.946317026s` |
| 当前 `v12 hotblock` | `17:00.04` | `9,148,200 KB` | `707%` | `4m55.148484446s` | `11m33.961170351s` |

当前结论：

- 相比旧 `v12`，当前版本已经明显更快，并且 CPU 利用率恢复
- 相比 `v11`，当前版本仍然更慢，峰值 RSS 也更高
- 当前内存回升的直接表象是最终 save 阶段待写 IR 明显增多：
  - 旧 `v12`: `finishing save cache instruction(len:1278489)`
  - 当前 `v12 hotblock`: `finishing save cache instruction(len:1922361)`

也就是说，`BasicBlock` 恢复为热指令以后，稳定性问题暂时消失了，但更多 IR 被保留到了 save 阶段，导致峰值内存和 save 压力一起上升。

## TODO

当前先接受 `BasicBlock` 常驻。

下一步更合理的方向是：

- 运行时不要对“尚未 finish 的函数内部 IR”启动 TTL / 淘汰
- 只在函数 finish 之后，才允许其内部 IR 进入 runtime spill

这样理论上可以同时保住：

- block/scope 的正确性
- 更平缓的内存曲线
- 避免把大量未完成函数状态过早标记为可清理对象

## 测试

已覆盖的核心行为：

- `common/utils/last_non_zero_test.go`
  - `LastNonZero` 泛型工具
- `common/utils/dbcache/cache_test.go`
  - TTL 命中后等待 save ack 才删除
  - pending 项再次读取会回到 resident
  - 删除会取消后续落库
  - `ttl=0 && max=0` 时保持常驻直到 close
- `common/yak/ssa/database_cache_test.go`
  - TTL 过期后从数据库 reload
  - 删除的 instruction 不会落库
  - dirty lazy instruction 能正确 writeback
