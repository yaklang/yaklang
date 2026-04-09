# ytokenizer - Qwen BPE Token Estimator

Qwen 模型的本地 BPE (Byte Pair Encoding) 分词器，用于精确估算 token 数量。

## 核心特性

- **精确计数**：使用 Qwen 官方 BPE 词表（151,643 mergeable + 208 special = 151,851 tokens），与 Qwen/Qwen2/Qwen3 系列模型完全一致
- **零外部依赖**：不引入新的 `go.mod` 依赖，仅使用项目已有的 `dlclark/regexp2`
- **纯本地运行**：词表通过 `go:embed` 内嵌（gzip 压缩，1.1MB），无运行时文件依赖
- **延迟初始化**：`sync.Once` 保证首次调用时解压加载词表，后续调用零开销

## 使用方式

```go
import "github.com/yaklang/yaklang/common/ai/tokenizer"

// 计算 token 数量（含 special token 处理）
count := ytokenizer.CalcTokenCount("你好，世界！")

// 不处理 special token 的计数
count := ytokenizer.CalcOrdinaryTokenCount("普通文本")

// 获取 token ID 列表
ids := ytokenizer.Encode("<|im_start|>user\n你好<|im_end|>")

// token ID 还原为文本
text := ytokenizer.Decode(ids)
```

## API

| 函数 | 说明 |
|------|------|
| `CalcTokenCount(text string) int` | 计算 token 数，识别 `<\|im_start\|>` 等 special tokens |
| `CalcOrdinaryTokenCount(text string) int` | 计算 token 数，不处理 special tokens |
| `Encode(text string) []int` | 编码为 token ID 列表，识别 special tokens |
| `EncodeOrdinary(text string) []int` | 编码为 token ID 列表，不处理 special tokens |
| `Decode(tokens []int) string` | 将 token ID 列表解码回文本 |

## Token/Bytes 比率参考

基于真实 Agent prompt 和多种内容类型的实测数据：

| 内容类型 | Bytes/Token (B/T) | Runes/Token (R/T) | Tokens/Rune (T/R) | 说明 |
|----------|-------------------|--------------------|--------------------|------|
| 英文 | ~4.6 | ~4.5 | ~0.22 | 纯英文 prompt，约 4.5 字符一个 token |
| 中文 | ~3.9 | ~2.2 | ~0.45 | 纯中文文本，约 2.2 字符一个 token |
| 中英混合 | ~4.0 | ~3.2 | ~0.31 | Agent 系统 prompt 典型场景 |
| 代码 | ~3.4 | ~3.4 | ~0.29 | Go/Python/Shell/JSON |

### 快速估算公式

当无法调用分词器时，可用以下经验公式近似估算：

```
tokens ~ runes * T/R
```

- 纯英文：`tokens ~ len(text) * 0.22`
- 纯中文：`tokens ~ runeCount * 0.45`
- 中英混合 prompt：`tokens ~ runeCount * 0.31`
- 代码：`tokens ~ len(text) * 0.29`

**注意**：这些是粗略估算值，实际使用时应调用 `CalcTokenCount()` 获取精确结果。

### 实测数据样本

以下为项目真实 prompt 文件的实测结果：

| 文件 | Bytes | Runes | Tokens | B/T | R/T |
|------|-------|-------|--------|-----|-----|
| base.txt (Agent 系统 prompt) | 5,995 | 3,939 | 1,423 | 4.21 | 2.77 |
| verification.txt (任务验证) | 22,936 | 12,928 | 5,869 | 3.91 | 2.20 |
| tool-params.txt (工具参数) | 7,434 | 7,138 | 1,947 | 3.82 | 3.67 |
| interval-review.txt (执行监控) | 2,967 | 2,859 | 660 | 4.50 | 4.33 |
| phase2_scan_instruction.txt (安全审计) | 7,039 | 4,143 | 1,807 | 3.90 | 2.29 |

## 实现原理

### BPE 编码流程

```
输入文本
  |
  v
[正则预分词] -- 按 Qwen 的 pattern 将文本切分为 chunks
  |            pattern: (?i:'s|'t|'re|...) | \p{L}+ | \p{N} | ...
  v
[Special Token 分割] -- 识别 <|im_start|> <|im_end|> 等并直接映射 ID
  |
  v
[BPE 编码] -- 对每个 chunk:
  |           1. 拆为单字节序列
  |           2. 查找 rank 最小的相邻 pair
  |           3. 合并该 pair
  |           4. 重复直到无可合并的 pair
  v
Token ID 列表
```

### 词表来源

- BPE 词表文件 `qwen.tiktoken` 来自 [CharLemAznable/qwen-tokenizer](https://github.com/CharLemAznable/qwen-tokenizer)
- 原始来源：[Qwen-7B HuggingFace](https://huggingface.co/Qwen/Qwen-7B/resolve/main/qwen.tiktoken)
- 词表大小：151,643 BPE merges + 208 special tokens
- Qwen/Qwen1.5/Qwen2/Qwen3 系列共享相同词表

### 存储优化

词表文件使用 gzip 压缩后通过 `go:embed` 内嵌：

| 状态 | 大小 |
|------|------|
| 原始 `qwen.tiktoken` | 2.4 MB |
| gzip -9 压缩 | 1.1 MB |
| 压缩率 | 54% |

首次调用时通过 `sync.Once` 解压并构建内存索引（map），后续调用直接使用。

## 测试

```bash
# 运行全部测试（含比率分析报告）
go test -v ./common/ai/tokenizer/

# 运行 benchmark
go test -bench=. -benchmem ./common/ai/tokenizer/
```

测试覆盖：

1. **正确性测试** -- 与 CharLemAznable/qwen-tokenizer 的 golden vector 对齐
2. **编解码往返测试** -- 英文/中文/代码/特殊字符/Unicode/长文本
3. **幂等性测试** -- 同一文本多次编码结果一致
4. **Token/Bytes 比率分析** -- 15 种内容类型的详细比率报告
5. **比率边界验证** -- 确保各类型文本的 B/T 在合理范围内
6. **真实 prompt 测试** -- 使用 `common/ai/aid/` 中的 Agent prompt 作为测试素材
7. **Chat 消息开销测试** -- 验证 Qwen chat template 的 overhead 估算精度
8. **性能 benchmark** -- 短文本/中等文本/长代码的吞吐量基准

## 文件结构

```
common/ai/tokenizer/
  qwen.tiktoken.gz      -- gzip 压缩的 BPE 词表 (1.1MB, embed 到二进制)
  ytokenizer.go          -- 实现: 词表加载 + BPE 算法 + 公共 API
  ytokenizer_test.go     -- 测试: 正确性 + 比率分析 + 稳定性 + benchmark
  README.md              -- 本文档
```
