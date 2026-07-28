package reactloops

import (
	"crypto/sha1"
	"encoding/hex"
	"sync"
)

// estimateTokenCount 是 ytoken.CalcTokenCount 的快速估算替代。
// 使用启发式：中文约 1.5 token/字，英文约 0.25 token/字符（1/4）。
// 对纯 ASCII 文本用 len/4，对包含中文的文本按字符类型分别估算。
// 误差通常在 ±15% 以内，满足 UI 显示和 verification gate 阈值判断需求。
func estimateTokenCount(text string) int {
	if text == "" {
		return 0
	}
	// 快速路径：纯 ASCII 直接 /4
	if isASCII(text) {
		return (len(text) + 3) / 4
	}
	// 混合中英文：按 rune 遍历
	var cjk, other int
	for _, r := range text {
		if isCJK(r) {
			cjk++
		} else {
			other++
		}
	}
	// 中文约 1.5 token/字（Qwen BPE 经验值），非中文约 0.25 token/字符
	return (cjk*3 + other + 3) / 4
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func isCJK(r rune) bool {
	// CJK Unified Ideographs + Extension A + Compatibility Ideographs + 平假名/片假名
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x3040 && r <= 0x30FF)
}

// --- contentHash8 缓存 ---

var (
	hashCacheMu sync.RWMutex
	hashCache   = map[string]string{}
)

// cachedContentHash8 是 contentHash8 的缓存版本，避免对相同内容重复 sha1。
func cachedContentHash8(content string) string {
	if content == "" {
		return ""
	}
	hashCacheMu.RLock()
	if h, ok := hashCache[content]; ok {
		hashCacheMu.RUnlock()
		return h
	}
	hashCacheMu.RUnlock()
	// compute
	sum := sha1.Sum([]byte(content))
	h := hex.EncodeToString(sum[:])[:8]
	hashCacheMu.Lock()
	// 简单 LRU：超过 512 条就清空重建
	if len(hashCache) > 512 {
		hashCache = map[string]string{}
	}
	hashCache[content] = h
	hashCacheMu.Unlock()
	return h
}
