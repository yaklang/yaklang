package minirehs

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"
)

// mitmRule 对应 rule4yak 的 yakit-mitm-replacer-rules-config.json 单条规则.
type mitmRule struct {
	Rule        string `json:"Rule"`
	Index       int    `json:"Index"`
	VerboseName string `json:"VerboseName"`
}

// loadMITMRules 读取 testdata/rules.json (来自 SexyBeast233/rule4yak), 返回 89 条规则.
func loadMITMRules(tb testing.TB) []mitmRule {
	tb.Helper()
	raw, err := os.ReadFile("testdata/rules.json")
	if err != nil {
		tb.Fatalf("read rules.json: %v", err)
	}
	var rules []mitmRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		tb.Fatalf("unmarshal rules.json: %v", err)
	}
	if len(rules) == 0 {
		tb.Fatal("no rules loaded")
	}
	return rules
}

// mitmPatterns 把 MITM 规则转换为 minirehs Pattern 集合 (默认对不支持的构造 Reject;
// 调用方可指定策略). 同时返回每个 PatternID -> 规则名的映射, 便于报告.
func mitmPatterns(tb testing.TB) ([]Pattern, map[PatternID]string) {
	tb.Helper()
	rules := loadMITMRules(tb)
	patterns := make([]Pattern, 0, len(rules))
	names := make(map[PatternID]string, len(rules))
	for i, r := range rules {
		if r.Rule == "" {
			continue
		}
		id := PatternID(i + 1)
		patterns = append(patterns, Pattern{ID: id, Expr: r.Rule})
		names[id] = r.VerboseName
	}
	return patterns, names
}

// loadCorpus 读取 testdata/traffic_corpus.bin (来自本地 yaklang 项目库的真实流量),
// 解析为一组报文记录. 同时返回拼接后的总字节切片.
//
// 性能: 默认在"快速回归"模式下截断到前 corpusDefaultLimit 条 (仍覆盖多样报文形态),
// 让 minirehs 全量回归不被真实语料 (1332 条 / 5.2MB) 的反复 oracle 扫描卡住.
// 全量语料通过环境变量 MINIREHS_FULL_CORPUS=1 恢复 (CI / 发布回归).
// testing.Short() 时由各调用方进一步裁剪 (已遍布各测试).
func loadCorpus(tb testing.TB) (records [][]byte, joined []byte) {
	tb.Helper()
	raw, err := os.ReadFile("testdata/traffic_corpus.bin")
	if err != nil {
		tb.Skipf("traffic corpus not found (run: go run testdata/gen_corpus.go): %v", err)
	}
	i := 0
	for i+4 <= len(raw) {
		n := int(binary.LittleEndian.Uint32(raw[i : i+4]))
		i += 4
		if n < 0 || i+n > len(raw) {
			break
		}
		rec := raw[i : i+n]
		records = append(records, rec)
		joined = append(joined, rec...)
		i += n
	}
	if len(records) == 0 {
		tb.Skip("empty corpus")
	}
	if !fullCorpusRequested() && len(records) > corpusDefaultLimit {
		records = records[:corpusDefaultLimit]
		joined = joined[:0]
		for _, r := range records {
			joined = append(joined, r...)
		}
	}
	return records, joined
}
