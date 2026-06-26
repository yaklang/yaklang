/*
 * fixture_check.c - amalgamation 单文件的真实负载校验器 (供 ASan/UBSan 深度护栏).
 *
 * 读取由 Go 测试 TestMVSEmitKernelFixture 导出的三件套 (blob.bin / inputs.bin / expected.txt),
 * 用单文件内核对每条真实流量跑 "合并 always-on 扫描 + 逐 pattern 存在性", 累计命中数与 Go 参考
 * 期望比对. 它只编 mvscan.c + 本文件 (零依赖), 故可在 ASan+UBSan 下秒级跑真实 NFA 执行路径
 * (parse_unit / nfa_run / utf8 解码 / 合并发射), 暴露任何越界 / 未定义行为.
 *
 * 用法:
 *   gcc -O1 -g -std=c99 -fsanitize=address,undefined -fno-sanitize-recover=all \
 *       mvscan.c fixture_check.c -o fixture_check
 *   ./fixture_check /path/to/fixture_dir
 *
 * 退出码 0 且打印 "fixture check OK" 表示通过.
 */
#include "mvscan.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static uint8_t *read_file(const char *path, size_t *outLen) {
    FILE *f = fopen(path, "rb");
    if (!f) { fprintf(stderr, "open %s failed\n", path); return NULL; }
    if (fseek(f, 0, SEEK_END) != 0) { fclose(f); return NULL; }
    long sz = ftell(f);
    if (sz < 0) { fclose(f); return NULL; }
    if (fseek(f, 0, SEEK_SET) != 0) { fclose(f); return NULL; }
    uint8_t *buf = (uint8_t *)malloc((size_t)sz + 1);
    if (!buf) { fclose(f); return NULL; }
    size_t rd = fread(buf, 1, (size_t)sz, f);
    fclose(f);
    if (rd != (size_t)sz) { free(buf); return NULL; }
    buf[sz] = 0;
    *outLen = (size_t)sz;
    return buf;
}

static uint32_t le32(const uint8_t *p) {
    return (uint32_t)p[0] | ((uint32_t)p[1] << 8) |
           ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "usage: %s <fixture_dir>\n", argv[0]);
        return 2;
    }
    char path[4096];
    size_t blobLen = 0, inLen = 0, expLen = 0;

    snprintf(path, sizeof(path), "%s/blob.bin", argv[1]);
    uint8_t *blob = read_file(path, &blobLen);
    snprintf(path, sizeof(path), "%s/inputs.bin", argv[1]);
    uint8_t *inputs = read_file(path, &inLen);
    snprintf(path, sizeof(path), "%s/expected.txt", argv[1]);
    uint8_t *expected = read_file(path, &expLen);
    if (!blob || !inputs || !expected) {
        fprintf(stderr, "FAIL: cannot read fixture files\n");
        return 2;
    }

    long expNpat = 0, expMerged = 0, expExists = 0;
    if (sscanf((const char *)expected, "%ld %ld %ld", &expNpat, &expMerged, &expExists) != 3) {
        fprintf(stderr, "FAIL: bad expected.txt\n");
        return 2;
    }

    mvscan_db *db = mvscan_db_open(blob, blobLen);
    if (!db) { fprintf(stderr, "FAIL: mvscan_db_open returned NULL\n"); return 1; }

    int32_t npat = mvscan_db_npat(db);
    if (npat != (int32_t)expNpat) {
        fprintf(stderr, "FAIL: npat C=%d expected=%ld\n", npat, expNpat);
        return 1;
    }

    /* 去重位图 + 命中输出缓冲 (容量 npat 必不截断). */
    uint8_t *seen = (uint8_t *)malloc((size_t)(npat > 0 ? npat : 1));
    int32_t *out = (int32_t *)malloc((size_t)(npat > 0 ? npat : 1) * sizeof(int32_t));
    if (!seen || !out) { fprintf(stderr, "FAIL: oom\n"); return 1; }

    long gotMerged = 0, gotExists = 0;

    if (inLen < 4) { fprintf(stderr, "FAIL: inputs too short\n"); return 1; }
    uint32_t count = le32(inputs);
    size_t cur = 4;
    for (uint32_t k = 0; k < count; k++) {
        if (cur + 4 > inLen) { fprintf(stderr, "FAIL: truncated inputs at %u\n", k); return 1; }
        uint32_t rlen = le32(inputs + cur);
        cur += 4;
        if (cur + rlen > inLen) { fprintf(stderr, "FAIL: truncated record at %u\n", k); return 1; }
        const uint8_t *rec = inputs + cur;
        cur += rlen;

        if (mvscan_db_has_merged(db)) {
            memset(seen, 0, (size_t)npat);
            int32_t n = mvscan_db_merged_scan(db, rec, rlen, seen, npat, out, npat);
            if (n < 0) { fprintf(stderr, "FAIL: merged_scan returned %d\n", n); return 1; }
            if (n > npat) n = npat; /* 理论不发生 */
            gotMerged += n;
        }
        for (int32_t idx = 0; idx < npat; idx++) {
            int r = mvscan_db_nfa_exists(db, idx, rec, rlen);
            if (r == 1) gotExists++;
        }
    }

    int rc = 0;
    if (gotMerged != expMerged) {
        fprintf(stderr, "FAIL: totalMerged C=%ld expected=%ld\n", gotMerged, expMerged);
        rc = 1;
    }
    if (gotExists != expExists) {
        fprintf(stderr, "FAIL: totalExists C=%ld expected=%ld\n", gotExists, expExists);
        rc = 1;
    }

    if (rc == 0) {
        printf("fixture check OK: inputs=%u npat=%d totalMerged=%ld totalExists=%ld (simd=%d)\n",
               count, npat, gotMerged, gotExists, mvscan_simd_enabled());
    }

    mvscan_db_close(db);
    free(seen);
    free(out);
    free(blob);
    free(inputs);
    free(expected);
    return rc;
}
