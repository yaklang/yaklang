package aicache

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// loadFixtureRawPrompt 解析 testdata/fixtures/<name> 中的 aicache dump 文件
//
// dump 文件结构由 debug_dump.go:renderDebugDump 产出：先是元数据 + sections
// + hit report + advices，最后是 "## raw prompt (<bytes> bytes)\n" 之后的
// 原 prompt 字节。本 helper 只关心元数据与原 prompt 两段，便于测试做端到
// 端断言（dump 声明的 chunk 数 / section 名 / bytes vs Split() 结果）。
//
// 关键词: aicache, fixture loader, dump 解析, testdata
func loadFixtureRawPrompt(t *testing.T, name string) *FixtureMeta {
	t.Helper()
	path := filepath.Join("testdata", "fixtures", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}

	meta := &FixtureMeta{File: name}
	rest, err := parseFixtureMeta(string(data), meta)
	if err != nil {
		t.Fatalf("parse fixture %s failed: %v", name, err)
	}
	meta.Raw = rest
	return meta
}

// FixtureMeta 是解析 dump 文件后暴露给测试的最小信息集合
// 关键词: aicache, FixtureMeta, dump 元数据
type FixtureMeta struct {
	File           string
	Seq            int
	Model          string
	DeclaredBytes  int
	DeclaredChunks int
	Sections       []FixtureSection
	Hit            FixtureHitReport
	Advices        []string
	Raw            string
}

// FixtureSection 是一个 dump 中声明的 chunk
// 关键词: aicache, FixtureSection
type FixtureSection struct {
	Index   int
	Section string
	Nonce   string
	Bytes   int
	Hash    string // 短哈希（前 16 位）
	Seen    int
}

// FixtureHitReport 是 dump 中声明的命中率报告
// 关键词: aicache, FixtureHitReport
type FixtureHitReport struct {
	PrefixHitChunks    int
	PrefixHitBytes     int
	PrefixHitRatioPct  float64
	GlobalUniqueChunks int
	GlobalCacheBytes   int
	TotalRequests      int
	SectionHashCount   map[string]int
}

var (
	reFixtureSeq      = regexp.MustCompile(`^seq:\s+(\d+)`)
	reFixtureModel    = regexp.MustCompile(`^model:\s+(.+)$`)
	reFixtureTotal    = regexp.MustCompile(`^total:\s+(\d+)\s+bytes\s+/\s+(\d+)\s+chunks`)
	reFixtureSection  = regexp.MustCompile(`^\[(\d+)\]\s+section=(\S+)\s+nonce=(\S+)\s+bytes=(\d+)\s+hash=(\S+)\s+seen=(\d+)`)
	reFixturePrefixCk = regexp.MustCompile(`^prefix_hit_chunks:\s+(\d+)`)
	reFixturePrefixBy = regexp.MustCompile(`^prefix_hit_bytes:\s+(\d+)`)
	reFixturePrefixRt = regexp.MustCompile(`^prefix_hit_ratio:\s+([0-9.]+)%`)
	reFixtureGlobUq   = regexp.MustCompile(`^global_uniq_chunks:\s+(\d+)`)
	reFixtureGlobBy   = regexp.MustCompile(`^global_cache_bytes:\s+(\d+)`)
	reFixtureTotalReq = regexp.MustCompile(`^total_requests:\s+(\d+)`)
	reFixtureSecHash  = regexp.MustCompile(`^\s+-\s+(\S+):\s+(\d+)`)
	reFixtureRawHead  = regexp.MustCompile(`^##\s+raw prompt\s+\((\d+)\s+bytes\)\s*$`)
	reFixtureAdvice   = regexp.MustCompile(`^-\s+(.+)$`)
)

// parseFixtureMeta 扫描 dump 文件文本，把元数据填进 meta，并返回原 prompt 文本
//
// 状态机段名: header / sections / hit_report / section_hash_count / advices / raw
// 关键词: aicache, fixture parser, 状态机
func parseFixtureMeta(content string, meta *FixtureMeta) (string, error) {
	meta.Hit.SectionHashCount = make(map[string]int)
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	state := "header"
	rawStartLine := -1
	lineNo := 0

	// 计算 raw prompt 在原文中的字节偏移：扫到 "## raw prompt (...)" 行后，
	// 它的下一字节即 raw 段开始。直接用 strings.Index 重新定位最稳，避免
	// 行分隔符（\n vs \r\n）带来的偏移误差。
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if state == "raw" {
			break
		}
		switch state {
		case "header":
			if m := reFixtureSeq.FindStringSubmatch(line); m != nil {
				meta.Seq, _ = strconv.Atoi(m[1])
				continue
			}
			if m := reFixtureModel.FindStringSubmatch(line); m != nil {
				meta.Model = strings.TrimSpace(m[1])
				continue
			}
			if m := reFixtureTotal.FindStringSubmatch(line); m != nil {
				meta.DeclaredBytes, _ = strconv.Atoi(m[1])
				meta.DeclaredChunks, _ = strconv.Atoi(m[2])
				continue
			}
			if line == "## sections" {
				state = "sections"
				continue
			}
		case "sections":
			if line == "## hit report" {
				state = "hit_report"
				continue
			}
			if m := reFixtureSection.FindStringSubmatch(line); m != nil {
				idx, _ := strconv.Atoi(m[1])
				bytes, _ := strconv.Atoi(m[4])
				seen, _ := strconv.Atoi(m[6])
				meta.Sections = append(meta.Sections, FixtureSection{
					Index:   idx,
					Section: m[2],
					Nonce:   m[3],
					Bytes:   bytes,
					Hash:    m[5],
					Seen:    seen,
				})
			}
		case "hit_report":
			if line == "section_hash_count:" {
				state = "section_hash_count"
				continue
			}
			if line == "section_total_uses:" {
				// section_total_uses 是 advice reuse_rate 用的诊断信息,
				// 当前 fixture 校验无关 - 只需吞掉直到下一个标题
				// 关键词: parseFixtureMeta section_total_uses skip
				state = "section_total_uses"
				continue
			}
			if line == "## advices" {
				state = "advices"
				continue
			}
			if m := reFixturePrefixCk.FindStringSubmatch(line); m != nil {
				meta.Hit.PrefixHitChunks, _ = strconv.Atoi(m[1])
				continue
			}
			if m := reFixturePrefixBy.FindStringSubmatch(line); m != nil {
				meta.Hit.PrefixHitBytes, _ = strconv.Atoi(m[1])
				continue
			}
			if m := reFixturePrefixRt.FindStringSubmatch(line); m != nil {
				meta.Hit.PrefixHitRatioPct, _ = strconv.ParseFloat(m[1], 64)
				continue
			}
			if m := reFixtureGlobUq.FindStringSubmatch(line); m != nil {
				meta.Hit.GlobalUniqueChunks, _ = strconv.Atoi(m[1])
				continue
			}
			if m := reFixtureGlobBy.FindStringSubmatch(line); m != nil {
				meta.Hit.GlobalCacheBytes, _ = strconv.Atoi(m[1])
				continue
			}
			if m := reFixtureTotalReq.FindStringSubmatch(line); m != nil {
				meta.Hit.TotalRequests, _ = strconv.Atoi(m[1])
				continue
			}
		case "section_hash_count":
			if line == "section_total_uses:" {
				state = "section_total_uses"
				continue
			}
			if line == "## advices" {
				state = "advices"
				continue
			}
			if m := reFixtureSecHash.FindStringSubmatch(line); m != nil {
				count, _ := strconv.Atoi(m[2])
				meta.Hit.SectionHashCount[m[1]] = count
			}
		case "section_total_uses":
			// 只吞行, 等待 ## advices 标题切换. 若未来需要解析具体 total/distinct,
			// 在这里加正则即可.
			if line == "## advices" {
				state = "advices"
				continue
			}
		case "advices":
			if reFixtureRawHead.MatchString(line) {
				state = "raw"
				rawStartLine = lineNo
				continue
			}
			if line == "" || line == "- (none)" {
				continue
			}
			if m := reFixtureAdvice.FindStringSubmatch(line); m != nil {
				meta.Advices = append(meta.Advices, m[1])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	_ = rawStartLine

	// 重新在原 content 中定位 "## raw prompt (...)" 行，取它的下一字节作为
	// raw 段开始，避免按行扫描时的换行符歧义。
	headerRe := regexp.MustCompile(`(?m)^## raw prompt \(\d+ bytes\)\s*\n`)
	loc := headerRe.FindStringIndex(content)
	if loc == nil {
		// 没有 raw 段，按当前实现这种 dump 不会出现；返回剩余空串以容错
		return "", nil
	}
	raw := content[loc[1]:]
	// dumpDebug 在 split.Original 末尾会确保有一个 \n；测试时使用原 prompt
	// 字节就行，不再裁剪末尾换行（与 dump 头部声明的 bytes 对齐时再做修正）。
	// 把末尾恰好的 1 个 trailing newline 去掉：dumpDebug 在 split.Original 不
	// 以 \n 结尾时主动补了一个；该 newline 不属于 split.Bytes，需要在测试
	// 端删掉以保证 len(raw) == DeclaredBytes。
	if len(raw) > meta.DeclaredBytes {
		// 多余字节通常就是这个 trailing newline（或 \r\n）
		extra := len(raw) - meta.DeclaredBytes
		if extra <= 2 {
			raw = raw[:meta.DeclaredBytes]
		}
	}
	return raw, nil
}
