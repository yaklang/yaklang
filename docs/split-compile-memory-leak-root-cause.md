# Split Compile 内存泄漏根因分析

## 测试数据

**Javacms/core** (1.8GB, 7476 Java files, 1556 compile units, 6 batches):

```
Batch 1: 270.8 MB HeapInuse
Batch 2: 289.8 MB (+19 MB, +7%)
Batch 3: 493.6 MB (+204 MB, +70%)
Batch 4: 821.2 MB (+328 MB, +66%)
最终 RSS: 9.1 GB (!) - 系统内存和 swap 被占满
```

**预期 vs 实际**:
- 预期：每个 batch 后内存下降，保持在 ~500MB 窗口
- 实际：每个 batch 后内存暴涨，累积到 9GB+

## 根本原因

### 问题 1: DB Cache 和内存对象图分离

**当前机制**:
```go
// FlushCompileUnit 流程
1. flushCompileUnitWriter() 
   -> 收集非边界 IR (所有 body 指令)
   -> FlushKeys(ids) 标记为 pending
   -> 异步持久化到 DB
   -> FinishPersist(success=true) 从 writer.data 删除

2. ReleasePersistedEditors()
   -> 从 sourceStore 删除已持久化的 editor

3. runtime.GC()
   -> 尝试回收内存
```

**问题所在**:
```go
// Program 持有所有对象的强引用
type Program struct {
    Funcs     *omap.OrderedMap[string, *Function]  // ❌ 累积所有函数
    Blueprint *omap.OrderedMap[string, *Blueprint] // ❌ 累积所有类型
    UpStream  *omap.OrderedMap[string, *Program]   // ❌ 累积所有依赖
}

// Function 持有完整的 body
type Function struct {
    Blocks []*BasicBlock  // ❌ 所有基本块
    // ...
}

// BasicBlock 持有所有指令
type BasicBlock struct {
    Insts []Instruction  // ❌ 所有指令对象
}
```

**内存泄漏链**:
```
Program.Funcs["com.dotcms.Foo.bar"]
  └─> Function
       └─> Blocks []*BasicBlock
            └─> Insts []Instruction
                 └─> 每个指令持有 Value, User, 各种引用

即使 writer cache 驱逐了 IR 条目，
Program.Funcs 仍然持有完整的对象图，
GC 无法回收任何东西！
```

### 问题 2: shouldKeepCompileUnitBoundaryResident 的误解

**当前实现**:
```go
func shouldKeepCompileUnitBoundaryResident(inst Instruction) bool {
    switch inst.GetOpcode() {
    case SSAOpcodeFunction,      // 保留
         SSAOpcodeParameter,     // 保留
         SSAOpcodeFreeValue,     // 保留
         SSAOpcodeParameterMember, // 保留
         SSAOpcodeSideEffect,    // 保留
         SSAOpcodeExternLib:     // 保留
        return true
    default:
        return false  // Call, Assign, Return, If, Phi, etc. 全部 flush
    }
}
```

**设计意图 vs 实际效果**:
- **设计意图**: 只保留函数签名（Function + Parameter），body 指令可以从 DB reload
- **实际效果**: 
  - Writer cache 驱逐了 body 指令 ✓
  - 但 Program.Funcs 仍持有完整的 Function 对象 ✗
  - Function.Blocks 仍持有所有 BasicBlock ✗
  - BasicBlock.Insts 仍持有所有 Instruction 对象 ✗
  - 内存中的对象图完全没有释放！✗

### 问题 3: Batch 累积效应

**6 个 batches 的累积**:
```
Batch 1: 254 units → +270 MB
Batch 2: 349 units → +19 MB   (cumulative 289 MB)
Batch 3: 80 units  → +204 MB  (cumulative 494 MB)
Batch 4: 119 units → +328 MB  (cumulative 821 MB)
Batch 5: 630 units → 预计 +1GB+ (最大 batch, 5393 files)
Batch 6: 124 units → 预计 +500MB+

总计: 1556 units, 7476 files → 9.1 GB RSS
```

## 为什么小项目 Split 更慢？

**Spring-data-mongodb** (73MB, 1008 files, 37 units, 2 batches):
- Split: 1903 MB, 2:04
- Legacy: 1164 MB, 1:46

**原因**:
1. 项目太小，Legacy 可以全量装入内存而不 OOM
2. Split 的固定开销（依赖图、batch 切分、flush/GC）>= 收益
3. 内存泄漏问题在小项目上也存在，只是不明显
4. 2 个 batches 累积的内存 < Legacy 的峰值，但仍高于理论值

## 为什么大项目会 OOM？

**Javacms/core** (1.8GB, 7476 files, 1556 units, 6 batches):
- 6 个 batches 累积
- 每个 batch 平均 ~1.5GB 内存增长
- Program.Funcs 累积 1556 个 Function 对象
- 每个 Function 持有完整的 body (Blocks + Insts)
- 最终达到 9.1 GB，超过系统可用内存

## 解决方案

### 方案 1: 清空 Function bodies (推荐)

```go
func (prog *Program) ReleaseCompletedUnitFunctionBodies(unitKeys []string) {
    // 遍历 unitKeys 对应的所有 Function
    // 清空 Function.Blocks，只保留签名
    // 如果未来需要，从 DB reload
    
    for _, fn := range functionsInUnits(unitKeys) {
        if !fn.IsExported {
            fn.Blocks = nil  // 释放 body
            fn.EnterBlock = nil
            fn.ExitBlock = nil
        }
    }
}
```

**优点**:
- 不破坏 Program.Funcs 的结构
- 保留函数签名供跨单元引用
- Body 可以从 DB 按需 reload

**缺点**:
- 需要实现 reload 机制
- 可能影响后续的 deferred build

### 方案 2: 从 Program.Funcs 删除已完成单元的函数

```go
func (prog *Program) ReleaseCompletedUnitFunctions(unitKeys []string) {
    for _, key := range getFunctionKeysInUnits(unitKeys) {
        if !isExported(key) {
            prog.Funcs.Delete(key)
        }
    }
}
```

**优点**:
- 彻底释放内存
- GC 可以回收所有对象

**缺点**:
- 可能破坏跨单元引用
- Deferred build 可能找不到函数
- 需要谨慎判断哪些函数可以删除

### 方案 3: 使用弱引用 (长期方案)

```go
type Program struct {
    Funcs *omap.OrderedMap[string, *WeakRef[Function]]
}
```

**优点**:
- 允许 GC 自动回收
- 不需要显式清理逻辑

**缺点**:
- 需要重构大量代码
- Go 没有原生弱引用支持

## 推荐修复步骤

### 立即修复 (Critical)

1. **实现 Function body 清理**:
   ```go
   // 在 FlushCompileUnit 后调用
   prog.ReleaseCompletedUnitFunctionBodies(unitKeys)
   ```

2. **添加内存压力监控**:
   ```go
   if runtime.MemStats.HeapInuse > threshold {
       log.Warnf("[split-compile] memory pressure detected, forcing cleanup")
       prog.ForceCleanupNonExportedFunctions()
   }
   ```

3. **改进 telemetry**:
   ```go
   log.Infof("batch %d/%d: heap=%dMB funcs=%d released_funcs=%d",
       batchIndex, totalBatches,
       heapInuse/1024/1024,
       prog.Funcs.Len(),
       releasedCount)
   ```

### 中期优化

1. 实现按需 reload 机制
2. 优化 deferred build，不依赖已释放的函数
3. 添加环境变量控制清理策略

### 长期重构

1. 设计更好的内存管理抽象
2. 考虑分代式编译（类似 JVM）
3. 探索增量编译和缓存策略

## 参考数据

| 项目 | 规模 | Units | Batches | Legacy RSS | Split RSS | 内存泄漏 |
|------|------|-------|---------|-----------|-----------|---------|
| spring-data-mongodb | 73MB, 1008 files | 37 | 2 | 1164 MB | 1903 MB | +63% |
| javacms/core | 1.8GB, 7476 files | 1556 | 6 | N/A (OOM?) | 9100 MB | +极端 |

**结论**: 当前 split compile 实现没有真正释放内存，导致大项目编译失败。必须实现 Function body 清理才能达到设计目标。
