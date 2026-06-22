/*
 * teddy.c - minirehs 自带的 SIMD 多字面量预过滤内核 (零外部依赖).
 *
 * 设计: 复用 Go 侧构建好的 Aho-Corasick 转移表, 在 C 中以紧凑循环扫描 (无 Go 边界检查
 * 与逐字节回调开销), 并对"处于根状态"的常见情形用 shufti 风格 SIMD 跳过非起始字节,
 * 从而在低命中文本上以向量速度快进. 真伪由 Go 侧完整正则验证判定, 此处只产出候选.
 *
 * 跨架构: x86_64 走 SSE2 (基线, 运行时再按 SSSE3 分发), arm64 走 NEON, 其它架构走标量.
 * 各分支用 #if defined 架构宏自我隔离, 保证任意架构都能编译.
 *
 * 正确性: shufti 跳过过滤只产生假阳 (多检查几个位置), 绝无假阴; AC 扫描本身精确.
 *
 * 关键词: minirehs, prefilter, SIMD, shufti, Aho-Corasick, NEON, SSE2
 */

#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>

#if defined(__x86_64__) || defined(_M_X64)
#include <emmintrin.h> /* SSE2 */
#define MREHS_X86 1
#if defined(__GNUC__) || defined(__clang__)
#include <tmmintrin.h> /* SSSE3 intrinsics (pshufb) */
#define MREHS_X86_SSSE3 1
#endif
#elif defined(__aarch64__) || defined(_M_ARM64)
#include <arm_neon.h>
#define MREHS_ARM64 1
#endif

typedef struct mrehs_pf {
    int32_t  *next;     /* numStates * 256 转移表 */
    int32_t  *outOff;   /* numStates + 1 输出偏移 */
    int32_t  *outFlat;  /* 压平的命中 litID 列表 */
    int32_t   numStates;
    int32_t   numLit;
    int32_t   numLitWords; /* (numLit + 63) / 64 */

    /* shufti 根跳过表: 仅对"能作为某字面量首字节"的字节集合做 SIMD 成员测试. */
    uint8_t   loMask[16];
    uint8_t   hiMask[16];
    uint8_t   active[256]; /* active[c]=1 表示 c 可作为字面量首字节 */
} mrehs_pf;

/* 由根行 next[0..255] 推导首字节集合, 并构建 shufti 的 lo/hi nibble 掩码. */
static void build_shufti(mrehs_pf *pf) {
    memset(pf->loMask, 0, sizeof(pf->loMask));
    memset(pf->hiMask, 0, sizeof(pf->hiMask));
    memset(pf->active, 0, sizeof(pf->active));
    int bucket = 0;
    for (int c = 0; c < 256; c++) {
        /* 根状态对字节 c 的转移非 0, 说明 c 可作为某字面量首字节. */
        if (pf->next[c] != 0) {
            pf->active[c] = 1;
            uint8_t bit = (uint8_t)(1u << (bucket & 7));
            pf->loMask[c & 0xf] |= bit;
            pf->hiMask[(c >> 4) & 0xf] |= bit;
            bucket++;
        }
    }
}

mrehs_pf *mrehs_pf_new(const int32_t *next, int32_t numStates,
                       const int32_t *outOff, const int32_t *outFlat,
                       int32_t outFlatLen, int32_t numLit) {
    mrehs_pf *pf = (mrehs_pf *)calloc(1, sizeof(mrehs_pf));
    if (!pf) return NULL;
    pf->numStates = numStates;
    pf->numLit = numLit;
    pf->numLitWords = (numLit + 63) / 64;

    size_t nextBytes = (size_t)numStates * 256 * sizeof(int32_t);
    size_t offBytes = (size_t)(numStates + 1) * sizeof(int32_t);
    size_t flatBytes = (size_t)outFlatLen * sizeof(int32_t);

    pf->next = (int32_t *)malloc(nextBytes);
    pf->outOff = (int32_t *)malloc(offBytes);
    pf->outFlat = (int32_t *)malloc(flatBytes > 0 ? flatBytes : 1);
    if (!pf->next || !pf->outOff || !pf->outFlat) {
        if (pf->next) free(pf->next);
        if (pf->outOff) free(pf->outOff);
        if (pf->outFlat) free(pf->outFlat);
        free(pf);
        return NULL;
    }
    memcpy(pf->next, next, nextBytes);
    memcpy(pf->outOff, outOff, offBytes);
    if (flatBytes > 0) memcpy(pf->outFlat, outFlat, flatBytes);

    build_shufti(pf);
    return pf;
}

void mrehs_pf_free(mrehs_pf *pf) {
    if (!pf) return;
    free(pf->next);
    free(pf->outOff);
    free(pf->outFlat);
    free(pf);
}

#if defined(MREHS_X86_SSSE3)
/* SSSE3 shufti: 用 pshufb 做 lo/hi nibble 成员查表, 一次处理 16 字节.
 * 用 target 属性局部启用 SSSE3, 由调用方按 __builtin_cpu_supports 运行时分发. */
__attribute__((target("ssse3")))
static size_t skip_to_active_ssse3(const mrehs_pf *pf, const uint8_t *data,
                                   size_t i, size_t len) {
    __m128i loV = _mm_loadu_si128((const __m128i *)pf->loMask);
    __m128i hiV = _mm_loadu_si128((const __m128i *)pf->hiMask);
    __m128i lowNibMask = _mm_set1_epi8(0x0f);
    __m128i zero = _mm_setzero_si128();
    while (i + 16 <= len) {
        __m128i v = _mm_loadu_si128((const __m128i *)(data + i));
        __m128i lo = _mm_and_si128(v, lowNibMask);
        __m128i hi = _mm_and_si128(_mm_srli_epi16(v, 4), lowNibMask);
        __m128i a = _mm_shuffle_epi8(loV, lo);
        __m128i b = _mm_shuffle_epi8(hiV, hi);
        __m128i r = _mm_and_si128(a, b);
        __m128i eqz = _mm_cmpeq_epi8(r, zero);
        int mask = _mm_movemask_epi8(eqz); /* bit=1 表示该 lane 为 0 (非候选) */
        if (mask != 0xffff) {
            for (int k = 0; k < 16; k++) {
                if (pf->active[data[i + k]]) return i + k;
            }
        }
        i += 16;
    }
    while (i < len) {
        if (pf->active[data[i]]) return i;
        i++;
    }
    return len;
}
#endif

/* 在根状态下从 data[i..len) 找到下一个可能的字面量首字节位置, 用 SIMD 加速跳过. */
static inline size_t skip_to_active(const mrehs_pf *pf, const uint8_t *data,
                                    size_t i, size_t len) {
#if defined(MREHS_X86_SSSE3)
    if (__builtin_cpu_supports("ssse3")) {
        return skip_to_active_ssse3(pf, data, i, len);
    }
    /* 无 SSSE3 的老 x86: 标量窗口检查 (仍受益于紧凑循环). */
    while (i < len) {
        if (pf->active[data[i]]) return i;
        i++;
    }
    return len;
#elif defined(MREHS_X86)
    while (i < len) {
        if (pf->active[data[i]]) return i;
        i++;
    }
    return len;
#elif defined(MREHS_ARM64)
    uint8x16_t loV = vld1q_u8(pf->loMask);
    uint8x16_t hiV = vld1q_u8(pf->hiMask);
    uint8x16_t lowNib = vdupq_n_u8(0x0f);
    while (i + 16 <= len) {
        uint8x16_t v = vld1q_u8(data + i);
        uint8x16_t lo = vandq_u8(v, lowNib);
        uint8x16_t hi = vandq_u8(vshrq_n_u8(v, 4), lowNib);
        uint8x16_t a = vqtbl1q_u8(loV, lo);
        uint8x16_t b = vqtbl1q_u8(hiV, hi);
        uint8x16_t r = vandq_u8(a, b);
        /* r 中非零的 lane 即候选位置. 取最大值快速判断窗口是否全空. */
        if (vmaxvq_u8(r) != 0) {
            for (int k = 0; k < 16; k++) {
                if (pf->active[data[i + k]]) return i + k;
            }
        }
        i += 16;
    }
#endif
    /* 标量尾巴 / 非 SIMD 架构. */
    while (i < len) {
        if (pf->active[data[i]]) return i;
        i++;
    }
    return len;
}

/*
 * 扫描 data, 把每个字面量命中作为 (end, litID) 对写入 outPairs (容量 capPairs 对).
 * 返回命中的总对数 (可能 > capPairs); 若返回值 > capPairs 表示缓冲不足、发生截断,
 * 调用方应扩容后重扫. end 是字面量在 data 中的结束位置 (exclusive).
 */
int32_t mrehs_pf_scan(const mrehs_pf *pf, const uint8_t *data, size_t len,
                      int32_t *outPairs, int32_t capPairs) {
    const int32_t *next = pf->next;
    const int32_t *outOff = pf->outOff;
    const int32_t *outFlat = pf->outFlat;
    int32_t state = 0;
    int32_t total = 0;
    size_t i = 0;
    while (i < len) {
        if (state == 0) {
            /* 处于根状态: 用 SIMD 跳过不可能开始任何字面量的连续字节. */
            i = skip_to_active(pf, data, i, len);
            if (i >= len) break;
        }
        state = next[(state << 8) | data[i]];
        int32_t off = outOff[state];
        int32_t end = outOff[state + 1];
        for (; off < end; off++) {
            if (total < capPairs) {
                outPairs[total * 2] = (int32_t)(i + 1);
                outPairs[total * 2 + 1] = outFlat[off];
            }
            total++;
        }
        i++;
    }
    return total;
}
