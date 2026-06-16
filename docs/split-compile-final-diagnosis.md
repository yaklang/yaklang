# Split Compile Memory Fix - 最终诊断

## 测试结果总结

### 配置演进

| 版本 | MinFiles | MinBytes | Batches | 策略 | javacms/core 结果 |
|------|----------|----------|---------|------|-------------------|
| 原始 | 512 | 4 MB | 6 | 无 | OOM at 12.6 GB |
| v1 | 128 | 1 MB | 15 | Triple GC | OOM at 11.7 GB (batch 11: 750 MB) |
| v2 | 64 | 512 KB | 24 | Triple GC | OOM at 12 GB (batch 18: 763 MB) |
| v3 | 64 | 512 KB | 24 | Aggressive (清理 caches) | OOM at ~12 GB (batch 13: 489 MB) |
| **v4 (最终)** | **64** | **512 KB** | **24** | **Nuclear (清理 Blueprint/UpStream)** | **✅ 完成，10.7 GB** |

### v4 详细内存趋势

```
Batch 1-6:   234-272 MB  (✓ 前期非常稳定)
Batch 7-8:   307-311 MB  (✓ 小幅增长)
Batch 9-16:  386-527 MB  (⚠ 线性增长)
Batch 17-18: 732-771 MB  (✗ 大跳跃)
最终 RSS:    10.7 GB
```

### 清理效果演进

**前 8 batches (有效期)**:
- freed 190-345 MB per batch
- 内存保持在 234-311 MB

**后 10+ batches (失效期)**:
- freed 10-33 MB per batch
- 内存从 386 MB 累积到 771 MB

## 根本问题

### 1. 内存累积的真正原因

虽然我们清理了：
- ✅ cacheExternInstance
- ✅ externType
- ✅ ExternInstance
- ✅ OffsetMap
- ✅ Blueprint (除了 GlobalVariables)
- ✅ UpStream/DownStream

**但仍然累积，说明还有其他大对象未清理**：
- ❌ **instructionStore.writer cache** - 这才是最大的内存占用
- ❌ **Scope/SymbolTable** - 持有所有变量引用
- ❌ **Function.Blocks** - 虽然 Program.Funcs 是空的，但对象仍在其他地方

### 2. 为什么前期有效，后期失效

**前期 (batch 1-8)**:
- 每个 batch 较小
- 清理能够跟上累积速度
- freed 190-345 MB > 增长量

**后期 (batch 9+)**:
- 累积的对象越来越多
- GC 无法回收（仍被引用）
- freed 10-33 MB < 增长量

### 3. Batch 17-18 的大跳跃

这两个 batch 包含特别大的文件或复杂的类型定义，导致：
- 创建了大量 Instruction 对象
- 这些对象被 instructionStore.writer cache 持有
- 即使 FlushKeys 后，仍有引用未释放

## 架构问题

### Writer Cache 模式的根本缺陷

在 split compile 的 writer cache 模式下：

```
编译 Unit A → 创建对象 → writer cache → FlushKeys
                ↓
            对象被 Scope 引用
                ↓
            GC 无法回收
                ↓
编译 Unit B → 创建对象 → writer cache → FlushKeys
                ↓
            对象 A + 对象 B 都在内存
                ↓
            持续累积...
```

**关键发现**：
- FlushKeys 只是驱逐 cache 条目
- 对象本身仍被 Scope/SymbolTable 持有
- **清理 Blueprint/UpStream 也不够**

## 唯一有效的解决方案

### 方案 A: 清理 Scope/SymbolTable (需要深入改动)

```go
func (prog *Program) ClearCompletedUnitScopes(unitKeys []string) {
    // 1. 遍历所有 Scope
    // 2. 清理已完成单元的 symbol table
    // 3. 打破 Value/Instruction 引用链
}
```

**风险**: 可能破坏跨单元引用

### 方案 B: 不使用 Writer Cache (回退到 Memory 模式)

```go
// 强制使用纯内存模式，不使用 writer cache
ProgramCacheKind = ProgramCacheMemory
```

**缺点**: 失去 split compile 的初衷

### 方案 C: 限制最大内存 + 重启策略

```go
if heapMB > 8192 {
    // 保存当前进度
    // 退出进程
    // 外部脚本重启并继续
}
```

**缺点**: 复杂，需要外部协调

## 实际效果评估

### 相比原始版本

| 项目 | 原始 (512/4MB) | 最终 (64/512KB + Nuclear) | 改善 |
|------|---------------|--------------------------|------|
| javacms/core | OOM at 12.6 GB | ✅ 完成，10.7 GB | **15% 改善** |
| spring-data-mongodb | 1903 MB | 560 MB | **71% 改善** |

### 是否达到目标

**预期目标**: javacms/core 在 <2 GB 内完成
**实际结果**: 10.7 GB

**❌ 未达到预期目标**

但是：
- ✅ 成功完成编译（vs 之前 OOM）
- ✅ 小项目改善 71%
- ✅ 大项目也能完成（虽然内存高）

## 结论

### 技术层面

1. **降低 batch 大小** - 有限效果，只是延缓
2. **Triple GC** - 有限效果，10-30 MB
3. **清理 caches** - 有限效果，前期有效后期失效
4. **清理 Blueprint/UpStream** - 有限效果，前期 freed 300+ MB，后期 <50 MB

**根本问题未解决**: Scope/SymbolTable 持有的引用链

### 实用价值

虽然未达到理想目标（<2 GB），但：
- ✅ 提供了可工作的解决方案
- ✅ 在有 16 GB+ 内存的机器上可以完成大项目编译
- ✅ 小项目有显著改善（71%）
- ✅ 完全理解了问题的本质

### 最终建议

**短期**:
- 接受当前方案（10.7 GB）
- 文档化内存需求：大项目需要 16 GB+ RAM
- 提供内存不足时的清晰错误提示

**中期**:
- 实现 Scope.Clear() 清理符号表
- 预期可降至 5-6 GB

**长期**:
- 重构 writer cache 机制
- 实现真正的流式编译
- 预期可降至 <2 GB

## 文件清单

```
common/yak/ssaapi/ssa_compile_fs.go          - 降低 batch 阈值到 64/512KB
common/yak/ssa/database_cache.go             - 调用 AggressiveClearMemory
common/yak/ssa/program_unit_cleanup.go       - Nuclear 清理策略
docs/split-compile-*.md                       - 完整分析文档
```

## 提交历史

```
e89e2db - 降低到 128 files / 1MB + Triple GC
7e14e8f - 进一步降低到 64 files / 512KB
c10846d - 实现 Nuclear 清理 (Blueprint/UpStream)
```
