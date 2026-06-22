//go:build !cgo || !minirehs_cgo

package minirehs

// newPrefilter 在默认 (无 CGO SIMD) 构建下返回纯 Go 标量 Aho-Corasick 预过滤.
// 这是可移植基线: 任何平台/架构 CGO_ENABLED=0 都走这里.
func newPrefilter(li *literalIndex) prefilter {
	return newScalarPrefilter(li)
}

// simdPrefilterAvailable 报告当前构建是否编入了 SIMD 预过滤 (用于档位标注).
func simdPrefilterAvailable() bool { return false }

// engineTier 是纯 Go 标量预过滤构建的引擎档位.
const engineTier = 3
