# SSA 存储共享：后续重构方案

状态：设计记录，未在 `enhance/ssa/parallel-unit-compile` 实施数据库重构。

当前分支保持表、列、索引和数据库读取格式兼容，优化编译时对象、字符串、方法表、变量 range，以及重复生成和重复写入。本文的拆表、迁移和新 ID 关系留给后续独立分支。

## 方案摘要

| 对象 | 保持现有数据库和读取行为 | 允许存储升级或类型行为调整 |
| --- | --- | --- |
| Offset | 内存共享 range 描述；完整绑定去重；dirty 批量保存；按实际查询评估索引；确认无效关联生命周期 | range 行打包 `value_ids` 和配对的变量绑定，并选择反向查询表示；或 range 表加整数关联表。Code 的 `primary_range_id` 只覆盖主位置 |
| Type | 共享不可变描述、字符串及完整快照编码；跳过未变化写入；完整可达性审查后减少临时类型保存 | 先共享 payload、保留 type 身份；再冻结类型核心，外置局部状态，直接复用对象和 Type ID |

逻辑绑定可以聚合到列表中。Offset 打包方案须保留变量名与 value 的对应关系；Type 合并须比较完整类型语义和可修改状态。主表减行、数据库总体积、写入次数和查询速度应分别验收。

详细兼容边界和方案见下方“2026-10-09 补充”。当前源码与已验证二进制的区别、编译和扫描实测、pprof 热点及待验证修改集中记录在 [SSA 性能状态](ssa-performance-status.md)。

## 依据与统计口径

2026-10-08 的 GoBlog 基线：源码 `b93d2284033e3a394489dc9d4429aae78378770e`，Yaklang `19fd2ebc2c1f7467930f38c88e913f2d59b1c0b3`。

本机证据目录：`/mnt/data/scan-outputs/ir-dirty-rebase-main-20261008-1441/goblog/`。Code、Type、Index 数量通过只读 SQL 复核；Offset 数量、共享 range 数、最大关联数、体积、其他表计数和查询计划于 `2026-10-08T17:05:21+08:00` 复核，证据在 `/mnt/data/scan-outputs/ssa-memory-reuse-20261008-1550/goblog/storage-review.json`。源码另经 `json.loads(quoted_code)` 解码后计数，命令口径和时间保存在该证据中。

| 数据 | 基线观察 | 需要区分的内容 |
| --- | --- | --- |
| Code | 503,970 条；常量 360,882 条；placeholder 298,302 条 | 常量值、一次源码使用、定义与用户关系 |
| Type | 约 424,000 条；322,757 个 string 类型的保存字段相同 | 不可变表示与可修改的类型身份/方法/完整名称 |
| Index | 475,568 条；全部关联字段去重后 204,117 条 | 完全相同的关联与不同 scope/version/owner 的合法关联 |
| Offset | 227,473 条；74,403 个文件和起止位置组合，平均 3.06 个关联，最多 325 个 | range 描述与 variable/value 使用关系 |
| Offset 体积 | 表约 38.3 MiB；8 个 SQLite 索引约 60.3 MiB | 数据重复与查询所需的索引成本 |
| Source | 审查统计 266 条含目录；本机 267 条 = 227 份 Go 源码 + 1 个 go.mod + 39 条目录 | 明确文件/目录计数口径，源码内容未发现重复 |
| Name pool | 5,693 个不同名称 | 现有名称池去重有效 |
| Program | 一条元数据；`line_count=89,431`；保存 Go 源码 44,432 个 LF，按 MemEditor 口径 44,659 行；加上 go.mod 的 130 行，全部保存文件共 44,789 行 | 唯一文件行数与多次编译/延迟构建访问次数；Go 的两种行数相差每文件一行 |

`Type.String()` 相等不证明类型结构、方法或可修改状态相等。pprof 的分配栈也不证明对象被哪个引用保留。统计必须分别记录 SQL 行数、SQL 唯一关系、进程峰值 RSS、GC 后 live heap、累计分配和实际完成的编译结果。

## 当前分支能够做的优化

1. 同一 block、editor 和准确 source range 中的同值 placeholder，通过程序构建期间的 `utils.SafeMap` 复用。源码 literal 保留独立身份；已修改 type、extern、origin、pointer、变量或成员状态的 placeholder 不再命中复用。builder 完成后释放索引。
2. Index 写队列前按完整关联键去重；内存查询索引的 name/value 关联也只存一次。键包含 program、value、variable、class、field、scope、owner、version，以及每个 nullable ID 的存在位。不同字段、NULL/零、作用域、版本的记录继续保留。
3. 类型共享不可变描述和字符串，按需创建方法状态，一次合并一批方法。可修改的 Type 指针与 ID 保持独立；减少重复 placeholder 构造，也直接减少其默认 string 类型的构造和保存。
4. 保存后释放可回载的匿名 IR。回载须保留常量分类、unary、origin、成员关系、函数和编译 package 上下文；setter 重新发布 dirty。不能只检查 SQL 写计数。
5. 按唯一源码路径累计行数，重复 `PushEditor`/`PopEditor(true)` 不再累加同一文件。行数是元数据修复，不是删除源码。

这些优化无需新增表或迁移旧数据库。跨 range 的所有常量统一 ID、全部类型 ID 合并、range 拆表、删除 SQL 索引均属于后续设计。

## 当前分支的性能实测

同一 GoBlog 输入的 `ssa-compile`，2026-10-08 本机单次对照。这里的“优化版本”是 `yak-optimized-final` 对应的源码快照；2026-10-09 又修改了指令查找、作用域版本链和 SSAAPI 成员搜索，这批修改尚未取得构建、修改后回归及新性能对照结果，不能将以下指标直接归给最新工作区：

| 指标 | rebase 后基线 | 优化版本 |
| --- | ---: | ---: |
| 总耗时 | 275.20 秒 | 145.13 秒 |
| user CPU | 2,398.71 秒 | 1,704.06 秒 |
| 峰值 RSS | 2.290 GiB | 2.314 GiB |
| 最终 pprof live heap | 1,090.90 MiB | 817.78 MiB |
| 累计分配 | 约 64.10 GiB | 52.01 GiB |
| placeholder Code | 298,302 | 32,149 |
| Type 行 | 424,138 | 158,427 |
| Index 行 | 475,568 | 206,147 |
| 完全重复的 Index 关系 | 271,451 | 0 |

数据库表、列、SQL 索引定义与基线一致。`line_count` 已按全部唯一保存文件累计为 44,789。Type 行数下降主要来自减少 placeholder 对应类型的生成，尚未合并可修改 Type 身份。

峰值 RSS 没有下降，不能把最终 live heap 的下降当成峰值改善。CPU profile 中 GC 仍占较高比例，ANTLR 仍是主要累计分配来源，本轮未调整它们。另一组仅恢复原匿名 IR 常驻策略的诊断对照耗时 140.34 秒、最终 live heap 835.69 MiB；它帮助区分缓存驻留成本，不能证明释放策略独立带来加速。

性能证据：`/mnt/data/scan-outputs/ssa-memory-reuse-20261008-1550/goblog-final/`。编译动态行为造成的成员/对象图差异保留在报告中，按用户要求由其他分支处理；本分支的验收重点是性能、重复写入和内存生命周期。

同一输入、相同规则目录和默认预算下，`code-scan` 的 source/struct/ssa 全流程已单独完成两版对照：

| 指标 | rebase 后基线 | 优化版本 |
| --- | ---: | ---: |
| 端到端总耗时 | 290.27 秒 | 184.23 秒 |
| 编译与结构规则阶段 | 232.42 秒 | 128.78 秒 |
| SSA 规则阶段 | 37.21 秒 | 32.48 秒 |
| 峰值 RSS | 2.799 GiB | 2.454 GiB |
| 扫描结束 live heap | 1,302.58 MiB | 1,037.59 MiB |
| 扫描累计分配 | 84.63 GiB | 72.88 GiB |

两个版本均退出码 0，各阶段成功。同步规则目录的内容指纹一致；记录的 source/struct/ssa 阶段风险指标均为 123/286/19。2026-10-09 进一步核对发现，struct 的 286 是结果对象数量；实际 struct 风险为 20 条，详见 [三段规则分析](ssa-performance-status.md)。SSA 查询成功 73、按适用条件跳过 275、失败 0；SARIF 均为 162 条结果，各规则结果数量一致。默认 work limit 和 wall timeout 未触发，两版同样记录 183 条既有 recursive visit limit 提示，不能将此对照称为无限遍历或全部图语义等价证明。

扫描的峰值 RSS 下降 12.32%，总耗时下降 36.53%；这组结果与纯编译峰值基本不变的观察应分别报告。证据：`/mnt/data/scan-outputs/ssa-memory-reuse-20261008-1550/codescan-summary.md` 和对应两个 `goblog-codescan-*` 目录。本轮均为单次对照，不是多轮统计。

## 常量：共享值，保留使用位置

建议模型：

```text
constant_payload(payload_id, representation_kind, raw_value, raw_type, unary, ...)
code(code_id, payload_id, opcode, type_ref, function, block, origin, ...)
code_occurrence(occurrence_id, code_id, range_id, function, block, ...)
```

首阶段只共享 payload，Code 身份及现有 dataflow 保留。这可以共享大字符串和数值表示，但不会直接减少所有 Code 行数。

后续如要共享同一个 Const 的 `code_id`，必须把每次使用的 range 和 block/function 身份放到 occurrence，并明确：

- SyntaxFlow 的 literal 搜索、成员 key 搜索、证据路径、IDE 跳转和报告返回哪些 occurrence。
- `GetRange()` 的兼容行为，以及新的多位置读取 API；不能仅取第一个位置后丢失其他位置。
- source literal 与内部 placeholder 的分类不会相互覆盖。
- 原始值的类型、数值精度、nil、unary、origin 和派生类型属于等价判定；不能只按格式化字符串判定。
- object/key/member pairs、users、mask、point/pointer 和变量版本是否引用 code 或 occurrence，关系在删除/替换时如何同步。
- 一个使用的 `SetType`、`SetExtern` 等修改如何从共享节点分离，其他使用保持原状态。

源码中相同文本、不同 editor、不同 source range 或不同函数，不可以直接合并为一个拥有单个 range 的现有 `ConstInst`。

## Type：共享完整表示，保留可修改身份

建议模型：

```text
type_shape(shape_id, kind, name_id, full_name_ids, structure, method_signature_refs, ...)
type_ref(type_id, program_id, shape_id)
code(..., type_id)
```

`type_shape` 不可变，相同完整表示只保存一次。`type_ref` 是小的身份关联，承载当前 compiler 的可修改 Type ID。变更类型时生成/命中新 shape，更新对应 ref；其他身份不会被污染。现有 BasicType 可以修改完整名称、方法及 ID；直接共享该对象会让一次修改影响其他调用者。冻结类型核心、把局部状态移到 ref/alias 后，可以进一步共享 Type 对象和持久化 ID，而不只共享字符串。

实现要求：

- 不仅比较 `kind/String/ExtraInformation`：现有类型保存信息未覆盖全部方法、结构、annotation 和 receiver 状态，先明确新 schema 的完整语义。
- 处理递归 ObjectType、FunctionType、Blueprint 和别名，避免对递归图直接展开字符串造成环或重复工作。
- canonical key 包含所有语义字段；内容 hash 冲突仍要比较实际表示。`nil`/空列表是否等价须逐字段规定，完整名称的顺序不能擅自改变。
- shape 和 ref 同事务写入；失败不发布“已保存”，重复 clean flush 直接跳过。
- 内存模式与数据库模式使用同一不可变描述池和字符串池；新增无序索引用 `utils.SafeMap`，有明确顺序要求才用 `omap`。
- 所有数据库 type lookup、直接 SQL、缓存失效、程序删除/重编译、导入导出和旧数据库加载，都必须理解 ref。

该设计减少重复类型表示的体积和编码次数，但身份 ref 仍有成本，报告中应分别列 shape 行数、ref 行数、总写入量和空间，不能把 refs 隐去后宣称全部对象都消失。

## Range：共享位置描述，关联可以打包

一种候选模型是独立的 range 表和窄关联表：

```text
source_range(range_id, program_id, file_hash, start_offset, end_offset)
offset_relation(range_id, variable_id, value_id, ...)
```

对基线可共享 74,403 个 range 描述，同时表示 227,473 条不同关联。保留关联信息不要求每个关联都占一个 SQL 行：也可以在一个 range 行里保存 `value_ids` 和变量绑定列表。这两个候选方案的具体取舍见下方补充。

内存 canonical range 拥有自己的 Position，不引用 parser 可修改的 scratch 对象。同内容的不同 editor 在编译期间保持独立；数据库 range 的内容寻址是否允许共享，应结合 program、source hash 和文件路径语义规定。

现有数据库唯一约束是 `(program, value, file, start, end, COALESCE(variable_name, ''))`，已经包含 variable name；旧内存写入去重键遗漏了 name，会跳过同值同位置的不同变量别名。当前优化将内存键与已有约束对齐，无需迁移。后续 range/relation 拆分仍须保留这些别名关系。

## 2026-10-09 补充：Offset 与 Type 的兼容方案和存储升级方案

本节是方案记录，不改变当前分支的数据库模型或 SQL 索引。核验时间：`2026-10-09T12:01:29+08:00`，`Fri Oct 9 12:01:29 CST 2026`。数据来自只读打开的 `/mnt/data/scan-outputs/ssa-memory-reuse-20261008-1550/goblog-final/ssadb.db`，源码为本 worktree 的未提交优化版本。以下行数推算来自现有库，空间和查询速度收益尚未通过新存储格式的实测确认。

### 兼容边界

1. **表、列、格式和现有行为均不变**：可以共享内存描述、编码结果，减少重复写入；旧 SQL 和旧程序继续按原模型读写。
2. **新程序可以读旧库**：增加存储版本及旧格式读取分支后，可以采用新的表、列或编码。这是存储升级，不能称为完全不改 schema。
3. **旧程序也可以直接读写新库**：除了旧库读取分支，还要适配旧 SQL、ORM 迁移、写入及删除行为。兼容视图只能作为候选，不能未经验证就承诺旧写入也兼容。长期同时写两套完整旧/新数据会抵消空间和写入收益，不作为默认方案。

### Offset：当前数据究竟是什么

当前 `ir_offsets` 的一行同时保存位置描述和一次绑定：`program_name, file_hash, start_offset, end_offset, variable_name, value_id`。其 `id` 是绑定行的 ID，不是共享 range ID。

`ir_codes` 另外保存指令自身的 `source_code_hash/start/end`。`ir_indices_v1` 保存名称、作用域、版本、owner 等检索关系，通过 `value_id` 找 Code，没有 `offset_id`。Offset 的两个已确认读取路径是：

- `program + variable_name + value_id` 查多个位置，供 `GetVariableFromDB` 恢复定义和使用 range。
- `file_hash + end_offset <= cursor_end`，按 end 倒序取一个 value，供数据库语言服务使用。

| 优化版 GoBlog 统计 | 数量 |
| --- | ---: |
| Offset 绑定行 | 221,848 |
| 不同位置描述 `(program,file,start,end)` | 62,043 |
| 不同 `(program,value,file,start,end)` | 216,586 |
| 不同 `(program,value,variable_name)` | 142,590 |
| 有 Offset 的不同 value | 130,867 |
| 对应多个不同位置的 value | 13,876，最多 370 个位置 |
| 对应多个不同 value 的位置 | 9,711，最多 703 个 value |
| 完整绑定重复 | 0 |
| Offset 表体积 | 38.77 MiB |
| Offset 的 8 个 SQL 索引体积合计 | 62.13 MiB |

`221,848 - 216,586 = 5,262` 是进一步保留不同变量名后的绑定差异。例如，实际同一文件的 `[5590,5619)` 有 8 条绑定、7 个不同 value：其中一个 value 同时绑定 `#200169.id` 和 `#200174.id`，其他绑定是各自的 offset/limit/target 成员。只保留去重后的 `value_ids` 会丢掉变量名与 value 的对应关系。

字段不重复也不证明每条绑定都应永久存在：此库有 1,487 条 Offset 查不到同程序 Code，基线也有该类记录；另有 7 条无变量名绑定同时被具名绑定覆盖。这些是生命周期/冗余审查候选，不能把 159,805 个重复位置描述直接当成可删除的绑定。

### Offset 方案 A：一个 range 行保存 value_ids 和绑定列表

```text
offset_ranges(
  range_id, program, file_hash, start, end,
  value_ids, bindings, preferred_value_id
)

value_ids = [V1, V2, ...]
bindings = [(variable_name_id, value_index), ...]
```

`value_ids` 内部只保留不同 value；bindings 保留变量名与 value 的对应关系，包括空名称。编码可以先用可检查的格式，后续对 ID 列表评估排序、差值及紧凑二进制表示。格式必须自带版本，不能把多个 ID 塞进现有标量 `value_id` 假装兼容。

位置到 Code 的读取可以走 `位置 -> range 行 -> value_ids -> 按 program/code_id 批量载入 Code`，不必再给 Code 加 offset_id 才能查询。本次实际查询计划表明现有 `program_name + code_id IN (...)` 使用 `ux_ir_codes_program_code`。这里只确认可以复用现有检索路径，是否比其他候选快仍需新格式实测。

**收益推算**：Offset 主表从 221,848 行降到约 62,043 行，减少约 72.03%。221,848 个逻辑绑定仍存在于列表中，SQL 行头、时间字段、program/file/range 描述及其索引项不再按每个绑定重复。

**反向查询有三种配套办法**：

1. 按 program/file 分批加载列表，在内存构建 `(value,variable) -> range_ids`，使用 `utils.SafeMap`。避免另存一张完整关联表，但要评估加载时间、常驻内存和多 program 生命周期。
2. 保存打包的反向表 `value_locations(program,value,variable_name_id,range_ids)`，键维持现有标量等值查询。按当前数据，约 142,590 条反向行，加上 range 主表约 204,633 行。这个总数不能用“主表减少 72%”代替；反向表不再重复文件和位置描述，但逻辑 ID 关联会存两个方向。
3. 列表字段直接按成员检索，仅作为低频路径候选。旧 `WHERE value_id=? AND variable_name=?` 无法直接读取新列表；不能在没有新模型查询计划和实测的情况下声称读取仍然同速。

写入须按 range 收集变化并批量提交，clean range 快速跳过。不能每新增一个 value 都读回、改写整个列表，否则热点 range 的多次修改可能抵消节省。跨编译单元新增绑定、并发合并、失败重试、成功后释放内存都需要测试。保存正向和反向表示时必须保证事务一致。

现有 `GetValueBeforeEndOffset` 返回单个 value。新的全量 API 可以返回所有 value；兼容的单值 API 要保留优先值和相同 end offset 的选择规则，不能简单改成“排序后最小的 ID”。

### Offset 方案 B：range 表 + 窄关联表

```text
source_range(range_id, program, file_hash, start, end)
offset_relation(range_id, value_id, variable_name_id)
```

按当前库，主表约 62,043 行，关联表约 221,848 行，总计约 283,891 行。总行数增加，但关联行可以只带几个整数，并按真实查询方向建立索引，不重复宽位置字段和每条绑定的完整 ORM 元数据。

优点是标量查询、按 value/变量反查、单条增删和唯一绑定判定更直接；代价是仍有 22 万条窄关联及连接读取。应与方案 A 同时测量写入量、数据库体积、加载内存和查询耗时，不能只按 SQL 行数决定。

### Offset 方案 C：Code 引用 offset_id

```text
code(..., primary_range_id)
```

这适合共享 Code 的主位置。查询 `program + primary_range_id` 的 Code 时，可以设计对应复合索引，并只取 Code ID；需要实际查询计划和耗时确认速度。

仅这个字段无法表示同一 value 的多次变量使用位置。要覆盖全部位置，需要额外的 occurrence/绑定，或者在 Code 中保存带变量信息的多个 range ID。此时“给 Code 一个 offset_id”只是主位置优化，完整模型仍需要方案 A 的列表或方案 B 的关联。

### Offset：完全兼容现有格式时能做什么

- 在内存把相同位置描述转为内部 range ID，缩小 `offsetSaved` 的绑定键；数据库写出时展开原字段。新增 range 池本身也占内存，要测净收益。
- 继续保留完整绑定的去重和跨批次状态，避免重复 enqueue、重复 flush；不同变量名不合并。
- 对查询索引做单独评估。本次只读 `EXPLAIN QUERY PLAN`：变量反查用了 `ux_ir_offsets_program_value_file_range` 的 program/value 前缀；光标查询用了 file_hash 单列索引并建立临时排序 B-tree。`(file_hash,end_offset)` 是可以测量的候选，是否加入 program 要与查询的程序隔离语义一致。加索引会增加写入和空间，必须结合可被替代的已有索引验证。
- 关联生命周期问题确认后减少无效记录的生成；不按共有位置自动删除。

以上保持现有宽行格式，主要减少内存、写入和查询工作。要让一行直接带多个 value，必须改变当前标量读取约定或底层存储模型。

### Type：为什么现在还有很多相同表示

`NewBasicType(kind,name)` 当前共享不可变 description，但每次返回新的 BasicType。它仍各自持有 ID、fullTypeName 和可修改的方法状态；`typeStore.remember` 按对象身份分配/保留 ID，`ir_codes.type_id` 引用这个 ID。因此大量普通 string/number 对象会形成不同 Type ID，序列化字段却相同。

| 优化版 GoBlog 统计 | 数量 |
| --- | ---: |
| Type 身份行 | 158,427 |
| 保存字段 `(program,kind,string,extra_information)` 的不同表示 | 8,754 |
| 完全相同的普通 string 表示 | 56,620 行 |
| 完全相同的普通 number 表示 | 33,808 行 |
| 没有被 Code.type_id 直接引用的 Type | 22,802 行，尚未做完整可达性判断 |
| Type 表体积 | 31.50 MiB |
| Type SQL 索引体积合计 | 12.04 MiB |

现有序列化保存基础 kind/name/fullTypeName 以及部分类型额外字段，并未将全部内存方法和结构状态直接存入这些字段。8,754 是现有保存表示的统计，不是证明完整语义只有 8,754 种类型。

### Type 方案 A：保持身份和旧格式，共享编码及不可变状态

继续复用 description、类型名和完整名称的字符串，按完整快照复用 JSON/编码结果；相同 payload 对象可共享，IrType 行及可修改 Type 身份保持独立。只为变化的类型写入，释放完成保存阶段持有的编码缓冲和可回载状态。

这降低编码、临时分配及缓存中的字符串成本，不能把 158,427 个持久化身份行直接变成 8,754 行。不能为了命中池而遗漏完整名称、nil/空列表、顺序或类型特有字段。

另一条保持旧 schema 的实际减行方向是：从 Code、类型间引用、导出类型和必要的程序元数据建立完整根集合，识别构造后被替换、且编译完成后不可达的临时 Type，减少它们的登记和保存。当前 22,802 条没有 Code 直接引用只是候选上界的一部分信息；父类型、接口、容器及其他类型关系仍可能引用它们，不能直接按 NOT EXISTS(Code.type_id) 删除。要先补完整引用保留、跨批次与回载回归，再决定能省掉多少行。

### Type 方案 B：共享保存 payload，保留 type_id 关联

```text
type_payload(payload_id, kind, string, extra_information)
type_ref(program, type_id, payload_id)
code(..., type_id)
```

这是保留现有可修改身份的存储升级。第一阶段对现有完整保存字段做无损共享，不以“相同 String”合并内存对象；后续再完善结构、方法等类型表示。相同表示只存一份，某个身份改变时改为引用新 payload，其他身份不变化。

按当前表示估计 payload 约 8,754 行，ref 仍约 158,427 行，总行数并非下降 94.47%。真正压缩的是重复表示以及宽字段索引。也可以探索打包 `type_id -> payload_id` 字典，但按 ID 查询、更新和加载内存的取舍需要另测。

### Type 方案 C：冻结类型核心，直接复用 Type 对象和 ID

```text
interned_type(type_id, 完整不可变类型结构)
code(..., type_id, 局部类型注解/别名引用)
```

先对普通 string/number/boolean/null 等无局部注解的基本类型建立 program 内的 canonical Type；随后考虑递归结构、函数和复合类型。包限定名称、名义类型身份、局部方法/receiver 等属于 canonical key 或独立的 alias/ref，不能因显示名相同而丢掉差异。

这能真正减少 Type 对象数及数据库 Type 行。前提是类型核心不可修改，所有变更都由拥有该类型的 Code/ref 显式替换引用。当前调用者可以直接 `value.GetType().AddFullTypeName(...)`，而 BasicType.Kind 等字段可以直接修改；仅在共享 Type 对象内部复制 payload，并不能知道应该把哪个 Code 的引用分离出去。因此该方案需要调整可修改 API/字段、类型所有权和缓存，不是简单在 NewBasicType 外面加一个 map。

保留同一 ir_types 表并重新映射重复 type_id 也必须检查上述语义、所有类型 ID 引用及旧 reader 的共享缓存行为。schema 不变不等于行为兼容，不能归为无条件“不 break”。

### 推荐推进顺序与待验证项

当前分支采用完全兼容的内存/编码/减少写入优化。后续存储分支优先比较 **Offset range + 打包绑定列表 + 反向查询表示** 和 **range + 窄关联表**；Code.primary_range_id 作为主位置共享补充。Type 先做 payload/ref 的无损共享，再评估冻结基本类型核心并复用 Type ID。

验收要分别报告主表行数、辅助/反向行数、逻辑绑定数、表及索引体积、写入次数、反向查询、光标查询、载入时间和 peak/live heap。迁移旧库后必须逐一比较变量位置集合、别名、多 value 同 range、多个 use range、优先 value、类型修改隔离和 ID 引用；还需验证并发、事务失败及增量更新。

本轮访问 SQLite 官方 JSON 和表达式索引资料时，shell 请求因 DNS 失败，浏览工具因所需审批与 `approval policy=never` 被拒绝。因此此处性能取舍依据本地读取路径、只读 SQL 与查询计划，数组编码和新格式索引性能保留为待实测项，不声称某种 JSON/二进制格式具备未经核验的查询能力。

## Index 与 SQL 索引

完全相同的 Index 关系可在队列前去重。后续数据库约束需要包含全部 nullable 字段并定义 NULL 的等价规则，不能只使用 value/name，或依赖普通 nullable UNIQUE 自动消除所有重复。

Offset 的 8 个索引暂不删除。后续先收集实际查询：

- program + variable + value 的位置查询；
- file + end offset 的前一个值查询；
- file + range 覆盖/相交查询；
- program 删除和重编译；
- 新 range/relation 连接查询。

保存真实参数的 `EXPLAIN QUERY PLAN`、查询耗时、命中数量和 SQLite `dbstat` 体积，再确定复合索引及可被覆盖的单列索引。已采样的 `file_hash + end_offset <= ? ORDER BY end_offset DESC` 查询只使用 file_hash 单列索引，并创建临时排序 B-tree；这是后续评估复合索引的依据，尚未修改索引。既测试读查询，也测试写放大。没有实际查询计划，不按体积直接删索引。

## 迁移与验收

独立分支分阶段实施：payload/string 共享 → type_shape/ref → source_range/relation → occurrence 与 code ID 共享。每阶段都保留旧库读取和明确的 schema 版本，不把迁移混入当前编译性能分支。

迁移应可中断、可重复执行，先在副本完成并验证，失败保留原库。程序删除需覆盖所有新表，跨 program 共享的 shape 必须有清晰的生命周期和引用策略。

测试先行：

1. 同值不同源码位置、不同函数、不同 program；同文本不同 editor。
2. 一个类型增加方法/完整名、一个 constant 改 type/extern/origin，其他引用不变化。
3. Index 的 NULL/零、scope/version/owner 差异保留，重复 enqueue 与重复 flush 只保存一次。
4. variable 别名同值同 range、多值同 range，IDE 和 SyntaxFlow 仍返回完整证据位置。
5. 旧数据库 round trip、递归类型、删除/重编译、失败重试、事务回滚、并发 race。
6. 同一 GoBlog 输入完成编译后检查 `quick_check`、所有 ID 引用、函数/调用/返回/成员关系、diagnostics、唯一源码行数，再比较 pprof 和数据库空间。

基准保持源码、GC 设置、运行模式一致，并记录并发负载和 I/O 等待。完成耗时、CPU 工作量、peak RSS、live heap、累计分配和数据库体积分别报告，不能用中间 heap 快照代替最终结果。
