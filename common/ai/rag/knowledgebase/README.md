# YakLang KnowledgeBase 知识库系统

## 概述

KnowledgeBase 是 YakLang 项目中的智能知识库管理系统，专为安全知识的存储、检索和管理而设计。它基于 RAG（Retrieval-Augmented Generation）技术，提供了高效的语义搜索能力，将传统的结构化数据存储与现代的向量检索技术完美结合。

## 系统架构

### 核心组件

```
┌─────────────────────────────────────┐
│           KnowledgeBase             │
├─────────────────────────────────────┤
│  ┌───────────────┐ ┌──────────────┐ │
│  │   Database    │ │   RAGSystem  │ │  
│  │   (SQLite)    │ │   (向量检索)  │ │
│  └───────────────┘ └──────────────┘ │
└─────────────────────────────────────┘
         │                    │
         ▼                    ▼
┌─────────────────┐  ┌─────────────────┐
│ KnowledgeBase   │  │ VectorStore     │
│ Info & Entry    │  │ Collection &    │
│ (结构化数据)      │  │ Document        │
│                 │  │ (向量化数据)      │
└─────────────────┘  └─────────────────┘
```

### 数据模型

1. **KnowledgeBaseInfo** - 知识库元信息
   - `knowledge_base_name`: 知识库名称（唯一）
   - `knowledge_base_description`: 知识库描述
   - `knowledge_base_type`: 知识库类型

2. **KnowledgeBaseEntry** - 知识条目
   - `knowledge_title`: 知识标题
   - `knowledge_type`: 知识类型（CoreConcept、Standard、Guideline等）
   - `importance_score`: 重要性评分（1-10）
   - `keywords`: 关键词列表
   - `knowledge_details`: 详细内容
   - `summary`: 摘要
   - `potential_questions`: 潜在问题列表

3. **VectorStoreCollection** - RAG向量集合
   - `name`: 集合名称
   - `description`: 集合描述
   - `dimension`: 向量维度

4. **VectorStoreDocument** - RAG向量文档
   - `document_id`: 文档ID（对应知识条目ID）
   - `content`: 文档内容
   - `embedding`: 嵌入向量
   - `metadata`: 元数据

## 主要功能

### 1. 知识库管理

#### 创建知识库
```go
// 创建或获取知识库（如果不存在则创建）
kb, err := NewKnowledgeBase(db, "security-kb", "安全知识库", "security")

// 创建全新知识库（如果已存在会返回错误）
kb, err := CreateKnowledgeBase(db, "new-kb", "新知识库", "general")

// 加载已存在的知识库
kb, err := LoadKnowledgeBase(db, "existing-kb")
```

### 2. 知识条目操作

#### 添加知识条目
```go
entry := &schema.KnowledgeBaseEntry{
    KnowledgeBaseID:    1,
    KnowledgeTitle:     "SQL注入攻击原理",
    KnowledgeType:      "Vulnerability",
    ImportanceScore:    9,
    Keywords:           []string{"SQL注入", "安全漏洞", "数据库"},
    KnowledgeDetails:   "SQL注入是一种代码注入技术...",
    Summary:            "SQL注入攻击的基本原理和防护方法",
    PotentialQuestions: []string{"什么是SQL注入?", "如何防护SQL注入?"},
}

err := kb.AddKnowledgeEntry(entry)
```

#### 更新和删除
```go
// 更新知识条目
err := kb.UpdateKnowledgeEntry(entry)

// 删除知识条目
err := kb.DeleteKnowledgeEntry(entryID)

// 获取知识条目
entry, err := kb.GetKnowledgeEntry(entryID)
```

### 3. 智能搜索

#### 语义搜索
```go
// 基础搜索，返回知识条目
results, err := kb.SearchKnowledgeEntries("SQL注入防护", 5)

// 带相似度分数的搜索
resultsWithScore, err := kb.SearchKnowledgeEntriesWithScore("缓冲区溢出", 10)
for _, result := range resultsWithScore {
    fmt.Printf("标题: %s, 相似度: %.4f\n", 
        result.Entry.KnowledgeTitle, result.Score)
}
```

#### 分页查询
```go
// 分页获取知识条目列表
entries, err := kb.ListKnowledgeEntries("关键词", 1, 20) // 第1页，每页20条
```

### 4. 同步管理

#### 检查同步状态
```go
status, err := kb.GetSyncStatus()
fmt.Printf("数据库条目: %d, RAG文档: %d, 同步状态: %v\n",
    status.DatabaseEntries, status.RAGDocuments, status.InSync)
```

#### 执行同步
```go
// 全量同步（以数据库为准）
syncResult, err := kb.SyncKnowledgeBaseWithRAG()

// 批量同步指定条目
entryIDs := []int64{1, 2, 3}
syncResult, err := kb.BatchSyncEntries(entryIDs)
```

## RAG 集成点

### 1. 双层存储架构

知识库采用双层存储架构：
- **结构化存储**：使用 SQLite 存储知识条目的完整信息
- **向量存储**：使用 RAG 系统存储知识条目的向量化表示

### 2. 自动向量化

当添加知识条目时，系统自动：
1. 将条目存储到数据库
2. 构建文档内容（标题 + 摘要 + 详细信息）
3. 生成嵌入向量
4. 存储到RAG向量数据库

```go
// 内部向量化过程
func (kb *KnowledgeBase) addEntryToVectorIndex(entry *schema.KnowledgeBaseEntry) error {
    // 构建文档内容
    content := entry.KnowledgeTitle
    if entry.Summary != "" {
        content += "\n\n" + entry.Summary
    }
    if entry.KnowledgeDetails != "" {
        content += "\n\n" + entry.KnowledgeDetails
    }

    // 构建元数据
    metadata := map[string]any{
        "knowledge_base_id":   entry.KnowledgeBaseID,
        "knowledge_title":     entry.KnowledgeTitle,
        "knowledge_type":      entry.KnowledgeType,
        "importance_score":    entry.ImportanceScore,
        // ...
    }

    // 添加到RAG系统
    return kb.ragSystem.Add(documentID, content, rag.WithDocumentRawMetadata(metadata))
}
```

### 3. 语义搜索流程

```
用户查询 → 生成查询向量 → RAG向量搜索 → 获取文档ID → 查询数据库条目 → 返回结构化结果
```

### 4. 数据一致性保证

- **事务保护**：数据库操作使用事务确保一致性
- **回滚机制**：向量索引失败时自动回滚数据库操作
- **同步检查**：提供同步状态检查和修复功能

## 使用示例

### 完整的知识库使用流程

```go
package main

import (
    "fmt"
    "github.com/yaklang/yaklang/common/ai/rag"
    "github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
    "github.com/yaklang/yaklang/common/schema"
)

func main() {
    // 1. 创建数据库连接
    db, err := rag.NewRagDatabase("./security_kb.db")
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // 2. 创建知识库
    kb, err := knowledgebase.NewKnowledgeBase(
        db,
        "security-knowledge",
        "网络安全知识库",
        "security",
        // 可选配置
        rag.WithEmbeddingModel("text-embedding-3-small"),
        rag.WithModelDimension(1536),
    )
    if err != nil {
        panic(err)
    }

    // 3. 添加知识条目
    entries := []*schema.KnowledgeBaseEntry{
        {
            KnowledgeBaseID:  1,
            KnowledgeTitle:   "XSS攻击原理与防护",
            KnowledgeType:    "Vulnerability",
            ImportanceScore:  8,
            Keywords:         []string{"XSS", "跨站脚本", "Web安全"},
            KnowledgeDetails: "跨站脚本攻击(XSS)是一种常见的Web安全漏洞...",
            Summary:          "XSS攻击原理及其防护措施",
            PotentialQuestions: []string{
                "什么是XSS攻击?",
                "如何防护XSS攻击?",
                "XSS有哪些类型?",
            },
        },
        {
            KnowledgeBaseID:  1,
            KnowledgeTitle:   "CSRF攻击与防护",
            KnowledgeType:    "Vulnerability",
            ImportanceScore:  7,
            Keywords:         []string{"CSRF", "跨站请求伪造", "Web安全"},
            KnowledgeDetails: "跨站请求伪造(CSRF)是一种挟制用户在当前已登录的Web应用程序上执行非本意的操作的攻击方法...",
            Summary:          "CSRF攻击原理和防护方法",
            PotentialQuestions: []string{
                "什么是CSRF攻击?",
                "CSRF如何防护?",
            },
        },
    }

    for _, entry := range entries {
        if err := kb.AddKnowledgeEntry(entry); err != nil {
            fmt.Printf("添加知识条目失败: %v\n", err)
            continue
        }
        fmt.Printf("成功添加知识条目: %s\n", entry.KnowledgeTitle)
    }

    // 4. 搜索知识
    fmt.Println("\n=== 搜索测试 ===")
    
    // 搜索Web安全相关内容
    results, err := kb.SearchKnowledgeEntries("Web安全漏洞防护", 5)
    if err != nil {
        fmt.Printf("搜索失败: %v\n", err)
        return
    }

    fmt.Printf("找到 %d 个相关结果:\n", len(results))
    for i, result := range results {
        fmt.Printf("%d. %s\n", i+1, result.KnowledgeTitle)
        fmt.Printf("   类型: %s, 重要性: %d\n", 
            result.KnowledgeType, result.ImportanceScore)
        fmt.Printf("   关键词: %v\n", result.Keywords)
        fmt.Printf("   摘要: %s\n\n", result.Summary)
    }

    // 5. 带相似度分数的搜索
    fmt.Println("=== 相似度搜索 ===")
    scoreResults, err := kb.SearchKnowledgeEntriesWithScore("跨站攻击", 3)
    if err != nil {
        fmt.Printf("相似度搜索失败: %v\n", err)
        return
    }

    for i, result := range scoreResults {
        fmt.Printf("%d. %s (相似度: %.4f)\n", 
            i+1, result.Entry.KnowledgeTitle, result.Score)
    }

    // 6. 检查同步状态
    fmt.Println("\n=== 同步状态 ===")
    status, err := kb.GetSyncStatus()
    if err != nil {
        fmt.Printf("获取同步状态失败: %v\n", err)
        return
    }

    fmt.Printf("数据库条目: %d\n", status.DatabaseEntries)
    fmt.Printf("RAG文档: %d\n", status.RAGDocuments)
    fmt.Printf("同步状态: %v\n", status.InSync)
}
```

### 高级用法示例

#### 1. 批量导入知识条目

```go
func ImportFromJSON(kb *knowledgebase.KnowledgeBase, jsonFile string) error {
    // 读取JSON文件
    data, err := ioutil.ReadFile(jsonFile)
    if err != nil {
        return err
    }

    var entries []*schema.KnowledgeBaseEntry
    if err := json.Unmarshal(data, &entries); err != nil {
        return err
    }

    // 批量添加
    for _, entry := range entries {
        if err := kb.AddKnowledgeEntry(entry); err != nil {
            log.Printf("导入条目失败: %s, 错误: %v", entry.KnowledgeTitle, err)
            continue
        }
    }

    return nil
}
```

#### 2. 智能问答助手

```go
func AnswerQuestion(kb *knowledgebase.KnowledgeBase, question string) (string, error) {
    // 搜索相关知识
    results, err := kb.SearchKnowledgeEntriesWithScore(question, 3)
    if err != nil {
        return "", err
    }

    if len(results) == 0 {
        return "抱歉，未找到相关知识。", nil
    }

    // 构建上下文
    var context strings.Builder
    context.WriteString("基于以下知识库内容回答问题:\n\n")
    
    for i, result := range results {
        if result.Score > 0.5 { // 只使用相似度高的结果
            context.WriteString(fmt.Sprintf("知识%d: %s\n", i+1, result.Entry.KnowledgeTitle))
            context.WriteString(fmt.Sprintf("内容: %s\n\n", result.Entry.KnowledgeDetails))
        }
    }

    context.WriteString(fmt.Sprintf("问题: %s\n", question))
    context.WriteString("请基于上述知识库内容回答问题:")

    // 这里可以调用AI模型生成回答
    // answer := callAIModel(context.String())
    
    return context.String(), nil
}
```

## 性能优化

### 1. 向量检索优化

- 支持 HNSW（Hierarchical Navigable Small World）算法进行高效向量检索
- 可配置向量维度和距离计算方法（余弦相似度）
- 支持分页查询，避免大量数据一次性加载

### 2. 数据库优化

- 使用索引优化查询性能
- 支持批量操作减少数据库访问次数
- 事务保护确保数据一致性

### 3. 内存管理

- 支持内存向量存储（用于小规模数据）
- 延迟加载大型文档内容
- 合理的分页机制控制内存使用

## 扩展性

### 1. 自定义嵌入模型

```go
// 使用自定义嵌入模型
kb, err := knowledgebase.NewKnowledgeBase(
    db,
    "custom-kb",
    "自定义知识库",
    "custom",
    rag.WithEmbeddingModel("custom-model"),
    rag.WithModelDimension(768),
    rag.WithEmbeddingClient(customEmbedder),
)
```

### 2. 自定义知识类型

```go
// 定义自定义知识类型
const (
    KnowledgeTypeVulnerability = "Vulnerability"
    KnowledgeTypeTool         = "Tool"
    KnowledgeTypeFramework    = "Framework"
    KnowledgeTypeBestPractice = "BestPractice"
)
```

### 3. 元数据扩展

可以通过 `Metadata` 字段存储额外的自定义信息：

```go
metadata := map[string]any{
    "author":      "张三",
    "create_time": time.Now(),
    "tags":        []string{"web", "security"},
    "difficulty":  "intermediate",
}
```

## 最佳实践

### 1. 知识条目设计

- **标题明确**：使用清晰、具体的标题
- **关键词丰富**：添加相关关键词提高搜索准确性
- **结构化内容**：合理组织详细信息和摘要
- **问题导向**：添加潜在问题帮助用户快速找到答案

### 2. 搜索策略

- **多关键词搜索**：使用多个相关关键词提高召回率
- **分数阈值**：设置合适的相似度阈值过滤无关结果
- **分页处理**：对于大量结果使用分页避免性能问题

### 3. 数据维护

- **定期同步**：定期检查和修复数据库与RAG的同步状态
- **清理重复**：避免添加重复或近似的知识条目
- **更新及时**：及时更新过时的安全知识

## 测试

项目包含完整的测试套件：

- `knowledgebase_test.go` - 基础功能测试
- `integration_test.go` - 完整集成测试
- `sync_test.go` - 同步功能测试

运行测试：

```bash
cd common/ai/rag/knowledgebase
go test -v
```

## 故障排除

### 常见问题

1. **向量化失败**
   - 检查嵌入模型配置
   - 确认网络连接正常
   - 验证文本内容格式

2. **搜索结果不准确**
   - 调整搜索关键词
   - 检查知识条目的关键词设置
   - 考虑重新训练嵌入模型

3. **同步状态异常**
   - 运行同步检查
   - 使用批量同步修复
   - 检查数据库完整性

### 调试输出

使用 `common/log` 包进行调试：

```go
import "github.com/yaklang/yaklang/common/log"

log.Infof("Knowledge base search completed: found %d results", len(results))
log.Warnf("Sync status check: database=%d, rag=%d", dbCount, ragCount)
log.Errorf("Failed to add knowledge entry: %v", err)
```

## 贡献

欢迎提交 Issue 和 Pull Request 来改进知识库系统。请确保：

1. 遵循现有的代码风格
2. 添加适当的测试覆盖
3. 更新相关文档
4. 使用中文注释和文档

## 许可证

本项目遵循 YakLang 项目的许可证。
