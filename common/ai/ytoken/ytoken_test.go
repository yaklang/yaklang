package ytoken

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// 1. Correctness tests (golden vectors from CharLemAznable/qwen-tokenizer)
// ---------------------------------------------------------------------------

func TestEncodeOrdinary_Chinese(t *testing.T) {
	prompt := "如果现在要你走十万八千里路，需要多长的时间才能到达？ "
	ids := EncodeOrdinary(prompt)
	expect := []int{
		62244, 99601, 30534, 56568, 99314, 110860,
		99568, 107903, 45995, 3837, 85106, 42140,
		45861, 101975, 101901, 104658, 11319, 220,
	}
	if !reflect.DeepEqual(ids, expect) {
		t.Fatalf("EncodeOrdinary mismatch\ngot:    %v\nexpect: %v", ids, expect)
	}
	decoded := Decode(ids)
	if decoded != prompt {
		t.Fatalf("Decode mismatch\ngot:    %q\nexpect: %q", decoded, prompt)
	}
}

func TestEncode_ChatTemplate(t *testing.T) {
	prompt := "<|im_start|>system\nYour are a helpful assistant.<|im_end|>\n<|im_start|>user\nSanFrancisco is a<|im_end|>\n<|im_start|>assistant\n"
	ids := Encode(prompt)
	expect := []int{
		151644, 8948, 198, 7771, 525, 264, 10950,
		17847, 13, 151645, 198, 151644, 872, 198,
		23729, 80328, 9464, 374, 264, 151645, 198,
		151644, 77091, 198,
	}
	if !reflect.DeepEqual(ids, expect) {
		t.Fatalf("Encode mismatch\ngot:    %v\nexpect: %v", ids, expect)
	}
	decoded := Decode(ids)
	if decoded != prompt {
		t.Fatalf("Decode mismatch\ngot:    %q\nexpect: %q", decoded, prompt)
	}
}

func TestCalcTokenCount(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		count int
	}{
		{"chinese", "如果现在要你走十万八千里路，需要多长的时间才能到达？ ", 18},
		{"chat_template", "<|im_start|>system\nYour are a helpful assistant.<|im_end|>\n<|im_start|>user\nSanFrancisco is a<|im_end|>\n<|im_start|>assistant\n", 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CalcTokenCount(tc.text)
			if got != tc.count {
				t.Errorf("CalcTokenCount = %d, want %d", got, tc.count)
			}
		})
	}
}

func TestCalcOrdinaryTokenCount(t *testing.T) {
	text := "如果现在要你走十万八千里路，需要多长的时间才能到达？ "
	got := CalcOrdinaryTokenCount(text)
	if got != 18 {
		t.Errorf("CalcOrdinaryTokenCount = %d, want 18", got)
	}
}

func TestEncode_EmptyString(t *testing.T) {
	ids := Encode("")
	if len(ids) != 0 {
		t.Errorf("Encode(\"\") = %v, want empty", ids)
	}
	if c := CalcTokenCount(""); c != 0 {
		t.Errorf("CalcTokenCount(\"\") = %d, want 0", c)
	}
}

func TestDecode_SpecialTokens(t *testing.T) {
	ids := []int{151644, 151645, 151643}
	decoded := Decode(ids)
	expect := "<|im_start|><|im_end|><|endoftext|>"
	if decoded != expect {
		t.Errorf("Decode special tokens\ngot:    %q\nexpect: %q", decoded, expect)
	}
}

// ---------------------------------------------------------------------------
// 2. Roundtrip stability tests
// ---------------------------------------------------------------------------

func TestRoundtrip_PureEnglish(t *testing.T) {
	assertRoundtrip(t, "Hello, how are you?")
}

func TestRoundtrip_MixedContent(t *testing.T) {
	assertRoundtrip(t, "请帮我写一个 Python 函数，实现 quicksort 算法")
}

func TestRoundtrip_CodeSnippet(t *testing.T) {
	assertRoundtrip(t, "func main() {\n\tfmt.Println(\"Hello\")\n}")
}

func TestRoundtrip_SpecialChars(t *testing.T) {
	assertRoundtrip(t, "tab\there\nnewline\r\nCRLF end")
}

func TestRoundtrip_Numbers(t *testing.T) {
	assertRoundtrip(t, "Pi is 3.14159265358979323846, e is 2.71828182845904523536")
}

func TestRoundtrip_Punctuation(t *testing.T) {
	assertRoundtrip(t, `!@#$%^&*()_+-=[]{}|;':",.<>?/~` + "`")
}

func TestRoundtrip_Unicode(t *testing.T) {
	assertRoundtrip(t, "日本語テスト 한국어 العربية")
}

func TestRoundtrip_LongRepeat(t *testing.T) {
	assertRoundtrip(t, strings.Repeat("AAAA ", 200))
}

func assertRoundtrip(t *testing.T, text string) {
	t.Helper()
	ids := Encode(text)
	if len(ids) == 0 && len(text) > 0 {
		t.Fatalf("Encode returned empty for non-empty text")
	}
	decoded := Decode(ids)
	if decoded != text {
		t.Fatalf("roundtrip mismatch\ngot:    %q\nexpect: %q", decoded, text)
	}
}

// ---------------------------------------------------------------------------
// 3. Idempotency / stability tests
// ---------------------------------------------------------------------------

func TestStability_RepeatedEncoding(t *testing.T) {
	texts := []string{
		"Hello, how are you?",
		"如果现在要你走十万八千里路",
		"def foo():\n    return 42\n",
		"<|im_start|>user\ntest<|im_end|>",
	}
	for _, text := range texts {
		first := Encode(text)
		for i := 0; i < 50; i++ {
			again := Encode(text)
			if !reflect.DeepEqual(first, again) {
				t.Fatalf("non-deterministic: iteration %d differs for %q", i, text)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Token/Bytes ratio analysis with real prompts from common/ai/aid/
// ---------------------------------------------------------------------------

type ratioTestCase struct {
	name     string
	category string // "chinese", "english", "mixed", "code"
	text     string
}

func loadPromptFile(t *testing.T, relPath string) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	fullPath := filepath.Join(projectRoot, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Skipf("prompt file not available: %s", relPath)
	}
	return string(data)
}

func buildRatioTestCases(t *testing.T) []ratioTestCase {
	t.Helper()
	cases := []ratioTestCase{
		// --- inline short samples ---
		{
			name:     "short_english",
			category: "english",
			text:     "Hello, how are you? This is a simple test sentence for the tokenizer.",
		},
		{
			name:     "short_chinese",
			category: "chinese",
			text:     "你好，世界！这是一个用于分词器测试的简单句子。我们希望通过这段文本来测试中文分词的效率。",
		},
		{
			name:     "short_mixed",
			category: "mixed",
			text:     "请帮我写一个 Python 函数，实现 quicksort 算法。要求时间复杂度为 O(n log n)，空间复杂度为 O(log n)。",
		},
		{
			name:     "go_code",
			category: "code",
			text: `package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			fmt.Printf("goroutine %d running\n", n)
		}(i)
	}
	wg.Wait()
	fmt.Println("all goroutines finished")
}`,
		},
		{
			name:     "python_code",
			category: "code",
			text: `import asyncio
from typing import List, Optional

async def fetch_data(urls: List[str], timeout: Optional[float] = 30.0) -> dict:
    """Fetch data from multiple URLs concurrently."""
    results = {}
    async with asyncio.TaskGroup() as tg:
        for url in urls:
            task = tg.create_task(process_url(url, timeout))
            results[url] = task
    return {url: task.result() for url, task in results.items()}

async def process_url(url: str, timeout: float) -> str:
    await asyncio.sleep(0.1)  # simulated network delay
    return f"response from {url}"
`,
		},
		{
			name:     "json_data",
			category: "code",
			text: `{
  "model": "qwen-max",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What is the capital of France?"}
  ],
  "temperature": 0.7,
  "max_tokens": 2048,
  "top_p": 0.95,
  "stream": true,
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather information for a location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {"type": "string"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
          }
        }
      }
    }
  ]
}`,
		},
		{
			name:     "markdown_doc",
			category: "mixed",
			text: `# 代码安全审计报告

## 概述

本次审计针对 Web 应用的后端 API 进行了全面的安全扫描，覆盖以下漏洞类别：

| 类别 | 风险等级 | 发现数量 |
|------|----------|----------|
| SQL Injection | 高 | 3 |
| XSS (Reflected) | 中 | 5 |
| CSRF | 中 | 2 |
| Path Traversal | 高 | 1 |

## 详细发现

### 1. SQL Injection in User Search API

- **位置**: ` + "`" + `/api/v1/users/search` + "`" + `
- **参数**: ` + "`" + `query` + "`" + `
- **严重程度**: Critical (CVSS 9.8)

用户输入的 ` + "`" + `query` + "`" + ` 参数未经参数化处理直接拼接到 SQL 语句中：

` + "```" + `go
db.Raw("SELECT * FROM users WHERE name LIKE '%" + query + "%'")
` + "```" + `

**修复建议**: 使用参数化查询 ` + "`" + `db.Where("name LIKE ?", "%"+query+"%")` + "`" + `
`,
		},
		{
			name:     "security_prompt_chinese",
			category: "chinese",
			text: `你是代码安全审计专家。当前阶段任务：专注搜索分配给你的单一漏洞类别，通过两阶段工作流系统覆盖所有潜在漏洞点。

核心两阶段工作流：

阶段A：关键词搜索（可多次 grep 累积）
目标：根据 Sink 语义提示结合已知技术栈，自主推导合适的 grep 关键词，使用 files_with_matches 模式先发现哪些文件包含匹配，每次 grep 后立即调用 lock_target_files 追加命中文件。

关键词推导原则：
- Sink 语义提示中的 Examples 是参考，不是固定关键词
- 要结合实际技术栈选择最匹配该语言或框架的写法
- 例如同样是SQL执行，PHP 项目应搜 ->query(，Go 项目应搜 db.Exec(，Java 项目应搜 createNativeQuery
- 搜索关键词应尽量简短精准

阶段B：逐文件审计
目标：对每个目标文件依次用 read_file 读取代码，发现漏洞调用 add_finding，完成后调用 mark_file_done。

Finding 提交标准：
- 用户可控输入直接或间接到达危险 Sink
- 中间无有效防护（参数化查询、类型强转、白名单）
- confidence >= 6

不提交：
- 测试文件
- Sink 参数来自配置或硬编码
- confidence < 6`,
		},
		{
			name:     "agent_system_prompt_english",
			category: "english",
			text: `You are monitoring a long-running tool execution. Your task is to review the current execution status and decide whether it should continue or be cancelled.

Based on the above information, you need to decide whether to continue or cancel this tool execution.

When to CONTINUE:
- The tool is making meaningful progress toward the user's goal
- Output shows expected behavior for the operation
- Execution time is reasonable for the task complexity
- No critical errors in stderr

When to CANCEL:
- The tool appears stuck with no new output
- Execution time significantly exceeds expectations for this type of task
- Critical errors or exceptions in stderr indicate failure
- The output suggests the tool is in an infinite loop
- The tool's behavior doesn't align with the user's original request
- User explicitly requested cancellation conditions that are now met
- Resource exhaustion signs (memory, disk, network issues)

Key Questions to Consider:
1. Is the current output progressing toward what the user asked for?
2. Does the elapsed time seem reasonable?
3. Are there any error patterns in stderr that suggest failure?
4. Should the user be informed about the current status?`,
		},
		{
			name:     "shell_commands",
			category: "code",
			text: `#!/bin/bash
set -euo pipefail

echo "=== System Information ==="
uname -a
hostnamectl 2>/dev/null || true

echo "=== Disk Usage ==="
df -h | grep -v tmpfs

echo "=== Memory ==="
free -h 2>/dev/null || vm_stat

echo "=== Network Interfaces ==="
ip addr show 2>/dev/null || ifconfig

echo "=== Listening Ports ==="
ss -tuln 2>/dev/null || netstat -tuln

echo "=== Running Processes (top 10 by CPU) ==="
ps aux --sort=-%cpu 2>/dev/null | head -11 || ps aux | head -11

echo "=== Docker Status ==="
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || echo "Docker not available"

echo "=== Recent Logs ==="
journalctl --no-pager -n 20 2>/dev/null || tail -20 /var/log/syslog 2>/dev/null || echo "No logs available"
`,
		},
	}

	promptFiles := []struct {
		name     string
		category string
		relPath  string
	}{
		{"real_prompt_base", "mixed", "common/ai/aid/aireact/prompts/base/base.txt"},
		{"real_prompt_verification", "chinese", "common/ai/aid/aireact/prompts/verification/verification.txt"},
		{"real_prompt_tool_params", "mixed", "common/ai/aid/aireact/prompts/tool-params/tool-params.txt"},
		{"real_prompt_interval_review", "english", "common/ai/aid/aireact/prompts/tool/interval-review.txt"},
		{"real_prompt_security_audit", "chinese", "common/ai/aid/aireact/reactloops/loop_code_security_audit/prompts/phase2_scan_instruction.txt"},
	}
	for _, pf := range promptFiles {
		content := loadPromptFile(t, pf.relPath)
		if content != "" {
			cases = append(cases, ratioTestCase{
				name:     pf.name,
				category: pf.category,
				text:     content,
			})
		}
	}

	return cases
}

type ratioResult struct {
	name           string
	category       string
	byteCount      int
	runeCount      int
	tokenCount     int
	bytesPerToken  float64
	runesPerToken  float64
	tokensPerRune  float64
}

func TestTokenBytesRatio(t *testing.T) {
	cases := buildRatioTestCases(t)

	var results []ratioResult
	categoryStats := make(map[string]struct {
		totalBytes, totalRunes, totalTokens int
		count                               int
	})

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tokens := CalcTokenCount(tc.text)
			if tokens == 0 {
				t.Fatalf("CalcTokenCount returned 0 for %q", tc.name)
			}

			byteLen := len(tc.text)
			runeLen := utf8.RuneCountInString(tc.text)

			r := ratioResult{
				name:           tc.name,
				category:       tc.category,
				byteCount:      byteLen,
				runeCount:      runeLen,
				tokenCount:     tokens,
				bytesPerToken:  float64(byteLen) / float64(tokens),
				runesPerToken:  float64(runeLen) / float64(tokens),
				tokensPerRune:  float64(tokens) / float64(runeLen),
			}
			results = append(results, r)

			cs := categoryStats[tc.category]
			cs.totalBytes += byteLen
			cs.totalRunes += runeLen
			cs.totalTokens += tokens
			cs.count++
			categoryStats[tc.category] = cs

			// Encode roundtrip for every case
			ids := Encode(tc.text)
			decoded := Decode(ids)
			if decoded != tc.text {
				t.Errorf("roundtrip FAILED for %s (len=%d)", tc.name, byteLen)
			}
		})
	}

	// --- Print report ---
	t.Run("ratio_report", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString("\n")
		sb.WriteString("====================================================================\n")
		sb.WriteString("  Qwen BPE Token/Bytes Ratio Analysis Report\n")
		sb.WriteString("====================================================================\n\n")

		sb.WriteString(fmt.Sprintf("%-35s %8s %8s %8s %8s %8s\n",
			"Name", "Bytes", "Runes", "Tokens", "B/T", "R/T"))
		sb.WriteString(strings.Repeat("-", 83) + "\n")

		for _, r := range results {
			sb.WriteString(fmt.Sprintf("%-35s %8d %8d %8d %8.2f %8.2f\n",
				r.name, r.byteCount, r.runeCount, r.tokenCount,
				r.bytesPerToken, r.runesPerToken))
		}

		sb.WriteString("\n")
		sb.WriteString("====================================================================\n")
		sb.WriteString("  Category Averages\n")
		sb.WriteString("====================================================================\n\n")
		sb.WriteString(fmt.Sprintf("%-12s %8s %8s %8s %8s %8s %8s\n",
			"Category", "Samples", "Bytes", "Tokens", "B/T", "R/T", "T/R"))
		sb.WriteString(strings.Repeat("-", 68) + "\n")

		for _, cat := range []string{"english", "chinese", "mixed", "code"} {
			cs, ok := categoryStats[cat]
			if !ok {
				continue
			}
			avgBT := float64(cs.totalBytes) / float64(cs.totalTokens)
			avgRT := float64(cs.totalRunes) / float64(cs.totalTokens)
			avgTR := float64(cs.totalTokens) / float64(cs.totalRunes)
			sb.WriteString(fmt.Sprintf("%-12s %8d %8d %8d %8.2f %8.2f %8.4f\n",
				cat, cs.count, cs.totalBytes, cs.totalTokens, avgBT, avgRT, avgTR))
		}

		sb.WriteString("\n")
		sb.WriteString("====================================================================\n")
		sb.WriteString("  Interpretation Guide\n")
		sb.WriteString("====================================================================\n")
		sb.WriteString("  B/T  = Bytes per Token  (higher = more bytes consumed per token)\n")
		sb.WriteString("  R/T  = Runes per Token  (higher = more characters per token)\n")
		sb.WriteString("  T/R  = Tokens per Rune  (for estimation: tokens ~ runes * T/R)\n")
		sb.WriteString("  Expected ranges:\n")
		sb.WriteString("    English : B/T ~ 3.5-4.5, R/T ~ 3.5-4.5\n")
		sb.WriteString("    Chinese : B/T ~ 4.0-6.0, R/T ~ 1.3-1.8\n")
		sb.WriteString("    Mixed   : B/T ~ 3.5-5.5, R/T ~ 2.0-3.5\n")
		sb.WriteString("    Code    : B/T ~ 2.5-4.0, R/T ~ 2.5-4.0\n")
		sb.WriteString("====================================================================\n")

		t.Log(sb.String())
	})
}

// ---------------------------------------------------------------------------
// 5. Ratio bounds validation
// ---------------------------------------------------------------------------

func TestRatioBounds_English(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog. This is a longer sentence to establish a reliable ratio measurement for English language tokenization performance."
	tokens := CalcTokenCount(text)
	ratio := float64(len(text)) / float64(tokens)
	// English typically: 3.0 - 5.5 bytes/token
	if ratio < 2.5 || ratio > 6.0 {
		t.Errorf("English B/T ratio %.2f outside expected range [2.5, 6.0]", ratio)
	}
}

func TestRatioBounds_Chinese(t *testing.T) {
	text := "人工智能是计算机科学的一个分支，它企图了解智能的实质，并生产出一种新的能以人类智能相似的方式做出反应的智能机器。该领域的研究包括机器人、语言识别、图像识别、自然语言处理和专家系统等。"
	tokens := CalcTokenCount(text)
	runeLen := utf8.RuneCountInString(text)
	runesPerToken := float64(runeLen) / float64(tokens)
	// Chinese typically: 1.0 - 2.5 runes/token
	if runesPerToken < 0.8 || runesPerToken > 3.0 {
		t.Errorf("Chinese R/T ratio %.2f outside expected range [0.8, 3.0]", runesPerToken)
	}
}

func TestRatioBounds_Code(t *testing.T) {
	text := `func BinarySearch(arr []int, target int) int {
	lo, hi := 0, len(arr)-1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if arr[mid] == target {
			return mid
		} else if arr[mid] < target {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return -1
}`
	tokens := CalcTokenCount(text)
	ratio := float64(len(text)) / float64(tokens)
	// Code typically: 2.0 - 5.0 bytes/token
	if ratio < 1.5 || ratio > 6.0 {
		t.Errorf("Code B/T ratio %.2f outside expected range [1.5, 6.0]", ratio)
	}
}

// ---------------------------------------------------------------------------
// 6. Real prompt files as golden test cases
// ---------------------------------------------------------------------------

func TestRealPrompt_BasePrompt(t *testing.T) {
	text := loadPromptFile(t, "common/ai/aid/aireact/prompts/base/base.txt")
	tokens := CalcTokenCount(text)
	assertRoundtrip(t, text)
	t.Logf("base.txt: %d bytes, %d runes, %d tokens, B/T=%.2f, R/T=%.2f",
		len(text), utf8.RuneCountInString(text), tokens,
		float64(len(text))/float64(tokens),
		float64(utf8.RuneCountInString(text))/float64(tokens))
}

func TestRealPrompt_Verification(t *testing.T) {
	text := loadPromptFile(t, "common/ai/aid/aireact/prompts/verification/verification.txt")
	tokens := CalcTokenCount(text)
	assertRoundtrip(t, text)
	t.Logf("verification.txt: %d bytes, %d runes, %d tokens, B/T=%.2f, R/T=%.2f",
		len(text), utf8.RuneCountInString(text), tokens,
		float64(len(text))/float64(tokens),
		float64(utf8.RuneCountInString(text))/float64(tokens))
}

func TestRealPrompt_ToolParams(t *testing.T) {
	text := loadPromptFile(t, "common/ai/aid/aireact/prompts/tool-params/tool-params.txt")
	tokens := CalcTokenCount(text)
	assertRoundtrip(t, text)
	t.Logf("tool-params.txt: %d bytes, %d runes, %d tokens, B/T=%.2f, R/T=%.2f",
		len(text), utf8.RuneCountInString(text), tokens,
		float64(len(text))/float64(tokens),
		float64(utf8.RuneCountInString(text))/float64(tokens))
}

func TestRealPrompt_IntervalReview(t *testing.T) {
	text := loadPromptFile(t, "common/ai/aid/aireact/prompts/tool/interval-review.txt")
	tokens := CalcTokenCount(text)
	assertRoundtrip(t, text)
	t.Logf("interval-review.txt: %d bytes, %d runes, %d tokens, B/T=%.2f, R/T=%.2f",
		len(text), utf8.RuneCountInString(text), tokens,
		float64(len(text))/float64(tokens),
		float64(utf8.RuneCountInString(text))/float64(tokens))
}

func TestRealPrompt_SecurityAudit(t *testing.T) {
	text := loadPromptFile(t, "common/ai/aid/aireact/reactloops/loop_code_security_audit/prompts/phase2_scan_instruction.txt")
	tokens := CalcTokenCount(text)
	assertRoundtrip(t, text)
	t.Logf("phase2_scan_instruction.txt: %d bytes, %d runes, %d tokens, B/T=%.2f, R/T=%.2f",
		len(text), utf8.RuneCountInString(text), tokens,
		float64(len(text))/float64(tokens),
		float64(utf8.RuneCountInString(text))/float64(tokens))
}

// ---------------------------------------------------------------------------
// 7. Chat message token estimation
// ---------------------------------------------------------------------------

func TestChatMessageOverhead(t *testing.T) {
	type msg struct {
		role, content string
	}
	messages := []msg{
		{"system", "You are a helpful assistant."},
		{"user", "What is quantum computing?"},
		{"assistant", "Quantum computing uses quantum-mechanical phenomena such as superposition and entanglement to perform computation."},
	}

	totalContent := 0
	for _, m := range messages {
		totalContent += CalcTokenCount(m.role) + CalcTokenCount(m.content)
	}

	// Qwen chat format overhead per message:
	// <|im_start|>{role}\n{content}<|im_end|>\n  ~= 4 tokens
	perMsgOverhead := 4
	estimated := totalContent + len(messages)*perMsgOverhead + 2 // +2 for assistant priming

	var fullChat strings.Builder
	for _, m := range messages {
		fullChat.WriteString("<|im_start|>")
		fullChat.WriteString(m.role)
		fullChat.WriteString("\n")
		fullChat.WriteString(m.content)
		fullChat.WriteString("<|im_end|>\n")
	}
	fullChat.WriteString("<|im_start|>assistant\n")
	actual := CalcTokenCount(fullChat.String())

	diff := actual - estimated
	diffPct := float64(diff) / float64(actual) * 100
	t.Logf("Chat estimation: actual=%d, estimated=%d, diff=%d (%.1f%%)", actual, estimated, diff, diffPct)

	// Allow 15% tolerance
	if diffPct > 15 || diffPct < -15 {
		t.Errorf("Chat overhead estimation off by %.1f%%, exceeds 15%% tolerance", diffPct)
	}
}

// ---------------------------------------------------------------------------
// 8. Performance / throughput benchmark
// ---------------------------------------------------------------------------

func BenchmarkCalcTokenCount_ShortEnglish(b *testing.B) {
	text := "Hello, how are you doing today?"
	ensureInit()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalcTokenCount(text)
	}
}

func BenchmarkCalcTokenCount_ShortChinese(b *testing.B) {
	text := "如果现在要你走十万八千里路，需要多长的时间才能到达？"
	ensureInit()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalcTokenCount(text)
	}
}

func BenchmarkCalcTokenCount_MediumMixed(b *testing.B) {
	text := strings.Repeat("请帮我写一个 Python function，实现 quicksort。", 10)
	ensureInit()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalcTokenCount(text)
	}
}

func BenchmarkCalcTokenCount_LargeCode(b *testing.B) {
	text := strings.Repeat(`func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fmt.Fprintf(w, "received %d bytes", len(body))
}
`, 20)
	ensureInit()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalcTokenCount(text)
	}
}
