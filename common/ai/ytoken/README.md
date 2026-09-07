# ytoken - high-performance Qwen BPE tokenizer

`ytoken` is a pure-Go-hot-path token counter and encoder built around the
embedded Qwen BPE vocabulary. It is intended for prompt budgeting, truncation,
and local token estimation without a Python or Rust runtime.

The package preserves the historical ytoken vocabulary and special-token
surface (`<|endoftext|>`, `<|im_start|>`, `<|im_end|>`, and
`<|extra_0|>` through `<|extra_204|>`). Model families that define different
special tokens should use an explicitly versioned tokenizer rather than
assuming every Qwen generation has an identical special-token map.

## API

```go
import "github.com/yaklang/yaklang/common/ai/ytoken"

count := ytoken.CalcTokenCount("你好，世界！")
ordinaryCount := ytoken.CalcOrdinaryTokenCount("ordinary text")

// Exact when exceeded is false. When true, count is an early-exit value
// greater than the limit, not the complete token count.
count, exceeded := ytoken.CalcTokenCountUpTo(longPrompt, 16_000)

// Fast hard-limit check. This avoids tokenization entirely when valid UTF-8
// input bytes already prove the text fits and otherwise stops after crossing.
if ytoken.TokenCountExceeds(longPrompt, 16_000) {
	// shrink or reject
}

ids := ytoken.Encode("<|im_start|>user\n你好<|im_end|>")
text := ytoken.Decode(ids)
```

| Function | Description |
|---|---|
| `CalcTokenCount` | Exact count with historical ytoken special tokens |
| `CalcOrdinaryTokenCount` | Exact count without special-token recognition |
| `CalcTokenCountUpTo` | Count with an early-exit token limit |
| `TokenCountExceeds` | Allocation-light hard-limit predicate |
| `Encode` / `EncodeOrdinary` | Return token IDs |
| `Decode` | Convert token IDs back to text |

## Implementation

The tokenizer performs the Qwen-compatible regex pre-split, followed by
byte-level BPE:

- A complete regex piece is looked up directly before running BPE.
- Pieces shorter than 100 bytes use a stack-backed compact boundary array.
  This retains the cache-friendly `O(m*n)` merge algorithm while bounding `n`.
- Larger pieces use a minimum heap plus linked byte offsets. Only the two pairs
  affected by a merge are recomputed, giving `O(n log n)` worst-case time and
  `O(n)` state instead of repeated whole-piece scans and copies.
- Counting has a dedicated path and does not construct a token-ID slice.
- Special tokens are recognized in one bounded scan instead of splitting the
  input once for each of the 208 token strings.
- The Qwen regex is implemented as an allocation-free pure-Go scanner for Go's
  assigned Unicode table. Code points not assigned in that table take the
  historical PCRE2 compatibility path, preserving category behavior when the
  bundled PCRE2 has newer Unicode data.
- The gzip vocabulary is decoded as a stream during one-time initialization,
  avoiding a second in-memory copy of the decompressed file.

No new module dependency is required. The regex pre-tokenizer uses the PCRE2
implementation already present in the Yaklang module.

## Performance

Representative count-only benchmarks on Go 1.22.12, darwin/arm64. Benchmarks
are documentation and are not executed by normal CI tests.

| Input | Previous implementation | Optimized implementation | Allocations before / after |
|---|---:|---:|---:|
| Short English | 13.9-22.6 us | 0.28 us | 361 / 0 |
| Short Chinese | 180-199 us | 4.45 us | 4,352 / 0 |
| Medium mixed | 450-457 us | 20.0 us | 12,466 / 0 |
| About 5 KB Go code | 2.62-2.93 ms | 0.073 ms | about 68,000 / 0 |
| 8.5 KB real Agent base prompt | about 15.9 ms | 0.70 ms | not recorded / 2 |

Results vary by CPU and workload. Run the benchmarks locally when changing the
merge structure, regex iteration, vocabulary representation, or count path.

## Correctness and tests

```bash
go test ./common/ai/ytoken
go test -race ./common/ai/ytoken
go test ./common/ai/ytoken -run '^$' \
  -bench '^BenchmarkCalcTokenCount_' -benchmem
```

The normal test suite includes:

- published Qwen golden vectors;
- encode/decode round trips for Chinese, English, code, Unicode, whitespace,
  and special tokens;
- a deterministic differential corpus comparing the optimized merge engine
  with ytoken's former reference algorithm, including the large-piece path;
- legacy special-token splitting compatibility, including invalid and nested
  markers;
- bounded-count and invalid UTF-8 behavior;
- real Agent prompt coverage.

The embedded `qwen.tiktoken.gz` contains 151,643 mergeable tokens. Initialization
builds the encoder and decoder tables once through `sync.Once`.
