package minirehs

import "github.com/yaklang/yaklang/common/utils"

// selectBackend 按配置选择后端.
//   - Auto / BackendEngine: 自研引擎 (Tier 2/3), 始终可用, RE2 精确偏移语义。
//   - BackendStdlib: 逐条匹配后端 (Tier 4, 基线/对照与正确性兜底)。
//   - BackendMVS: 自托管 mvscan 存在性后端 (CGO 时纯 C99 位并行内核, 否则纯 Go), 全程零外部依赖。
func selectBackend(cfg *config) (backendImpl, error) {
	switch cfg.backend {
	case Auto, BackendEngine:
		return &engineBackend{}, nil
	case BackendStdlib:
		return &stdlibBackend{}, nil
	case BackendMVS:
		return &mvsBackend{}, nil
	default:
		return nil, utils.Errorf("minirehs: unknown backend kind %d", cfg.backend)
	}
}
