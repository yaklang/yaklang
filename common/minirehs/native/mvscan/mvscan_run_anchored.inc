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
    }

done:
    if (nword > 8) { free(prev); free(cand); }
    return found;
}