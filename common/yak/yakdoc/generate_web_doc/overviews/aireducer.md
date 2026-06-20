`aireducer` 库用于把大文本/大文件按块（chunk）切分，并对每一块逐步执行回调——这正是 "reduce" 的含义。它常用于配合 AI：把超长内容分段喂给模型处理（如逐段摘要、逐段分析），避免一次性超出上下文长度限制。

典型使用场景：

- 直接处理数据源：`aireducer.File` / `aireducer.Reader` / `aireducer.String` 传入文件名、`io.Reader` 或字符串，并给一个 `func(chunk)` 回调，库会自动分块并逐块回调。
- 构建可复用 Reducer：`aireducer.NewReducerFromFile` / `NewReducerFromReader` / `NewReducerFromString` 返回 `*Reducer` 句柄按需驱动。
- 切分策略：`aireducer.chunkSize` 按字节大小、`aireducer.lines` 按行数、`aireducer.separator` / `aireducer.separatorAsBoundary` 按分隔符切块；`aireducer.timeTriggerInterval` 支持按时间触发；`aireducer.memory` 接入上下文记忆。

与相邻库的关系：`aireducer` 是"分块器"，处理后的每块通常交给 `ai` 做模型调用、或交给 `rag` 做入库与检索，是处理超长素材的 AI 预处理工具。
