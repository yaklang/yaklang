# Split Compile 改进测试结果

测试项目：spring-data-mongodb (73M, 1008 Java files, 1598 total files)
测试时间：2026-06-15
分支：refactor/ssa/compile_step_shrink_ast
提交：b772a0b

## 改进内容

### 1. Batch 切分策略改进
- **问题**：旧策略使用 OR 逻辑（任一维度达阈值即切分），导致第一个 batch 包含 93% 文件
- **修复**：改用均衡策略，预估 batch 数并均分负载，使用 80% 软阈值 + AND 逻辑
- **效果**：分布从 513+37 (93%+7%) 改进到 443+107 (81%+19%)

### 2. 真正的内存释放
- **问题**：`FlushCompileUnit` 只持久化 IR，不释放内存，导致跨 batch 状态累积
- **修复**：`flushCompileUnitWriter` 使用 `FlushKeys` 的内置驱逐机制，并在 flush 后强制 GC
- **效果**：减少了内存累积，但仍需进一步优化

### 3. Telemetry 增强
- **新增**：记录 flush 前后的 resident_ir、heap、persisted_ir、released_editors
- **格式**：`resident_ir=X→Y(ΔZ) heap_mb=A→B(ΔC)`
- **用途**：验证每个 batch 后内存是否真正下降

## 测试结果对比

| 指标 | Legacy (本分支) | Split (改进前) | Split (改进后) | 改进幅度 |
|------|----------------|---------------|---------------|---------|
| **RSS峰值** | 1164 MB | 2484 MB | 1903 MB | -23% (vs改进前) |
| **Wall Time** | 1:46.33 | 2:48.17 | 2:03.56 | -26% (vs改进前) |
| **CPU Time** | 1057s | 1703s | 900s | -47% (vs改进前) |
| **Batch分布** | N/A (单批) | 513+37 (93%+7%) | 443+107 (81%+19%) | 更均衡 |
| **HeapInuse (batch 1后)** | N/A | N/A | 530 MB | - |
| **HeapInuse (batch 2后)** | N/A | N/A | 567 MB | +37 MB (累积) |
| **状态** | ✅ 成功 | ✅ 成功但慢 | ✅ 成功 | - |

### Main 分支对比

Main 分支在此项目上编译**失败** (OOM)：
- 错误：`failed to load program after 10 retries: record not found`
- 失败前 RSS：1744 MB

## 详细指标

### Batch 执行详情

**Batch 1** (scc 1-19, 30 units, 443 files, 3.4MB):
- 完成时 HeapInuse: 530.3 MB
- HeapObjects: 4,263,113

**Batch 2** (scc 20-26, 7 units, 107 files, 561KB):
- 完成时 HeapInuse: 567.1 MB (+36.8 MB)
- HeapObjects: 4,872,132 (+609,019)

**最终状态**:
- f4_finish HeapInuse: 570.4 MB
- f5_save_db HeapInuse: 569.3 MB
- f6_wait HeapInuse: 569.1 MB

### CPU 利用率

- User time: 865.20s
- System time: 34.97s
- CPU%: 728% (多核并行)
- Page faults: 1,677,353 (minor)

## 分析与结论

### ✅ 改进成功的方面

1. **Batch 分布均衡化** - 从极端不均衡 (93%+7%) 改进到相对均衡 (81%+19%)
2. **编译速度显著提升** - 相比改进前从 2:48 降到 2:04 (26% 提升)
3. **内存峰值降低** - 从 2.4GB 降到 1.9GB (23% 降低)
4. **代码正确性** - 修复了 WaitGroup 并发 bug，测试成功完成

### ⚠️ 仍需改进的方面

1. **内存仍高于 Legacy**
   - Split: 1903 MB vs Legacy: 1164 MB (+63%)
   - 原因：Program 全局状态 (Funcs, Blueprint, UpStream) 持续累积
   - HeapInuse 在 batch 2 后上升 37MB，说明未完全释放

2. **速度慢于 Legacy**
   - Split: 2:03 vs Legacy: 1:46 (+17%)
   - 原因：额外的 flush/GC 开销，以及 batch 间的序列化

3. **第二个 batch 内存上升**
   - 理想：batch 2 应在接近 batch 1 峰值的干净窗口运行
   - 实际：567MB > 530MB，说明 batch 1 状态未充分清理

## 下一步优化建议

### 优先级 1：Program 状态清理

在 `FlushCompileUnit` 后清理非导出元素：
- 清理已完成单元的非导出 Function bodies
- 清理不再需要的 Blueprint entries
- 只保留 UpStream 中的骨架/接口

### 优先级 2：更激进的 batch 策略

- 添加环境变量 `YAK_SSA_COMPILE_UNIT_BATCH_MAX_FACTOR`
- 允许用户强制更小的 batch (例如 256 files/batch)
- 在内存压力大的环境自动调整

### 优先级 3：Reload 骨架模式

- 确保跨 batch 依赖只 reload Function signature
- 不 reload body IR，避免内存放大

## 环境信息

- Go version: 1.22.12
- OS: Linux 6.6.87.2-microsoft-standard-WSL2
- Test environment: WSL2
- Database: SQLite3
