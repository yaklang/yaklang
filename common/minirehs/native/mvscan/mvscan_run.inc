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
        int32_t r;
        int size = mvs_decode_rune(data + i, len - i, &r);
        i += (size_t)size;
        int sym = symbol_of(a, r);
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
