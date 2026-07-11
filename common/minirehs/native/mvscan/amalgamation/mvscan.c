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

/* M3: SIMD 加速档. x86_64 用 SSE2 (ABI 基线必有), arm64 用 NEON.
 * 更高指令集必须放在独立的 target 函数里并经运行期 CPU 探测后调用；不能根据
 * __AVX2__ 替换本翻译单元的基线路径，否则用 -mavx2 构建的产物会在旧 CPU 上 SIGILL. */
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

    /* ---- LimEx 多字扩展 (v3 blob, merged NFA). ---- */
    int       hasLimEx;
    uint64_t *chainTarget;     /* [nword], 链边目标位 */
    uint64_t *excMask;         /* [nword], 含异常后继的源位置 */
    uint64_t *excFollow;       /* [npos*nword], 已移除 p->p+1 链边 */

    /* ---- 断言扩展 (hasAssert, v2 blob). 对应 Go condFirst/condFollow/condAccept. ---- */
    int      hasAssert;        /* flags bit1: 是否含零宽断言 guard */
    /* LimEx 链/异常拆分 (单字 nword==1 快路径, 对应 Go chainTarget1/excMask1/excFollow1). */
    uint64_t chainTarget1;     /* 位 q 置位 <=> 链边 (q-1)->q */
    uint64_t excMask1;         /* 位 p 置位 <=> excFollow1Flat 中有异常后继 */
    uint64_t *excFollow1Flat;  /* [npos] 每位置的异常后继 (单 uint64) */
    uint64_t condFollowMask1;  /* 位 p 置位 <=> 有 condFollow 条目 */
    /* condFirst: 条件起点注入. nCondFirst 条, 每条 = (u8 guard, u64 bits). */
    int32_t  nCondFirst;
    uint8_t *condFirstGuard;   /* [nCondFirst] */
    uint64_t *condFirstBits;   /* [nCondFirst] */
    /* condFollow: 条件后继 (per-position, jagged). 扁平化为 (pos, guard, bits) 三元组. */
    int32_t  nCondFollow;
    int32_t *condFollowPos;    /* [nCondFollow] */
    uint8_t *condFollowGuard;  /* [nCondFollow] */
    uint64_t *condFollowBits;  /* [nCondFollow] */
    /* condAccept: 条件接受. nCondAccept 条, 每条 = (u8 guard, u64 bits). */
    int32_t  nCondAccept;
    uint8_t *condAcceptGuard;  /* [nCondAccept] */
    uint64_t *condAcceptBits;  /* [nCondAccept] */
    /* guard 求值确定化表 (仅 nword==1): B=0..63 直接映射位集，消除热循环条件扫描。 */
    uint64_t *condFirstEval;   /* [64] */
    uint64_t *condFollowEval;  /* [64*npos], B-major */
    uint64_t *condAcceptEval;  /* [64] */

    /* ---- 必要条件预过滤 (v2 blob). 在 NFA 扫描前做快速字节检查. ---- */
    int32_t  necMinRunLen;     /* 0=无约束; >0 表示需要 >= necMinRunLen 个连续 necRunClass 字节 */
    int32_t  necRunClass;      /* 1=digit, 2=hex. 见 mvs_byte_in_class. */
    int32_t  necRequiredByte;  /* 0-255, -1=无 */
    int32_t  necRequiredCount; /* 最少出现次数 */
    /* Vermicelli 加速 */
    int      hasStartByteMask;
    uint8_t  startByteMask[256];

    /* ---- DFA 转换 (小规模 NFA 的确定性化). 当 hasDFA=1 时用 dfa_run 替代 nfa_run. ---- */
    int      hasDFA;
    int32_t  dfaNstates;
    int32_t *dfaNext;    /* [nstates*256] 转移表: next[state*256 + byte] (-1=死) */
    uint8_t *dfaAccept;  /* [nstates] 接受状态标记 */
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

/* SIMD 字向量原语. SSE2 (128-bit) / NEON (128-bit) 二选一. */
#if defined(MVS_SSE2)
static inline void row_copy_v(uint64_t *d, const uint64_t *s, int n) {
    int w = 0;
    for (; w + 4 <= n; w += 4) {
        _mm_storeu_si128((__m128i *)(d + w), _mm_loadu_si128((const __m128i *)(s + w)));
        _mm_storeu_si128((__m128i *)(d + w + 2), _mm_loadu_si128((const __m128i *)(s + w + 2)));
    }
    for (; w + 2 <= n; w += 2)
        _mm_storeu_si128((__m128i *)(d + w), _mm_loadu_si128((const __m128i *)(s + w)));
    for (; w < n; w++) d[w] = s[w];
}
static inline void row_or_v(uint64_t *d, const uint64_t *s, int n) {
    int w = 0;
    for (; w + 4 <= n; w += 4) {
        __m128i a0 = _mm_loadu_si128((const __m128i *)(d + w));
        __m128i b0 = _mm_loadu_si128((const __m128i *)(s + w));
        _mm_storeu_si128((__m128i *)(d + w), _mm_or_si128(a0, b0));
        __m128i a1 = _mm_loadu_si128((const __m128i *)(d + w + 2));
        __m128i b1 = _mm_loadu_si128((const __m128i *)(s + w + 2));
        _mm_storeu_si128((__m128i *)(d + w + 2), _mm_or_si128(a1, b1));
    }
    for (; w + 2 <= n; w += 2) {
        __m128i a = _mm_loadu_si128((const __m128i *)(d + w));
        __m128i b = _mm_loadu_si128((const __m128i *)(s + w));
        _mm_storeu_si128((__m128i *)(d + w), _mm_or_si128(a, b));
    }
    for (; w < n; w++) d[w] |= s[w];
}
static inline void row_and_v(uint64_t *d, const uint64_t *x, const uint64_t *y, int n) {
    int w = 0;
    for (; w + 4 <= n; w += 4) {
        __m128i a0 = _mm_loadu_si128((const __m128i *)(x + w));
        __m128i b0 = _mm_loadu_si128((const __m128i *)(y + w));
        _mm_storeu_si128((__m128i *)(d + w), _mm_and_si128(a0, b0));
        __m128i a1 = _mm_loadu_si128((const __m128i *)(x + w + 2));
        __m128i b1 = _mm_loadu_si128((const __m128i *)(y + w + 2));
        _mm_storeu_si128((__m128i *)(d + w + 2), _mm_and_si128(a1, b1));
    }
    for (; w + 2 <= n; w += 2) {
        __m128i a = _mm_loadu_si128((const __m128i *)(x + w));
        __m128i b = _mm_loadu_si128((const __m128i *)(y + w));
        _mm_storeu_si128((__m128i *)(d + w), _mm_and_si128(a, b));
    }
    for (; w < n; w++) d[w] = x[w] & y[w];
}
static inline void row_zero_v(uint64_t *d, int n) {
    int w = 0;
    __m128i z = _mm_setzero_si128();
    for (; w + 4 <= n; w += 4) {
        _mm_storeu_si128((__m128i *)(d + w), z);
        _mm_storeu_si128((__m128i *)(d + w + 2), z);
    }
    for (; w + 2 <= n; w += 2)
        _mm_storeu_si128((__m128i *)(d + w), z);
    for (; w < n; w++) d[w] = 0;
}

#elif defined(MVS_NEON)
/* arm64 NEON: 一次处理 4 个 uint64 = 256 位 (两条 vld1q/vst1q 指令, 双发射). */
static inline void row_copy_v(uint64_t *d, const uint64_t *s, int n) {
    int w = 0;
    for (; w + 4 <= n; w += 4) {
        uint64x2_t a = vld1q_u64(s + w);
        uint64x2_t b = vld1q_u64(s + w + 2);
        vst1q_u64(d + w, a);
        vst1q_u64(d + w + 2, b);
    }
    for (; w + 2 <= n; w += 2) vst1q_u64(d + w, vld1q_u64(s + w));
    for (; w < n; w++) d[w] = s[w];
}
static inline void row_or_v(uint64_t *d, const uint64_t *s, int n) {
    int w = 0;
    for (; w + 4 <= n; w += 4) {
        uint64x2_t a0 = vld1q_u64(d + w);
        uint64x2_t b0 = vld1q_u64(s + w);
        uint64x2_t a1 = vld1q_u64(d + w + 2);
        uint64x2_t b1 = vld1q_u64(s + w + 2);
        vst1q_u64(d + w, vorrq_u64(a0, b0));
        vst1q_u64(d + w + 2, vorrq_u64(a1, b1));
    }
    for (; w + 2 <= n; w += 2) vst1q_u64(d + w, vorrq_u64(vld1q_u64(d + w), vld1q_u64(s + w)));
    for (; w < n; w++) d[w] |= s[w];
}
static inline void row_and_v(uint64_t *d, const uint64_t *x, const uint64_t *y, int n) {
    int w = 0;
    for (; w + 4 <= n; w += 4) {
        uint64x2_t a0 = vld1q_u64(x + w);
        uint64x2_t b0 = vld1q_u64(y + w);
        uint64x2_t a1 = vld1q_u64(x + w + 2);
        uint64x2_t b1 = vld1q_u64(y + w + 2);
        vst1q_u64(d + w, vandq_u64(a0, b0));
        vst1q_u64(d + w + 2, vandq_u64(a1, b1));
    }
    for (; w + 2 <= n; w += 2) vst1q_u64(d + w, vandq_u64(vld1q_u64(x + w), vld1q_u64(y + w)));
    for (; w < n; w++) d[w] = x[w] & y[w];
}
static inline void row_zero_v(uint64_t *d, int n) {
    int w = 0;
    uint64x2_t z = vdupq_n_u64(0);
    for (; w + 4 <= n; w += 4) {
        vst1q_u64(d + w, z);
        vst1q_u64(d + w + 2, z);
    }
    for (; w + 2 <= n; w += 2) vst1q_u64(d + w, z);
    for (; w < n; w++) d[w] = 0;
}
#else
/* 未知架构的 combined scanner 也会引用 ROW_V 名称；让它们显式落到标量孪生，
 * 保证非 x86_64 / 非 arm64 构建既能编译，也不会尝试执行任何 SIMD 指令。 */
#define row_copy_v row_copy_s
#define row_or_v row_or_s
#define row_and_v row_and_s
#define row_zero_v row_zero_s
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

        if (a->hasLimEx) {
            /* LimEx: 所有 p->p+1 链边由一次跨字左移推进；仅枚举稀疏异常源。 */
            uint64_t carry = 0;
            for (int w = 0; w < nword; w++) {
                uint64_t v = prev[w];
                uint64_t shifted = (v << 1) | carry;
                carry = v >> 63;
                cand[w] = a->firstUnanchored[w] | (shifted & a->chainTarget[w]);
            }
            if (atStart && a->hasAnchored) ROW_OR(cand, a->firstAnchored, nword);
            for (int w = 0; w < nword; w++) {
                uint64_t ex = prev[w] & a->excMask[w];
                while (ex) {
                    int p = (w << 6) + mvs_ctz64(ex);
                    ex &= ex - 1;
                    ROW_OR(cand, a->excFollow + (size_t)p * nword, nword);
                }
            }
        } else {
            /* 通用 Glushkov 后继并集。 */
            ROW_COPY(cand, a->firstUnanchored, nword);
            if (atStart && a->hasAnchored) ROW_OR(cand, a->firstAnchored, nword);
            for (int w = 0; w < nword; w++) {
                uint64_t pw = prev[w];
                while (pw) {
                    int p = (w << 6) + mvs_ctz64(pw);
                    pw &= pw - 1;
                    ROW_OR(cand, a->follow + (size_t)p * nword, nword);
                }
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
        /* Vermicelli 加速: 活跃集空 + firstUnanchored 注入无效 (该字节无法开始新匹配) =>
         * NFA 完全休眠, 跳到下一个能开始匹配的字节 (startByteMask[b] != 0).
         * 省去逐字节走过多数不激活 NFA 的 HTTP 文本. */
        if (anyActive == 0 && a->hasStartByteMask) {
            /* 从当前 i 向前找第一个 startByteMask[data[j]] != 0 的 j.
             * 非 ASCII 字节 (>=0x80) 不跳过 (startByteMask 仅覆盖 ASCII). */
            size_t j = i;
            while (j < len && data[j] < 0x80 && !a->startByteMask[data[j]]) j++;
            if (j > i) i = j;
        }
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

        if (a->hasLimEx) {
            /* LimEx: 所有 p->p+1 链边由一次跨字左移推进；仅枚举稀疏异常源。 */
            uint64_t carry = 0;
            for (int w = 0; w < nword; w++) {
                uint64_t v = prev[w];
                uint64_t shifted = (v << 1) | carry;
                carry = v >> 63;
                cand[w] = a->firstUnanchored[w] | (shifted & a->chainTarget[w]);
            }
            if (atStart && a->hasAnchored) ROW_OR(cand, a->firstAnchored, nword);
            for (int w = 0; w < nword; w++) {
                uint64_t ex = prev[w] & a->excMask[w];
                while (ex) {
                    int p = (w << 6) + mvs_ctz64(ex);
                    ex &= ex - 1;
                    ROW_OR(cand, a->excFollow + (size_t)p * nword, nword);
                }
            }
        } else {
            /* 通用 Glushkov 后继并集。 */
            ROW_COPY(cand, a->firstUnanchored, nword);
            if (atStart && a->hasAnchored) ROW_OR(cand, a->firstAnchored, nword);
            for (int w = 0; w < nword; w++) {
                uint64_t pw = prev[w];
                while (pw) {
                    int p = (w << 6) + mvs_ctz64(pw);
                    pw &= pw - 1;
                    ROW_OR(cand, a->follow + (size_t)p * nword, nword);
                }
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
        /* Vermicelli 加速: 活跃集空 + firstUnanchored 注入无效 (该字节无法开始新匹配) =>
         * NFA 完全休眠, 跳到下一个能开始匹配的字节 (startByteMask[b] != 0).
         * 省去逐字节走过多数不激活 NFA 的 HTTP 文本. */
        if (anyActive == 0 && a->hasStartByteMask) {
            /* 从当前 i 向前找第一个 startByteMask[data[j]] != 0 的 j.
             * 非 ASCII 字节 (>=0x80) 不跳过 (startByteMask 仅覆盖 ASCII). */
            size_t j = i;
            while (j < len && data[j] < 0x80 && !a->startByteMask[data[j]]) j++;
            if (j > i) i = j;
        }
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

/* 单条存在性验证绝大多数是一个 64-bit 状态字。通用 nfa_run 为 merged/multiword
 * 保留了栈数组和 ROW_* 抽象；此处把同一 Glushkov 递推留在寄存器中。merged 的 mode=1
 * 仍走通用实现，因为它需要逐接受位置发射成员。 */
static int nfa_run_1_exists(const mvs_nfa *a, const uint8_t *data, size_t len) {
    uint64_t prev = 0;
    size_t i = 0;
    while (i < len) {
        int atStart = (i == 0);
        int sym;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            i++;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            i += (size_t)size;
            sym = symbol_of(a, r);
        }
        uint64_t cand = a->firstUnanchored[0];
        if (atStart && a->hasAnchored) cand |= a->firstAnchored[0];
        for (uint64_t pw = prev; pw; pw &= pw - 1) {
            cand |= a->follow[mvs_ctz64(pw)];
        }
        prev = cand & a->reach[sym];
        if (prev & a->lastAny[0]) return 1;
        if (i == len && (prev & a->lastEnd[0])) return 1;
        if (prev == 0 && a->unanchoredEmpty) break;
    }
    return 0;
}

/* nfa_find_loc_1 是 nword==1 lean NFA 的单个 leftmost-longest 定位器。
 * 和 Go findLocFrom1 同构：每个活跃位置携带最小起点；接受时取最小起点，
 * 同起点延长到最长终点；当更早起点的活跃线程全部消亡即提前停止。 */
static int nfa_find_loc_1(const mvs_nfa *a, const uint8_t *data, size_t len,
                          size_t searchFrom, int32_t *fromOut, int32_t *toOut) {
    if (searchFrom > len || (a->hasAnchored && searchFrom > 0)) return 0;

    int32_t candStart[64];
    int32_t prevStart[64];
    uint64_t prev = 0;
    int hasPrev = 0;
    int32_t bestStart = -1, bestEnd = -1;
    size_t i = searchFrom;

    while (i < len) {
        size_t runeStart = i;
        int sym;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            i++;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            i += (size_t)size;
            sym = symbol_of(a, r);
        }

        uint64_t cand = 0;
        if (hasPrev) {
            for (uint64_t pw = prev; pw; pw &= pw - 1) {
                int p = mvs_ctz64(pw);
                int32_t start = prevStart[p];
                for (uint64_t fb = a->follow[p]; fb; fb &= fb - 1) {
                    int q = mvs_ctz64(fb);
                    uint64_t bit = UINT64_C(1) << q;
                    if (!(cand & bit) || start < candStart[q]) {
                        cand |= bit;
                        candStart[q] = start;
                    }
                }
            }
        }
        uint64_t first = a->firstUnanchored[0];
        if (runeStart == 0 && a->hasAnchored) first |= a->firstAnchored[0];
        for (uint64_t fb = first; fb; fb &= fb - 1) {
            int q = mvs_ctz64(fb);
            uint64_t bit = UINT64_C(1) << q;
            if (!(cand & bit) || (int32_t)runeStart < candStart[q]) {
                cand |= bit;
                candStart[q] = (int32_t)runeStart;
            }
        }

        uint64_t active = cand & a->reach[sym];
        int32_t minActiveStart = INT32_MAX;
        int32_t minAcceptStart = INT32_MAX;
        uint64_t accept = active & a->lastAny[0];
        if (i == len) accept |= active & a->lastEnd[0];
        for (uint64_t bits = active; bits; bits &= bits - 1) {
            int q = mvs_ctz64(bits);
            int32_t start = candStart[q];
            prevStart[q] = start;
            if (start < minActiveStart) minActiveStart = start;
        }
        for (uint64_t bits = accept; bits; bits &= bits - 1) {
            int q = mvs_ctz64(bits);
            if (candStart[q] < minAcceptStart) minAcceptStart = candStart[q];
        }
        prev = active;
        hasPrev = active != 0;

        if (minAcceptStart != INT32_MAX) {
            if (bestEnd < 0 || minAcceptStart < bestStart ||
                (minAcceptStart == bestStart && (int32_t)i > bestEnd)) {
                bestStart = minAcceptStart;
                bestEnd = (int32_t)i;
            }
        }
        if (!hasPrev) {
            if (a->hasAnchored || bestEnd >= 0) break;
            continue;
        }
        if (bestEnd >= 0 && minActiveStart > bestStart) break;
    }
    if (bestEnd < 0) return 0;
    *fromOut = bestStart;
    *toOut = bestEnd;
    return 1;
}

/* ====================================================================
 * 断言 NFA: 零宽断言 guard 门控的位并行递推 (对应 Go mvs_assert.go).
 *
 * compute_boundaries: 复刻 Go computeBoundaries. 逐 rune 走 data, 在每个 rune 起始
 * 与末尾处写入边界条件集 bound[i] (uint8, 6 位: BeginText/EndText/BeginLine/EndLine/
 * WordBoundary/NoWordBoundary). bound 长度 = len+1.
 *
 * nfa_run_assert_1: 断言 NFA 的单字 (nword==1) 存在性扫描, 含 LimEx 链/异常拆分 +
 * condFirst/condFollow/condAccept guard 门控. 与 Go existsInAssertShared1 逐位一致.
 * ==================================================================== */

/* 边界条件位 (与 Go condBeginText 等完全一致). */
#define MVS_COND_BEGIN_TEXT      0x01u
#define MVS_COND_END_TEXT        0x02u
#define MVS_COND_BEGIN_LINE      0x04u
#define MVS_COND_END_LINE        0x08u
#define MVS_COND_WORD_BOUNDARY   0x10u
#define MVS_COND_NO_WORD_BOUND   0x20u

/* mvs_is_word_byte: 复刻 Go isWordByte (ASCII [0-9A-Za-z_]). */
static inline int mvs_is_word_byte(uint8_t c) {
    return (c >= '0' && c <= '9') ||
           (c >= 'a' && c <= 'z') ||
           (c >= 'A' && c <= 'Z') ||
           c == '_';
}

static inline int mvs_is_word_rune(int32_t r) {
    return (r >= '0' && r <= '9') ||
           (r >= 'a' && r <= 'z') ||
           (r >= 'A' && r <= 'Z') ||
           r == '_';
}

/* before/after 为 -1 表示文本端点；与 Go boundaryConds 完全同构。 */
static inline uint8_t mvs_boundary_conds(int32_t before, int32_t after) {
    uint8_t b = 0;
    if (before < 0) b |= MVS_COND_BEGIN_TEXT | MVS_COND_BEGIN_LINE;
    else if (before == '\n') b |= MVS_COND_BEGIN_LINE;
    if (after < 0) b |= MVS_COND_END_TEXT | MVS_COND_END_LINE;
    else if (after == '\n') b |= MVS_COND_END_LINE;
    if (mvs_is_word_rune(before) != mvs_is_word_rune(after)) b |= MVS_COND_WORD_BOUNDARY;
    else b |= MVS_COND_NO_WORD_BOUND;
    return b;
}

/* mvscan_compute_boundaries: 复刻 Go computeBoundaries. 产出 bound[0..len] (len+1 字节).
 * buf 由调用方提供 (容量 >= len+1). ASCII 快路径 (c<0x80 直接字节判定, 省 DecodeRune). */
void mvscan_compute_boundaries(const uint8_t *data, size_t len, uint8_t *buf) {
    int prevWord = 0;       /* isWordRune(prev); prev==-1 => false */
    int prevIsNewline = 0;  /* prev == '\n' */
    int prevIsStart = 1;    /* prev < 0 (文本始) */
    size_t i = 0;
    while (i < len) {
        uint8_t c = data[i];
        uint8_t b = 0;
        if (prevIsStart) {
            b |= MVS_COND_BEGIN_TEXT | MVS_COND_BEGIN_LINE;
        } else if (prevIsNewline) {
            b |= MVS_COND_BEGIN_LINE;
        }
        if (c < 0x80) {
            int curWord = mvs_is_word_byte(c);
            int curIsNewline = (c == '\n');
            if (curIsNewline) b |= MVS_COND_END_LINE;
            if (prevWord != curWord) b |= MVS_COND_WORD_BOUNDARY;
            else b |= MVS_COND_NO_WORD_BOUND;
            buf[i] = b;
            prevWord = curWord;
            prevIsNewline = curIsNewline;
            prevIsStart = 0;
            i++;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            int curWord = mvs_is_word_rune(r);
            int curIsNewline = (r == '\n');
            if (curIsNewline) b |= MVS_COND_END_LINE;
            if (prevWord != curWord) b |= MVS_COND_WORD_BOUNDARY;
            else b |= MVS_COND_NO_WORD_BOUND;
            buf[i] = b;
            prevWord = curWord;
            prevIsNewline = curIsNewline;
            prevIsStart = 0;
            i += (size_t)size;
        }
    }
    /* 末尾: after = -1 (文本末). */
    uint8_t b = 0;
    if (prevIsStart) {
        b |= MVS_COND_BEGIN_TEXT | MVS_COND_BEGIN_LINE;
    } else if (prevIsNewline) {
        b |= MVS_COND_BEGIN_LINE;
    }
    b |= MVS_COND_END_TEXT | MVS_COND_END_LINE;
    if (prevWord) b |= MVS_COND_WORD_BOUNDARY;
    else b |= MVS_COND_NO_WORD_BOUND;
    buf[len] = b;
}

/* dfa_run: DFA 存在性扫描 — 每字节一次查表 (O(1)), 比 NFA 位递推快 O(npos) 倍.
 * 仅处理纯 ASCII 输入 (byte < 128); 非 ASCII 字节回退到 NFA.
 * 返回 1 命中 / 0 不命中 / -1 需回退 NFA (非 ASCII 数据). */
static int dfa_run(const mvs_nfa *a, const uint8_t *data, size_t len) {
    const int32_t *next = a->dfaNext;
    const uint8_t *acc = a->dfaAccept;
    /* 快速检查: 是否纯 ASCII? 若有非 ASCII 字节, 回退 NFA. */
    for (size_t i = 0; i < len; i++) {
        if (data[i] >= 0x80) return -1; /* 非 ASCII: 回退 NFA */
    }
    /* 纯 ASCII: DFA 快速扫描 (每字节一次查表). */
    int32_t state = 0;
    for (size_t i = 0; i < len; i++) {
        uint8_t b = data[i];
        state = next[state * 256 + b];
        if (state < 0) {
            /* 死状态: 从初始重新开始 (无锚 NFA 每步注入 first).
             * DFA 状态 0 已含 first 注入, 故直接用 next[0*256+b]. */
            state = next[0 * 256 + b];
            if (state < 0) state = 0;
        }
        if (acc[state]) return 1;
    }
    return 0;
}

/* mvs_byte_in_class: 复刻 Go byteInClass (仅 C 内核用). */
static inline int mvs_byte_in_class_c(uint8_t c, int32_t cls) {
    switch (cls) {
    case 1: return c >= '0' && c <= '9';            /* digit */
    case 2: return (c >= '0' && c <= '9') ||        /* hex */
                 (c >= 'a' && c <= 'f') ||
                 (c >= 'A' && c <= 'F');
    }
    return 0;
}

/* mvs_nec_check: 必要条件预过滤. 返回 0 = 绝不可能命中, 可跳过 NFA 扫描.
 * 在 C 内核中执行, 与 NFA 同侧, 无跨界开销. 对不匹配记录省去整段 NFA 位递推. */
static inline int mvs_nec_check(const mvs_nfa *a, const uint8_t *data, size_t len) {
    /* 连续序列约束 */
    if (a->necMinRunLen > 0) {
        int maxRun = 0, curRun = 0;
        for (size_t i = 0; i < len; i++) {
            if (mvs_byte_in_class_c(data[i], a->necRunClass)) {
                curRun++;
                if (curRun > maxRun) maxRun = curRun;
            } else {
                curRun = 0;
            }
        }
        if (maxRun < a->necMinRunLen) return 0;
    }
    /* 稀有字节计数约束 */
    if (a->necRequiredByte >= 0 && a->necRequiredCount > 0) {
        int cnt = 0;
        uint8_t target = (uint8_t)a->necRequiredByte;
        for (size_t i = 0; i < len; i++) {
            if (data[i] == target) {
                cnt++;
                if (cnt >= a->necRequiredCount) break;
            }
        }
        if (cnt < a->necRequiredCount) return 0;
    }
    return 1; /* 可能命中, 需跑 NFA */
}

/* nfa_run_assert_1: 断言 NFA 单字 (nword==1) 存在性扫描.
 * 与 Go existsInAssertShared1 逐位一致: LimEx 链/异常 + condFirst/condFollow/condAccept guard.
 * bound 由调用方预算 (mvscan_compute_boundaries). 含必要条件预过滤 (C 内核同侧, 零跨界开销). */
static int nfa_run_assert_1(const mvs_nfa *a, const uint8_t *data, size_t len,
                            const uint8_t *bound) {
    /* 必要条件预过滤: C 内核同侧检查, 不满足则直接返回 0 (省去整段位递推). */
    if (!mvs_nec_check(a, data, len)) return 0;
    uint64_t prev = 0;
    size_t i = 0;
    while (i < len) {
        int sym;
        size_t ni;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            ni = i + 1;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            ni = i + (size_t)size;
            sym = symbol_of(a, r);
        }
        uint8_t bpre = bound[i];
        uint8_t bpost = bound[ni];

        /* LimEx: 链边左移批量推进; 异常边逐个 OR. */
        uint64_t shifted = (prev << 1) & a->chainTarget1;
        uint64_t cand = a->firstUnanchored[0] | shifted;

        cand |= a->condFirstEval[bpre & 63u];

        /* 异常边: 仅对活跃的异常位置展开. */
        uint64_t exc = prev & a->excMask1;
        while (exc) {
            int p = mvs_ctz64(exc);
            exc &= exc - 1;
            cand |= a->excFollow1Flat[p];
        }

        /* condFollow: 对有 condFollow 条目的活跃位置展开. */
        uint64_t cfm = prev & a->condFollowMask1;
        while (cfm) {
            int p = mvs_ctz64(cfm);
            cfm &= cfm - 1;
            cand |= a->condFollowEval[(size_t)(bpre & 63u) * a->npos + p];
        }

        uint64_t active = cand & a->reach[sym];
        if (active & a->lastAny[0]) return 1;
        if (active & a->condAcceptEval[bpost & 63u]) return 1;
        prev = active;
        i = ni;
    }
    return 0;
}

/* nfa_run_assert_mw: 断言 NFA 多字 (nword>1) 存在性扫描.
 * 与 Go existsInAssertShared 逐位一致: condFirst/condFollow/condAccept guard 门控.
 * 使用 ROW_* 宏做多字向量 OR/AND/COPY (SIMD 加速). */
static int nfa_run_assert_mw(const mvs_nfa *a, const uint8_t *data, size_t len,
                             const uint8_t *bound) {
    /* 必要条件预过滤 */
    if (!mvs_nec_check(a, data, len)) return 0;
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

    size_t i = 0;
    while (i < len) {
        int sym;
        size_t ni;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            ni = i + 1;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            ni = i + (size_t)size;
            sym = symbol_of(a, r);
        }
        uint8_t bpre = bound[i];
        uint8_t bpost = bound[ni];

        /* cand = firstUnanchored + condFirst (按 bpre 门控). */
        row_copy_v(cand, a->firstUnanchored, nword);
        for (int k = 0; k < a->nCondFirst; k++) {
            if ((a->condFirstGuard[k] & bpre) == a->condFirstGuard[k]) {
                for (int w = 0; w < nword; w++) cand[w] |= a->condFirstBits[k * nword + w];
            }
        }
        /* 后继并集: OR follow[p] for p in prev. */
        for (int w = 0; w < nword; w++) {
            uint64_t pw = prev[w];
            while (pw) {
                int p = (w << 6) + mvs_ctz64(pw);
                pw &= pw - 1;
                row_or_v(cand, a->follow + (size_t)p * nword, nword);
            }
        }
        /* condFollow: 对有 condFollow 条目的活跃位置展开 (扁平三元组, bits 为 [nword]). */
        for (int k = 0; k < a->nCondFollow; k++) {
            int p = a->condFollowPos[k];
            int pw = p >> 6;
            uint64_t pbit = 1ULL << (p & 63);
            if (pw < nword && (prev[pw] & pbit)) {
                if ((a->condFollowGuard[k] & bpre) == a->condFollowGuard[k]) {
                    for (int ww = 0; ww < nword; ww++)
                        cand[ww] |= a->condFollowBits[(size_t)k * nword + ww];
                }
            }
        }

        /* active = cand & reach[sym]; 检查接受. */
        const uint64_t *rc = a->reach + (size_t)sym * nword;
        for (int w = 0; w < nword; w++) {
            prev[w] = cand[w] & rc[w];
            if (prev[w] & a->lastAny[w]) {
                if (nword > 8) { free(prev); free(cand); }
                return 1;
            }
        }
        /* condAccept: 按 bpost 门控. */
        for (int k = 0; k < a->nCondAccept; k++) {
            if ((a->condAcceptGuard[k] & bpost) == a->condAcceptGuard[k]) {
                for (int w = 0; w < nword; w++) {
                    if (prev[w] & a->condAcceptBits[(size_t)k * nword + w]) {
                        if (nword > 8) { free(prev); free(cand); }
                        return 1;
                    }
                }
            }
        }
        i = ni;
    }
    if (nword > 8) { free(prev); free(cand); }
    return 0;
}

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

/* nword==1 是真实规则集锚定 verifier 的主路径。通用运行器为任意字宽保留了
 * 栈数组、ROW_* 调用和按 word 的循环；这里将状态留在寄存器中，逐位 OR follow
 * 仍与通用递推完全同构。它只用于 !anchoredStart 的 span 注入语义。 */
static int nfa_run_anchored_1(const mvs_nfa *a, const uint8_t *data, size_t len,
                              const int32_t *loArr, const int32_t *hiArr, int32_t nspan) {
    if (nspan <= 0) return 0;
    uint64_t prev = 0;
    int hasActive = 0;
    int32_t si = 0;
    int32_t lastHi = hiArr[nspan - 1];
    int32_t curLo = loArr[0], curHi = hiArr[0];
    size_t i = mvs_rune_start(data, len, (size_t)curLo);

    while (i < len) {
        size_t runeStart = i;
        int sym;
        uint8_t c0 = data[i];
        if (c0 < 0x80) {
            sym = (int)a->asciiSym[c0];
            i++;
        } else {
            int32_t r;
            int size = mvs_decode_rune(data + i, len - i, &r);
            i += (size_t)size;
            sym = symbol_of(a, r);
        }
        int atEnd = (i == len);

        while (si < nspan && runeStart >= (size_t)curHi) {
            si++;
            if (si < nspan) {
                curLo = loArr[si];
                curHi = hiArr[si];
            }
        }
        uint64_t cand = (si < nspan && runeStart >= (size_t)curLo) ? a->firstUnanchored[0] : 0;
        for (uint64_t pw = prev; pw; pw &= pw - 1) {
            int p = mvs_ctz64(pw);
            cand |= a->follow[p];
        }
        uint64_t active = cand & a->reach[sym];
        if (active & a->lastAny[0]) return 1;
        if (atEnd && (active & a->lastEnd[0])) return 1;

        hasActive = (active != 0);
        if (!hasActive && (si >= nspan || (size_t)lastHi <= runeStart)) return 0;
        if (!hasActive && si < nspan && (size_t)curLo > runeStart + 32) {
            size_t jump = mvs_rune_start(data, len, (size_t)curLo);
            if (jump > i) {
                i = jump;
                continue;
            }
        }
        prev = active;
    }
    return 0;
}

/* nfa_run_anchored 分发: nword>=2 且有 SIMD 档时走 SIMD, 否则标量孪生. */
static int nfa_run_anchored_dispatch(const mvs_nfa *a, const uint8_t *data, size_t len,
                                     const int32_t *lo, const int32_t *hi, int32_t nspan,
                                     int forceScalar) {
	if (a->nword == 1)
		return nfa_run_anchored_1(a, data, len, lo, hi, nspan);
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
	/* DFA 快路径: 存在性模式时优先用 DFA. 非 ASCII 输入回退 NFA. */
	if (mode == 0 && a->hasDFA) {
		int r = dfa_run(a, data, len);
		if (r >= 0) return r; /* DFA 结果确定 (0 或 1) */
		/* r == -1: 非 ASCII 输入, 回退 NFA */
	}
	if (mode == 0 && a->nword == 1)
		return nfa_run_1_exists(a, data, len);
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
static int parse_unit(const uint8_t *blob, size_t blobLen, size_t off, size_t unitLen, mvs_nfa *out) {
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

    /* LimEx 多字扩展 (v3, flags bit3): chainTarget[nword], excMask[nword],
     * excFollow[npos*nword]. 仅 merged unit 写入。 */
    out->hasLimEx = (flags & 0x8u) ? 1 : 0;
    if (out->hasLimEx) {
        size_t limexU64 = (size_t)nword * 2 + (size_t)npos * nword;
        if (cur + limexU64 * 8 > off + unitLen || cur + limexU64 * 8 > blobLen) return -1;
        out->chainTarget = (uint64_t *)malloc((size_t)nword * 8);
        out->excMask = (uint64_t *)malloc((size_t)nword * 8);
        out->excFollow = (uint64_t *)malloc((size_t)npos * nword * 8);
        if (!out->chainTarget || !out->excMask || !out->excFollow) return -1;
        for (int w = 0; w < nword; w++) { out->chainTarget[w] = le_u64(blob + cur); cur += 8; }
        for (int w = 0; w < nword; w++) { out->excMask[w] = le_u64(blob + cur); cur += 8; }
        for (size_t j = 0; j < (size_t)npos * nword; j++) { out->excFollow[j] = le_u64(blob + cur); cur += 8; }
    }

    /* 断言扩展 (v2 blob, flags bit1 = hasAssert): 在 posPat 之后追加 assert 字段.
     * 布局: u64 chainTarget1, u64 excMask1, u64[npos] excFollow1Flat, u64 condFollowMask1,
     *   i32 nCondFirst, {u8[nCondFirst] guard, u64[nCondFirst] bits},
     *   i32 nCondFollow, {i32[nCondFollow] pos, u8[nCondFollow] guard, u64[nCondFollow] bits},
     *   i32 nCondAccept, {u8[nCondAccept] guard, u64[nCondAccept] bits}.
     * 仅 nword==1 的断言 NFA 才有序列化 (Go 侧保证). */
    out->hasAssert = (flags & 0x2u) ? 1 : 0;
    if (out->hasAssert) {
        /* 计算剩余所需字节数. */
        size_t assertU64 = 3 + (size_t)npos; /* chainTarget1, excMask1, condFollowMask1 + excFollow1Flat[npos] */
        /* 先读固定头部 (chainTarget1, excMask1, excFollow1Flat, condFollowMask1) */
        if (cur + assertU64 * 8 > blobLen) return -1;
        out->chainTarget1 = le_u64(blob + cur); cur += 8;
        out->excMask1 = le_u64(blob + cur); cur += 8;
        out->excFollow1Flat = (uint64_t *)malloc((size_t)npos * 8);
        if (!out->excFollow1Flat) return -1;
        for (int _i = 0; _i < npos; _i++) { out->excFollow1Flat[_i] = le_u64(blob + cur); cur += 8; }
        out->condFollowMask1 = le_u64(blob + cur); cur += 8;

        /* condFirst: 每个 guard 条目的 bits 为 nword 个 uint64 */
        if (cur + 4 > blobLen) return -1;
        out->nCondFirst = (int32_t)le_i32(blob + cur); cur += 4;
        if (out->nCondFirst > 0) {
            size_t guardBytes = (size_t)out->nCondFirst;
            size_t bitsBytes = (size_t)out->nCondFirst * (size_t)nword * 8;
            if (cur + guardBytes + bitsBytes > blobLen) return -1;
            out->condFirstGuard = (uint8_t *)malloc(guardBytes);
            out->condFirstBits = (uint64_t *)malloc(bitsBytes);
            if (!out->condFirstGuard || !out->condFirstBits) return -1;
            for (int _i = 0; _i < out->nCondFirst; _i++) { out->condFirstGuard[_i] = blob[cur + _i]; }
            cur += guardBytes;
            for (int _i = 0; _i < out->nCondFirst * nword; _i++) { out->condFirstBits[_i] = le_u64(blob + cur + (size_t)_i * 8); }
            cur += bitsBytes;
        }

        /* condFollow (扁平三元组: pos, guard, bits[nword]) */
        if (cur + 4 > blobLen) return -1;
        out->nCondFollow = (int32_t)le_i32(blob + cur); cur += 4;
        if (out->nCondFollow > 0) {
            size_t posBytes = (size_t)out->nCondFollow * 4;
            size_t guardBytes = (size_t)out->nCondFollow;
            size_t bitsBytes = (size_t)out->nCondFollow * (size_t)nword * 8;
            if (cur + posBytes + guardBytes + bitsBytes > blobLen) return -1;
            out->condFollowPos = (int32_t *)malloc(posBytes);
            out->condFollowGuard = (uint8_t *)malloc(guardBytes);
            out->condFollowBits = (uint64_t *)malloc(bitsBytes);
            if (!out->condFollowPos || !out->condFollowGuard || !out->condFollowBits) return -1;
            for (int _i = 0; _i < out->nCondFollow; _i++) { out->condFollowPos[_i] = le_i32(blob + cur + (size_t)_i * 4); }
            cur += posBytes;
            for (int _i = 0; _i < out->nCondFollow; _i++) { out->condFollowGuard[_i] = blob[cur + _i]; }
            cur += guardBytes;
            for (int _i = 0; _i < out->nCondFollow * nword; _i++) { out->condFollowBits[_i] = le_u64(blob + cur + (size_t)_i * 8); }
            cur += bitsBytes;
        }

        /* condAccept: 每个 guard 条目的 bits 为 nword 个 uint64 */
        if (cur + 4 > blobLen) return -1;
        out->nCondAccept = (int32_t)le_i32(blob + cur); cur += 4;
        if (out->nCondAccept > 0) {
            size_t guardBytes = (size_t)out->nCondAccept;
            size_t bitsBytes = (size_t)out->nCondAccept * (size_t)nword * 8;
            if (cur + guardBytes + bitsBytes > blobLen) return -1;
            out->condAcceptGuard = (uint8_t *)malloc(guardBytes);
            out->condAcceptBits = (uint64_t *)malloc(bitsBytes);
            if (!out->condAcceptGuard || !out->condAcceptBits) return -1;
            for (int _i = 0; _i < out->nCondAccept; _i++) { out->condAcceptGuard[_i] = blob[cur + _i]; }
            cur += guardBytes;
            for (int _i = 0; _i < out->nCondAccept * nword; _i++) { out->condAcceptBits[_i] = le_u64(blob + cur + (size_t)_i * 8); }
            cur += bitsBytes;
        }

        /* 必要条件预过滤字段 (4 个 i32: necMinRunLen, necRunClass, necRequiredByte, necRequiredCount). */
        if (cur + 16 > blobLen) return -1;
        out->necMinRunLen = (int32_t)le_i32(blob + cur); cur += 4;
        out->necRunClass = (int32_t)le_i32(blob + cur); cur += 4;
        out->necRequiredByte = (int32_t)le_i32(blob + cur); cur += 4;
        out->necRequiredCount = (int32_t)le_i32(blob + cur); cur += 4;

        if (nword == 1) {
            out->condFirstEval = (uint64_t *)calloc(64, sizeof(uint64_t));
            out->condFollowEval = (uint64_t *)calloc((size_t)64 * npos, sizeof(uint64_t));
            out->condAcceptEval = (uint64_t *)calloc(64, sizeof(uint64_t));
            if (!out->condFirstEval || !out->condFollowEval || !out->condAcceptEval) return -1;
            for (int B = 0; B < 64; B++) {
                for (int k = 0; k < out->nCondFirst; k++) {
                    if ((out->condFirstGuard[k] & B) == out->condFirstGuard[k])
                        out->condFirstEval[B] |= out->condFirstBits[k];
                }
                for (int k = 0; k < out->nCondFollow; k++) {
                    if ((out->condFollowGuard[k] & B) == out->condFollowGuard[k]) {
                        int32_t p = out->condFollowPos[k];
                        if (p >= 0 && p < npos)
                            out->condFollowEval[(size_t)B * npos + p] |= out->condFollowBits[k];
                    }
                }
                for (int k = 0; k < out->nCondAccept; k++) {
                    if ((out->condAcceptGuard[k] & B) == out->condAcceptGuard[k])
                        out->condAcceptEval[B] |= out->condAcceptBits[k];
                }
            }
        }
    }

    uint64_t fu = 0;
    for (int w = 0; w < nword; w++) fu |= out->firstUnanchored[w];
    out->unanchoredEmpty = (fu == 0) ? 1 : 0;

    /* Vermicelli: 构建 startByteMask — 哪些字节可以开始匹配. */
    out->hasStartByteMask = 0;
    if (!out->unanchoredEmpty) {
        memset(out->startByteMask, 0, 256);
        int anyStart = 0;
        for (int b = 0; b < 128; b++) {
            int sym = (int)out->asciiSym[b];
            const uint64_t *rc = out->reach + (size_t)sym * nword;
            uint64_t overlap = 0;
            for (int w = 0; w < nword; w++) overlap |= (rc[w] & out->firstUnanchored[w]);
            if (overlap) { out->startByteMask[b] = 1; anyStart = 1; }
        }
        out->hasStartByteMask = anyStart ? 1 : 0;
    }

    /* DFA 转换 (可选, flags bit2 = hasDFA). 在所有其他字段之后.
     * 布局: i32 dfaNstates, {i32[nstates*256] next, u8[nstates] accept}. */
    out->hasDFA = (flags & 0x4u) ? 1 : 0;
    if (out->hasDFA) {
        if (cur + 4 > blobLen) return -1;
        out->dfaNstates = (int32_t)le_i32(blob + cur); cur += 4;
        if (out->dfaNstates <= 0 || out->dfaNstates > 512) return -1;
        size_t nextBytes = (size_t)out->dfaNstates * 256 * 4;
        size_t acceptBytes = (size_t)out->dfaNstates;
        if (cur + nextBytes + acceptBytes > blobLen) return -1;
        out->dfaNext = (int32_t *)malloc(nextBytes);
        out->dfaAccept = (uint8_t *)malloc(acceptBytes);
        if (!out->dfaNext || !out->dfaAccept) return -1;
        for (int _i = 0; _i < out->dfaNstates * 256; _i++) {
            out->dfaNext[_i] = (int32_t)le_i32(blob + cur + (size_t)_i * 4);
        }
        cur += nextBytes;
        for (int _i = 0; _i < out->dfaNstates; _i++) {
            out->dfaAccept[_i] = blob[cur + _i];
        }
        cur += acceptBytes;
    }
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
    free(u->chainTarget);
    free(u->excMask);
    free(u->excFollow);
    /* 断言扩展字段. */
    free(u->excFollow1Flat);
    free(u->condFirstGuard);
    free(u->condFirstBits);
    free(u->condFollowPos);
    free(u->condFollowGuard);
    free(u->condFollowBits);
    free(u->condAcceptGuard);
    free(u->condAcceptBits);
    free(u->condFirstEval);
    free(u->condFollowEval);
    free(u->condAcceptEval);
    free(u->dfaNext);
    free(u->dfaAccept);
    memset(u, 0, sizeof(*u));
}

/* db 头布局 (LE): magic[4]="MVS1", u32 version, u32 npat, i32 mergedUnit, u32 nUnits;
 *   i32[npat] slotUnit; u32[nUnits] unitOff; u32[nUnits] unitLen; units... */
mvscan_db *mvscan_db_open(const uint8_t *blob, size_t len) {
    if (!blob || len < 20) return NULL;
    if (blob[0] != 'M' || blob[1] != 'V' || blob[2] != 'S' || blob[3] != '1') return NULL;
    uint32_t version = le_u32(blob + 4);
    if (version != 3) return NULL;
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
    for (int i = 0; i < nUnits; i++) {
        uint32_t uoff = le_u32(offTab + (size_t)i * 4);
        uint32_t ulen = le_u32(lenTab + (size_t)i * 4);
        if ((size_t)uoff + (size_t)ulen > len ||
            parse_unit(blob, len, uoff, ulen, &db->units[i]) != 0) {
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

int32_t mvscan_db_nfa_find_all_1(const mvscan_db *db, int32_t idx,
                                  const uint8_t *data, size_t len,
                                  int32_t *out, int32_t capPairs) {
    if (!db || !data || idx < 0 || idx >= db->npat || capPairs < 0) return -1;
    int32_t u = db->slotUnit[idx];
    if (u < 0 || u >= db->nUnits) return -1;
    const mvs_nfa *a = &db->units[u];
    if (a->nword != 1 || a->hasAssert) return -1;

    int32_t total = 0;
    size_t pos = 0;
    while (pos <= len) {
        int32_t from, to;
        if (!nfa_find_loc_1(a, data, len, pos, &from, &to)) break;
        if (total < capPairs && out) {
            out[(size_t)total * 2] = from;
            out[(size_t)total * 2 + 1] = to;
        }
        total++;
        if (to > (int32_t)pos) pos = (size_t)to;
        else pos++;
    }
    return total;
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
    /* Rose-lite 必要条件预检: 数据不含任何能开始匹配的字节 => 跳过整段扫描. */
    mvs_nfa *mu = &db->units[db->mergedUnit];
    if (mu->hasStartByteMask) {
        int anyStart = 0;
        int hasNonASCII = 0;
        for (size_t i = 0; i < len; i++) {
            uint8_t b = data[i];
            if (b < 0x80) {
                if (mu->startByteMask[b]) { anyStart = 1; break; }
            } else {
                hasNonASCII = 1; break; /* 非 ASCII: 无法用 startByteMask 判定, 不跳过 */
            }
        }
        if (!anyStart && !hasNonASCII) return 0;
    }
    int32_t total = 0;
    nfa_run(mu, data, len, 1, seen, seenLen, out, cap, &total);
    return total;
}

/* mvscan_db_merged_scan_batch: 批量扫描多条记录, 每条记录独立.
 * 对每条记录跑 merged NFA (重置 prev=0), 把 (recIdx, memberIdx) 写入 out. */
int32_t mvscan_db_merged_scan_batch(const mvscan_db *db,
                                    const uint8_t *data, size_t totalLen,
                                    const int32_t *recOff, int32_t nrec,
                                    int32_t *out, int32_t capPairs) {
    if (!db || db->mergedUnit < 0 || db->mergedUnit >= db->nUnits) return 0;
    if (!data || !recOff || nrec <= 0 || !out) return 0;
    mvs_nfa *mu = &db->units[db->mergedUnit];
    int32_t total = 0;
    /* seen 缓冲: 每条记录重置 (per-record dedup). 用栈缓冲 (npat 通常 < 1024). */
    uint8_t stackSeen[1024];
    uint8_t *seen = (db->npat <= 1024) ? stackSeen : (uint8_t *)malloc((size_t)db->npat);
    if (!seen) return 0;
    int32_t stackOut[256]; /* 每条记录的临时输出 (通常命中数 < 5) */
    for (int32_t r = 0; r < nrec; r++) {
        size_t off0 = (size_t)recOff[r];
        size_t off1 = (size_t)recOff[r + 1];
        if (off0 >= totalLen || off1 > totalLen || off1 <= off0) continue;
        size_t rlen = off1 - off0;
        const uint8_t *rdata = data + off0;
        /* Rose-lite skip */
        if (mu->hasStartByteMask) {
            int anyStart = 0;
            int hasNonASCII = 0;
            for (size_t i = 0; i < rlen; i++) {
                uint8_t b = rdata[i];
                if (b < 0x80) {
                    if (mu->startByteMask[b]) { anyStart = 1; break; }
                } else {
                    hasNonASCII = 1; break;
                }
            }
            if (!anyStart && !hasNonASCII) continue;
        }
        /* 重置 seen */
        memset(seen, 0, (size_t)db->npat);
        int32_t recTotal = 0;
        nfa_run(mu, rdata, rlen, 1, seen, db->npat, stackOut, 256, &recTotal);
        /* 输出 (recIdx, memberIdx) 对 */
        for (int32_t k = 0; k < recTotal && k < 256; k++) {
            if (total < capPairs) {
                out[total * 2] = r;           /* recIdx */
                out[total * 2 + 1] = stackOut[k]; /* memberIdx */
            }
            total++;
        }
    }
    if (seen != stackSeen) free(seen);
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

/* mvscan_db_combined_scan: 单次数据遍历同时跑 merged NFA + 多个 assert NFA.
 * 消除第二次数据遍历 (merged + assert 各扫一遍 -> 合并为一次).
 * 对每个字节: 先推进 merged NFA 状态, 再推进每个 assert NFA 状态.
 * assert NFA 的边界条件与 rune 解码融合在线产出，避免预算边界 + NFA 的双趟扫描. */
int32_t mvscan_db_combined_scan(const mvscan_db *db,
                                const uint8_t *data, size_t len,
                                uint8_t *mergedSeen, int32_t mergedSeenLen,
                                int32_t *mergedOut, int32_t mergedCap,
                                int32_t *mergedTotalOut,
                                const int32_t *assertIdxs, int32_t nAssert,
                                uint8_t *boundBuf,
                                int32_t *assertOut, int32_t assertCap) {
    if (!db || !data || len == 0) return 0;

    /* 获取 assert NFA 单元 */
    mvs_nfa *stackAssertNFAs[16];
    mvs_nfa **assertNFAs = NULL;
    int assertNFAsHeap = 0;
    int32_t nAssertReal = 0;
    if (nAssert > 0 && assertIdxs) {
        if (nAssert <= 16) {
            assertNFAs = stackAssertNFAs;
        } else {
            assertNFAs = (mvs_nfa **)malloc((size_t)nAssert * sizeof(mvs_nfa *));
            assertNFAsHeap = 1;
        }
        if (!assertNFAs) return 0;
        for (int32_t i = 0; i < nAssert; i++) {
            int32_t idx = assertIdxs[i];
            if (idx < 0 || idx >= db->npat) { assertNFAs[i] = NULL; continue; }
            int32_t u = db->slotUnit[idx];
            if (u < 0 || u >= db->nUnits) { assertNFAs[i] = NULL; continue; }
            mvs_nfa *a = &db->units[u];
            if (!a->hasAssert) { assertNFAs[i] = NULL; continue; }
            assertNFAs[i] = a;
            nAssertReal++;
        }
    }

    /* 获取 merged NFA */
    mvs_nfa *mu = NULL;
    if (db->mergedUnit >= 0 && db->mergedUnit < db->nUnits) {
        mu = &db->units[db->mergedUnit];
        /* Rose-lite skip */
        if (mu->hasStartByteMask) {
            int anyStart = 0, hasNonASCII = 0;
            for (size_t i = 0; i < len; i++) {
                uint8_t b = data[i];
                if (b < 0x80) { if (mu->startByteMask[b]) { anyStart = 1; break; } }
                else { hasNonASCII = 1; break; }
            }
            if (!anyStart && !hasNonASCII) {
                /* merged 不会命中, 只跑 assert */
                mu = NULL;
            }
        }
    }

    /* 如果没有 merged 也没有 assert, 直接返回 */
    if (!mu && nAssertReal == 0) {
        if (assertNFAsHeap) free(assertNFAs);
        if (mergedTotalOut) *mergedTotalOut = 0;
        return 0;
    }

    /* 分配 merged NFA 工作缓冲 (用栈缓冲避免 malloc) */
    int mw = mu ? mu->nword : 0;
    uint64_t stackMPrev[8], stackMCand[8];
    uint64_t *mPrev = NULL, *mCand = NULL;
    if (mu) {
        if (mw <= 8) { mPrev = stackMPrev; mCand = stackMCand; memset(mPrev, 0, (size_t)mw * 8); }
        else { mPrev = (uint64_t *)calloc((size_t)mw, sizeof(uint64_t)); mCand = (uint64_t *)malloc((size_t)mw * sizeof(uint64_t)); }
    }

    /* 分配 assert NFA 工作缓冲 (用栈缓冲) */
    uint64_t stackAPrev[16];
    uint8_t stackADone[16];
    uint64_t *aPrev = NULL;
    uint8_t *aDone = NULL;
    if (nAssertReal > 0) {
        if (nAssert <= 16) { aPrev = stackAPrev; aDone = stackADone; memset(aPrev, 0, (size_t)nAssert * 8); memset(aDone, 0, (size_t)nAssert); }
        else { aPrev = (uint64_t *)calloc((size_t)nAssert, sizeof(uint64_t)); aDone = (uint8_t *)calloc((size_t)nAssert, 1); }
    }

    int32_t mergedTotal = 0;
    int32_t assertTotal = 0;

    size_t i = 0;
    int32_t curRune;
    int curSize = mvs_decode_rune(data, len, &curRune);
    uint8_t bpre = mvs_boundary_conds(-1, curRune);
    while (i < len) {
        /* 当前 rune 已由上一轮 lookahead 解码；同时窥视下一 rune，在线生成两侧边界。 */
        size_t ni = i + (size_t)curSize;
        int32_t nextRune = -1;
        int nextSize = 0;
        if (ni < len) nextSize = mvs_decode_rune(data + ni, len - ni, &nextRune);
        uint8_t bpost = mvs_boundary_conds(curRune, nextRune);
        if (boundBuf) {
            boundBuf[i] = bpre;
            boundBuf[ni] = bpost;
        }
        int sym = 0;
        if (mu) {
            sym = curRune >= 0 && curRune < 0x80 ? (int)mu->asciiSym[curRune] : symbol_of(mu, curRune);
        }
        int atStart = (i == 0);
        int atEnd = (ni == len);

        /* ---- merged NFA 递推 ---- */
        if (mu) {
            if (mu->hasLimEx) {
                uint64_t carry = 0;
                for (int w = 0; w < mw; w++) {
                    uint64_t v = mPrev[w];
                    uint64_t shifted = (v << 1) | carry;
                    carry = v >> 63;
                    mCand[w] = mu->firstUnanchored[w] | (shifted & mu->chainTarget[w]);
                }
                if (atStart && mu->hasAnchored) row_or_v(mCand, mu->firstAnchored, mw);
                for (int w = 0; w < mw; w++) {
                    uint64_t ex = mPrev[w] & mu->excMask[w];
                    while (ex) {
                        int p = (w << 6) + mvs_ctz64(ex);
                        ex &= ex - 1;
                        row_or_v(mCand, mu->excFollow + (size_t)p * mw, mw);
                    }
                }
            } else {
                row_copy_v(mCand, mu->firstUnanchored, mw);
                if (atStart && mu->hasAnchored) row_or_v(mCand, mu->firstAnchored, mw);
                for (int w = 0; w < mw; w++) {
                    uint64_t pw = mPrev[w];
                    while (pw) {
                        int p = (w << 6) + mvs_ctz64(pw);
                        pw &= pw - 1;
                        row_or_v(mCand, mu->follow + (size_t)p * mw, mw);
                    }
                }
            }
            const uint64_t *rc = mu->reach + (size_t)sym * mw;
            uint64_t anyActive = 0;
            for (int w = 0; w < mw; w++) {
                mPrev[w] = mCand[w] & rc[w];
                anyActive |= mPrev[w];
                uint64_t acc = mPrev[w] & mu->lastAny[w];
                if (atEnd) acc |= mPrev[w] & mu->lastEnd[w];
                while (acc) {
                    int p = (w << 6) + mvs_ctz64(acc);
                    acc &= acc - 1;
                    int32_t id = mu->posPat[p];
                    if (id >= 0 && id < mergedSeenLen && !mergedSeen[id]) {
                        mergedSeen[id] = 1;
                        if (mergedTotal < mergedCap) mergedOut[mergedTotal] = id;
                        mergedTotal++;
                    }
                }
            }
            /* Vermicelli skip for merged */
            if (anyActive == 0 && mu->hasStartByteMask) {
                size_t j = i + 1;
                while (j < len && data[j] < 0x80 && !mu->startByteMask[data[j]]) j++;
                if (j > ni) {
                    /* 跳过 — 但仍需推进 assert NFA */
                    /* 简化: 不跳, 逐字节走 (assert NFA 需要每字节) */
                }
            }
        }

        /* ---- assert NFA 递推 (每个单字 NFA) ---- */
        if (nAssertReal > 0 && boundBuf) {
            for (int32_t k = 0; k < nAssert; k++) {
                if (!assertNFAs[k] || aDone[k]) continue;
                mvs_nfa *a = assertNFAs[k];
                if (a->nword != 1) continue; /* 仅单字 assert */
				/* 每条 NFA 的压缩字母表独立，不能复用 merged unit 的 sym。 */
				int asym = curRune >= 0 && curRune < 0x80 ?
					(int)a->asciiSym[curRune] : symbol_of(a, curRune);

                uint64_t prev = aPrev[k];
                /* LimEx: 链边 + 异常 */
                uint64_t shifted = (prev << 1) & a->chainTarget1;
                uint64_t cand = a->firstUnanchored[0] | shifted;
                cand |= a->condFirstEval[bpre & 63u];
                /* exc */
                uint64_t exc = prev & a->excMask1;
                while (exc) {
                    int p = mvs_ctz64(exc);
                    exc &= exc - 1;
                    cand |= a->excFollow1Flat[p];
                }
                /* condFollow */
                uint64_t cfm = prev & a->condFollowMask1;
                while (cfm) {
                    int p = mvs_ctz64(cfm);
                    cfm &= cfm - 1;
                    cand |= a->condFollowEval[(size_t)(bpre & 63u) * a->npos + p];
                }
                /* active + accept */
                uint64_t active = cand & a->reach[(size_t)asym * a->nword];
                if (active & a->lastAny[0]) {
                    aDone[k] = 1;
                    if (assertTotal < assertCap) assertOut[assertTotal] = assertIdxs[k];
                    assertTotal++;
                }
                if (!aDone[k] && (active & a->condAcceptEval[bpost & 63u])) {
                    aDone[k] = 1;
                    if (assertTotal < assertCap) assertOut[assertTotal] = assertIdxs[k];
                    assertTotal++;
                }
                aPrev[k] = active;
            }
        }

        i = ni;
        curRune = nextRune;
        curSize = nextSize;
        bpre = bpost;
    }

    /* 清理 (仅 free 非栈缓冲) */
    if (mu && mw > 8) { free(mPrev); free(mCand); }
    if (nAssertReal > 0 && nAssert > 16) { free(aPrev); free(aDone); }
    if (assertNFAsHeap) free(assertNFAs);
    if (mergedTotalOut) *mergedTotalOut = mergedTotal;
    return assertTotal;
}

/* mvscan_db_dfa_scan_batch: 在单次调用中对多个 DFA 模式扫描同一段 data.
 * 对每个 idx 逐个跑 dfa_run, 把命中的 idx 写入 out. 返回命中数.
 * 非 ASCII 输入时 DFA 回退 NFA (逐个 idx). 一次 cgo 调用完成所有 DFA 模式. */
int32_t mvscan_db_dfa_scan_batch(const mvscan_db *db,
                                 const uint8_t *data, size_t len,
                                 const int32_t *idxs, int32_t nidx,
                                 int32_t *out, int32_t cap) {
    if (!db || !data || !idxs || nidx <= 0) return 0;
    int32_t total = 0;
    for (int32_t i = 0; i < nidx; i++) {
        int32_t idx = idxs[i];
        if (idx < 0 || idx >= db->npat) continue;
        int32_t u = db->slotUnit[idx];
        if (u < 0 || u >= db->nUnits) continue;
        mvs_nfa *a = &db->units[u];
        if (!a->hasDFA) continue;
        int r = dfa_run(a, data, len);
        if (r < 0) {
            /* 非 ASCII 回退 NFA */
            r = nfa_run_dispatch(a, data, len, 0, NULL, 0, NULL, 0, NULL, 0);
        }
        if (r == 1) {
            if (total < cap) out[total] = idx;
            total++;
        }
    }
    return total;
}

/* mvscan_db_nfa_exists_assert_self: 自包含断言扫描 — 内部预算边界, 不需外部 bound 参数.
 * 省去 Go 侧 sharedBound 一次 cgo 调用 (每报文省 1 次跨界). */
int mvscan_db_nfa_exists_assert_self(const mvscan_db *db, int32_t idx,
                                     const uint8_t *data, size_t len,
                                     uint8_t *boundBuf) {
    if (!db || idx < 0 || idx >= db->npat) return -1;
    int32_t u = db->slotUnit[idx];
    if (u < 0 || u >= db->nUnits) return -1;
    mvs_nfa *a = &db->units[u];
    if (!a->hasAssert) return -1;
    /* 内部预算边界 */
    if (data && len > 0 && boundBuf) {
        mvscan_compute_boundaries(data, len, boundBuf);
    } else if (boundBuf && len == 0) {
        boundBuf[0] = MVS_COND_BEGIN_TEXT | MVS_COND_END_TEXT | MVS_COND_BEGIN_LINE | MVS_COND_END_LINE | MVS_COND_NO_WORD_BOUND;
    }
    if (a->nword == 1)
        return nfa_run_assert_1(a, data, len, boundBuf);
    return nfa_run_assert_mw(a, data, len, boundBuf);
}

/* ====================================================================
 * 断言 NFA 公共 API (v2 blob 扩展).
 * ==================================================================== */

/* mvscan_compute_boundaries: 公共入口, 复刻 Go computeBoundaries. */
void mvscan_compute_boundaries_pub(const uint8_t *data, size_t len, uint8_t *buf) {
    mvscan_compute_boundaries(data, len, buf);
}

/* mvscan_db_nfa_exists_assert: 断言 NFA 单字存在性扫描 (含 guard 门控).
 * bound 由调用方预算 (mvscan_compute_boundaries_pub). 仅用于 hasAssert 且 nword==1 的 NFA.
 * 返回 1 命中 / 0 不命中 / -1 无 NFA 或非断言 NFA. */
int mvscan_db_nfa_exists_assert(const mvscan_db *db, int32_t idx,
                                const uint8_t *data, size_t len,
                                const uint8_t *bound) {
    if (!db || idx < 0 || idx >= db->npat) return -1;
    int32_t u = db->slotUnit[idx];
    if (u < 0 || u >= db->nUnits) return -1;
    mvs_nfa *a = &db->units[u];
    if (!a->hasAssert) return -1;
    if (a->nword == 1)
        return nfa_run_assert_1(a, data, len, bound);
    return nfa_run_assert_mw(a, data, len, bound);
}

/* mvscan_db_nfa_exists_assert_many: 一次 cgo 对多条断言 always-on NFA 各自做断言存在性,
 * 内部预算边界 (每报文一次), 摊薄 "每 pattern 一次 cgo" 的跨界开销.
 * boundBuf 由调用方提供 (容量 >= len+1). 仅用于 hasAssert 且 nword==1 的 NFA. */
void mvscan_db_nfa_exists_assert_many(const mvscan_db *db,
                                      const uint8_t *data, size_t len,
                                      const int32_t *idxs, int32_t nidx,
                                      uint8_t *boundBuf,
                                      uint8_t *out) {
    if (!out) return;
    if (nidx <= 0) return;
    /* 预算边界一次 (跨多条断言 NFA 共享). */
    if (data && len > 0 && boundBuf) {
        mvscan_compute_boundaries(data, len, boundBuf);
    } else if (boundBuf && len == 0) {
        boundBuf[0] = MVS_COND_BEGIN_TEXT | MVS_COND_END_TEXT | MVS_COND_BEGIN_LINE | MVS_COND_END_LINE | MVS_COND_NO_WORD_BOUND;
    }
    for (int32_t i = 0; i < nidx; i++) {
        uint8_t r = 0;
        if (db && idxs && boundBuf) {
            int32_t idx = idxs[i];
            if (idx >= 0 && idx < db->npat) {
                int32_t u = db->slotUnit[idx];
                if (u >= 0 && u < db->nUnits) {
                    mvs_nfa *a = &db->units[u];
                    if (a->hasAssert && a->nword == 1) {
                        r = (uint8_t)(nfa_run_assert_1(a, data, len, boundBuf) == 1);
                    }
                }
            }
        }
        out[i] = r;
    }
}
