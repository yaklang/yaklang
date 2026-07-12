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

/*
 * mvscan_db_merged_scan_batch: 批量扫描多条记录 (拼接为一个 buffer), 每条记录独立扫描.
 * recOff 长度 nrec+1: 第 i 条记录 = data[recOff[i]..recOff[i+1]).
 * 对每条记录跑 merged NFA, 把 (recIdx, memberIdx) 对写入 out (容量 capPairs).
 * 返回命中总对数 (可能 > capPairs, 表示截断).
 * 省去每记录一次 cgo 调用 (N 次 cgo -> 1 次 cgo).
 */
int32_t mvscan_db_merged_scan_batch(const mvscan_db *db,
                                    const uint8_t *data, size_t totalLen,
                                    const int32_t *recOff, int32_t nrec,
                                    int32_t *out, int32_t capPairs);

/* 强制标量孪生入口 (语义同上, 但绕过 SIMD 分发恒走标量). 供差分测试对照默认 (SIMD) 分发,
 * 二者命中结果必逐位一致; 生产路径用非 _scalar 版本. */
int mvscan_db_nfa_exists_scalar(const mvscan_db *db, int32_t idx,
                                const uint8_t *data, size_t len);
int32_t mvscan_db_merged_scan_scalar(const mvscan_db *db,
                                     const uint8_t *data, size_t len,
                                     uint8_t *seen, int32_t seenLen,
                                     int32_t *out, int32_t cap);

/*
 * mvscan_db_nfa_find_all_1 为单字、无零宽断言的 NFA 枚举全部 leftmost-longest、
 * 非重叠匹配区间。out 为平铺的 (from,to) 对；返回总对数，可能大于 capPairs
 * 表示输出被截断。返回 -1 表示 idx 不适用（调用方必须回退其通用定位器）。
 *
 * 这是存在性 C 内核的定位孪生：避免「C 判命中后 Go 再完整扫描一次」；复杂多字
 * 与断言 NFA 仍由 Go 的已验证定位器处理。
 */
int32_t mvscan_db_nfa_find_all_1(const mvscan_db *db, int32_t idx,
                                  const uint8_t *data, size_t len,
                                  int32_t *out, int32_t capPairs);

/* 通用多字版本；语义同 _1，覆盖所有无断言 lean NFA。 */
int32_t mvscan_db_nfa_find_all(const mvscan_db *db, int32_t idx,
                                const uint8_t *data, size_t len,
                                int32_t *out, int32_t capPairs);

/*
 * mvs_span 是锚定式扫描的注入区间 [lo, hi) (字节偏移). 仅当 rune 起始落入某 span 时
 * 才注入 NFA 起点 first, 其余位置不注入, 实现提前消亡 (对应 Go existsInAnchored).
 * spans 须已按 lo 升序排序并合并重叠/相邻区间.
 */
typedef struct {
    int32_t lo;
    int32_t hi;
} mvs_span;

/*
 * mvscan_db_nfa_exists_anchored 判定 pattern idx 的 NFA 是否在 data 中存在命中,
 * 但仅在 spans 描述的注入区间内注入起点 (锚定式语义). 语义与 Go existsInAnchored 逐位一致:
 * 任一匹配必起于某注入区间, 区间外不注入 => 无假阳; 匹配起点必落某区间 => 无假阴.
 * spans 以两个平行数组 lo[0..nspan) 和 hi[0..nspan) 传入 (已排序合并), 避免 Go 侧 C 结构体分配.
 * 返回 1 命中 / 0 不命中 / -1 表示该 idx 无 NFA.
 */
int mvscan_db_nfa_exists_anchored(const mvscan_db *db, int32_t idx,
                                   const uint8_t *data, size_t len,
                                   const int32_t *lo, const int32_t *hi, int32_t nspan);

/* 强制标量孪生入口 (语义同上, 绕过 SIMD 分发恒走标量). 供差分测试. */
int mvscan_db_nfa_exists_anchored_scalar(const mvscan_db *db, int32_t idx,
                                          const uint8_t *data, size_t len,
                                          const int32_t *lo, const int32_t *hi, int32_t nspan);

/*
 * mvscan_db_nfa_exists_anchored_many 一次 cgo 调用对多条 anchorable pattern 各自做
 * 锚定式存在性, 摊薄 "每 pattern 一次 cgo" 的跨界开销 (锚定批处理的 cgo 调用数从
 * O(触发数) 降到 O(1)). 各 pattern 的 spans 平铺在 spansLo[]/spansHi[] 中, 用
 * patSpanOff[i+1]-patSpanOff[i] 取第 i 条 pattern 的 span 数; idxs[i] 为其 pattern 下标.
 * out[i] 写入命中结果 (1 命中 / 0 不命中或无 NFA). data 在调用期间须存活.
 */
void mvscan_db_nfa_exists_anchored_many(const mvscan_db *db,
                                         const uint8_t *data, size_t len,
                                         const int32_t *idxs, int32_t npat,
                                         const int32_t *patSpanOff,
                                         const int32_t *spansLo, const int32_t *spansHi,
                                         int32_t totalSpans,
                                         uint8_t *out);

/* mvscan_simd_enabled 报告本次构建是否编入 SIMD 加速档 (1) 还是纯标量 (0). */
int mvscan_simd_enabled(void);

/*
 * mvscan_compute_boundaries_pub 预计算 data 的零宽断言边界条件集 (复刻 Go computeBoundaries).
 * 产出 bound[0..len] (len+1 字节), 供 mvscan_db_nfa_exists_assert 使用.
 * buf 容量须 >= len+1. 多条断言 NFA 可共享同一份 bound (每报文一次).
 */
void mvscan_compute_boundaries_pub(const uint8_t *data, size_t len, uint8_t *buf);

/*
 * mvscan_db_nfa_exists_assert 判定断言 NFA (hasAssert, nword==1) 在 data 中是否存在命中.
 * bound 为 mvscan_compute_boundaries_pub 产出的共享边界数组 (len+1 字节).
 * 返回 1 命中 / 0 不命中 / -1 无 NFA 或非断言 NFA (应由 Go 侧兜底).
 */
int mvscan_db_nfa_exists_assert(const mvscan_db *db, int32_t idx,
                                const uint8_t *data, size_t len,
                                const uint8_t *bound);

/* mvscan_db_nfa_exists_assert_self: 自包含断言扫描 — 内部预算边界, 省去 Go 侧一次 cgo. */
int mvscan_db_nfa_exists_assert_self(const mvscan_db *db, int32_t idx,
                                     const uint8_t *data, size_t len,
                                     uint8_t *boundBuf);

/* 多字断言在线边界单趟扫描；仅 hasAssert && nword>1，其他形态返回 -1。 */
int mvscan_db_nfa_exists_assert_online(const mvscan_db *db, int32_t idx,
                                       const uint8_t *data, size_t len);

/* mvscan_db_dfa_scan_batch: 在单次调用中对多个 DFA 模式扫描同一段 data.
 * 对每个 idx 逐个跑 dfa_run, 把命中的 idx 写入 out. 返回命中数. */
int32_t mvscan_db_dfa_scan_batch(const mvscan_db *db,
                                 const uint8_t *data, size_t len,
                                 const int32_t *idxs, int32_t nidx,
                                 int32_t *out, int32_t cap);

/*
 * mvscan_db_combined_scan: 单次数据遍历同时跑 merged NFA + assert NFA.
 * mergedOut: 合并 NFA 命中的成员 idx (去重 via seen).
 * assertIdxs: 断言 NFA 的 pattern idx 列表.
 * assertOut: 断言命中的 pattern idx.
 * boundBuf: 边界缓冲 (len+1 字节), 内部预算.
 * 返回 assertOut 中的命中数; mergedOut 通过 mergedSeen/mergedTotal 写入.
 */
int32_t mvscan_db_combined_scan(const mvscan_db *db,
                                const uint8_t *data, size_t len,
                                uint8_t *mergedSeen, int32_t mergedSeenLen,
                                int32_t *mergedOut, int32_t mergedCap,
                                int32_t *mergedTotalOut,
                                const int32_t *assertIdxs, int32_t nAssert,
                                uint8_t *boundBuf,
                                int32_t *assertOut, int32_t assertCap);

/*
 * mvscan_db_nfa_exists_assert_many 一次 cgo 对多条断言 always-on NFA 各自做断言存在性,
 * 内部预算边界 (每报文一次, 跨多条 NFA 共享), 摊薄跨界开销.
 * boundBuf 容量须 >= len+1. out[i] 写入命中结果 (1 命中 / 0 不命中或无 C 断言 NFA).
 */
void mvscan_db_nfa_exists_assert_many(const mvscan_db *db,
                                      const uint8_t *data, size_t len,
                                      const int32_t *idxs, int32_t nidx,
                                      uint8_t *boundBuf,
                                      uint8_t *out);

#ifdef __cplusplus
}
#endif

#endif /* MVSCAN_H */
