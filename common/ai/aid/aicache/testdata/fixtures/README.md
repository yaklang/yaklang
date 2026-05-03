# aicache fixtures

本目录的 `*.txt` 是从真实生产环境中采下来的 aicache 调试 dump，用作单元测试
的输入素材。每份 dump 由 `aicache.dumpDebug` 产出，文件结构包含：

```
# aicache prompt dump
seq:    NNNNNN
time:   <RFC3339>
model:  <model>
total:  <bytes> bytes / <n> chunks

## sections
[k] section=<section>   nonce=<nonce>                  bytes=<n> hash=<hex16> seen=<n> first=<RFC3339>
...

## hit report
prefix_hit_chunks: <n>
prefix_hit_bytes:  <n>
prefix_hit_ratio:  <pct>%
global_uniq_chunks: <n>
global_cache_bytes: <n>
total_requests:    <n>
section_hash_count:
  - <section>: <n>

## advices
- ...

## raw prompt (<bytes> bytes)
<原 prompt 字节，不再做任何包装>
```

测试 fixture loader（`testdata_helper_test.go:loadFixtureRawPrompt`）只关心
两段：dump 头部声明的 section/hit 元数据，以及 `## raw prompt` 之后的原文。

## 来源

`/Users/v1ll4n/yakit-projects/temp/aicache/20260503-113529-56086/`

采样时段：2026-05-03 11:35-11:38（约 3 分钟内的 73 次连续 ChatBase 调用）

## 选样意图

精选 8 份覆盖 aicache 切片器与 hijacker 需要兼顾的典型场景：

| 文件 | chunks | sections | 说明 |
|---|---|---|---|
| `000001.txt` | 3 | high-static / semi-dynamic / dynamic | 无 timeline 的 LiteForge 早期形态；hijacker 对 3 段也要正确切出 high-static |
| `000005.txt` | 4 | high-static / semi-dynamic / timeline / dynamic | 中等规模完整 4 段，hijacker 主路径 |
| `000010.txt` | 4 | high-static(8KB) / semi-dynamic(12KB) / timeline / dynamic | high-static 与 semi-dynamic 都不算小；hash 后续被多次复用 |
| `000020.txt` | 4 | high-static(10KB) / 普通 semi-dynamic / timeline / dynamic(8KB) | 模拟 dynamic 漂移、其他段稳定 |
| `000040.txt` | 4 | 4 段正常 | 中段调用，全部段都已"漂移过几次"，advice 路径 |
| `000045.txt` | 1 | raw | 无任何 PROMPT_SECTION 标签的 raw prompt；hijacker 必须返回 nil |
| `000060.txt` | 4 | high-static(16KB) seen=7 / semi-dynamic seen=6 / timeline / dynamic | 高度稳定 high-static + semi-dynamic 的真实命中案例 |
| `000073.txt` | 4 | high-static seen=5 / semi-dynamic seen=5 / timeline(75KB) / dynamic(35KB) | 大 prompt + 前缀部分命中（prefix_hit_ratio 16.2%） |

## 使用约定

- 不要对 fixture 内容做任何"清洗"修改，否则 hash 不再匹配 dump 头部声明，
  无法做端到端断言。
- 如果未来要新增 fixture，请：
  1. 从生产 dump 目录直接复制，不修改任何字节
  2. 在上面的表格新增一行说明"这条样本想覆盖什么"
  3. 同步在 `hijacker_test.go` 或 `aicache_test.go` 中添加针对性断言
