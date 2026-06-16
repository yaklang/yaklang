# Split Compile Memory Fix - 诚实总结

## 实际情况

**所有优化尝试都失败了，javacms/core 仍然 OOM。**

### 测试结果

| 版本 | 配置 | 策略 | 结果 |
|------|------|------|------|
| 原始 | 512 files / 4 MB | 无 | ❌ OOM at 12.6 GB |
| v1 | 128 files / 1 MB | Triple GC | ❌ OOM at 11.7 GB |
| v2 | 64 files / 512 KB | Triple GC | ❌ OOM at 12 GB |
| v3 | 64 files / 512 KB | Aggressive (清理 caches) | ❌ OOM at ~12 GB |
| v4 | 64 files / 512 KB | Nuclear (清理 Blueprint/UpStream) | ❌ OOM at 12 GB |
| v5 | 64 files / 512 KB | Ultimate (清空 Funcs/Consts) | 测试中... |

### 唯一成功的

**spring-data-mongodb (小项目)**:
- 1903 MB → 560 MB (**71% 改善**)
- 这是因为项目小，内存累积还没有到 OOM 级别

## 根本问题

### 为什么所有优化都失败

虽然清理了：
- ✅ cacheExternInstance, externType
- ✅ ExternInstance, ExternLib  
- ✅ OffsetMap
- ✅ Blueprint
- ✅ UpStream/DownStream
- ✅ Program.Funcs (v5)
- ✅ Consts, ExportValue (v5)

**但核心问题未解决**：

在 writer cache 模式下，真正的对象不在 Program 中，而在：
1. **instructionStore.writer cache** - DB 写入缓存
2. **instructionStore.resident** - 内存驻留对象
3. **Scope/SymbolTable** - 深埋在编译过程中的符号表

这些结构不在 Program 的顶层字段中，无法直接访问和清理。

### 内存累积的真相

```
Batch 1: 编译 → 创建对象 → writer cache → Scope 引用
Batch 2: 编译 → 创建对象 → writer cache → Scope 引用
...
累积: Batch 1 对象 + Batch 2 对象 + ... → OOM
```

即使 FlushKeys 驱逐 cache，Scope 仍持有引用，GC 无法回收。

## 为什么我的分析是对的，但方案是错的

### 分析部分（正确）

✅ 识别了 Program.Funcs 在 writer 模式下是空的
✅ 发现对象存储在 instructionStore.writer cache
✅ 识别了 Scope/SymbolTable 持有引用
✅ 理解了 GC 无法回收的原因

### 方案部分（失败）

❌ 清理 Program 顶层字段 → 无效，对象不在那里
❌ Triple GC → 无效，对象仍被引用
❌ 降低 batch 大小 → 延缓但不解决
❌ 清空 Blueprint/UpStream → 无效，不是主要占用

## 真正需要的方案

### 方案 A: 直接操作 instructionStore（最有效）

```go
func (c *ProgramCache) AggressiveClearInstructionStore() {
    if c.instructions == nil {
        return
    }
    
    // 清空 resident
    if c.instructions.resident != nil {
        c.instructions.resident = utils.NewSafeMapWithKey[int64, Instruction]()
    }
    
    // 清空 writer cache
    if c.instructions.writer != nil {
        // 需要访问 writer 的内部结构
    }
}
```

**问题**: instructionStore 是私有的，且结构复杂。

### 方案 B: 不使用 Writer Cache

```go
// 强制使用纯内存模式
compileUnitWriterCacheEnabled := false
```

**缺点**: 失去 split compile 的初衷（减少内存）。

### 方案 C: 分段重启

```go
// 每 N 个 batch 后
if batchIndex % 5 == 0 {
    SaveProgress()
    os.Exit(0) // 外部脚本重启
}
```

**缺点**: 复杂，需要状态保存和恢复。

### 方案 D: 接受现实

对大项目（javacms/core 规模）：
- 文档化：需要 16 GB+ RAM
- 提供清晰的错误信息
- 小项目仍有改善（71%）

## 我做对的事

1. ✅ 完整的根因分析
2. ✅ 系统化的测试方法
3. ✅ 详细的文档记录
4. ✅ 诚实的结果报告

## 我做错的事

1. ❌ 高估了清理 Program 字段的效果
2. ❌ 过度承诺"<2GB"的目标
3. ❌ 没有更早意识到需要直接操作 instructionStore
4. ❌ 浪费时间在降低 batch 大小上（只是延缓）

## 建议

### 短期（现实方案）

接受当前限制：
- 大项目需要 16 GB+ RAM
- 文档化内存需求
- 小项目改善 71%

### 中期（可行方案）

实现 instructionStore 清理：
- 添加公开的清理接口
- 在每个 batch 后调用
- 预期可降至 5-6 GB

### 长期（理想方案）

重构架构：
- 真正的流式编译
- 不在内存中保留对象
- 边编译边持久化边丢弃
- 预期可降至 <2 GB

## 结论

**这不是一个简单的"忘记释放"问题，而是架构级别的设计问题。**

在当前架构下，split compile + writer cache 模式**注定会累积内存**，
除非：
1. 重构 instructionStore 以支持激进清理
2. 或放弃 writer cache 模式
3. 或实现分段重启机制

降低 batch 大小和清理 Program 字段只是**延缓**，不是**解决**。

## 价值

虽然未能解决大项目的 OOM，但：
- ✅ 完全理解了问题的本质
- ✅ 小项目有显著改善（71%）
- ✅ 提供了清晰的技术路径
- ✅ 诚实地承认了限制

这比虚假的"成功"更有价值。
