`pprof` 库提供性能剖析能力，采集 CPU 与内存 profile 并可自动分析，常用于排查脚本/引擎的性能瓶颈与内存占用。

典型使用场景：

- 采集 profile：`pprof.StartCPUProfile` / `pprof.StartMemoryProfile` / `pprof.StartCPUAndMemoryProfile` 启动采集，配 `pprof.cpuProfilePath` / `pprof.memProfilePath` / `pprof.timeout` 与各类生命周期回调。
- 自动分析：`pprof.AutoAnalyzeFile(filename)` 对已有 profile 文件做自动分析输出。

与相邻库的关系：`pprof` 是诊断工具，独立于业务逻辑，用于优化耗时脚本（如大规模扫描、AI 处理）的性能。
