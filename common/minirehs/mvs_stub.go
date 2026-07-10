//go:build !cgo

package minirehs

// 本文件是纯 C99 运行期内核的退化桩: 当 CGO_ENABLED=0 (无 C 工具链/交叉编译) 时
// mvsKernel 不可用, newMVSKernel 返回 nil, mvsDB 全程走纯 Go 参考执行器 (existsIn / scanExist).
// 这是可移植基线: 任何平台/架构无需 C 工具链即可编译运行, 功能与 C 内核构建完全一致.
// (启用 CGO 时改编 mvs_cgo.go 的 C 内核档, 见该文件.)
//
// 关键词: mvscan, stub, 优雅退化, 纯 Go 基线

// mvsKernel 在退化构建下是空类型, 仅为让 mvsDB.kernel 字段在所有构建下可编译.
type mvsKernel struct{}

// mvsKernelAvailable 报告当前构建是否编入了 C 运行期内核 (退化构建恒为 false).
func mvsKernelAvailable() bool { return false }

// newMVSKernel 在退化构建下恒返回 nil (不序列化 blob, 无任何开销).
func newMVSKernel(d *mvsDB) *mvsKernel { return nil }

func (k *mvsKernel) close() {}

func (k *mvsKernel) nfaExists(idx int, data []byte) bool { return false }

func (k *mvsKernel) nfaExistsAnchored(idx int, data []byte, spans []anchorSpan) bool { return false }

func (k *mvsKernel) nfaExistsAnchoredMany(idxs []int32, data []byte, patSpanOff []int32, spansLo, spansHi []int32, sc *scratch) []byte {
	return nil
}

func (k *mvsKernel) nfaExistsMany(idxs []int32, data []byte, sc *scratch) []byte { return nil }

func (k *mvsKernel) mergedScan(data []byte, sc *scratch) []int { return sc.mergedHits[:0] }
