//go:build cgo

package minirehs

/*
#cgo CFLAGS: -I${SRCDIR}/native/mvscan -O3 -std=c99 -Wall

#include "mvscan.h"
// 默认编入 native/mvscan 多文件源; 当 minirehs_mvs_amalg 构建标签存在时 (经
// mvs_cgo_amalg.go 注入 -DMVS_USE_AMALGAMATION), 改编入单文件 amalgamation 产物, 从而用
// 同一套差分/oracle 测试矩阵真实验证 "宿主丢两个文件即可编" 的发行件运行期行为.
#ifdef MVS_USE_AMALGAMATION
#include "native/mvscan/amalgamation/mvscan.c"
#else
#include "native/mvscan/mvscan.c"
#endif
*/
import "C"

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// anchSpanPool 复用锚定式 spans 的 lo/hi []int32 切片, 避免 cgo 热路径每次分配.
// 锚定批处理每报文每 pattern 调用 nfaExistsAnchored, 若每次 make 会成为分配大头.
var anchSpanPool = sync.Pool{
	New: func() interface{} {
		return &anchSpanBufs{lo: make([]int32, 0, 16), hi: make([]int32, 0, 16)}
	},
}

type anchSpanBufs struct {
	lo, hi []int32
}

// anchSpanBuf 取一对容量 >= n 的 lo/hi 切片 (复用池). 调用方在 cgo 调用后须调 anchSpanPut 归还.
func anchSpanBuf(n int) *anchSpanBufs {
	b := anchSpanPool.Get().(*anchSpanBufs)
	if cap(b.lo) < n {
		b.lo = make([]int32, n)
		b.hi = make([]int32, n)
	} else {
		b.lo = b.lo[:n]
		b.hi = b.hi[:n]
	}
	return b
}

func anchSpanPut(b *anchSpanBufs) {
	anchSpanPool.Put(b)
}

// 本文件是纯 C99 运行期内核 (native/mvscan) 的 cgo 薄封装 (范式同 prefilter_cgo.go):
// Go 把编译产物序列化为平台无关 blob 传入 C, C 出力做位并行存在性扫描. 只要启用 CGO
// (CGO_ENABLED=1) 即默认编入此 C 内核 (无需额外 build tag), 这是"默认 CGO 最强优化"档;
// CGO_ENABLED=0 时走 mvs_stub.go 退化为纯 Go 参考执行器, 全平台可移植零依赖.
//
// 契约: 存在性 (nfaExists / mergedScan) 走 C, 与 Go existsIn / scanExist 逐位一致 (差分护栏
// 见 mvs_cgo_test.go). 定位 (findAllLoc) 始终走已验证的 Go 路径, C 内核只判存在性 (符合
// IMPL "存在性匹配" 的运行期内核定位).
//
// 关键词: mvscan, cgo, pure C kernel, blob, bit-parallel NFA, existence

// mvsKernel 持有 C 侧解析好的只读 db 句柄. 不可变、可被多 goroutine 只读共享 (扫描不写 db,
// 仅写调用方提供的 scratch 缓冲).
type mvsKernel struct {
	db   *C.mvscan_db
	npat int
}

// mvsKernelAvailable 报告当前构建是否编入了 C 运行期内核.
func mvsKernelAvailable() bool { return true }

// newMVSKernel 在 minirehs_mvs 构建下把 db 的 per-pattern NFA + 合并 always-on 序列化为
// 平台无关 blob 并交 C 内核解析; 失败 (无 NFA / 解析错误) 返回 nil, 调用方据此退化为纯 Go.
func newMVSKernel(d *mvsDB) *mvsKernel {
	if d == nil {
		return nil
	}
	blob := buildMVSBlob(d.nfas, d.merged, d.assertNecFactor, d.hasLiterals)
	return openMVSKernel(blob, d.n)
}

// openMVSKernel 解析一段 blob 为 C 内核句柄. 供 newMVSKernel 与差分测试共用.
func openMVSKernel(blob []byte, npat int) *mvsKernel {
	if len(blob) == 0 {
		return nil
	}
	h := C.mvscan_db_open((*C.uint8_t)(unsafe.Pointer(&blob[0])), C.size_t(len(blob)))
	if h == nil {
		return nil
	}
	k := &mvsKernel{db: h, npat: npat}
	// 兜底: 即便调用方漏调 close, GC 回收时也释放 C 内存 (close 仍是首选, 及时确定性释放).
	runtime.SetFinalizer(k, func(x *mvsKernel) { x.close() })
	return k
}

func (k *mvsKernel) close() {
	if k == nil || k.db == nil {
		return
	}
	C.mvscan_db_close(k.db)
	k.db = nil
	runtime.SetFinalizer(k, nil)
}

// nfaExists 判定 pattern 下标 idx 的 NFA 是否在 data 中存在命中 (C 实现, == Go existsIn).
func (k *mvsKernel) nfaExists(idx int, data []byte) bool {
	return k.nfaExistsImpl(idx, data, false)
}

// nfaExistsScalar 强制走 C 标量孪生 (绕过 SIMD 分发), 仅供差分测试.
func (k *mvsKernel) nfaExistsScalar(idx int, data []byte) bool {
	return k.nfaExistsImpl(idx, data, true)
}

// nfaExistsAnchored 判定 pattern idx 的 NFA 是否在 data 中存在命中, 但仅在 spans
// 注入区间内注入起点 (锚定式语义, 对应 Go existsInAnchored). spans 须已排序合并.
// 与 Go existsInAnchored 逐位一致 (差分护栏). 返回 true 命中.
func (k *mvsKernel) nfaExistsAnchored(idx int, data []byte, spans []anchorSpan) bool {
	return k.nfaExistsAnchoredImpl(idx, data, spans, false)
}

// nfaExistsAnchoredScalar 强制走 C 标量孪生, 仅供差分测试.
func (k *mvsKernel) nfaExistsAnchoredScalar(idx int, data []byte, spans []anchorSpan) bool {
	return k.nfaExistsAnchoredImpl(idx, data, spans, true)
}

func (k *mvsKernel) nfaExistsAnchoredImpl(idx int, data []byte, spans []anchorSpan, scalar bool) bool {
	if k == nil || k.db == nil || len(spans) == 0 {
		return false
	}
	// 把 Go anchorSpan (lo,hi int32) 拆为两个 []int32 传 C, 避免 C.mvs_span 的 Go 分配.
	// 用 sync.Pool 复用 lo/hi 切片, 消除热路径 make 开销 (cgo 调用后归还).
	n := len(spans)
	buf := anchSpanBuf(n)
	lo, hi := buf.lo, buf.hi
	for i, s := range spans {
		lo[i] = int32(s.lo)
		hi[i] = int32(s.hi)
	}
	var dptr *C.uint8_t
	if len(data) > 0 {
		dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	var r C.int
	if scalar {
		r = C.mvscan_db_nfa_exists_anchored_scalar(k.db, C.int32_t(idx), dptr, C.size_t(len(data)),
			(*C.int32_t)(unsafe.Pointer(&lo[0])), (*C.int32_t)(unsafe.Pointer(&hi[0])), C.int32_t(n))
	} else {
		r = C.mvscan_db_nfa_exists_anchored(k.db, C.int32_t(idx), dptr, C.size_t(len(data)),
			(*C.int32_t)(unsafe.Pointer(&lo[0])), (*C.int32_t)(unsafe.Pointer(&hi[0])), C.int32_t(n))
	}
	keepAlive(data)
	runtime.KeepAlive(lo)
	runtime.KeepAlive(hi)
	anchSpanPut(buf)
	return r == 1
}

// nfaExistsAssert 判定断言 NFA (hasAssert) 在 data 中是否存在命中 (C 实现).
// bound 为预算的共享边界数组 (computeBoundariesC). 与 Go existsInAssertShared1 逐位一致.
// 返回 true 命中 / false 不命中或无 C 断言 NFA.
func (k *mvsKernel) nfaExistsAssert(idx int, data []byte, bound []byte) bool {
	if k == nil || k.db == nil || len(data) == 0 || len(bound) < len(data)+1 {
		return false
	}
	var dptr *C.uint8_t
	if len(data) > 0 {
		dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	var bptr *C.uint8_t
	if len(bound) > 0 {
		bptr = (*C.uint8_t)(unsafe.Pointer(&bound[0]))
	}
	r := C.mvscan_db_nfa_exists_assert(k.db, C.int32_t(idx), dptr, C.size_t(len(data)), bptr)
	keepAlive(data)
	runtime.KeepAlive(bound)
	return r == 1
}

// nfaExistsAssertSelf 自包含断言扫描 — C 内部预算边界, 省去 Go 侧 sharedBound 一次 cgo.
func (k *mvsKernel) nfaExistsAssertSelf(idx int, data []byte, boundBuf []byte) bool {
	if k == nil || k.db == nil || len(data) == 0 {
		return false
	}
	if cap(boundBuf) < len(data)+1 {
		boundBuf = make([]byte, len(data)+1)
	} else {
		boundBuf = boundBuf[:len(data)+1]
	}
	var dptr *C.uint8_t
	dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	var bptr *C.uint8_t
	bptr = (*C.uint8_t)(unsafe.Pointer(&boundBuf[0]))
	r := C.mvscan_db_nfa_exists_assert_self(k.db, C.int32_t(idx), dptr, C.size_t(len(data)), bptr)
	keepAlive(data)
	runtime.KeepAlive(boundBuf)
	return r == 1
}

// nfaExistsAssertMany 一次 cgo 对多条断言 always-on NFA 做断言存在性扫描,
// 内部在 C 预算边界 (每报文一次, 跨多条 NFA 共享). 省去每条 NFA 一次 cgo.
// idxs 为断言 always-on pattern 下标. boundBuf 容量须 >= len(data)+1.
// 结果复用 sc.assertBatchOut (len==len(idxs), 1 命中/0 不命中).
// dfaScanBatch 在单次 cgo 调用中对多个 DFA 模式扫描同一段 data.
// 返回命中的 pattern idx 列表.
func (k *mvsKernel) dfaScanBatch(idxs []int32, data []byte, sc *scratch) []int32 {
	if k == nil || k.db == nil || len(idxs) == 0 || len(data) == 0 {
		return nil
	}
	capOut := len(idxs)
	if cap(sc.cmerged) < capOut {
		sc.cmerged = make([]int32, capOut)
	} else {
		sc.cmerged = sc.cmerged[:capOut]
	}
	var dptr *C.uint8_t
	dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	var iptr *C.int32_t
	if len(idxs) > 0 {
		iptr = (*C.int32_t)(unsafe.Pointer(&idxs[0]))
	}
	var optr *C.int32_t
	if len(sc.cmerged) > 0 {
		optr = (*C.int32_t)(unsafe.Pointer(&sc.cmerged[0]))
	}
	got := int32(C.mvscan_db_dfa_scan_batch(k.db, dptr, C.size_t(len(data)),
		iptr, C.int(len(idxs)), optr, C.int(capOut)))
	keepAlive(data)
	runtime.KeepAlive(idxs)
	runtime.KeepAlive(sc.cmerged)
	return sc.cmerged[:got]
}

func (k *mvsKernel) nfaExistsAssertMany(idxs []int32, data []byte, sc *scratch) []byte {
	if k == nil || k.db == nil || len(idxs) == 0 {
		return nil
	}
	n := len(data)
	// 确保 boundBuf 容量 >= n+1
	if cap(sc.assertBound) < n+1 {
		sc.assertBound = make([]byte, n+1)
	} else {
		sc.assertBound = sc.assertBound[:n+1]
	}
	if cap(sc.assertBatchOut) < len(idxs) {
		sc.assertBatchOut = make([]byte, len(idxs))
	} else {
		sc.assertBatchOut = sc.assertBatchOut[:len(idxs)]
	}
	out := sc.assertBatchOut
	var dptr *C.uint8_t
	if len(data) > 0 {
		dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	var bptr *C.uint8_t
	if len(sc.assertBound) > 0 {
		bptr = (*C.uint8_t)(unsafe.Pointer(&sc.assertBound[0]))
	}
	// 构建 C int32 数组传 idxs (复用 scratch 缓冲, 避免热路径分配)
	if cap(sc.anchorCIdx) < len(idxs) {
		sc.anchorCIdx = make([]int32, len(idxs))
	} else {
		sc.anchorCIdx = sc.anchorCIdx[:len(idxs)]
	}
	copy(sc.anchorCIdx, idxs)
	var iptr *C.int32_t
	if len(sc.anchorCIdx) > 0 {
		iptr = (*C.int32_t)(unsafe.Pointer(&sc.anchorCIdx[0]))
	}
	var optr *C.uint8_t
	if len(out) > 0 {
		optr = (*C.uint8_t)(unsafe.Pointer(&out[0]))
	}
	C.mvscan_db_nfa_exists_assert_many(k.db, dptr, C.size_t(len(data)),
		iptr, C.int32_t(len(idxs)), bptr, optr)
	keepAlive(data)
	runtime.KeepAlive(sc.assertBound)
	runtime.KeepAlive(sc.anchorCIdx)
	runtime.KeepAlive(out)
	return out
}

// combinedScan: 单次数据遍历同时跑 merged NFA + assert NFA.
// 返回 (mergedHits, assertHits).
func (k *mvsKernel) combinedScan(data []byte, assertIdxs []int32, sc *scratch) ([]int, []int) {
	sc.mergedHits = sc.mergedHits[:0]
	sc.assertHits = sc.assertHits[:0]
	if k == nil || k.db == nil || len(data) == 0 {
		return sc.mergedHits, sc.assertHits
	}
	n := len(data)
	// mergedSeen
	if cap(sc.cseen) < k.npat {
		sc.cseen = make([]byte, k.npat)
	} else {
		sc.cseen = sc.cseen[:k.npat]
		for i := range sc.cseen {
			sc.cseen[i] = 0
		}
	}
	// mergedOut
	mergedCap := k.npat
	if cap(sc.cmerged) < mergedCap {
		sc.cmerged = make([]int32, mergedCap)
	}
	// boundBuf
	if cap(sc.assertBound) < n+1 {
		sc.assertBound = make([]byte, n+1)
	} else {
		sc.assertBound = sc.assertBound[:n+1]
	}
	// assertOut (复用 scratch 缓冲)
	assertCap := len(assertIdxs)
	if assertCap < 1 {
		assertCap = 1
	}
	if cap(sc.assertBatchOutIdx) < assertCap {
		sc.assertBatchOutIdx = make([]int32, assertCap)
	} else {
		sc.assertBatchOutIdx = sc.assertBatchOutIdx[:assertCap]
	}
	assertOutBuf := sc.assertBatchOutIdx

	// assertIdxs C pointer
	var assertPtr *C.int32_t
	if len(assertIdxs) > 0 {
		assertPtr = (*C.int32_t)(unsafe.Pointer(&assertIdxs[0]))
	}

	var mergedTotal int32
	var dptr *C.uint8_t
	dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	var mergedSeenPtr *C.uint8_t
	if len(sc.cseen) > 0 {
		mergedSeenPtr = (*C.uint8_t)(unsafe.Pointer(&sc.cseen[0]))
	}
	var mergedOutPtr *C.int32_t
	if len(sc.cmerged) > 0 {
		mergedOutPtr = (*C.int32_t)(unsafe.Pointer(&sc.cmerged[0]))
	}
	var boundPtr *C.uint8_t
	if len(sc.assertBound) > 0 {
		boundPtr = (*C.uint8_t)(unsafe.Pointer(&sc.assertBound[0]))
	}
	var assertOutPtr *C.int32_t
	if len(assertOutBuf) > 0 {
		assertOutPtr = (*C.int32_t)(unsafe.Pointer(&assertOutBuf[0]))
	}

	assertTotal := int32(C.mvscan_db_combined_scan(k.db, dptr, C.size_t(len(data)),
		mergedSeenPtr, C.int32_t(k.npat),
		mergedOutPtr, C.int32_t(mergedCap),
		(*C.int32_t)(unsafe.Pointer(&mergedTotal)),
		assertPtr, C.int32_t(len(assertIdxs)),
		boundPtr,
		assertOutPtr, C.int32_t(assertCap)))

	keepAlive(data)
	runtime.KeepAlive(sc.cseen)
	runtime.KeepAlive(sc.cmerged)
	runtime.KeepAlive(sc.assertBound)
	runtime.KeepAlive(assertIdxs)
	runtime.KeepAlive(assertOutBuf)

	for i := int32(0); i < mergedTotal && i < int32(mergedCap); i++ {
		sc.mergedHits = append(sc.mergedHits, int(sc.cmerged[i]))
	}
	for i := int32(0); i < assertTotal && i < int32(assertCap); i++ {
		sc.assertHits = append(sc.assertHits, int(assertOutBuf[i]))
	}
	return sc.mergedHits, sc.assertHits
}

// buf 容量须 >= len(data)+1. 返回 buf[:len(data)+1].
func (k *mvsKernel) computeBoundariesC(data []byte, buf []byte) []byte {
	if len(data) == 0 {
		if cap(buf) >= 1 {
			buf = buf[:1]
		} else {
			buf = make([]byte, 1)
		}
	} else {
		if cap(buf) < len(data)+1 {
			buf = make([]byte, len(data)+1)
		} else {
			buf = buf[:len(data)+1]
		}
	}
	if len(data) == 0 {
		// 空输入: 文本始=文本末
		buf[0] = 0x01 | 0x02 | 0x04 | 0x08 | 0x20 // BeginText|EndText|BeginLine|EndLine|NoWordBoundary
		return buf
	}
	var dptr *C.uint8_t
	dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	var bptr *C.uint8_t
	bptr = (*C.uint8_t)(unsafe.Pointer(&buf[0]))
	C.mvscan_compute_boundaries_pub(dptr, C.size_t(len(data)), bptr)
	keepAlive(data)
	runtime.KeepAlive(buf)
	return buf
}

// nfaExistsAnchoredMany 一次 cgo 调用对多条 anchorable pattern 各自做锚定式存在性,
// 摊薄 "每 pattern 一次 cgo" 的跨界开销 (锚定批处理 cgo 次数从 O(触发数) 降到 O(1)).
// idxs[i] 为 pattern 下标, patSpanOff[i+1]-patSpanOff[i] 为其 spans 数, spansLo/Hi 平铺.
// 结果复用 scratch 返回 (len==len(idxs), 1 命中/0 不命中). 由 A/B 门控的 backend
// 把同一报文所有 lean anchored verifier 合并为一次跨界；默认路径仍可保守回退 Go gap-jump。
func (k *mvsKernel) nfaExistsAnchoredMany(idxs []int32, data []byte,
	patSpanOff []int32, spansLo, spansHi []int32, sc *scratch) []byte {
	if k == nil || k.db == nil || len(idxs) == 0 {
		return nil
	}
	if cap(sc.anchorBatchOut) < len(idxs) {
		sc.anchorBatchOut = make([]byte, len(idxs))
	} else {
		sc.anchorBatchOut = sc.anchorBatchOut[:len(idxs)]
	}
	out := sc.anchorBatchOut
	var dptr *C.uint8_t
	if len(data) > 0 {
		dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	var idxPtr *C.int32_t
	if len(idxs) > 0 {
		idxPtr = (*C.int32_t)(unsafe.Pointer(&idxs[0]))
	}
	var offPtr *C.int32_t
	if len(patSpanOff) > 0 {
		offPtr = (*C.int32_t)(unsafe.Pointer(&patSpanOff[0]))
	}
	var loPtr, hiPtr *C.int32_t
	if len(spansLo) > 0 {
		loPtr = (*C.int32_t)(unsafe.Pointer(&spansLo[0]))
		hiPtr = (*C.int32_t)(unsafe.Pointer(&spansHi[0]))
	}
	C.mvscan_db_nfa_exists_anchored_many(k.db, dptr, C.size_t(len(data)),
		idxPtr, C.int32_t(len(idxs)),
		offPtr, loPtr, hiPtr, C.int32_t(len(spansLo)),
		(*C.uint8_t)(unsafe.Pointer(&out[0])))
	keepAlive(data)
	runtime.KeepAlive(idxs)
	runtime.KeepAlive(patSpanOff)
	runtime.KeepAlive(spansLo)
	runtime.KeepAlive(spansHi)
	return out
}

// simdEnabled 报告 C 内核是否编入 SIMD 加速档.
func (k *mvsKernel) simdEnabled() bool { return C.mvscan_simd_enabled() != 0 }

func (k *mvsKernel) nfaExistsImpl(idx int, data []byte, scalar bool) bool {
	if k == nil || k.db == nil {
		return false
	}
	if cgoDiagEnabled {
		atomic.AddInt64(&cgoNfaExistsCalls, 1)
		atomic.AddInt64(&cgoNfaExistsBytes, int64(len(data)))
	}
	var dptr *C.uint8_t
	if len(data) > 0 {
		dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	var r C.int
	if scalar {
		r = C.mvscan_db_nfa_exists_scalar(k.db, C.int32_t(idx), dptr, C.size_t(len(data)))
	} else {
		r = C.mvscan_db_nfa_exists(k.db, C.int32_t(idx), dptr, C.size_t(len(data)))
	}
	keepAlive(data)
	return r == 1
}

// nfaExistsMany 一次 cgo 调用判定 idxs 中每个 pattern 的 per-pattern NFA 是否在 data 命中,
// 结果写入并返回 sc.batchOut (len==len(idxs), 1 命中/0 不命中). 把"每 pattern 一次 cgo"摊薄为
// "每报文一次 cgo" (Phase 2 批处理). idxs 必须都是非断言、有 NFA 的 idx (断言 NFA 不入 C blob).
func (k *mvsKernel) nfaExistsMany(idxs []int32, data []byte, sc *scratch) []byte {
	if k == nil || k.db == nil || len(idxs) == 0 {
		return nil
	}
	if cap(sc.batchOut) < len(idxs) {
		sc.batchOut = make([]byte, len(idxs))
	} else {
		sc.batchOut = sc.batchOut[:len(idxs)]
	}
	var dptr *C.uint8_t
	if len(data) > 0 {
		dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	C.mvscan_db_nfa_exists_many(k.db, dptr, C.size_t(len(data)),
		(*C.int32_t)(unsafe.Pointer(&idxs[0])), C.int32_t(len(idxs)),
		(*C.uint8_t)(unsafe.Pointer(&sc.batchOut[0])))
	keepAlive(data)
	runtime.KeepAlive(idxs)
	return sc.batchOut
}

// mergedScan 单趟扫描 data, 返回合并 always-on 自动机命中的成员 idx (按成员去重, 不触碰
// fullDone; 跨步去重由调用方完成). 复用 sc.cseen / sc.cmerged / sc.mergedHits 缓冲.
func (k *mvsKernel) mergedScan(data []byte, sc *scratch) []int {
	return k.mergedScanImpl(data, sc, false)
}

// mergedScanBatch 批量扫描多条记录 (拼接为一个 buffer), 一次 cgo 调用.
// recOff 长度 nrec+1: 第 i 条 = data[recOff[i]..recOff[i+1]).
// 返回 (recIdx, memberIdx) 对列表.
func (k *mvsKernel) mergedScanBatch(data []byte, recOff []int32, sc *scratch) [][2]int32 {
	if k == nil || k.db == nil || C.mvscan_db_has_merged(k.db) == 0 || len(recOff) <= 1 {
		return nil
	}
	nrec := int32(len(recOff) - 1)
	capPairs := nrec * 8
	if capPairs < 64 {
		capPairs = 64
	}
	if cap(sc.cpairs) < int(capPairs)*2 {
		sc.cpairs = make([]int32, capPairs*2)
	} else {
		sc.cpairs = sc.cpairs[:capPairs*2]
	}
	var dptr *C.uint8_t
	if len(data) > 0 {
		dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	var optr *C.int32_t
	var recOffPtr *C.int32_t
	if len(recOff) > 0 {
		recOffPtr = (*C.int32_t)(unsafe.Pointer(&recOff[0]))
	}
	if len(sc.cpairs) > 0 {
		optr = (*C.int32_t)(unsafe.Pointer(&sc.cpairs[0]))
	}
	got := int32(C.mvscan_db_merged_scan_batch(k.db, dptr, C.size_t(len(data)),
		recOffPtr, C.int(nrec), optr, C.int(capPairs)))
	keepAlive(data)
	runtime.KeepAlive(recOff)
	runtime.KeepAlive(sc.cpairs)
	out := make([][2]int32, 0, got)
	for i := int32(0); i < got; i++ {
		out = append(out, [2]int32{sc.cpairs[i*2], sc.cpairs[i*2+1]})
	}
	return out
}

// mergedScanScalar 强制走 C 标量孪生, 仅供差分测试.
func (k *mvsKernel) mergedScanScalar(data []byte, sc *scratch) []int {
	return k.mergedScanImpl(data, sc, true)
}

func (k *mvsKernel) mergedScanImpl(data []byte, sc *scratch, scalar bool) []int {
	sc.mergedHits = sc.mergedHits[:0]
	if k == nil || k.db == nil || C.mvscan_db_has_merged(k.db) == 0 {
		return sc.mergedHits
	}
	// 去重位图 (长度 npat, 清零).
	if cap(sc.cseen) < k.npat {
		sc.cseen = make([]byte, k.npat)
	} else {
		sc.cseen = sc.cseen[:k.npat]
		for i := range sc.cseen {
			sc.cseen[i] = 0
		}
	}
	// 命中 idx 输出缓冲 (合并成员数 <= npat, 故 npat 容量必不截断; 仍留扩容兜底).
	capOut := k.npat
	if capOut < 8 {
		capOut = 8
	}
	if cap(sc.cmerged) < capOut {
		sc.cmerged = make([]int32, capOut)
	} else {
		sc.cmerged = sc.cmerged[:capOut]
	}

	if cgoDiagEnabled {
		atomic.AddInt64(&cgoMergedCalls, 1)
		atomic.AddInt64(&cgoMergedBytes, int64(len(data)))
	}
	var dptr *C.uint8_t
	if len(data) > 0 {
		dptr = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	var seenPtr *C.uint8_t
	if len(sc.cseen) > 0 {
		seenPtr = (*C.uint8_t)(unsafe.Pointer(&sc.cseen[0]))
	}
	callScan := func() int {
		if scalar {
			return int(C.mvscan_db_merged_scan_scalar(k.db, dptr, C.size_t(len(data)),
				seenPtr, C.int32_t(len(sc.cseen)),
				(*C.int32_t)(unsafe.Pointer(&sc.cmerged[0])), C.int32_t(len(sc.cmerged))))
		}
		return int(C.mvscan_db_merged_scan(k.db, dptr, C.size_t(len(data)),
			seenPtr, C.int32_t(len(sc.cseen)),
			(*C.int32_t)(unsafe.Pointer(&sc.cmerged[0])), C.int32_t(len(sc.cmerged))))
	}
	n := callScan()

	if n > len(sc.cmerged) {
		// 极少: 缓冲不足截断, 清零去重位图后扩容重扫一次.
		for i := range sc.cseen {
			sc.cseen[i] = 0
		}
		sc.cmerged = make([]int32, n)
		n = callScan()
		if n > len(sc.cmerged) {
			n = len(sc.cmerged)
		}
	}
	keepAlive(data)
	for i := 0; i < n; i++ {
		sc.mergedHits = append(sc.mergedHits, int(sc.cmerged[i]))
	}
	return sc.mergedHits
}

// keepAlive 防止传入 C 期间 GC 回收底层数组 (cgo 调用同步, 此处显式标注语义).
func keepAlive(b []byte) { runtime.KeepAlive(b) }
