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
	"sync/atomic"
	"unsafe"
)

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
	blob := buildMVSBlob(d.nfas, d.merged)
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
