//go:build cgo && minirehs_cgo

package minirehs

/*
#cgo CFLAGS: -O3 -Wall

#include <stdint.h>
#include <stddef.h>

typedef struct mrehs_pf mrehs_pf;
mrehs_pf *mrehs_pf_new(const int32_t *next, int32_t numStates,
                       const int32_t *outOff, const int32_t *outFlat,
                       int32_t outFlatLen, int32_t numLit,
                       const uint8_t *litFlat, const int32_t *litOff);
void mrehs_pf_free(mrehs_pf *pf);
int32_t mrehs_pf_scan(const mrehs_pf *pf, const uint8_t *data, size_t len,
                      int32_t *outPairs, int32_t capPairs);
int32_t mrehs_pf_scan_scalar(const mrehs_pf *pf, const uint8_t *data, size_t len,
                             int32_t *outPairs, int32_t capPairs);
int32_t mrehs_pf_use_teddy(const mrehs_pf *pf);
int32_t mrehs_pf_teddy_m(const mrehs_pf *pf);

#include "native/teddy.c"
*/
import "C"

import (
	"unsafe"
)

// simdPrefilterAvailable 在 CGO SIMD 构建下为 true (用于档位标注).
func simdPrefilterAvailable() bool { return true }

// engineTier 是 SIMD 预过滤构建的引擎档位.
const engineTier = 2

// newPrefilter 在 cgo && minirehs_cgo 构建下优先返回 SIMD 预过滤; 构造失败则优雅退化到
// 纯 Go 标量 Aho-Corasick, 保证功能一致.
func newPrefilter(li *literalIndex) prefilter {
	if p := newCGOPrefilter(li); p != nil {
		return p
	}
	return newScalarPrefilter(li)
}

// cgoPrefilter 用 C SIMD 内核扫描 (复用 Go 构建的 AC 表), 输出字面量命中 (含位置).
type cgoPrefilter struct {
	handle *C.mrehs_pf
	li     *literalIndex
}

func newCGOPrefilter(li *literalIndex) *cgoPrefilter {
	if li.empty() {
		return nil
	}
	// 复用纯 Go 的 Aho-Corasick 构建, 得到不可变转移/输出表, 再拷贝进 C 内存.
	b := newACBuilder()
	for id, lit := range li.literals {
		b.add(lit, int32(id))
	}
	ac := b.build(len(li.literals))

	numStates := len(ac.outOff) - 1
	if numStates <= 0 || len(ac.next) == 0 {
		return nil
	}

	// 拼接字面量字节 + 偏移表 (litOff 长度 numLit+1), 供 C 侧 Teddy 构建指纹与 confirm.
	// litID == li.literals 下标 == AC add 时的 id, 三者一致.
	litFlat, litOff := flattenLiterals(li.literals)

	var outFlatPtr *C.int32_t
	if len(ac.outFlat) > 0 {
		outFlatPtr = (*C.int32_t)(unsafe.Pointer(&ac.outFlat[0]))
	}
	var litFlatPtr *C.uint8_t
	if len(litFlat) > 0 {
		litFlatPtr = (*C.uint8_t)(unsafe.Pointer(&litFlat[0]))
	}
	handle := C.mrehs_pf_new(
		(*C.int32_t)(unsafe.Pointer(&ac.next[0])),
		C.int32_t(numStates),
		(*C.int32_t)(unsafe.Pointer(&ac.outOff[0])),
		outFlatPtr,
		C.int32_t(len(ac.outFlat)),
		C.int32_t(ac.numLit),
		litFlatPtr,
		(*C.int32_t)(unsafe.Pointer(&litOff[0])),
	)
	if handle == nil {
		return nil
	}
	return &cgoPrefilter{
		handle: handle,
		li:     li,
	}
}

// flattenLiterals 把字面量集合拼接为连续字节 + 偏移表 (litOff 长度 len+1, litOff[0]=0,
// 第 j 条 = litFlat[litOff[j]:litOff[j+1]]). 供 C 侧 Teddy 零依赖地读取.
func flattenLiterals(lits []string) (litFlat []byte, litOff []int32) {
	litOff = make([]int32, len(lits)+1)
	total := 0
	for _, l := range lits {
		total += len(l)
	}
	litFlat = make([]byte, 0, total)
	for i, l := range lits {
		litFlat = append(litFlat, l...)
		litOff[i+1] = int32(len(litFlat))
	}
	return litFlat, litOff
}

func (p *cgoPrefilter) simd() bool { return true }

// useTeddy / teddyM 暴露 C 侧 Teddy 启用状态与指纹长度 (供测试与可观测).
func (p *cgoPrefilter) useTeddy() bool {
	return p.handle != nil && C.mrehs_pf_use_teddy(p.handle) != 0
}
func (p *cgoPrefilter) teddyM() int {
	if p.handle == nil {
		return 0
	}
	return int(C.mrehs_pf_teddy_m(p.handle))
}

func (p *cgoPrefilter) scanHits(data []byte, sc *scratch) []litHit {
	return p.scanHitsImpl(data, sc, false)
}

// scanHitsScalar 强制走 C 标量孪生 (Teddy 标量 / AC), 仅供差分测试对照 SIMD 分发.
func (p *cgoPrefilter) scanHitsScalar(data []byte, sc *scratch) []litHit {
	return p.scanHitsImpl(data, sc, true)
}

func (p *cgoPrefilter) scanHitsImpl(data []byte, sc *scratch, scalar bool) []litHit {
	sc.hits = sc.hits[:0]
	if p.handle == nil || len(data) == 0 {
		return sc.hits
	}
	lower := asciiLowerInto(data, &sc.lower)

	// cpairs 复用为 (end,litID) 对缓冲, 初值给一个与数据规模相关的容量.
	capPairs := len(data)/8 + 64
	if cap(sc.cpairs) < capPairs*2 {
		sc.cpairs = make([]int32, capPairs*2)
	}
	sc.cpairs = sc.cpairs[:capPairs*2]

	scan := func(cap int) int {
		if scalar {
			return int(C.mrehs_pf_scan_scalar(
				p.handle,
				(*C.uint8_t)(unsafe.Pointer(&lower[0])),
				C.size_t(len(lower)),
				(*C.int32_t)(unsafe.Pointer(&sc.cpairs[0])),
				C.int32_t(cap),
			))
		}
		return int(C.mrehs_pf_scan(
			p.handle,
			(*C.uint8_t)(unsafe.Pointer(&lower[0])),
			C.size_t(len(lower)),
			(*C.int32_t)(unsafe.Pointer(&sc.cpairs[0])),
			C.int32_t(cap),
		))
	}

	got := scan(capPairs)
	// 命中数超过缓冲容量时扩容重扫一次, 保证不漏报.
	if got > capPairs {
		capPairs = got
		sc.cpairs = make([]int32, capPairs*2)
		got = scan(capPairs)
		if got > capPairs {
			got = capPairs
		}
	}

	for i := 0; i < got; i++ {
		sc.hits = append(sc.hits, litHit{
			end:   sc.cpairs[i*2],
			litID: sc.cpairs[i*2+1],
		})
	}
	return sc.hits
}

func (p *cgoPrefilter) release() {
	if p.handle != nil {
		C.mrehs_pf_free(p.handle)
		p.handle = nil
	}
}
