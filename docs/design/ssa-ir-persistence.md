# SSA IR 持久化：并行单元编译与存储优化

本分支 `enhance/ssa/parallel-compile-persistence` 只做一件事：让 SSA 编译在
大项目上把内存留在可控范围内，同时不牺牲落库速度和查询正确性。下面按「改了
什么、为什么、效果」写。

## 1. 背景：原来的问题

编译一个 Hue 量级的项目时，原有实现有三处结构性开销：

1. **整棵 IR 图常驻**。每个函数体、每条指令、每个类型都留在 Go 堆上直到整个
   项目编译结束。内存峰值随项目线性增长，只能靠加内存或 OOM 收场。
2. **落库是逐条同步写**。指令一条一条 INSERT，SQLite 每个事务都要走一次
   B-tree 定位；大项目落库时间被写放大主导。
3. **类型行重复且很宽**。同一个类型体在每个 `type_id` 上各存一份，Hue 实测
   347 万行类型只对应 10.6 万种不同内容，多存的是宽度不是信息。

优化目标因此拆成三条：把图按编译单元释放、把落库改成批处理、把类型体去重。

## 2. 并行单元编译：先落库，再放内存

### 做法

- 每个编译单元（文件 / 包）构建完成后进入 flush：单元内的指令、索引、偏移
  先批量落库，然后把这一单元的**普通值**（普通指令、常量、临时值）从内存
  图上摘掉，只保留跨单元仍被引用的东西。
- 跨单元引用不能摘：包级变量、被闭包捕获的符号、函数与蓝图（类型）仍然常驻。
  `canSpillFinishedPrivateBlock` 只在「块已脱离」或「作用域链已空」时释放，
  否则保留。
- 释放之后仍要能读：被摘掉的值改由 `DetachRetained` 记住指令 id，下次
  `GetValue` 时按 id 从库里惰性取回。

### 为什么这样做

内存占用由「所有单元之和」变成「同时在飞的单元 + 常驻符号」。落库是释放的
前提，因此顺序必须是 flush → release，不能反过来。

### 踩过的坑（都已修）

- **去重键被过早丢弃**。落库去重依赖 `indexSaved` / `offsetSaved` 两张表，
  一开始把「已结算水位」也当成可清理状态，导致已落库的索引/偏移被重复写入：
  实测 `dropped_offsets=21572`、`dropped_indexes=12319`，重写后出现 1293 条
  重复 `ir_indices_v1`。修法是：结算水位只能裁剪 `residentKeys`，
  `indexSaved` / `offsetSaved` 永久保留。
- **保留绑定的晚到替换丢失**。值被 `DetachRetained` 换成 id 之后，调用方手里
  的还是旧对象指针，`Replace` 按指针比对就匹配不上，替换被静默吞掉。修法是
  指针或保留 id 任一匹配即接受。
- **类型身份快路径漏掉晚到的父类型 id**。类型持久化按身份键跳过未变化的行，
  身份键只覆盖名字与容器，父类型/接口后来才拿到 id 时就不会重写。修法是身份
  键里加入父类型与接口 id 的校验和。

## 3. 落库：批处理与单事务

- IR code 写入改成一次 `SaveInBatches`（原生 upsert / DM MERGE），同批次只有
  一条语句，重复的 `(program_name, code_id)` 在批内先去重（保留最后一条），
  避免 PostgreSQL 因同一条语句里重复出现冲突键而拒绝。
- 一个批次是一个事务：末行被拒时前面几块一起回滚，不会留下半个批次。
- `ir_codes` 的 `(program_name, code_id)` 唯一索引**始终存在**。曾经有一套
  「写前挂起唯一索引、写完恢复」的机制，它让写路径出现两条分支，也让常量查找
  在挂起期间撞上缺失的索引。现在只有一条写路径，那套挂起/恢复代码整个删除。

### 索引

- 模型上的 `gorm:"index"` 标签去掉，索引改为 `patchIrCodeIndex` 显式建：
  `ux_ir_codes_program_code`（唯一）、`idx_ir_codes_program_opcode`、以及
  常量查找用的部分索引 `idx_ir_codes_const_lookup`。
- SQLite / PostgreSQL 上额外剪掉 16 个被复合索引覆盖、或任何查询都不当谓词的
  单列索引。Hue 的库里这部分约占 860MB，且每次编译都要重建一遍 B-tree。
- `INDEXED BY` 在 SQLite 里是**硬约束**而不是提示：索引不存在时语句直接报错，
  不会退回全表扫描。常量查找因此加了 epoch：索引被挂起/恢复后提示立即失效，
  遇到 `no such index` 就重新探测并重试一次。

## 4. 类型存储：为什么是 id → payload 两级

Hue 实测（`hue-20261029-1911-b96dc7005e09`，`ssadb.db` 4.6GB）：

| 项目 | 数值 |
| --- | --- |
| `ir_codes` 行数 | 5,409,291 |
| 带 `type_id` 的 `ir_codes` 行 | 4,585,439 |
| 不同 `type_id` | 3,288,059 |
| `ir_type_ids_v1` 行数 | 3,469,539 |
| `ir_type_payloads_v1` 行数 | 105,927 |
| 平均每个 payload 被多少个 id 复用 | 32.8（最大 1,391,391） |

同一个库上把三种布局都物理建出来量了一遍（`dbstat` 页字节，单位字节）：

| 布局 | 类型相关总字节 | 说明 |
| --- | --- | --- |
| main 的单表 `ir_types`（含 gorm 三列） | 613,994,496 | 每个 id 一份完整类型体 |
| 单表但不带 gorm 三列 | 545,787,904 | 省掉 `id/created_at/updated_at` |
| 本分支 id + payload 两级 | 401,960,960 | 类型体只存 105,927 份 |

所以两级表省下的是 **212MB / 613MB ≈ 35%**，来源有两块：一是类型体从 347 万
份降到 10.6 万份（省 184MB），二是 id 行不带 gorm 三列（`IrTypeID` 刻意不嵌
`gorm.Model`，否则每行 ~185 字节，比它替换掉的 `ir_types` 行还宽）。

**为什么不是 `ir_codes` 直连 payload**：`ir_codes` 已经有 4.58M 行带类型，
如果每行直接存 payload 外键，`type_id` 这个语义就没有了 —— 但读路径需要它：
- `ir_codes.type_id` 是稳定标识，同一个类型在重编译后 id 不变；
- 蓝图（class/interface）之间按 `type_id` 互相引用父类型与接口；
- `GetTypeFromDB(cache, irID)` 的入参就是 `type_id`，缓存也按它做键。

若改成 `ir_codes → payload_id`，就要在 `ir_codes` 上加一列，并把
「父蓝图引用」从 `type_id` 改成 payload 引用 —— 而父蓝图引用的是**声明**，
不是内容：两个内容相同的类型可以是不同声明。合并它们会改变类型比较结果。

所以 `ir_type_ids` 不是转发层，它承载的是「类型 id → 类型内容」这条必须存在的
映射；`payload` 才是纯粹去重用的。这也回答了「是不是没必要改类型库」：单表
（第 2 行 545MB）确实比 main 省，但比两级（401MB）多 143MB，代价是类型体在
每次重编译时随 id 行一起重写。

写入代价也用同一份数据量过（20,000 行原地 upsert，SQLite WAL，
`synchronous=NORMAL`）：单表整行重写 0.04s，两级窄 id 行重写 0.02s，
**1.76×**。

## 5. 读路径

- 类型读取改走 `database/sql`：GORM 的 `Raw` 会被 program scope 重写，join
  出来的列名对不上；原生查询固定列序，也不受 scope 影响。
- offset 预读按 `(program_name, value_id)` 一次 `IN` 查询批量取回，程序侧排序，
  SQL 里不加 `ORDER BY`（加了就退回全表扫）。
- 类型去重表（`ir_type_ids` / `ir_type_payloads`）的唯一键是
  `(program_name, type_id)` 和 `(program_name, type_hash)`，重放 flush 不会
  写第二份。

## 6. 按项目绑定，而不是按进程

一个 yaklang 后端可以同时跑多个前端编译项目，所以下面这些状态都改成按
program 绑定：

- IR code 写入统计（新行 / upsert 行 / upsert 语句数）按 program 分开计数，
  `IrCodeWriteBreakdown(program)` 只报自己那一份。
- offset 读缓存的键是 `(program, generation, value_id)`。指令 id 只在项目内
  唯一，键里没有 program 就会串项目；写批次推进该 program 的 generation，
  旧快照自然失效，不用遍历清理。项目删除时连 generation 一起丢弃。
- `WithSaveByteBudget` / `WithSkipEviction` 在选项里就把元素类型擦成闭包，
  `config` 里不再有裸 `any` 字段；类型断言只在构造时发生一次。

## 7. 内存探测

`availableMemoryBytes` 原来直接读 `/proc/meminfo`，只能在 Linux 上用。现在
内存相关的两个函数放在 `common/yak/ssa/ssautil`：

- `ssautil.AvailableMemoryBytes()` / `ssautil.HasMemoryHeadroom(floor)`，
  数据来自 `common/utils/memorybudget`（gopsutil + cgroup 限制）。
- `ssadb` 只保留阈值常量 `indexSortMemoryFloorBytes`（6GiB）和一句判断，
  不自己读文件。
- 平台取不到数值时返回「未知」，调用方按「没有余量」处理，落到磁盘排序这条
  保守路径 —— 缺指标不能当成内存充足。

## 8. Go 的 forward type（构建期占位类型）

`common/yak/go2ssa/forward_type.go` 里的 `forwardNamedType` 是给 Go 源码用的
占位类型：Go 允许先使用后声明（同一包内），当编译器在解析阶段遇到一个还没见
过声明的具名类型时，先放一个只记住名字的占位符。所有函数体开始构建之前，占位
符会被真正的声明替换掉，所以后面文件里的字段类型不会冻结成 `any` 或 `0`。
这不是 Go 语言本身的特性，是本前端为了处理「一个包跨多个文件、声明顺序不定」
引入的中间表示。

## 9. 验证与代价

- 三个真实项目（ARTEX、yaklang 自身、Hue）在本分支上都跑过完整编译 + 扫描，
  退出码 0，结果与基线一致。Hue 的验收基线是 857.663s（`4afb6ffee810`），
  后续提交在 1036.8s / 1190.1s / 1668.9s 之间波动，属于编译期方差，不是回归。
- `scripts/ssa-test.sh -p 1` 覆盖 `ssa/...`、`ssaapi/...`、`go2ssa/...` 共 22
  个包，串行全过。
- 已知代价：类型两级表让读类型时多一次 join（命中 `ir_type_ids` 唯一索引，
  单行读取）；Hue 量级下这层 join 的耗时被缓存吸收，未进入 pprof 热路径。

## 10. 明确没做的事

- 不做任何数据库兼容/迁移代码。表名变了就变了（`ir_types` → `ir_type_ids` +
  `ir_type_payloads`），旧库不读。
- 不在编译结束前丢弃整棵编译图：SyntaxFlow 还需要 DB-only 的等价结果，那条
  路还没打通。
- 不合并文档：本文件是本分支唯一的说明文档。
