/*
 * teddy.c - minirehs 自带的 SIMD 多字面量预过滤内核 (零外部依赖).
 *
 * 两套互补的字面量扫描器, 由 mrehs_pf_scan 在运行期择优分发, 输出契约一致 (end, litID):
 *
 *  1) Teddy (主力, 见 build_teddy / teddy_scan_*): 真正的 Teddy 指纹算法. 取每条字面量
 *     前 M 个字节 (M=min(最短字面量, 4), 要求 >=2) 作"指纹", 把字面量分到 8 个桶, 用 PSHUFB
 *     (x86 SSSE3) / TBL (arm64 NEON) 对每 16 字节窗口的 lo/hi nibble 做并行成员查表, 一次
 *     得到 16 个起始位置各自"可能命中的桶位图". 仅在位图非零处做 memcmp 确认 (confirm) 并
 *     产出精确命中. 跨块用"重叠非对齐读"取 M 个偏移向量按位与 (规避跨 lane 移位的进位易错点),
 *     正确性等价于标量逐位置查表.
 *
 *  2) Aho-Corasick + shufti (回退, 见 mrehs_pf_scan 的 ac 分支): 当 Teddy 不适用 (存在长度 1
 *     的字面量, M<2) 时使用. 复用 Go 侧构建的 AC 转移表, 根状态用 shufti 风格 SIMD 跳过.
 *
 * 跨架构: x86_64 走 SSE2 基线 + 运行期探测 SSSE3 (pshufb); arm64 走 NEON; 其它架构走标量孪生.
 * 各分支用 #if defined 架构宏自我隔离, 任意架构都能编译. 标量孪生 (teddy_scan_scalar /
 * skip_to_active 标量尾) 既是非 SIMD 架构的实现, 也是 SIMD 路径的差分参照.
 *
 * 正确性: 预过滤只产出候选 (允许假阳, 绝无假阴), 真伪由 Go 侧完整正则验证判定. Teddy 的指纹
 * 过滤是保守的 (匹配指纹才确认), confirm 用 memcmp 精确比对, 故与 AC 产出同一命中集合.
 *
 * 关键词: minirehs, prefilter, Teddy, fingerprint, SIMD, shufti, PSHUFB, NEON, Aho-Corasick
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

#define TEDDY_BUCKETS 8 /* 1 字节位图 = 8 个桶 */
#define TEDDY_MAXM 4    /* 指纹最大长度 */

typedef struct mrehs_pf {
    int32_t  *next;     /* AC: numStates * 256 转移表 */
    int32_t  *outOff;   /* AC: numStates + 1 输出偏移 */
    int32_t  *outFlat;  /* AC: 压平的命中 litID 列表 */
    int32_t   numStates;
    int32_t   numLit;
    int32_t   numLitWords;

    /* shufti 根跳过表 (AC 回退用). */
    uint8_t   loMask[16];
    uint8_t   hiMask[16];
    uint8_t   active[256];

    /* ---- Teddy ---- */
    int32_t   useTeddy;       /* 1 表示扫描走 Teddy, 否则走 AC */
    int32_t   teddyM;         /* 指纹长度 1..4 */
    uint8_t   tLo[TEDDY_MAXM][16]; /* tLo[m][nibble] = 第 m 指纹字节低半字节命中的桶位图 */
    uint8_t   tHi[TEDDY_MAXM][16];
    uint8_t  *litBytes;       /* 拼接的字面量字节 (已小写) */
    int32_t  *litOff;         /* numLit+1 偏移; 第 j 条 = litBytes[litOff[j]..litOff[j+1]) */
    int32_t   bkOff[TEDDY_BUCKETS + 1]; /* 桶 -> bkLit 区间 */
    int32_t  *bkLit;          /* 按桶分组的字面量 id (长度 numLit) */
} mrehs_pf;

/* ---- AC 回退的 shufti 根跳过表 ---- */
static void build_shufti(mrehs_pf *pf) {
    memset(pf->loMask, 0, sizeof(pf->loMask));
    memset(pf->hiMask, 0, sizeof(pf->hiMask));
    memset(pf->active, 0, sizeof(pf->active));
    int bucket = 0;
    for (int c = 0; c < 256; c++) {
        if (pf->next[c] != 0) {
            pf->active[c] = 1;
            uint8_t bit = (uint8_t)(1u << (bucket & 7));
            pf->loMask[c & 0xf] |= bit;
            pf->hiMask[(c >> 4) & 0xf] |= bit;
            bucket++;
        }
    }
}

/* ---- Teddy 构建 ---- */
/* 入参: litFlat 拼接字面量字节, litOff 长度 numLit+1 的偏移表. 成功置 useTeddy=1. */
static void build_teddy(mrehs_pf *pf, const uint8_t *litFlat, const int32_t *litOff,
                        int32_t numLit) {
    pf->useTeddy = 0;
    if (numLit <= 0) return;

    int32_t minLen = 0x7fffffff;
    for (int32_t j = 0; j < numLit; j++) {
        int32_t l = litOff[j + 1] - litOff[j];
        if (l < minLen) minLen = l;
    }
    int M = minLen;
    if (M > TEDDY_MAXM) M = TEDDY_MAXM;
    if (M < 2) return; /* 存在长度 1 字面量: 交 AC 回退 (Teddy 选择性太差) */

    int32_t total = litOff[numLit];
    pf->litBytes = (uint8_t *)malloc(total > 0 ? (size_t)total : 1);
    pf->litOff = (int32_t *)malloc((size_t)(numLit + 1) * sizeof(int32_t));
    pf->bkLit = (int32_t *)malloc((size_t)numLit * sizeof(int32_t));
    if (!pf->litBytes || !pf->litOff || !pf->bkLit) {
        free(pf->litBytes); free(pf->litOff); free(pf->bkLit);
        pf->litBytes = NULL; pf->litOff = NULL; pf->bkLit = NULL;
        return;
    }
    if (total > 0) memcpy(pf->litBytes, litFlat, (size_t)total);
    memcpy(pf->litOff, litOff, (size_t)(numLit + 1) * sizeof(int32_t));

    pf->teddyM = M;
    memset(pf->tLo, 0, sizeof(pf->tLo));
    memset(pf->tHi, 0, sizeof(pf->tHi));

    /* 桶计数 -> 偏移 (bucket = j % 8). */
    int32_t cnt[TEDDY_BUCKETS] = {0};
    for (int32_t j = 0; j < numLit; j++) cnt[j % TEDDY_BUCKETS]++;
    pf->bkOff[0] = 0;
    for (int b = 0; b < TEDDY_BUCKETS; b++) pf->bkOff[b + 1] = pf->bkOff[b] + cnt[b];
    int32_t fill[TEDDY_BUCKETS];
    for (int b = 0; b < TEDDY_BUCKETS; b++) fill[b] = pf->bkOff[b];

    for (int32_t j = 0; j < numLit; j++) {
        int b = (int)(j % TEDDY_BUCKETS);
        pf->bkLit[fill[b]++] = j;
        const uint8_t *p = pf->litBytes + litOff[j];
        uint8_t bit = (uint8_t)(1u << b);
        for (int m = 0; m < M; m++) {
            uint8_t by = p[m];
            pf->tLo[m][by & 0xf] |= bit;
            pf->tHi[m][(by >> 4) & 0xf] |= bit;
        }
    }
    pf->useTeddy = 1;
}

mrehs_pf *mrehs_pf_new(const int32_t *next, int32_t numStates,
                       const int32_t *outOff, const int32_t *outFlat,
                       int32_t outFlatLen, int32_t numLit,
                       const uint8_t *litFlat, const int32_t *litOff) {
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
        free(pf->next); free(pf->outOff); free(pf->outFlat);
        free(pf);
        return NULL;
    }
    memcpy(pf->next, next, nextBytes);
    memcpy(pf->outOff, outOff, offBytes);
    if (flatBytes > 0) memcpy(pf->outFlat, outFlat, flatBytes);

    build_shufti(pf);
    if (litFlat != NULL && litOff != NULL) {
        build_teddy(pf, litFlat, litOff, numLit);
    }
    return pf;
}

void mrehs_pf_free(mrehs_pf *pf) {
    if (!pf) return;
    free(pf->next);
    free(pf->outOff);
    free(pf->outFlat);
    free(pf->litBytes);
    free(pf->litOff);
    free(pf->bkLit);
    free(pf);
}

/* confirm: 在位置 pos 对 bucketmap 中每个桶的字面量做 memcmp, 命中写入 (end,litID).
 * 返回新的 total (可能 > cap, 表示截断). */
static inline int32_t teddy_confirm(const mrehs_pf *pf, const uint8_t *data, size_t len,
                                    size_t pos, uint8_t bucketmap,
                                    int32_t *out, int32_t cap, int32_t total) {
    while (bucketmap) {
        int b = __builtin_ctz((unsigned)bucketmap);
        bucketmap &= (uint8_t)(bucketmap - 1);
        int32_t s = pf->bkOff[b], e = pf->bkOff[b + 1];
        for (int32_t jj = s; jj < e; jj++) {
            int32_t j = pf->bkLit[jj];
            int32_t off = pf->litOff[j];
            int32_t l = pf->litOff[j + 1] - off;
            if (pos + (size_t)l <= len && memcmp(data + pos, pf->litBytes + off, (size_t)l) == 0) {
                if (total < cap) {
                    out[total * 2] = (int32_t)(pos + (size_t)l); /* end (exclusive) */
                    out[total * 2 + 1] = j;                      /* litID == 字面量下标 */
                }
                total++;
            }
        }
    }
    return total;
}

/* Teddy 标量孪生: 逐起始位置查表取桶位图, 非零则确认. 全平台正确, 亦作 SIMD 差分参照. */
static int32_t teddy_scan_scalar(const mrehs_pf *pf, const uint8_t *data, size_t len,
                                 int32_t *out, int32_t cap) {
    int M = pf->teddyM;
    int32_t total = 0;
    if (len < (size_t)M) return 0;
    for (size_t pos = 0; pos + (size_t)M <= len; pos++) {
        uint8_t bm = 0xff;
        for (int m = 0; m < M; m++) {
            uint8_t by = data[pos + (size_t)m];
            bm &= (uint8_t)(pf->tLo[m][by & 0xf] & pf->tHi[m][(by >> 4) & 0xf]);
            if (bm == 0) break;
        }
        if (bm) total = teddy_confirm(pf, data, len, pos, bm, out, cap, total);
    }
    return total;
}

#if defined(MREHS_X86_SSSE3)
__attribute__((target("ssse3")))
static int32_t teddy_scan_ssse3(const mrehs_pf *pf, const uint8_t *data, size_t len,
                                int32_t *out, int32_t cap) {
    int M = pf->teddyM;
    int32_t total = 0;
    if (len < (size_t)M) return 0;
    __m128i loV[TEDDY_MAXM], hiV[TEDDY_MAXM];
    for (int m = 0; m < M; m++) {
        loV[m] = _mm_loadu_si128((const __m128i *)pf->tLo[m]);
        hiV[m] = _mm_loadu_si128((const __m128i *)pf->tHi[m]);
    }
    __m128i lowNib = _mm_set1_epi8(0x0f);
    __m128i zero = _mm_setzero_si128();
    __m128i ones = _mm_set1_epi8((char)0xff);

    size_t i = 0;
    /* 重叠非对齐读: 处理起始位置 i..i+15, 需读到 data[i+15+(M-1)]. */
    while (i + 16 + (size_t)(M - 1) <= len) {
        __m128i cand = ones;
        for (int m = 0; m < M; m++) {
            __m128i v = _mm_loadu_si128((const __m128i *)(data + i + (size_t)m));
            __m128i lo = _mm_and_si128(v, lowNib);
            __m128i hi = _mm_and_si128(_mm_srli_epi16(v, 4), lowNib);
            __m128i rl = _mm_shuffle_epi8(loV[m], lo);
            __m128i rh = _mm_shuffle_epi8(hiV[m], hi);
            cand = _mm_and_si128(cand, _mm_and_si128(rl, rh));
        }
        __m128i eqz = _mm_cmpeq_epi8(cand, zero);
        int mask = _mm_movemask_epi8(eqz); /* bit=1 => 该 lane 为 0 (无候选) */
        if (mask != 0xffff) {
            uint8_t buf[16];
            _mm_storeu_si128((__m128i *)buf, cand);
            for (int k = 0; k < 16; k++) {
                if (buf[k]) total = teddy_confirm(pf, data, len, i + (size_t)k, buf[k], out, cap, total);
            }
        }
        i += 16;
    }
    for (; i + (size_t)M <= len; i++) {
        uint8_t bm = 0xff;
        for (int m = 0; m < M; m++) {
            uint8_t by = data[i + (size_t)m];
            bm &= (uint8_t)(pf->tLo[m][by & 0xf] & pf->tHi[m][(by >> 4) & 0xf]);
            if (bm == 0) break;
        }
        if (bm) total = teddy_confirm(pf, data, len, i, bm, out, cap, total);
    }
    return total;
}
#endif

#if defined(MREHS_ARM64)
static int32_t teddy_scan_neon(const mrehs_pf *pf, const uint8_t *data, size_t len,
                               int32_t *out, int32_t cap) {
    int M = pf->teddyM;
    int32_t total = 0;
    if (len < (size_t)M) return 0;
    uint8x16_t loV[TEDDY_MAXM], hiV[TEDDY_MAXM];
    for (int m = 0; m < M; m++) {
        loV[m] = vld1q_u8(pf->tLo[m]);
        hiV[m] = vld1q_u8(pf->tHi[m]);
    }
    uint8x16_t lowNib = vdupq_n_u8(0x0f);

    size_t i = 0;
    while (i + 16 + (size_t)(M - 1) <= len) {
        uint8x16_t cand = vdupq_n_u8(0xff);
        for (int m = 0; m < M; m++) {
            uint8x16_t v = vld1q_u8(data + i + (size_t)m);
            uint8x16_t lo = vandq_u8(v, lowNib);
            uint8x16_t hi = vandq_u8(vshrq_n_u8(v, 4), lowNib);
            uint8x16_t rl = vqtbl1q_u8(loV[m], lo);
            uint8x16_t rh = vqtbl1q_u8(hiV[m], hi);
            cand = vandq_u8(cand, vandq_u8(rl, rh));
        }
        if (vmaxvq_u8(cand) != 0) {
            uint8_t buf[16];
            vst1q_u8(buf, cand);
            for (int k = 0; k < 16; k++) {
                if (buf[k]) total = teddy_confirm(pf, data, len, i + (size_t)k, buf[k], out, cap, total);
            }
        }
        i += 16;
    }
    for (; i + (size_t)M <= len; i++) {
        uint8_t bm = 0xff;
        for (int m = 0; m < M; m++) {
            uint8_t by = data[i + (size_t)m];
            bm &= (uint8_t)(pf->tLo[m][by & 0xf] & pf->tHi[m][(by >> 4) & 0xf]);
            if (bm == 0) break;
        }
        if (bm) total = teddy_confirm(pf, data, len, i, bm, out, cap, total);
    }
    return total;
}
#endif

/* Teddy 运行期分发: SSSE3 / NEON / 标量孪生. */
static inline int32_t teddy_scan(const mrehs_pf *pf, const uint8_t *data, size_t len,
                                 int32_t *out, int32_t cap) {
#if defined(MREHS_X86_SSSE3)
    if (__builtin_cpu_supports("ssse3")) {
        return teddy_scan_ssse3(pf, data, len, out, cap);
    }
    return teddy_scan_scalar(pf, data, len, out, cap);
#elif defined(MREHS_ARM64)
    return teddy_scan_neon(pf, data, len, out, cap);
#else
    return teddy_scan_scalar(pf, data, len, out, cap);
#endif
}

/* ---- AC 回退: 根状态 shufti SIMD 跳过 ---- */
#if defined(MREHS_X86_SSSE3)
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
        int mask = _mm_movemask_epi8(eqz);
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

static inline size_t skip_to_active(const mrehs_pf *pf, const uint8_t *data,
                                    size_t i, size_t len) {
#if defined(MREHS_X86_SSSE3)
    if (__builtin_cpu_supports("ssse3")) {
        return skip_to_active_ssse3(pf, data, i, len);
    }
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
        if (vmaxvq_u8(r) != 0) {
            for (int k = 0; k < 16; k++) {
                if (pf->active[data[i + k]]) return i + k;
            }
        }
        i += 16;
    }
#endif
    while (i < len) {
        if (pf->active[data[i]]) return i;
        i++;
    }
    return len;
}

static int32_t ac_scan(const mrehs_pf *pf, const uint8_t *data, size_t len,
                       int32_t *outPairs, int32_t capPairs) {
    const int32_t *next = pf->next;
    const int32_t *outOff = pf->outOff;
    const int32_t *outFlat = pf->outFlat;
    int32_t state = 0;
    int32_t total = 0;
    size_t i = 0;
    while (i < len) {
        if (state == 0) {
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

/*
 * 扫描 data, 把每个字面量命中作为 (end, litID) 对写入 outPairs (容量 capPairs 对).
 * 返回命中总对数 (可能 > capPairs, 表示截断, 调用方扩容重扫). Teddy 适用则走 Teddy, 否则 AC.
 */
int32_t mrehs_pf_scan(const mrehs_pf *pf, const uint8_t *data, size_t len,
                      int32_t *outPairs, int32_t capPairs) {
    if (pf->useTeddy) {
        return teddy_scan(pf, data, len, outPairs, capPairs);
    }
    return ac_scan(pf, data, len, outPairs, capPairs);
}

/* 强制走标量孪生 (Teddy 标量 / AC). 供差分测试对照 SIMD 分发结果, 二者命中集合必须逐项一致. */
int32_t mrehs_pf_scan_scalar(const mrehs_pf *pf, const uint8_t *data, size_t len,
                             int32_t *outPairs, int32_t capPairs) {
    if (pf->useTeddy) {
        return teddy_scan_scalar(pf, data, len, outPairs, capPairs);
    }
    return ac_scan(pf, data, len, outPairs, capPairs);
}

/* 可观测: 报告是否启用 Teddy 及指纹长度 (供测试/日志). */
int32_t mrehs_pf_use_teddy(const mrehs_pf *pf) { return pf->useTeddy; }
int32_t mrehs_pf_teddy_m(const mrehs_pf *pf) { return pf->teddyM; }
