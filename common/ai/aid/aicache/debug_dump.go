package aicache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// dumpBaseDir 是调试落盘的基础目录，单进程内只解析一次
// 关键词: aicache, dumpBaseDir, 调试目录
var (
	dumpBaseDir     string
	dumpBaseDirOnce sync.Once
	dumpBaseDirErr  error
	dumpSessionId   string
	dumpMu          sync.Mutex
)

// resolveDumpBaseDir 解析并创建调试落盘根目录
// 路径: <YakitBaseTempDir>/aicache/<sessionId>
// sessionId 形如 20260503-100123-12345
// 关键词: aicache, resolveDumpBaseDir
func resolveDumpBaseDir() (string, error) {
	dumpBaseDirOnce.Do(func() {
		base := consts.GetDefaultYakitBaseTempDir()
		if base == "" {
			base = os.TempDir()
		}
		dumpSessionId = fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), os.Getpid())
		full := filepath.Join(base, "aicache", dumpSessionId)
		if err := os.MkdirAll(full, 0o755); err != nil {
			dumpBaseDirErr = err
			return
		}
		dumpBaseDir = full
		log.Infof("aicache debug dump dir: %s", dumpBaseDir)
	})
	return dumpBaseDir, dumpBaseDirErr
}

// SessionId 返回当前进程的 aicache 会话 ID（懒初始化）
// 关键词: aicache, SessionId
func SessionId() string {
	_, _ = resolveDumpBaseDir()
	return dumpSessionId
}

// dumpDebug 把一次镜像观测的完整快照落盘
// 仅在 utils.InDebugMode() 或测试中触发
// 关键词: aicache, dumpDebug, DEBUG 落盘
func dumpDebug(rep *HitReport, split *PromptSplit, gc *globalCache) {
	if rep == nil || split == nil {
		return
	}
	dir, err := resolveDumpBaseDir()
	if err != nil || dir == "" {
		log.Warnf("aicache resolve dump dir failed: %v", err)
		return
	}

	dumpMu.Lock()
	defer dumpMu.Unlock()

	filename := fmt.Sprintf("%06d.txt", rep.SeqId)
	full := filepath.Join(dir, filename)

	body := renderDebugDump(rep, split, gc)
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		log.Warnf("aicache write dump file failed: %v", err)
	}
}

// renderDebugDump 把一次观测渲染成可读文本，格式参考 plan 第 8 节
// 关键词: aicache, renderDebugDump
func renderDebugDump(rep *HitReport, split *PromptSplit, gc *globalCache) string {
	var sb strings.Builder

	model := rep.Model
	if model == "" {
		model = "-"
	}

	sb.WriteString("# aicache prompt dump\n")
	fmt.Fprintf(&sb, "seq:    %06d\n", rep.SeqId)
	fmt.Fprintf(&sb, "time:   %s\n", rep.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "model:  %s\n", model)
	fmt.Fprintf(&sb, "total:  %d bytes / %d chunks\n", split.Bytes, len(split.Chunks))
	sb.WriteString("\n")

	sb.WriteString("## sections\n")
	for i, ch := range split.Chunks {
		var seenCount int64
		var firstSeen time.Time
		if gc != nil {
			if info := gc.ChunkInfoByHash(ch.Hash); info != nil {
				seenCount = info.HitCount
				firstSeen = info.FirstSeen
			}
		}
		hashShort := ch.Hash
		if len(hashShort) > 16 {
			hashShort = hashShort[:16]
		}
		fmt.Fprintf(&sb,
			"[%d] section=%-13s nonce=%-24s bytes=%d hash=%s seen=%d first=%s\n",
			i+1, ch.Section, truncate(ch.Nonce, 24), ch.Bytes, hashShort, seenCount, formatTimeRFC(firstSeen),
		)
	}
	sb.WriteString("\n")

	sb.WriteString("## hit report\n")
	fmt.Fprintf(&sb, "prefix_hit_chunks: %d\n", rep.PrefixHitChunks)
	fmt.Fprintf(&sb, "prefix_hit_bytes:  %d\n", rep.PrefixHitBytes)
	fmt.Fprintf(&sb, "prefix_hit_ratio:  %.1f%%\n", rep.PrefixHitRatio*100)
	fmt.Fprintf(&sb, "global_uniq_chunks: %d\n", rep.GlobalUniqueChunks)
	fmt.Fprintf(&sb, "global_cache_bytes: %d\n", rep.GlobalCacheBytes)
	fmt.Fprintf(&sb, "total_requests:    %d\n", rep.TotalRequests)
	if len(rep.SectionHashCount) > 0 {
		sb.WriteString("section_hash_count:\n")
		for _, section := range orderedSections(rep.SectionHashCount) {
			fmt.Fprintf(&sb, "  - %s: %d\n", section, rep.SectionHashCount[section])
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## advices\n")
	if len(rep.Advices) == 0 {
		sb.WriteString("- (none)\n")
	} else {
		for _, adv := range rep.Advices {
			fmt.Fprintf(&sb, "- %s\n", adv)
		}
	}
	sb.WriteString("\n")

	fmt.Fprintf(&sb, "## raw prompt (%d bytes)\n", split.Bytes)
	sb.WriteString(split.Original)
	if !strings.HasSuffix(split.Original, "\n") {
		sb.WriteString("\n")
	}
	return sb.String()
}

// orderedSections 按固定顺序输出 section 名，未知 section 字典序追加。
// SectionTimelineOpen 紧随 SectionTimeline, 两者并列展示时视觉相邻便于人工核对。
// 关键词: aicache, orderedSections, timeline / timeline-open 排序
func orderedSections(m map[string]int) []string {
	known := []string{SectionHighStatic, SectionSemiDynamic, SectionTimeline, SectionTimelineOpen, SectionDynamic, SectionRaw}
	seen := make(map[string]bool, len(known))
	var out []string
	for _, s := range known {
		if _, ok := m[s]; ok {
			out = append(out, s)
			seen[s] = true
		}
	}
	for k := range m {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// truncate 长字符串截断，便于 dump 行对齐
// 关键词: aicache, truncate
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

// formatTimeRFC 安全格式化时间，零值返回 "-"
// 关键词: aicache, formatTimeRFC
func formatTimeRFC(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}
