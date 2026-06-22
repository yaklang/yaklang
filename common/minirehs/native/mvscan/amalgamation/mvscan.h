/*
 * mvscan.h - mvscan 自托管多正则存在性引擎的纯 C99 运行期内核公共 API.
 *
 * 设计定位 (见 ../../MINI_VECTOR_SCAN_IMPL.md 第 1/2/9 节): 运行期内核为纯 C99,
 * 像 sqlite 一样平台无关 / CPU 无关 (兼容 + 退化). 它只认 "平台无关只读 blob + data",
 * 不依赖 libhs、不依赖系统正则、不依赖 Go 运行时. 前端 (Go) 把每条 pattern 编译为
 * rune 级 Glushkov 位并行 NFA, 序列化为小端 blob; 本内核零依赖地解析并执行存在性扫描.
 *
 * 里程碑 (M2->M3): bit-NFA 执行器 + blob 解析, 与 Go 参考执行器 (existsIn / merged
 * scanExist) 逐位一致. M3 叠加 SIMD 加速档 (x86_64 SSE2 / arm64 NEON, 字向量 OR/AND/COPY
 * 一次 2 字), 按 nword 与架构在 nfa_run 内分发, 并配等价标量孪生 (mvscan_db_*_scalar) 供
 * 逐位差分; 本文件公共 API 不变.
 *
 * 关键词: mvscan, pure C kernel, bit-parallel NFA, existence, platform-independent blob
 */
#ifndef MVSCAN_H
#define MVSCAN_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* mvscan_db 是从 blob 解析出的不可变只读结构 (多线程只读共享; 扫描不写它). */
typedef struct mvscan_db mvscan_db;

/*
 * mvscan_db_open 解析一段平台无关小端 blob (由 Go 前端 buildMVSBlob 产出) 为只读 db.
 * 解析会把 blob 内的数组按字段语义拷入对齐内存 (小数据, 拷贝成本可忽略), 因此对调用方
 * 传入 blob 的对齐 / 生命周期无要求, 且在任意字节序架构上都正确. 失败 (magic/version/
 * 截断) 返回 NULL.
 */
mvscan_db *mvscan_db_open(const uint8_t *blob, size_t len);

/* mvscan_db_close 释放 db 持有的全部内存. 传 NULL 安全. */
void mvscan_db_close(mvscan_db *db);

/* mvscan_db_npat 返回 pattern 槽位数 (与 Go 侧 d.n 一致). */
int32_t mvscan_db_npat(const mvscan_db *db);

/* mvscan_db_has_merged 返回是否含合并 always-on 自动机 (1/0). */
int mvscan_db_has_merged(const mvscan_db *db);

/*
 * mvscan_db_nfa_exists 判定 pattern 下标 idx 的 per-pattern NFA 是否在 data 中存在命中.
 * 语义与 Go mvsNFA.existsIn 逐位一致 (leftmost 不区分, 仅存在性). 返回 1 命中 / 0 不命中 /
 * -1 表示该 idx 无 NFA (应由 Go 侧兜底, 正常不应发生).
 */
int mvscan_db_nfa_exists(const mvscan_db *db, int32_t idx,
                         const uint8_t *data, size_t len);

/*
 * mvscan_db_nfa_exists_many 对 idxs[0..nidx) 各跑一次 per-pattern NFA 存在性, 把结果写入
 * out[i] (1 命中 / 0 不命中或无 NFA). 语义等价于对每个 idx 调一次 mvscan_db_nfa_exists, 但
 * 在 C 内循环, 一次 cgo 跨界完成多条验证, 摊薄 "每 pattern 一次 cgo" 的调用开销 (Phase 2 批处理).
 * out 必须由调用方提供且容量 >= nidx; data 在调用期间必须存活.
 */
void mvscan_db_nfa_exists_many(const mvscan_db *db,
                               const uint8_t *data, size_t len,
                               const int32_t *idxs, int32_t nidx,
                               uint8_t *out);

/*
 * mvscan_db_merged_scan 单趟扫描 data, 把合并 always-on 自动机命中的成员 pattern 下标
 * (去重) 追加到 out (容量 cap). seen 是调用方提供的、长度 >= npat 的清零缓冲, 用于去重
 * (会被本函数写入). 返回唯一命中数 total; 若 total > cap 表示 out 截断, 调用方可扩容重扫.
 * 语义与 Go mvsMergedNFA.scanExist 命中集合一致 (顺序为首次命中序).
 */
int32_t mvscan_db_merged_scan(const mvscan_db *db,
                              const uint8_t *data, size_t len,
                              uint8_t *seen, int32_t seenLen,
                              int32_t *out, int32_t cap);

/* 强制标量孪生入口 (语义同上, 但绕过 SIMD 分发恒走标量). 供差分测试对照默认 (SIMD) 分发,
 * 二者命中结果必逐位一致; 生产路径用非 _scalar 版本. */
int mvscan_db_nfa_exists_scalar(const mvscan_db *db, int32_t idx,
                                const uint8_t *data, size_t len);
int32_t mvscan_db_merged_scan_scalar(const mvscan_db *db,
                                     const uint8_t *data, size_t len,
                                     uint8_t *seen, int32_t seenLen,
                                     int32_t *out, int32_t cap);

/* mvscan_simd_enabled 报告本次构建是否编入 SIMD 加速档 (1) 还是纯标量 (0). */
int mvscan_simd_enabled(void);

#ifdef __cplusplus
}
#endif

#endif /* MVSCAN_H */
