# Split Compile 内存泄漏修复 - 当前状态与后续步骤

## 当前状态

### 已完成的工作 ✅

1. **完整的根因分析**
   - 识别了三层内存泄漏结构：Program.Funcs → Function.Blocks → Instructions
   - 发现 javacms/core 从 270MB 暴涨到 9.1GB 的根本原因
   - 文档：`docs/split-compile-memory-leak-root-cause.md`

2. **核心修复机制实现**
   - `ReleaseCompletedUnitMemory()` - 清空已完成单元的 Function.Blocks
   - `CheckMemoryPressure()` - 2GB/4GB 阈值监控
   - `ForceCleanupNonExportedFunctions()` - 紧急回退

3. **Bug 修复**
   - ✅ WaitGroup 并发 bug（FlushKeys 后不能立即 Delete）
   - ✅ GetCompileUnit() 逻辑（从函数 key 提取包名）
   - ✅ unitKey split（逗号分隔的字符串需要拆分）

4. **可观测性增强**
   - Telemetry 显示 funcs/blueprints delta
   - 详细的 INFO/DEBUG/WARN 日志
   - 统计：checked/skipped_public/skipped_nomatch

### 未解决的问题 ⚠️

**主要问题：函数释放机制未生效**

症状：
- 没有看到 "[split-compile] ReleaseCompletedUnitMemory called" 日志
- 没有看到 "Released X function bodies" 日志
- 内存仍在累积（spring-data-mongodb: 492→560 MB）

可能原因：
1. **测试使用了旧二进制**（最可能）
   - 编译时间和测试时间对不上
   - 需要确保测试用的是最新构建

2. **单元 key 匹配仍然失败**
   - extractUnitKeyFromFunctionKey() 的逻辑可能不正确
   - 需要打印实际的函数 key 和单元 key 对比

3. **FlushCompileUnit 没有被调用**
   - Writer cache 可能被禁用
   - 需要确认 writer_enabled=true

## 验证步骤

### 步骤 1: 确认代码生效（5分钟）

```bash
# 重新构建
go build -o ./yak ./common/yak/cmd/yak.go

# 快速测试 - 只编译少量文件
export YAK_SSA_HEAP_LOG=1
export YAK_SSA_COMPILE_UNIT_LOG=1
export YAK_SSA_COMPILE_UNIT_WRITER_CACHE=1
export YAKIT_HOME="$PWD/.db"

./yak ssa-compile \
  --target /home/wlz/Target/spring-project/spring-data-mongodb \
  --program verify-release \
  --language java \
  --re-compile \
  --log info 2>&1 | tee build/verify-release.log

# 检查日志
grep "ReleaseCompletedUnitMemory called" build/verify-release.log
grep "Released.*function" build/verify-release.log
```

**预期**：
- 应该看到 "ReleaseCompletedUnitMemory called: units=X keys=[...]"
- 如果看到这个日志但没有 "Released"，检查 WARN 日志的统计

### 步骤 2: 诊断匹配失败（如果步骤 1 看到 called 但没有 Released）

修改 `extractUnitKeyFromFunctionKey` 添加日志：

```go
func extractUnitKeyFromFunctionKey(funcKey string) string {
    // 现有逻辑...
    result := lang + ":" + packageName
    
    // 添加诊断
    if strings.Contains(funcKey, "springframework") {
        yaklog.Infof("[DEBUG] funcKey=%s → unitKey=%s", funcKey, result)
    }
    
    return result
}
```

重新编译测试，查看实际的 key 映射关系。

### 步骤 3: 如果仍然不工作 - 简化策略

替换为更激进的清理策略（不依赖单元匹配）：

```go
func (prog *Program) ReleaseCompletedUnitMemory(unitKeys []string) int {
    // 直接释放所有非公共函数，不检查单元
    releasedFuncs := 0
    
    app.Funcs.ForEach(func(funcKey string, fn *Function) bool {
        if fn == nil || fn.IsExtern() {
            return true
        }
        
        // 只保留大写字母开头的函数（公共 API）
        name := fn.GetName()
        if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
            return true
        }
        
        // 释放所有私有函数
        if len(fn.Blocks) > 0 {
            fn.Blocks = nil
            fn.EnterBlock = 0
            fn.ExitBlock = 0
            releasedFuncs++
        }
        return true
    })
    
    runtime.GC()
    yaklog.Infof("[split-compile] Aggressive release: %d functions", releasedFuncs)
    return releasedFuncs
}
```

这个版本不依赖单元识别，应该能立即看到效果。

## 提交历史

```
b772a0b - 初始改进（batch 切分 + telemetry）
d79c5d8 - 修复 WaitGroup 并发 bug
d65b64b - 根因分析文档
7a9d135 - 实现 Function body 清理机制
b6515a4 - 改进单元 key 提取逻辑
1b83b35 - 修复 unit key split + 增强诊断日志
```

## 长期优化建议

### 1. 真正的编译单元 tracking

在 Function 结构体中添加字段：

```go
type Function struct {
    // ... 现有字段
    CompileUnitKey string  // 在 BeginCompileUnit 时设置
}
```

在创建函数时记录：

```go
func (prog *Program) BeginCompileUnit(unitKey string) {
    prog.currentCompileUnit = unitKey
}

func NewFunction(prog *Program, ...) *Function {
    fn := &Function{
        CompileUnitKey: prog.currentCompileUnit,
        // ...
    }
    return fn
}
```

### 2. Blueprint 和 UpStream 清理

当前只清理了 Functions，但 Blueprint 和 UpStream 也在累积：

```go
func (prog *Program) ReleaseCompletedUnitBlueprints(unitKeys []string) int {
    // 类似 ReleaseCompletedUnitMemory 的逻辑
    // 清理非导出的 Blueprint
}

func (prog *Program) ReleaseUnusedUpStreams() int {
    // 检查 UpStream 中哪些库不再被引用
    // 从 map 中删除
}
```

### 3. 按需 reload 机制

实现从 DB 恢复 function bodies：

```go
func (fn *Function) LoadBodyFromDB() error {
    if len(fn.Blocks) > 0 {
        return nil  // Already loaded
    }
    
    // 从 DB reload blocks
    // ...
}
```

## 结论

我们已经完成了：
- ✅ 完整的根因分析
- ✅ 核心修复机制的实现
- ✅ 3 个关键 bug 的修复
- ✅ 完整的可观测性建设

**最后一步是验证修复生效**。根据诊断日志的设计，即使当前不工作，我们也有清晰的路径快速定位和解决问题。

建议：
1. 先运行验证步骤 1，确认代码是否被执行
2. 如果仍然不工作，使用简化策略（步骤 3）快速验证概念
3. 一旦看到内存下降，再优化单元匹配逻辑

预期效果（一旦生效）：
- spring-data-mongodb: <1.5GB（vs 1.9GB）
- javacms/core: <2GB 成功完成（vs 9.1GB OOM）
