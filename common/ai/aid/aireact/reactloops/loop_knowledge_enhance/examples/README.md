# loop_knowledge_enhance 测试示例

本目录包含 `loop_knowledge_enhance` 的测试脚本，用于验证知识增强循环的各项功能。

## 运行方式

```bash
go run common/yak/cmd/yak.go common/ai/aid/aireact/reactloops/loop_knowledge_enhance/examples/test_xxx.yak
```

## 前置条件

1. **知识库准备**：测试脚本使用 "逻辑漏洞" 知识库，请确保该知识库已存在并包含相关数据
2. **AI 配置**：确保 AI 服务配置正确（API Key、模型等）

## 测试脚本列表

| 脚本名称 | 测试功能 | 关键断言 |
|---------|---------|---------|
| `test_basic_knowledge_search.yak` | 基本语义搜索 | 搜索结果包含密码重置相关内容 |
| `test_keyword_search.yak` | 关键字搜索 | 搜索结果包含验证码相关内容 |
| `test_security_vulnerability_query.yak` | 安全漏洞查询 | 结果包含安全/漏洞关键词 |
| `test_multi_query_compression.yak` | 多查询压缩 | 产生多个 artifact 文件 |
| `test_score_filtering.yak` | 评分过滤机制 | 输出的分数 >= 0.4 |
| `test_final_document_generation.yak` | 最终文档生成 | 生成包含元信息的整合文档 |

## 测试场景说明

### 1. test_basic_knowledge_search.yak

**目标**：验证基本的语义搜索功能

**测试内容**：
- 使用 `search_knowledge_semantic` 搜索密码重置漏洞
- 验证搜索结果被保存到 artifact 文件
- 验证结果包含相关关键词

**关键断言**：
- 执行无错误
- 生成 artifact 文件
- 内容包含密码/重置相关词汇

### 2. test_keyword_search.yak

**目标**：验证关键字搜索功能

**测试内容**：
- 使用 `search_knowledge_keyword` 搜索验证码漏洞
- 验证关键字匹配的准确性

**关键断言**：
- 结果包含验证码相关内容

### 3. test_security_vulnerability_query.yak

**目标**：验证复杂安全查询场景

**测试内容**：
- 查询未授权访问和权限绕过漏洞
- 验证压缩事件输出
- 检查最终文档生成

**关键断言**：
- 内容包含安全/漏洞关键词
- 压缩事件被正确触发

### 4. test_multi_query_compression.yak

**目标**：验证多查询和压缩机制

**测试内容**：
- 多角度搜索：密码重置、验证码、未授权操作
- 验证多个 artifact 文件生成
- 验证 Score 评分输出

**关键断言**：
- 产生至少 1 个 artifact
- 有 Score 评分输出

### 5. test_score_filtering.yak

**目标**：验证评分过滤机制（>= 0.4）

**测试内容**：
- 解析 Stream 中的 Score 值
- 验证低分内容被过滤

**关键断言**：
- 所有输出的 Score >= 0.4
- 无低分内容泄露

### 6. test_final_document_generation.yak

**目标**：验证最终整合文档生成

**测试内容**：
- 多轮查询触发文档整合
- 验证 `knowledge_enhance_final` 文件生成
- 验证文档包含元信息

**关键断言**：
- 生成最终整合文档
- 文档包含：用户查询、查询轮数、生成时间

## 测试输出示例

成功运行时的输出：

```
[INFO] ========================================
[INFO] Testing loop_knowledge_enhance - Basic Knowledge Search
[INFO] ========================================
... (AI 执行过程)
[INFO] ========================================
[INFO] RUNNING ASSERTIONS
[INFO] ========================================
[INFO] ========================================
[INFO] ALL ASSERTIONS PASSED!
[INFO] ========================================
[INFO] Artifact Filename: /path/to/knowledge_round_1_xxx.md
[INFO] Content Length: 2048 bytes
[INFO] Content Preview: ...
```

## 常见问题

### Q: 测试失败：找不到知识库

确保 "逻辑漏洞" 知识库已创建并包含数据。可以在 Yakit 中查看知识库管理。

### Q: 测试失败：AI 调用超时

增加 `aim.timeout()` 的值，默认 300 秒可能不够。

### Q: 没有生成 artifact 文件

检查 AI 是否正确调用了 `search_knowledge_semantic` 或 `search_knowledge_keyword` action。

## 相关文件

- `../compress.go` - 评分压缩实现
- `../action_search.go` - 搜索 action 实现
- `../finalize.go` - 最终文档生成
- `../init.go` - Loop 初始化

