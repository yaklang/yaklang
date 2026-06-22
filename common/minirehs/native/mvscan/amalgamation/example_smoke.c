/*
 * example_smoke.c - amalgamation 单文件发行的最小独立编译/链接/运行冒烟示例.
 *
 * 它演示 "宿主只需 mvscan.c + mvscan.h 两个文件 + 一条命令" 的零依赖编译模型, 并在编译器
 * 矩阵 (clang / gcc / mingw / msvc) 上验证单文件可编、可链、可跑. 深层运行期正确性 (与 Go
 * 参考执行器逐位一致) 由 minirehs_mvs_amalg 构建标签下的差分/oracle 测试矩阵覆盖, 此处只做
 * API 表面 + 拒绝非法 blob 的冒烟.
 *
 * 编译运行 (任选其一):
 *   clang -O2 -std=c99 -Wall mvscan.c example_smoke.c -o smoke && ./smoke
 *   gcc   -O2 -std=c99 -Wall mvscan.c example_smoke.c -o smoke && ./smoke
 *   x86_64-w64-mingw32-gcc -O2 -std=c99 mvscan.c example_smoke.c -o smoke.exe   (Windows 交叉编译)
 *
 * 退出码 0 表示冒烟通过.
 */
#include "mvscan.h"

#include <stdio.h>
#include <string.h>

int main(void) {
    int rc = 0;

    /* 1) NULL / 过短 blob 必须被拒 (返回 NULL), 不得崩溃. */
    if (mvscan_db_open(NULL, 0) != NULL) {
        fprintf(stderr, "FAIL: open(NULL,0) should return NULL\n");
        rc = 1;
    }
    {
        unsigned char tiny[4] = { 'M', 'V', 'S', '1' };
        if (mvscan_db_open(tiny, sizeof(tiny)) != NULL) {
            fprintf(stderr, "FAIL: open(short blob) should return NULL\n");
            rc = 1;
        }
    }
    {
        /* 错误 magic 也必须被拒. */
        unsigned char bad[24];
        memset(bad, 0, sizeof(bad));
        bad[0] = 'X';
        if (mvscan_db_open(bad, sizeof(bad)) != NULL) {
            fprintf(stderr, "FAIL: open(bad magic) should return NULL\n");
            rc = 1;
        }
    }

    /* 2) SIMD 档自报 (1=编入 SSE2/NEON, 0=纯标量), 仅打印不做断言 (随平台变化). */
    printf("mvscan_simd_enabled=%d\n", mvscan_simd_enabled());

    if (rc == 0) {
        printf("amalgamation smoke OK\n");
    }
    return rc;
}
