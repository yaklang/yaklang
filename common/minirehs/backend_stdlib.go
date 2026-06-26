package minirehs

// stdlibBackend 是逐条匹配后端 (Tier 4). 它不做任何预过滤, 对每条 pattern 用其 verifier
// 扫描整段数据. 它既是正确性兜底, 也作为差分测试的 oracle 与性能对照基线.
//
// 关键词: stdlib backend, oracle, baseline, 逐条匹配
type stdlibBackend struct{}

func (b *stdlibBackend) kind() BackendKind { return BackendStdlib }
func (b *stdlibBackend) tier() int         { return 4 }
func (b *stdlibBackend) simd() bool        { return false }

func (b *stdlibBackend) compile(patterns []*compiledPattern, cfg *config) (compiledDB, error) {
	return &stdlibDB{patterns: patterns}, nil
}

type stdlibDB struct {
	patterns []*compiledPattern
}

func (d *stdlibDB) numAlwaysOn() int { return len(d.patterns) }

func (d *stdlibDB) close() error { return nil }

func (d *stdlibDB) scan(data []byte, sc *scratch, handler MatchHandler) (bool, error) {
	for _, cp := range d.patterns {
		for _, loc := range cp.v.findAll(data) {
			if !handler(Match{ID: cp.id, From: loc[0], To: loc[1]}) {
				return true, nil
			}
		}
	}
	return false, nil
}
