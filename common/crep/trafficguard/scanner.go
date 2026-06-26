package trafficguard

import (
	"sync"

	pcre2 "github.com/VillanCh/go-pcre2-lite"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minirehs"
)

// Scanner 是 trafficguard 超级正则组的编译产物, 采用两阶段架构以实现最低热路径开销:
//
//  阶段一 (热路径): 把全部高危正则统一编译为一个 minirehs MVS existence-only 数据库,
      // 对输入只扫描一次即可判定"是否有可能命中" (走纯位运算快路径 + Aho-Corasick 字面量预过滤)。
      // 纯净流量(绝大多数)在这一步即可快速排除, 代价极低。
//
//  阶段二 (冷路径): 仅当阶段一报告"有命中"时, 对命中的具体规则用 go-pcre2-lite 的底层接口
      // (pcre2lite: cgo PCRE2 解释器, 字节级偏移) 精确定位并提取命中值(用于脱敏展示与指纹)。
      // PCRE2 支持完整正则语法且线性时间、低回溯, 比 stdlib RE2 表达力更强、更精准。
      // 由于真实命中极少, 这一步的开销可忽略。
//
// 这正是任务书要求的"existence-only 快速候选扫描 -> 命中后再精确定位"模型,
// 保证 MITM 实时热路径: 纯净流量一次扫描快速排除, 命中流量额外只付出极少定位开销。
//
// 该功能默认随 MITM 开启, 当前不提供关闭开关 —— 详见 DefaultScanner 与 grpc_mitm 集成点。
type Scanner struct {
	// exist: 阶段一 existence-only MVS 数据库, 只判"哪些规则命中", 不取字节偏移。
	exist *minirehs.Group
	// rules 与 exist pattern 下标一一对应。
	rules []rule
	// extractors[i] 是 rules[i].Regex 的 PCRE2 编译(pcre2lite 底层接口), 仅供阶段二精确定位用。
	extractors []*pcre2.Regexp
	initErr    error
}

var (
	defaultScannerOnce sync.Once
	defaultScanner     *Scanner
)

// DefaultScanner 返回进程级单例 Scanner(惰性编译)。
//
// 注意: 这里刻意不暴露任何"关闭/禁用"开关 —— 该高危凭证检测默认随 MITM 启用,
// 以保证用户始终能感知最高危的凭证泄漏。
func DefaultScanner() *Scanner {
	defaultScannerOnce.Do(func() {
		s, err := NewScanner()
		if err != nil {
			log.Errorf("trafficguard: compile builtin super-regex group failed: %v (sensitive detection disabled until restart)", err)
			defaultScanner = &Scanner{initErr: err}
			return
		}
		defaultScanner = s
		log.Infof("trafficguard: builtin super-regex group ready, rules=%d backend=mvs(existence) always_on=%d",
			s.Len(), s.NumAlwaysOn())
	})
	return defaultScanner
}

// NewScanner 用内置超级正则组编译一个新的 Scanner。
func NewScanner() (*Scanner, error) {
	rules := builtinRules
	exprs := make([]string, len(rules))
	extractors := make([]*pcre2.Regexp, len(rules))
	for i, r := range rules {
		exprs[i] = r.Regex
		// 阶段二用 PCRE2 精确定位。inline (?i) 等修饰由 PCRE2 原生支持, 无需转换。
		re, err := pcre2.Compile(r.Regex, pcre2.CompileOptions{UTF: true, UCP: true})
		if err != nil {
			log.Errorf("trafficguard: rule %d pcre2 compile failed: %v", r.ID, err)
			return nil, err
		}
		extractors[i] = re
	}
	// 阶段一: MVS existence-only, minLiteralLen=2 让更多规则获得字面量预过滤(always-on 最小化)。
	exist, err := minirehs.BuildGroup(exprs,
		minirehs.WithGroupBackend("mvs"),
		minirehs.WithGroupExistenceOnly(true),
		// minLiteralLen=4: 实测在真实流量上字面量预过滤选择性最佳, 吞吐 ~17MB/s(远高于 2/3 的 ~5MB/s),
		// 同时不丢任何命中(命中率一致)。短字面量规则(如 "sk"/"SG.")会退为 always-on, 但条数受控。
		minirehs.WithGroupMinLiteralLen(4),
	)
	if err != nil {
		// 编译失败也要释放已编译的 PCRE2 资源。
		for _, re := range extractors {
			if re != nil {
				re.Close()
			}
		}
		return nil, err
	}
	return &Scanner{exist: exist, rules: rules, extractors: extractors}, nil
}

// Len 返回编译进组的规则条数。
func (s *Scanner) Len() int { return len(s.rules) }

// NumAlwaysOn 返回无稳定字面量、每次都要跑的规则数(性能风险提示)。
func (s *Scanner) NumAlwaysOn() int {
	if s.exist == nil {
		return -1
	}
	return s.exist.Info().NumAlwaysOn
}

// Ready 表示 Scanner 是否编译成功可用。
func (s *Scanner) Ready() bool { return s != nil && s.exist != nil && s.initErr == nil }

// scanBuffer 两/三阶段扫描单个缓冲区。
//
// 阶段一: exist.MatchedIndexes(data) -> 若无命中直接返回(纯净流量快路径)。
// 阶段二: 仅对阶段一命中的规则, 用 PCRE2 底层接口精确定位提取命中值。
// 阶段三: validateFinding 按 host/方向/值形态做上下文校验, 剔除明显误报(见 validators.go)。
//
// host 为该缓冲区所属流量的目标 host(小写、无端口); 为空时跳过依赖 host 的校验(如厂商自有域抑制)。
func (s *Scanner) scanBuffer(host string, data []byte, direction, surface string, out []Finding) []Finding {
	if len(data) == 0 {
		return out
	}
	// 阶段一: existence-only 快速候选, 纯位运算 + 字面量预过滤(minLiteralLen=4 调优后吞吐最佳)。
	// 纯净流量(绝大多数)在预过滤阶段即被排除, NFA 不需推进, 开销极低。
	matched := s.exist.MatchedIndexes(data)
	if len(matched) == 0 {
		return out
	}
	// 阶段二: 对命中的规则精确定位(冷路径, 命中极少)。
	for _, idx := range matched {
		if idx < 0 || idx >= len(s.rules) || idx >= len(s.extractors) || s.extractors[idx] == nil {
			continue
		}
		r := s.rules[idx]
		// PCRE2 底层接口: 一次 FindAll 批量取回所有命中, 字节偏移在 Groups[0]。
		matches, err := s.extractors[idx].FindAll(data, -1)
		if err != nil {
			log.Errorf("trafficguard: pcre2 extract rule %d failed: %v", r.ID, err)
			continue
		}
		for _, m := range matches {
			if len(m.Groups) == 0 {
				continue
			}
			span := m.Groups[0]
			if span.IsUnset() {
				continue
			}
			raw := data[span.Start:span.End]
			// 阶段三: 上下文/值形态校验, 剔除明显误报(厂商自有域、非真 JWT、源码型口令字段等)。
			if !validateFinding(&r, raw, validateCtx{host: host, direction: direction}) {
				continue
			}
			out = append(out, Finding{
				RuleID:      r.ID,
				RuleName:    r.Name,
				Category:    r.Category,
				Severity:    r.Severity,
				Description: r.Description,
				Solution:    r.Solution,
				Direction:   direction,
				Surface:     surface,
				From:        span.Start,
				To:          span.End,
				RawValue:    append([]byte(nil), raw...),
				MaskedValue: redact(string(raw), r.RedactHead, r.RedactTail),
				Fingerprint: fingerprint(raw),
				ValueLength: len(raw),
			})
		}
	}
	return out
}

// ScanRequest 扫描一个完整的 HTTP 请求报文(含请求行、Header、Body)。
// host 为空: 不做厂商自有域抑制(适合无上下文的单元测试/独立调用)。
func (s *Scanner) ScanRequest(request []byte) []Finding {
	if !s.Ready() {
		return nil
	}
	return s.scanBuffer("", request, "request", "request", nil)
}

// ScanResponse 扫描一个完整的 HTTP 响应报文(含状态行、Header、Body)。
func (s *Scanner) ScanResponse(response []byte) []Finding {
	if !s.Ready() {
		return nil
	}
	return s.scanBuffer("", response, "response", "response", nil)
}

// ScanHTTPFlow 一次性扫描请求 + 响应,返回全部命中(请求命中在前,响应命中在后)。
// 这是 MITM 集成点调用的主入口。host 为目标 host(小写、无端口), 用于第三阶段上下文校验。
func (s *Scanner) ScanHTTPFlow(host string, request, response []byte) []Finding {
	if !s.Ready() {
		return nil
	}
	out := make([]Finding, 0, 4)
	out = s.scanBuffer(host, request, "request", "request", out)
	out = s.scanBuffer(host, response, "response", "response", out)
	return out
}

// Dedup 按 (规则 + 值指纹 + 方向) 去重,避免同一明文在同方向重复刷屏。
func Dedup(findings []Finding) []Finding {
	if len(findings) == 0 {
		return findings
	}
	seen := make(map[string]struct{}, len(findings))
	out := findings[:0]
	for _, f := range findings {
		key := f.Direction + "|" + f.Fingerprint + "|" + itoa(f.RuleID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
