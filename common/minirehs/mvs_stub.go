//go:build !cgo

package minirehs

// 本文件只提供 C99 运行期内核不可用时的桩方法，不提供 PCRE2 替代实现。
// 整个包仍需要 VillanCh/go-pcre2-lite 和 cgo，不能据此推断无 cgo 构建可用。
//
// 关键词: mvscan, stub, 优雅退化, 纯 Go 基线

// mvsKernel 在退化构建下是空类型, 仅为让 mvsDB.kernel 字段在所有构建下可编译.
type mvsKernel struct{}

// mvsKernelAvailable 报告当前构建是否编入了 C 运行期内核 (退化构建恒为 false).
func mvsKernelAvailable() bool { return false }

// openMVSKernel is unavailable without cgo.
func openMVSKernel(blob []byte, npat int) *mvsKernel { return nil }

func (k *mvsKernel) close() {}

func (k *mvsKernel) nfaExists(idx int, data []byte) bool { return false }

func (k *mvsKernel) findAllLoc(idx int, data []byte, sc *scratch) ([]int32, bool) { return nil, false }

func (k *mvsKernel) nfaExistsAssert(idx int, data []byte, bound []uint8) bool { return false }

func (k *mvsKernel) nfaExistsAssertMany(idxs []int32, data []byte, sc *scratch) []byte { return nil }

func (k *mvsKernel) computeBoundariesC(data []byte, buf []byte) []byte { return nil }

func (k *mvsKernel) nfaExistsAssertOnline(idx int, data []byte) bool { return false }

func (k *mvsKernel) nfaExistsAnchored(idx int, data []byte, spans []anchorSpan) bool { return false }

func (k *mvsKernel) nfaExistsAnchoredMany(idxs []int32, data []byte, patSpanOff []int32, spansLo, spansHi []int32, sc *scratch) []byte {
	return nil
}

func (k *mvsKernel) nfaExistsMany(idxs []int32, data []byte, sc *scratch) []byte { return nil }

func (k *mvsKernel) mergedScan(data []byte, sc *scratch) []int { return sc.mergedHits[:0] }

func (k *mvsKernel) combinedScan(data []byte, assertIdxs []int32, keepBound bool, sc *scratch) ([]int, []int) {
	return sc.mergedHits[:0], sc.assertHits[:0]
}

// These paths are unreachable without a native kernel; preserve the error
// contract if an internal caller nevertheless attempts to dispatch to one.
func (k *mvsKernel) nfaExistsErr(idx int, data []byte) (bool, error) {
	return false, &nativeExecutionError{stage: "exists", index: idx, code: -1}
}
func (k *mvsKernel) nfaExistsAssertErr(idx int, data, bound []byte) (bool, error) {
	return false, &nativeExecutionError{stage: "assert", index: idx, code: -1}
}
func (k *mvsKernel) nfaExistsAssertOnlineErr(idx int, data []byte) (bool, error) {
	return false, &nativeExecutionError{stage: "assert_online", index: idx, code: -1}
}
func (k *mvsKernel) nfaExistsAnchoredErr(idx int, data []byte, spans []anchorSpan) (bool, error) {
	return false, &nativeExecutionError{stage: "anchored", index: idx, code: -1}
}
func (k *mvsKernel) findAllLocErr(idx int, data []byte, sc *scratch) ([]int32, error) {
	return nil, &nativeExecutionError{stage: "locate", index: idx, code: -1}
}
func (k *mvsKernel) mergedScanErr(data []byte, sc *scratch) ([]int, error) {
	return nil, &nativeExecutionError{stage: "merged", index: -1, code: -1}
}
func (k *mvsKernel) combinedScanErr(data []byte, assertIdxs []int32, keepBound bool, sc *scratch) ([]int, []int, error) {
	return nil, nil, &nativeExecutionError{stage: "combined", index: -1, code: -1}
}
