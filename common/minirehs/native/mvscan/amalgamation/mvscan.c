/*
 * mvscan.c - mvscan 纯 C99 运行期内核 (AMALGAMATION 单文件发行, 自动生成, 请勿手改).
 *
 * 本文件由 common/minirehs/tools/amalgamate 从下列源拼接而成 (source of truth):
 *   native/mvscan/mvscan.c   (内核主体)
 *   native/mvscan/mvscan_run.inc            (位并行递推体, 以两套 ROW_* 宏 #include 两次)
 *   native/mvscan/mvscan_run_anchored.inc   (锚定式递推体, 以两套 ROW_* 宏 #include 两次)
 * 公共 API 头随附 mvscan.h (与 native/mvscan/mvscan.h 逐字一致).
 *
 * 用法: 宿主工程仅需此 mvscan.c + mvscan.h 两个文件, 任意 C99 编译器一条命令即可编译,
 * 无需 CMake / 第三方库. 若需修改, 请改 native/mvscan/ 源后重新生成 (有漂移护栏测试).
 *
 * 关键词: mvscan, amalgamation, single file, pure C99, drop-in
 */
/*
 * mvscan.c - mvscan 纯 C99 运行期内核 (标量基线档).
 *
 * 职责: 解析平台无关小端 blob -> rune 级 Glushkov 位并行 NFA; 执行存在性扫描
 * (per-pattern existsIn + 合并 always-on scanExist). 与 Go 参考执行器逐位一致.
 *
 * 平台 / CPU 无关 (见 IMPL 第 8/9 节):
 *   - 纯 C99, 只用 <stdint.h><stddef.h><stdlib.h><string.h>; 不碰 OS API / 线程 / dlopen.
 *   - blob 显式小端解码, 任意字节序架构正确; 解析期拷入对齐内存, 无对齐假设.
 *   - 当前为标量执行 (任意架构可编可跑). SIMD 加速档 (M3) 以函数指针分发叠加, 配标量孪生.
 *
 * 正确性基准: Go 侧 mvs_exec.go (existsIn) 与 mvs_merged.go (scanExist) 为可执行规格,
 * 由差分测试 (随机任意字节 + 真实流量) 逐位对照. rune 解码严格复刻 Go unicode/utf8.DecodeRune.
 *
 * 关键词: mvscan, bit-parallel NFA, Glushkov, utf8 decode, RuneError, alphabet compression
 */

#include "mvscan.h"

#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>

/* M3: SIMD 加速档. x86_64 用 SSE2 (基线必有), arm64 用 NEON (基线必有), 故无需运行期探测;
 * 其它架构走标量孪生. 仅用于 nword>=2 的字向量 OR/AND/COPY (单字 nword==1 走标量更快). */
#if defined(__x86_64__) || defined(_M_X64)
#include <emmintrin.h>
#define MVS_SSE2 1
#elif defined(__aarch64__) || defined(_M_ARM64)
#include <arm_neon.h>
#define MVS_NEON 1
#endif

/* ---- ctz64: 取最低置位下标 (跨编译器). ---- */
#if defined(_MSC_VER)
#include <intrin.h>
static inline int mvs_ctz64(uint64_t x) {
    unsigned long idx;
#if defined(_M_X64) || defined(_M_ARM64)
    _BitScanForward64(&idx, x);
    return (int)idx;
#else
    if ((uint32_t)x) { _BitScanForward(&idx, (uint32_t)x); return (int)idx; }
    _BitScanForward(&idx, (uint32_t)(x >> 32)); return (int)idx + 32;
#endif
}
#elif defined(__GNUC__) || defined(__clang__)
static inline int mvs_ctz64(uint64_t x) { return __builtin_ctzll(x); }
#else
static inline int mvs_ctz64(uint64_t x) {
    int n = 0;
    while ((x & 1ull) == 0) { x >>= 1; n++; }
    return n;
}
#endif

/* ---- 小端读取 helper (与机器字节序无关). ---- */
static inline uint32_t le_u32(const uint8_t *p) {
    return (uint32_t)p[0] | ((uint32_t)p[1] << 8) |
           ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}
static inline int32_t le_i32(const uint8_t *p) { return (int32_t)le_u32(p); }
static inline uint64_t le_u64(const uint8_t *p) {
    return (uint64_t)le_u32(p) | ((uint64_t)le_u32(p + 4) << 32);
}

/* ====================================================================
 * UTF-8 解码: 严格复刻 Go 标准库 unicode/utf8.DecodeRune.
 * 非法编码返回 (RuneError=0xFFFD, size=1); 与 Go regexp 逐 rune 语义一致.
 * 表 (first / acceptRanges) 取自 Go 源码, 逐字节移植.
 * ==================================================================== */

#define MVS_RUNE_ERROR 0xFFFD
#define MVS_MAX_RUNE 0x10FFFF

#define MVS_LOCB 0x80
#define MVS_HICB 0xBF

/* first[b]: 高 nibble 是 acceptRanges 下标或 F (单字节特例), 低 nibble 是长度 / 状态. */
static const uint8_t mvs_utf8_first[256] = {
    /* 0x00-0x7F: ASCII (as=0xF0) */
    0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,
    0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,
    0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,
    0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,
    0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,
    0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,
    0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,
    0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,0xF0,
    /* 0x80-0xBF: 连续字节作为首字节 -> 非法 (xx=0xF1) */
    0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,
    0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,
    0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,
    0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,
    /* 0xC0-0xDF: 2 字节 (0xC0,0xC1 非法) */
    0xF1,0xF1,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,
    0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,0x02,
    /* 0xE0-0xEF: 3 字节 (s2=0x13,s3=0x03,s4=0x23) */
    0x13,0x03,0x03,0x03,0x03,0x03,0x03,0x03,0x03,0x03,0x03,0x03,0x03,0x23,0x03,0x03,
    /* 0xF0-0xFF: 4 字节 (s5=0x34,s6=0x04,s7=0x44) 其余非法 */
    0x34,0x04,0x04,0x04,0x44,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,0xF1,
};

/* acceptRanges[i] = {lo, hi}: 第二字节的合法范围. */
static const uint8_t mvs_accept_lo[5] = { MVS_LOCB, 0xA0, MVS_LOCB, 0x90, MVS_LOCB };
static const uint8_t mvs_accept_hi[5] = { MVS_HICB, MVS_HICB, 0x9F, MVS_HICB, 0x8F };

/* mvs_decode_rune 解码 p[0..len) 的首个 rune, 写入 *rOut, 返回字节长度 (>=1). */
static inline int mvs_decode_rune(const uint8_t *p, size_t len, int32_t *rOut) {
    if (len < 1) { *rOut = MVS_RUNE_ERROR; return 0; }
    uint8_t p0 = p[0];
    uint8_t x = mvs_utf8_first[p0];
    if (x >= 0xF0) { /* as(ASCII) 或 xx(非法): 单字节 */
        /* x==0xF0 -> ASCII, 返回 p0; x==0xF1 -> 非法, 返回 RuneError. */
        if (x == 0xF0) { *rOut = (int32_t)p0; return 1; }
        *rOut = MVS_RUNE_ERROR; return 1;
    }
    int sz = (int)(x & 7);
    int ai = (int)(x >> 4);
    uint8_t accLo = mvs_accept_lo[ai];
    uint8_t accHi = mvs_accept_hi[ai];
    if ((int)len < sz) { *rOut = MVS_RUNE_ERROR; return 1; }
    uint8_t b1 = p[1];
    if (b1 < accLo || accHi < b1) { *rOut = MVS_RUNE_ERROR; return 1; }
    if (sz <= 2) {
        *rOut = ((int32_t)(p0 & 0x1F) << 6) | (int32_t)(b1 & 0x3F);
        return 2;
    }
    uint8_t b2 = p[2];
    if (b2 < MVS_LOCB || MVS_HICB < b2) { *rOut = MVS_RUNE_ERROR; return 1; }
    if (sz <= 3) {
        *rOut = ((int32_t)(p0 & 0x0F) << 12) | ((int32_t)(b1 & 0x3F) << 6) |
                (int32_t)(b2 & 0x3F);
        return 3;
    }
    uint8_t b3 = p[3];
    if (b3 < MVS_LOCB || MVS_HICB < b3) { *rOut = MVS_RUNE_ERROR; return 1; }
    *rOut = ((int32_t)(p0 & 0x07) << 18) | ((int32_t)(b1 & 0x3F) << 12) |
            ((int32_t)(b2 & 0x3F) << 6) | (int32_t)(b3 & 0x3F);
    return 4;
}

/* ====================================================================
 * NFA 结构 (从 blob 解析, 拥有对齐内存). per-pattern 与 merged 共用一个结构:
 * per-pattern 把 firstUnanchored/firstAnchored 由 anchoredStart 分桶填充, posPat 不使用;
 * merged 用全局 posPat 把命中位置映射到成员 idx.
 * ==================================================================== */
typedef struct {
    int32_t npos;
    int32_t nword;
    int32_t nsym;
    int     hasAnchored;       /* 是否有锚定 first (输入起点才注入) */
    int     unanchoredEmpty;   /* firstUnanchored 全零 (可提前停的判据) */

    uint64_t *firstUnanchored; /* [nword] */
    uint64_t *firstAnchored;   /* [nword] */
    uint64_t *lastAny;         /* [nword] */
    uint64_t *lastEnd;         /* [nword] */
    uint64_t *follow;          /* [npos*nword] */
    uint64_t *reach;           /* [nsym*nword] */

    int32_t *cuts;             /* [nsym+1] */
    int32_t *asciiSym;         /* [128] */
    int32_t *posPat;           /* [npos] */
} mvs_nfa;

struct mvscan_db {
    int32_t  npat;
    int32_t  nUnits;
    mvs_nfa *units;       /* [nUnits] 解析后的 NFA */
    int32_t *slotUnit;    /* [npat] pattern idx -> unit 下标 (-1 无 NFA) */
    int32_t  mergedUnit;  /* 合并自动机的 unit 下标 (-1 无) */
};

/* sym_index: 复刻 Go symIndex (cuts[i] <= r < cuts[i+1], 边界 clamp). ncuts = nsym+1. */
static inline int sym_index(const int32_t *cuts, int ncuts, int32_t r) {
    int lo = 0, hi = ncuts; /* 找第一个 cuts[k] > r */
    while (lo < hi) {
        int mid = lo + ((hi - lo) >> 1);
        if (cuts[mid] > r) hi = mid; else lo = mid + 1;
    }
    int i = lo - 1;
    if (i < 0) i = 0;
    if (i > ncuts - 2) i = ncuts - 2;
    return i;
}

/* symbol_of: 复刻 Go mvsNFA.symbolOf. */
static inline int symbol_of(const mvs_nfa *a, int32_t r) {
    if (r >= 0 && r < 128) return (int)a->asciiSym[r];
    if (r > MVS_MAX_RUNE) r = MVS_RUNE_ERROR;
    return sym_index(a->cuts, a->nsym + 1, r);
}

/* ====================================================================
 * 位并行递推核心. mode: 0 = 存在性 (命中即返回 1); 1 = 合并 scan (发射 posPat).
 * 与 Go existsIn / scanExist 同一递推:
 *   cand = firstUnanchored | (atStart? firstAnchored : 0) | OR(follow[p] for p in prev)
 *   active = cand & reach[sym]
 *   acc = active & lastAny; if atEnd: acc |= active & lastEnd
 *
 * 字向量 OR/AND/COPY 抽为 ROW_* 宏, 由 mvscan_run.inc 生成标量孪生与 SIMD 两份 (见下).
 * ==================================================================== */

/* 标量字向量原语 (孪生基线, 任意架构). */
static inline void row_copy_s(uint64_t *d, const uint64_t *s, int n) {
    for (int w = 0; w < n; w++) d[w] = s[w];
}
static inline void row_or_s(uint64_t *d, const uint64_t *s, int n) {
    for (int w = 0; w < n; w++) d[w] |= s[w];
}
static inline void row_and_s(uint64_t *d, const uint64_t *x, const uint64_t *y, int n) {
    for (int w = 0; w < n; w++) d[w] = x[w] & y[w];
}
static inline void row_zero_s(uint64_t *d, int n) {
    for (int w = 0; w < n; w++) d[w] = 0;
}

/* SIMD 字向量原语 (一次处理 2 个 uint64 = 128 位, 尾字标量). */
#if defined(MVS_SSE2)
static inline void row_copy_v(uint64_t *d, const uint64_t *s, int n) {
    int w = 0;
    for (; w + 2 <= n; w += 2)
        _mm_storeu_si128((__m128i *)(d + w), _mm_loadu_si128((const __m128i *)(s + w)));
    for (; w < n; w++) d[w] = s[w];
}
static inline void row_or_v(uint64_t *d, const uint64_t *s, int n) {
    int w = 0;
    for (; w + 2 <= n; w += 2) {
        __m128i a = _mm_loadu_si128((const __m128i *)(d + w));
        __m128i b = _mm_loadu_si128((const __m128i *)(s + w));
        _mm_storeu_si128((__m128i *)(d + w), _mm_or_si128(a, b));
    }
    for (; w < n; w++) d[w] |= s[w];
}
static inline void row_and_v(uint64_t *d, const uint64_t *x, const uint64_t *y, int n) {
    int w = 0;
    for (; w + 2 <= n; w += 2) {
        __m128i a = _mm_loadu_si128((const __m128i *)(x + w));
        __m128i b = _mm_loadu_si128((const __m128i *)(y + w));
        _mm_storeu_si128((__m128i *)(d + w), _mm_and_si128(a, b));
    }
    for (; w < n; w++) d[w] = x[w] & y[w];
}
static inline void row_zero_v(uint64_t *d, int n) {
    int w = 0;
    for (; w + 2 <= n; w += 2)
        _mm_storeu_si128((__m128i *)(d + w), _mm_setzero_si128());
    for (; w < n; w++) d[w] = 0;
}
#elif defined(MVS_NEON)
static inline void row_copy_v(uint64_t *d, const uint64_t *s, int n) {
    int w = 0;
    for (; w + 2 <= n; w += 2) vst1q_u64(d + w, vld1q_u64(s + w));
    for (; w < n; w++) d[w] = s[w];
}
static inline void row_or_v(uint64_t *d, const uint64_t *s, int n) {
    int w = 0;
    for (; w + 2 <= n; w += 2) vst1q_u64(d + w, vorrq_u64(vld1q_u64(d + w), vld1q_u64(s + w)));
    for (; w < n; w++) d[w] |= s[w];
}
static inline void row_and_v(uint64_t *d, const uint64_t *x, const uint64_t *y, int n) {
    int w = 0;
    for (; w + 2 <= n; w += 2) vst1q_u64(d + w, vandq_u64(vld1q_u64(x + w), vld1q_u64(y + w)));
    for (; w < n; w++) d[w] = x[w] & y[w];
}
static inline void row_zero_v(uint64_t *d, int n) {
    int w = 0;
    for (; w + 2 <= n; w += 2) vst1q_u64(d + w, vdupq_n_u64(0));
    for (; w < n; w++) d[w] = 0;
}
#endif

/* mvs_rune_start: 复刻 Go alignRuneStart — 把字节偏移 off 向左吸附到最近的 rune 起始.
 * 共享 bound 仅在 rune 起始与末尾处写入真实值, 故注入区间端点须 rune 对齐. */
static inline size_t mvs_rune_start(const uint8_t *data, size_t len, size_t off) {
    if (off <= 0) return 0;
    if (off >= len) return len;
    while (off > 0) {
        /* UTF-8 continuation bytes have (b & 0xC0) == 0x80; rune start otherwise. */
        if ((data[off] & 0xC0) != 0x80) break;
        off--;
    }
    return off;
}

/* 标量孪生: nfa_run_scalar. */
#define NFA_RUN_NAME nfa_run_scalar
#define ROW_COPY row_copy_s
#define ROW_OR row_or_s
#define ROW_AND row_and_s
/* >>> begin inlined mvscan_run.inc (copy 1) >>> */
/*
 * mvscan_run.inc - 位并行 Glushkov 递推体 (被 mvscan.c 以不同 ROW_* 宏 #include 两次).
 *
 * 生成两份逻辑同源的实现: 标量孪生 (nfa_run_scalar) 与 SIMD 档 (nfa_run_simd), 二者仅
 * 字向量 OR/AND/COPY 的实现不同 (其余 rune 解码 / 接受收集 / 提前停判据完全一致), 由 nfa_run
 * 按 nword 与架构分发. 差分测试逐位对照两者 (mvscan_db_*_scalar vs 默认分发), 必恒等.
 *
 * 不要单独编译本文件; 它依赖包含处的 mvs_nfa / mvs_decode_rune / symbol_of / mvs_ctz64
 * 及 ROW_COPY / ROW_OR / ROW_AND / NFA_RUN_NAME 宏.
 */
static int NFA_RUN_NAME(const mvs_nfa *a, const uint8_t *data, size_t len, int mode,
                        uint8_t *seen, int32_t seenLen, int32_t *out, int32_t cap,
                        int32_t *outTotal) {
    int nword = a->nword;
    uint64_t stackPrev[8], stackCand[8];
    uint64_t *prev, *cand;
    if (nword <= 8) {
        prev = stackPrev; cand = stackCand;
        memset(prev, 0, (size_t)nword * sizeof(uint64_t));
    } else {
        prev = (uint64_t *)calloc((size_t)nword, sizeof(uint64_t));
        cand = (uint64_t *)malloc((size_t)nword * sizeof(uint64_t));
        if (!prev || !cand) { free(prev); free(cand); return 0; }
    }

    int total = 0;
    int found = 0;
    size_t i = 0;
    while (i < len) {
        int atStart = (i == 0);
        /* ASCII 快路径: 单字节 (<0x80) 直接查 asciiSym, 省去 mvs_decode_rune 的
         * mvs_utf8_first 表查 + 多级分支 + symbol_of 调用 (语料绝大多数为 ASCII).
         * 与 mvs_decode_rune(ASCII)->symbol_of 逐位等价: ASCII 字节解码为自身, asciiSym[c]
         * 即其符号. 非 ASCII 才回退完整 UTF-8 解码 + 切点查找. */
        int sym;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            i += 1;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            i += (size_t)size;
            sym = symbol_of(a, r);
        }
        int atEnd = (i == len);

        /* cand = firstUnanchored (每步注入) | firstAnchored (仅起点). */
        ROW_COPY(cand, a->firstUnanchored, nword);
        if (atStart && a->hasAnchored) ROW_OR(cand, a->firstAnchored, nword);

        /* 后继并集: OR follow[p] for p in prev (读完旧 prev 后才覆写). */
        for (int w = 0; w < nword; w++) {
            uint64_t pw = prev[w];
            while (pw) {
                int p = (w << 6) + mvs_ctz64(pw);
                pw &= pw - 1;
                ROW_OR(cand, a->follow + (size_t)p * nword, nword);
            }
        }

        /* active = cand & reach[sym]; 写入 prev. */
        const uint64_t *rc = a->reach + (size_t)sym * nword;
        ROW_AND(prev, cand, rc, nword);

        uint64_t anyActive = 0;
        for (int w = 0; w < nword; w++) {
            uint64_t v = prev[w];
            anyActive |= v;
            uint64_t acc = v & a->lastAny[w];
            if (atEnd) acc |= v & a->lastEnd[w];
            if (acc) {
                if (mode == 0) { found = 1; goto done; }
                while (acc) {
                    int p = (w << 6) + mvs_ctz64(acc);
                    acc &= acc - 1;
                    int32_t id = a->posPat[p];
                    if (id >= 0 && id < seenLen && !seen[id]) {
                        seen[id] = 1;
                        if (total < cap) out[total] = id;
                        total++;
                    }
                }
            }
        }
        if (anyActive == 0 && a->unanchoredEmpty) break;
    }

done:
    if (nword > 8) { free(prev); free(cand); }
    if (outTotal) *outTotal = total;
    return found;
}
/* <<< end inlined mvscan_run.inc (copy 1) <<< */
#undef NFA_RUN_NAME
#undef ROW_COPY
#undef ROW_OR
#undef ROW_AND

/* SIMD 档: nfa_run_simd (仅在有 SSE2/NEON 时生成). */
#if defined(MVS_SSE2) || defined(MVS_NEON)
#define NFA_RUN_NAME nfa_run_simd
#define ROW_COPY row_copy_v
#define ROW_OR row_or_v
#define ROW_AND row_and_v
/* >>> begin inlined mvscan_run.inc (copy 2) >>> */
/*
 * mvscan_run.inc - 位并行 Glushkov 递推体 (被 mvscan.c 以不同 ROW_* 宏 #include 两次).
 *
 * 生成两份逻辑同源的实现: 标量孪生 (nfa_run_scalar) 与 SIMD 档 (nfa_run_simd), 二者仅
 * 字向量 OR/AND/COPY 的实现不同 (其余 rune 解码 / 接受收集 / 提前停判据完全一致), 由 nfa_run
 * 按 nword 与架构分发. 差分测试逐位对照两者 (mvscan_db_*_scalar vs 默认分发), 必恒等.
 *
 * 不要单独编译本文件; 它依赖包含处的 mvs_nfa / mvs_decode_rune / symbol_of / mvs_ctz64
 * 及 ROW_COPY / ROW_OR / ROW_AND / NFA_RUN_NAME 宏.
 */
static int NFA_RUN_NAME(const mvs_nfa *a, const uint8_t *data, size_t len, int mode,
                        uint8_t *seen, int32_t seenLen, int32_t *out, int32_t cap,
                        int32_t *outTotal) {
    int nword = a->nword;
    uint64_t stackPrev[8], stackCand[8];
    uint64_t *prev, *cand;
    if (nword <= 8) {
        prev = stackPrev; cand = stackCand;
        memset(prev, 0, (size_t)nword * sizeof(uint64_t));
    } else {
        prev = (uint64_t *)calloc((size_t)nword, sizeof(uint64_t));
        cand = (uint64_t *)malloc((size_t)nword * sizeof(uint64_t));
        if (!prev || !cand) { free(prev); free(cand); return 0; }
    }

    int total = 0;
    int found = 0;
    size_t i = 0;
    while (i < len) {
        int atStart = (i == 0);
        /* ASCII 快路径: 单字节 (<0x80) 直接查 asciiSym, 省去 mvs_decode_rune 的
         * mvs_utf8_first 表查 + 多级分支 + symbol_of 调用 (语料绝大多数为 ASCII).
         * 与 mvs_decode_rune(ASCII)->symbol_of 逐位等价: ASCII 字节解码为自身, asciiSym[c]
         * 即其符号. 非 ASCII 才回退完整 UTF-8 解码 + 切点查找. */
        int sym;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            i += 1;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            i += (size_t)size;
            sym = symbol_of(a, r);
        }
        int atEnd = (i == len);

        /* cand = firstUnanchored (每步注入) | firstAnchored (仅起点). */
        ROW_COPY(cand, a->firstUnanchored, nword);
        if (atStart && a->hasAnchored) ROW_OR(cand, a->firstAnchored, nword);

        /* 后继并集: OR follow[p] for p in prev (读完旧 prev 后才覆写). */
        for (int w = 0; w < nword; w++) {
            uint64_t pw = prev[w];
            while (pw) {
                int p = (w << 6) + mvs_ctz64(pw);
                pw &= pw - 1;
                ROW_OR(cand, a->follow + (size_t)p * nword, nword);
            }
        }

        /* active = cand & reach[sym]; 写入 prev. */
        const uint64_t *rc = a->reach + (size_t)sym * nword;
        ROW_AND(prev, cand, rc, nword);

        uint64_t anyActive = 0;
        for (int w = 0; w < nword; w++) {
            uint64_t v = prev[w];
            anyActive |= v;
            uint64_t acc = v & a->lastAny[w];
            if (atEnd) acc |= v & a->lastEnd[w];
            if (acc) {
                if (mode == 0) { found = 1; goto done; }
                while (acc) {
                    int p = (w << 6) + mvs_ctz64(acc);
                    acc &= acc - 1;
                    int32_t id = a->posPat[p];
                    if (id >= 0 && id < seenLen && !seen[id]) {
                        seen[id] = 1;
                        if (total < cap) out[total] = id;
                        total++;
                    }
                }
            }
        }
        if (anyActive == 0 && a->unanchoredEmpty) break;
    }

done:
    if (nword > 8) { free(prev); free(cand); }
    if (outTotal) *outTotal = total;
    return found;
}
/* <<< end inlined mvscan_run.inc (copy 2) <<< */
#undef NFA_RUN_NAME
#undef ROW_COPY
#undef ROW_OR
#undef ROW_AND
#endif

/* ====================================================================
 * 锚定式位并行递推 (对应 Go mvs_anchored.go existsInAnchored).
 * 仅在 spans 注入区间内注入 first, 其余位置不注入, 支持提前消亡.
 * 标量孪生 + SIMD 档 (nword>=2 走 SIMD), 与 Go 逐位一致 (差分护栏).
 * ==================================================================== */

/* 标量孪生: nfa_run_anchored_scalar. */
#define NFA_RUN_ANCHORED_NAME nfa_run_anchored_scalar
#define ROW_COPY row_copy_s
#define ROW_OR row_or_s
#define ROW_AND row_and_s
#define ROW_ZERO row_zero_s
/* >>> begin inlined mvscan_run_anchored.inc (copy 1) >>> */
/*
 * mvscan_run_anchored.inc - 锚定式位并行 Glushkov 递推体 (被 mvscan.c 以不同 ROW_* 宏
 * #include 两次, 生成标量孪生 nfa_run_anchored_scalar 与 SIMD 档 nfa_run_anchored_simd).
 *
 * 与 mvscan_run.inc 的区别 (对应 Go mvs_anchored.go existsInAnchored / existsInAnchored1):
 *   - 不再每步注入 firstUnanchored, 而是仅在 runeStart 落入某 span [lo,hi) 时注入 first.
 *   - firstAnchored 不使用 (锚定式 pattern 均为 !anchoredStart, 故 firstAnchored 为零).
 *   - 提前消亡: 活跃集空且已越过所有注入区间 (runeStart >= lastHi) => 不可能再命中, 立即返回.
 *
 * 正确性与 Go existsInAnchored 同源: 任一匹配 M 必含某必需字面量, 其命中结束于 h.end, 则
 * M.start >= h.end - head_L => M.start 落入某注入区间 [h.end-head_L, h.end] => 锚定式必能
 * 从该起点找到 M (无假阴). 区间外不注入 => 只会找到真实起点的匹配 (无假阳).
 *
 * 不要单独编译本文件; 它依赖包含处的 mvs_nfa / mvs_decode_rune / symbol_of / mvs_ctz64
 * / mvs_rune_start / ROW_COPY / ROW_OR / ROW_AND / NFA_RUN_ANCHORED_NAME 宏.
 */
static int NFA_RUN_ANCHORED_NAME(const mvs_nfa *a, const uint8_t *data, size_t len,
                                 const int32_t *loArr, const int32_t *hiArr, int32_t nspan) {
    if (nspan <= 0) return 0;
    int nword = a->nword;
    uint64_t stackPrev[8], stackCand[8];
    uint64_t *prev, *cand;
    if (nword <= 8) {
        prev = stackPrev; cand = stackCand;
        memset(prev, 0, (size_t)nword * sizeof(uint64_t));
    } else {
        prev = (uint64_t *)calloc((size_t)nword, sizeof(uint64_t));
        cand = (uint64_t *)malloc((size_t)nword * sizeof(uint64_t));
        if (!prev || !cand) { free(prev); free(cand); return 0; }
    }

    int found = 0;
    int32_t lastHi = hiArr[nspan - 1];
    int32_t si = 0;
    /* 起始位置: 对齐到 rune 起始 (与 Go alignRuneStart 一致). */
    size_t i = (size_t)mvs_rune_start(data, len, (size_t)loArr[0]);
    int hasActive = 0;

    while (i < len) {
        size_t runeStart = i;
        int sym;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            i += 1;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            i += (size_t)size;
            sym = symbol_of(a, r);
        }
        int atEnd = (i == len);

        /* 推进 si 到第一个 hi > runeStart 的 span (跳过已过去的区间). */
        while (si < nspan && (size_t)hiArr[si] <= runeStart) si++;
        int inject = (si < nspan && (size_t)loArr[si] <= runeStart);

        if (inject) {
            ROW_COPY(cand, a->firstUnanchored, nword);
        } else {
            ROW_ZERO(cand, nword);
        }
        if (hasActive) {
            for (int w = 0; w < nword; w++) {
                uint64_t pw = prev[w];
                while (pw) {
                    int p = (w << 6) + mvs_ctz64(pw);
                    pw &= pw - 1;
                    ROW_OR(cand, a->follow + (size_t)p * nword, nword);
                }
            }
        }

        const uint64_t *rc = a->reach + (size_t)sym * nword;
        ROW_AND(prev, cand, rc, nword);

        uint64_t anyActive = 0;
        for (int w = 0; w < nword; w++) {
            uint64_t v = prev[w];
            anyActive |= v;
            uint64_t acc = v & a->lastAny[w];
            if (atEnd) acc |= v & a->lastEnd[w];
            if (acc) { found = 1; goto done; }
        }

        hasActive = (anyActive != 0);
        if (!hasActive && (si >= nspan || (size_t)lastHi <= runeStart)) {
            /* 提前消亡: 活跃集空且已越过所有注入区间 => 不可能再命中. */
            goto done;
        }
        /* 大空洞跳跃与 Go existsInAnchored 保持同一策略：活跃集已经消亡时，
         * 下一个 literal 注入区间之前的 rune 不可能影响任何状态。直接跳到该
         * span 的 rune 起点，避免 C 批处理在稀疏触发的长报文上无意义地扫完整个洞。
         * 32 与 Go gapJumpMin 一致；mvs_rune_start 向左对齐且只在 jump>i 时采用，
         * 因而不会破坏前向单调性。 */
        if (!hasActive && si < nspan && (size_t)loArr[si] > runeStart + 32) {
            size_t jump = mvs_rune_start(data, len, (size_t)loArr[si]);
            if (jump > i) {
                i = jump;
                continue;
            }
        }
    }

done:
    if (nword > 8) { free(prev); free(cand); }
    return found;
}
/* <<< end inlined mvscan_run_anchored.inc (copy 1) <<< */
#undef NFA_RUN_ANCHORED_NAME
#undef ROW_COPY
#undef ROW_OR
#undef ROW_AND
#undef ROW_ZERO

/* SIMD 档: nfa_run_anchored_simd. */
#if defined(MVS_SSE2) || defined(MVS_NEON)
#define NFA_RUN_ANCHORED_NAME nfa_run_anchored_simd
#define ROW_COPY row_copy_v
#define ROW_OR row_or_v
#define ROW_AND row_and_v
#define ROW_ZERO row_zero_v
/* >>> begin inlined mvscan_run_anchored.inc (copy 2) >>> */
/*
 * mvscan_run_anchored.inc - 锚定式位并行 Glushkov 递推体 (被 mvscan.c 以不同 ROW_* 宏
 * #include 两次, 生成标量孪生 nfa_run_anchored_scalar 与 SIMD 档 nfa_run_anchored_simd).
 *
 * 与 mvscan_run.inc 的区别 (对应 Go mvs_anchored.go existsInAnchored / existsInAnchored1):
 *   - 不再每步注入 firstUnanchored, 而是仅在 runeStart 落入某 span [lo,hi) 时注入 first.
 *   - firstAnchored 不使用 (锚定式 pattern 均为 !anchoredStart, 故 firstAnchored 为零).
 *   - 提前消亡: 活跃集空且已越过所有注入区间 (runeStart >= lastHi) => 不可能再命中, 立即返回.
 *
 * 正确性与 Go existsInAnchored 同源: 任一匹配 M 必含某必需字面量, 其命中结束于 h.end, 则
 * M.start >= h.end - head_L => M.start 落入某注入区间 [h.end-head_L, h.end] => 锚定式必能
 * 从该起点找到 M (无假阴). 区间外不注入 => 只会找到真实起点的匹配 (无假阳).
 *
 * 不要单独编译本文件; 它依赖包含处的 mvs_nfa / mvs_decode_rune / symbol_of / mvs_ctz64
 * / mvs_rune_start / ROW_COPY / ROW_OR / ROW_AND / NFA_RUN_ANCHORED_NAME 宏.
 */
static int NFA_RUN_ANCHORED_NAME(const mvs_nfa *a, const uint8_t *data, size_t len,
                                 const int32_t *loArr, const int32_t *hiArr, int32_t nspan) {
    if (nspan <= 0) return 0;
    int nword = a->nword;
    uint64_t stackPrev[8], stackCand[8];
    uint64_t *prev, *cand;
    if (nword <= 8) {
        prev = stackPrev; cand = stackCand;
        memset(prev, 0, (size_t)nword * sizeof(uint64_t));
    } else {
        prev = (uint64_t *)calloc((size_t)nword, sizeof(uint64_t));
        cand = (uint64_t *)malloc((size_t)nword * sizeof(uint64_t));
        if (!prev || !cand) { free(prev); free(cand); return 0; }
    }

    int found = 0;
    int32_t lastHi = hiArr[nspan - 1];
    int32_t si = 0;
    /* 起始位置: 对齐到 rune 起始 (与 Go alignRuneStart 一致). */
    size_t i = (size_t)mvs_rune_start(data, len, (size_t)loArr[0]);
    int hasActive = 0;

    while (i < len) {
        size_t runeStart = i;
        int sym;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            i += 1;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            i += (size_t)size;
            sym = symbol_of(a, r);
        }
        int atEnd = (i == len);

        /* 推进 si 到第一个 hi > runeStart 的 span (跳过已过去的区间). */
        while (si < nspan && (size_t)hiArr[si] <= runeStart) si++;
        int inject = (si < nspan && (size_t)loArr[si] <= runeStart);

        if (inject) {
            ROW_COPY(cand, a->firstUnanchored, nword);
        } else {
            ROW_ZERO(cand, nword);
        }
        if (hasActive) {
            for (int w = 0; w < nword; w++) {
                uint64_t pw = prev[w];
                while (pw) {
                    int p = (w << 6) + mvs_ctz64(pw);
                    pw &= pw - 1;
                    ROW_OR(cand, a->follow + (size_t)p * nword, nword);
                }
            }
        }

        const uint64_t *rc = a->reach + (size_t)sym * nword;
        ROW_AND(prev, cand, rc, nword);

        uint64_t anyActive = 0;
        for (int w = 0; w < nword; w++) {
            uint64_t v = prev[w];
            anyActive |= v;
            uint64_t acc = v & a->lastAny[w];
            if (atEnd) acc |= v & a->lastEnd[w];
            if (acc) { found = 1; goto done; }
        }

        hasActive = (anyActive != 0);
        if (!hasActive && (si >= nspan || (size_t)lastHi <= runeStart)) {
            /* 提前消亡: 活跃集空且已越过所有注入区间 => 不可能再命中. */
            goto done;
        }
        /* 大空洞跳跃与 Go existsInAnchored 保持同一策略：活跃集已经消亡时，
         * 下一个 literal 注入区间之前的 rune 不可能影响任何状态。直接跳到该
         * span 的 rune 起点，避免 C 批处理在稀疏触发的长报文上无意义地扫完整个洞。
         * 32 与 Go gapJumpMin 一致；mvs_rune_start 向左对齐且只在 jump>i 时采用，
         * 因而不会破坏前向单调性。 */
        if (!hasActive && si < nspan && (size_t)loArr[si] > runeStart + 32) {
            size_t jump = mvs_rune_start(data, len, (size_t)loArr[si]);
            if (jump > i) {
                i = jump;
                continue;
            }
        }
    }

done:
    if (nword > 8) { free(prev); free(cand); }
    return found;
}
/* <<< end inlined mvscan_run_anchored.inc (copy 2) <<< */
#undef NFA_RUN_ANCHORED_NAME
#undef ROW_COPY
#undef ROW_OR
#undef ROW_AND
#undef ROW_ZERO
#endif

/* nfa_run_anchored 分发: nword>=2 且有 SIMD 档时走 SIMD, 否则标量孪生. */
static int nfa_run_anchored_dispatch(const mvs_nfa *a, const uint8_t *data, size_t len,
                                     const int32_t *lo, const int32_t *hi, int32_t nspan,
                                     int forceScalar) {
#if defined(MVS_SSE2) || defined(MVS_NEON)
    if (!forceScalar && a->nword >= 2)
        return nfa_run_anchored_simd(a, data, len, lo, hi, nspan);
#else
    (void)forceScalar;
#endif
    return nfa_run_anchored_scalar(a, data, len, lo, hi, nspan);
}

/* nfa_run 分发: nword>=2 且有 SIMD 档时走 SIMD (一次 2 字), 否则走标量孪生.
 * forceScalar!=0 强制标量 (供差分测试对照 SIMD 与标量逐位一致). */
static int nfa_run_dispatch(const mvs_nfa *a, const uint8_t *data, size_t len, int mode,
                            uint8_t *seen, int32_t seenLen, int32_t *out, int32_t cap,
                            int32_t *outTotal, int forceScalar) {
#if defined(MVS_SSE2) || defined(MVS_NEON)
    if (!forceScalar && a->nword >= 2)
        return nfa_run_simd(a, data, len, mode, seen, seenLen, out, cap, outTotal);
#else
    (void)forceScalar;
#endif
    return nfa_run_scalar(a, data, len, mode, seen, seenLen, out, cap, outTotal);
}

static inline int nfa_run(const mvs_nfa *a, const uint8_t *data, size_t len, int mode,
                          uint8_t *seen, int32_t seenLen, int32_t *out, int32_t cap,
                          int32_t *outTotal) {
    return nfa_run_dispatch(a, data, len, mode, seen, seenLen, out, cap, outTotal, 0);
}

/* ====================================================================
 * blob 解析.
 * unit 布局 (LE): u32 npos, u32 nword, u32 nsym, u32 flags(bit0=hasAnchored);
 *   u64[nword] firstUnanchored, firstAnchored, lastAny, lastEnd;
 *   u64[npos*nword] follow; u64[nsym*nword] reach;
 *   i32[nsym+1] cuts; i32[128] asciiSym; i32[npos] posPat.
 * ==================================================================== */
static int parse_unit(const uint8_t *blob, size_t blobLen, size_t off, mvs_nfa *out) {
    memset(out, 0, sizeof(*out));
    if (off + 16 > blobLen) return -1;
    const uint8_t *p = blob + off;
    int32_t npos = (int32_t)le_u32(p + 0);
    int32_t nword = (int32_t)le_u32(p + 4);
    int32_t nsym = (int32_t)le_u32(p + 8);
    uint32_t flags = le_u32(p + 12);
    if (npos <= 0 || nword <= 0 || nsym <= 0) return -1;

    out->npos = npos;
    out->nword = nword;
    out->nsym = nsym;
    out->hasAnchored = (flags & 1u) ? 1 : 0;

    size_t u64FirstBlocks = 4;                              /* 4 个 [nword] 向量 */
    size_t nU64 = u64FirstBlocks * (size_t)nword +
                  (size_t)npos * nword + (size_t)nsym * nword;
    size_t nI32 = (size_t)(nsym + 1) + 128 + (size_t)npos;
    size_t need = 16 + nU64 * 8 + nI32 * 4;
    if (off + need > blobLen) return -1;

    size_t cur = off + 16;
    out->firstUnanchored = (uint64_t *)malloc((size_t)nword * 8);
    out->firstAnchored = (uint64_t *)malloc((size_t)nword * 8);
    out->lastAny = (uint64_t *)malloc((size_t)nword * 8);
    out->lastEnd = (uint64_t *)malloc((size_t)nword * 8);
    out->follow = (uint64_t *)malloc((size_t)npos * nword * 8);
    out->reach = (uint64_t *)malloc((size_t)nsym * nword * 8);
    out->cuts = (int32_t *)malloc((size_t)(nsym + 1) * 4);
    out->asciiSym = (int32_t *)malloc(128 * 4);
    out->posPat = (int32_t *)malloc((size_t)npos * 4);
    if (!out->firstUnanchored || !out->firstAnchored || !out->lastAny ||
        !out->lastEnd || !out->follow || !out->reach || !out->cuts ||
        !out->asciiSym || !out->posPat) {
        return -1;
    }

#define MVS_RD_U64(dst, count)                                     \
    do {                                                           \
        for (size_t _i = 0; _i < (size_t)(count); _i++)            \
            (dst)[_i] = le_u64(blob + cur + _i * 8);               \
        cur += (size_t)(count) * 8;                                \
    } while (0)
#define MVS_RD_I32(dst, count)                                     \
    do {                                                           \
        for (size_t _i = 0; _i < (size_t)(count); _i++)            \
            (dst)[_i] = le_i32(blob + cur + _i * 4);               \
        cur += (size_t)(count) * 4;                                \
    } while (0)

    MVS_RD_U64(out->firstUnanchored, nword);
    MVS_RD_U64(out->firstAnchored, nword);
    MVS_RD_U64(out->lastAny, nword);
    MVS_RD_U64(out->lastEnd, nword);
    MVS_RD_U64(out->follow, (size_t)npos * nword);
    MVS_RD_U64(out->reach, (size_t)nsym * nword);
    MVS_RD_I32(out->cuts, nsym + 1);
    MVS_RD_I32(out->asciiSym, 128);
    MVS_RD_I32(out->posPat, npos);

#undef MVS_RD_U64
#undef MVS_RD_I32

    uint64_t fu = 0;
    for (int w = 0; w < nword; w++) fu |= out->firstUnanchored[w];
    out->unanchoredEmpty = (fu == 0) ? 1 : 0;
    return 0;
}

static void free_unit(mvs_nfa *u) {
    free(u->firstUnanchored);
    free(u->firstAnchored);
    free(u->lastAny);
    free(u->lastEnd);
    free(u->follow);
    free(u->reach);
    free(u->cuts);
    free(u->asciiSym);
    free(u->posPat);
    memset(u, 0, sizeof(*u));
}

/* db 头布局 (LE): magic[4]="MVS1", u32 version, u32 npat, i32 mergedUnit, u32 nUnits;
 *   i32[npat] slotUnit; u32[nUnits] unitOff; u32[nUnits] unitLen; units... */
mvscan_db *mvscan_db_open(const uint8_t *blob, size_t len) {
    if (!blob || len < 20) return NULL;
    if (blob[0] != 'M' || blob[1] != 'V' || blob[2] != 'S' || blob[3] != '1') return NULL;
    uint32_t version = le_u32(blob + 4);
    if (version != 1) return NULL;
    int32_t npat = (int32_t)le_u32(blob + 8);
    int32_t mergedUnit = (int32_t)le_u32(blob + 12);
    int32_t nUnits = (int32_t)le_u32(blob + 16);
    if (npat < 0 || nUnits < 0) return NULL;

    size_t headFixed = 20;
    size_t slotBytes = (size_t)npat * 4;
    size_t offBytes = (size_t)nUnits * 4;
    size_t lenBytes = (size_t)nUnits * 4;
    if (headFixed + slotBytes + offBytes + lenBytes > len) return NULL;

    mvscan_db *db = (mvscan_db *)calloc(1, sizeof(mvscan_db));
    if (!db) return NULL;
    db->npat = npat;
    db->nUnits = nUnits;
    db->mergedUnit = mergedUnit;
    db->slotUnit = (int32_t *)malloc(slotBytes > 0 ? slotBytes : 1);
    db->units = nUnits > 0 ? (mvs_nfa *)calloc((size_t)nUnits, sizeof(mvs_nfa)) : NULL;
    if (!db->slotUnit || (nUnits > 0 && !db->units)) { mvscan_db_close(db); return NULL; }

    size_t cur = headFixed;
    for (int i = 0; i < npat; i++) { db->slotUnit[i] = le_i32(blob + cur); cur += 4; }
    const uint8_t *offTab = blob + cur; cur += offBytes;
    const uint8_t *lenTab = blob + cur; cur += lenBytes;
    (void)lenTab;

    for (int i = 0; i < nUnits; i++) {
        uint32_t uoff = le_u32(offTab + (size_t)i * 4);
        if (parse_unit(blob, len, uoff, &db->units[i]) != 0) {
            mvscan_db_close(db);
            return NULL;
        }
    }
    return db;
}

void mvscan_db_close(mvscan_db *db) {
    if (!db) return;
    if (db->units) {
        for (int i = 0; i < db->nUnits; i++) free_unit(&db->units[i]);
        free(db->units);
    }
    free(db->slotUnit);
    free(db);
}

int32_t mvscan_db_npat(const mvscan_db *db) { return db ? db->npat : 0; }

int mvscan_db_has_merged(const mvscan_db *db) {
    return (db && db->mergedUnit >= 0 && db->mergedUnit < db->nUnits) ? 1 : 0;
}

int mvscan_db_nfa_exists(const mvscan_db *db, int32_t idx,
                         const uint8_t *data, size_t len) {
    if (!db || idx < 0 || idx >= db->npat) return -1;
    int32_t u = db->slotUnit[idx];
    if (u < 0 || u >= db->nUnits) return -1;
    return nfa_run(&db->units[u], data, len, 0, NULL, 0, NULL, 0, NULL);
}

void mvscan_db_nfa_exists_many(const mvscan_db *db,
                               const uint8_t *data, size_t len,
                               const int32_t *idxs, int32_t nidx,
                               uint8_t *out) {
    if (!out) return;
    for (int32_t i = 0; i < nidx; i++) {
        uint8_t r = 0;
        if (db && idxs) {
            int32_t idx = idxs[i];
            if (idx >= 0 && idx < db->npat) {
                int32_t u = db->slotUnit[idx];
                if (u >= 0 && u < db->nUnits) {
                    r = (uint8_t)(nfa_run(&db->units[u], data, len, 0, NULL, 0, NULL, 0, NULL) == 1);
                }
            }
        }
        out[i] = r;
    }
}

int32_t mvscan_db_merged_scan(const mvscan_db *db,
                              const uint8_t *data, size_t len,
                              uint8_t *seen, int32_t seenLen,
                              int32_t *out, int32_t cap) {
    if (!db || db->mergedUnit < 0 || db->mergedUnit >= db->nUnits) return 0;
    int32_t total = 0;
    nfa_run(&db->units[db->mergedUnit], data, len, 1, seen, seenLen, out, cap, &total);
    return total;
}

/* 强制标量孪生入口: 供差分测试对照默认 (SIMD) 分发, 二者命中必逐位一致. */
int mvscan_db_nfa_exists_scalar(const mvscan_db *db, int32_t idx,
                                const uint8_t *data, size_t len) {
    if (!db || idx < 0 || idx >= db->npat) return -1;
    int32_t u = db->slotUnit[idx];
    if (u < 0 || u >= db->nUnits) return -1;
    return nfa_run_dispatch(&db->units[u], data, len, 0, NULL, 0, NULL, 0, NULL, 1);
}

/* 锚定式存在性: 仅在 spans 注入区间内注入 first (对应 Go existsInAnchored).
 * spans 以平行数组 lo[0..nspan) / hi[0..nspan) 传入, 避免 Go 侧 C 结构体分配. */
int mvscan_db_nfa_exists_anchored(const mvscan_db *db, int32_t idx,
                                   const uint8_t *data, size_t len,
                                   const int32_t *lo, const int32_t *hi, int32_t nspan) {
    if (!db || idx < 0 || idx >= db->npat) return -1;
    int32_t u = db->slotUnit[idx];
    if (u < 0 || u >= db->nUnits) return -1;
    if (!lo || !hi || nspan <= 0) return 0;
    return nfa_run_anchored_dispatch(&db->units[u], data, len, lo, hi, nspan, 0);
}

/* 锚定式存在性 (强制标量孪生, 供差分测试). */
int mvscan_db_nfa_exists_anchored_scalar(const mvscan_db *db, int32_t idx,
                                          const uint8_t *data, size_t len,
                                          const int32_t *lo, const int32_t *hi, int32_t nspan) {
    if (!db || idx < 0 || idx >= db->npat) return -1;
    int32_t u = db->slotUnit[idx];
    if (u < 0 || u >= db->nUnits) return -1;
    if (!lo || !hi || nspan <= 0) return 0;
    return nfa_run_anchored_dispatch(&db->units[u], data, len, lo, hi, nspan, 1);
}

/* 批量锚定式存在性: 一次 cgo 对多条 pattern 各自做锚定式扫描, 摊薄跨界开销. */
void mvscan_db_nfa_exists_anchored_many(const mvscan_db *db,
                                         const uint8_t *data, size_t len,
                                         const int32_t *idxs, int32_t npat,
                                         const int32_t *patSpanOff,
                                         const int32_t *spansLo, const int32_t *spansHi,
                                         int32_t totalSpans,
                                         uint8_t *out) {
    if (!out) return;
    if (!db || !idxs || !patSpanOff || !spansLo || !spansHi) {
        for (int32_t i = 0; i < npat; i++) out[i] = 0;
        return;
    }
    (void)totalSpans;
    for (int32_t i = 0; i < npat; i++) {
        int32_t idx = idxs[i];
        uint8_t r = 0;
        if (idx >= 0 && idx < db->npat) {
            int32_t u = db->slotUnit[idx];
            if (u >= 0 && u < db->nUnits) {
                int32_t off0 = patSpanOff[i];
                int32_t off1 = patSpanOff[i + 1];
                int32_t nspan = off1 - off0;
                if (nspan > 0) {
                    const int32_t *lo = spansLo + off0;
                    const int32_t *hi = spansHi + off0;
                    r = (uint8_t)(nfa_run_anchored_dispatch(&db->units[u], data, len, lo, hi, nspan, 0) == 1);
                }
            }
        }
        out[i] = r;
    }
}

int32_t mvscan_db_merged_scan_scalar(const mvscan_db *db,
                                     const uint8_t *data, size_t len,
                                     uint8_t *seen, int32_t seenLen,
                                     int32_t *out, int32_t cap) {
    if (!db || db->mergedUnit < 0 || db->mergedUnit >= db->nUnits) return 0;
    int32_t total = 0;
    nfa_run_dispatch(&db->units[db->mergedUnit], data, len, 1, seen, seenLen, out, cap, &total, 1);
    return total;
}

/* 可观测: 报告是否编入 SIMD 档 (1) 还是纯标量 (0). */
int mvscan_simd_enabled(void) {
#if defined(MVS_SSE2) || defined(MVS_NEON)
    return 1;
#else
    return 0;
#endif
}
