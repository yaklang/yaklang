package minirehs

import "github.com/yaklang/yaklang/common/utils"

// selectBackend 按配置选择后端.
//   - Auto / BackendEngine: 自研引擎 (Tier 2/3), 始终可用, RE2 精确偏移语义。
//   - BackendStdlib: 逐条匹配后端 (Tier 4, 基线/对照与正确性兜底)。
//   - BackendVectorscan: 高性能存在性后端; 仅在 minirehs_vectorscan 构建且运行时能加载到
//     libhs 时可用, 否则优雅退化为引擎 (warn 一次, 绝不因环境缺失而失败/崩溃)。
func selectBackend(cfg *config) (backendImpl, error) {
	switch cfg.backend {
	case Auto, BackendEngine:
		return &engineBackend{}, nil
	case BackendStdlib:
		return &stdlibBackend{}, nil
	case BackendMVS:
		return &mvsBackend{}, nil
	case BackendVectorscan:
		if b := newVectorscanBackend(); b != nil {
			return b, nil
		}
		cfg.logger.Warnf("minirehs: vectorscan backend unavailable (not built with -tags minirehs_vectorscan, or libhs not loadable, or unsupported CPU); falling back to engine")
		return &engineBackend{}, nil
	default:
		return nil, utils.Errorf("minirehs: unknown backend kind %d", cfg.backend)
	}
}
