# AI Memory System Usage Guide

## 概述

AI Memory System 是一个基于 C.O.R.E. P.A.C.T. 框架的记忆管理系统，支持智能记忆条目的创建、存储和检索。

## 快速开始

### 1. 创建 AIMemory 实例

```go
import (
    "context"
    "github.com/yaklang/yaklang/common/ai/aid/aimem"
    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// 创建 AI Memory 实例
mem, err := aimem.NewAIMemory("session-id",
    aimem.WithInvoker(yourAIInvoker),
    aimem.WithContextProvider(func() (string, error) {
        // 返回当前已有的标签作为上下文
        return "已有标签：AI开发、记忆系统", nil
    }),
)
if err != nil {
    panic(err)
}
defer mem.Close()
```

### 2. 添加原始文本并生成记忆条目

```go
// 从原始文本生成记忆条目
entities, err := mem.AddRawText("用户正在开发AI记忆系统，需要实现语义搜索功能")
if err != nil {
    panic(err)
}

// 查看生成的记忆条目
for _, entity := range entities {
    fmt.Printf("Content: %s\n", entity.Content)
    fmt.Printf("Tags: %v\n", entity.Tags)
    fmt.Printf("C.O.R.E. P.A.C.T. Scores: C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f\n",
        entity.C_Score, entity.O_Score, entity.R_Score, entity.E_Score,
        entity.P_Score, entity.A_Score, entity.T_Score)
}
```

### 3. 保存记忆条目

```go
// 保存到数据库并索引到 RAG 系统
err = mem.SaveMemoryEntities("session-id", entities...)
if err != nil {
    panic(err)
}
```

### 4. 语义搜索

```go
// 通过语义搜索查找相关记忆
results, err := mem.SearchBySemantics("session-id", "如何实现语义搜索？", 10)
if err != nil {
    panic(err)
}

for _, result := range results {
    fmt.Printf("Score: %.4f, Content: %s\n", result.Score, result.Entity.Content)
}
```

### 5. 按评分搜索

```go
// 搜索高相关性和高可操作性的记忆
filter := &aimem.ScoreFilter{
    R_Min: 0.7,  // 相关性 >= 0.7
    R_Max: 1.0,
    A_Min: 0.6,  // 可操作性 >= 0.6
    A_Max: 1.0,
}

results, err := mem.SearchByScores("session-id", filter, 10)
if err != nil {
    panic(err)
}
```

### 6. 按标签搜索

```go
// 搜索包含特定标签的记忆
results, err := mem.SearchByTags("session-id", []string{"AI开发", "记忆系统"}, false, 10)
if err != nil {
    panic(err)
}

// matchAll=true 时，必须包含所有标签
// matchAll=false 时，至少包含一个标签
```

### 7. 获取所有标签

```go
// 获取当前会话的所有标签
tags, err := mem.GetAllTags("session-id")
if err != nil {
    panic(err)
}

fmt.Printf("Available tags: %v\n", tags)
```

### 8. 获取动态上下文

```go
// 获取包含已有标签的动态上下文，用于生成新记忆时提示 AI 复用标签
context, err := mem.GetDynamicContextWithTags("session-id")
if err != nil {
    panic(err)
}

fmt.Println(context)
// 输出：
// 已存储的记忆领域标签（请优先使用这些标签）：
// 1. AI开发
// 2. 记忆系统
// 3. 搜索功能
```

## C.O.R.E. P.A.C.T. 框架说明

所有评分都归一化到 0.0-1.0 范围：

- **C - Connectivity (关联度)**: 这个记忆与其他记忆的关联程度
- **O - Origin (来源与确定性)**: 信息来源的可信度
- **R - Relevance (相关性)**: 对用户目标的关键程度
- **E - Emotion (情感)**: 用户表达时的情绪状态
- **P - Preference (个人偏好)**: 是否绑定用户的个人风格
- **A - Actionability (可操作性)**: 是否可以从中学习并改进
- **T - Temporality (时效性)**: 记忆应该保留多久

详细说明请参考 [README.md](README.md)

## 数据库 Schema

记忆条目存储在 `ai_memory_entities` 表中，包含以下字段：

- `memory_id`: 记忆条目唯一标识
- `session_id`: 会话ID
- `content`: 记忆内容
- `tags`: 领域标签（JSON数组）
- `potential_questions`: 潜在问题列表（JSON数组）
- `c_score`, `o_score`, `r_score`, `e_score`, `p_score`, `a_score`, `t_score`: C.O.R.E. P.A.C.T. 评分
- `core_pact_vector`: 评分向量（JSON数组）

## RAG 索引

系统会自动将 `potential_questions` 索引到 RAG 系统中，每个问题作为一个文档，关联到对应的 `memory_id`。这样可以通过语义搜索快速找到相关的记忆条目。

## 注意事项

1. 每个会话使用独立的 RAG collection：`ai-memory-{sessionId}`
2. 记忆条目的 ID 使用 UUID 自动生成
3. 语义搜索基于 potential_questions，而不是直接搜索 content
4. 标签搜索支持大小写不敏感的匹配
5. 所有搜索操作都会自动过滤 session_id，确保会话隔离

