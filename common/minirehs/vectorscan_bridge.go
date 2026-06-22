//go:build minirehs_vectorscan

// vectorscan_bridge.go 提供一个"运行时按需加载 (dlopen)"的 Vectorscan/Hyperscan 加速后端。
//
// 设计要点 (为满足"易分发 + 不崩溃 + 及时退化"):
//   - 不在链接期依赖 libhs: 用 dlopen/dlsym 在运行时加载, 二进制本身不硬链接 libhs。
//     因此带本 tag 构建出的程序, 即使目标机器没有 libhs 也能正常启动, 只是退化为纯 Go 引擎。
//   - 不依赖 hs.h: 所需的函数签名、常量、结构体布局全部在此自声明 (已对照官方头文件核对),
//     故构建机器无需安装 Vectorscan。
//   - 批量命中缓冲: C 侧 match 回调把命中写入调用方提供的数组, 一次扫描结束后 Go 再读取,
//     避免"每命中一次跨语言回调"的高开销。
//   - 语义为"存在性匹配": 把所有正则编译进单一 SIMD 自动机, 用 HS_FLAG_SINGLEMATCH 让每条
//     正则至多上报一次, 判定"该规则是否命中"。命中以 From/To=-1 上报 (与 regexp2-only 一致),
//     契合 MITM 打标等以命中存在性为准的真实场景 (yaklang MITM replacer 第一步即 MatchString)。
//
// 运行时可用前提: 系统装有 Vectorscan/Hyperscan 的 libhs (如 macOS `brew install vectorscan`,
// Linux 包管理器, 或自编译)。可用环境变量 MINIREHS_HS_LIB 指定库路径。
//
// 关键词: vectorscan, hyperscan, dlopen, 运行时加载, 存在性匹配, MITM 打标, 优雅退化
package minirehs

/*
#cgo linux LDFLAGS: -ldl

#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>
#include <dlfcn.h>

// ---- 自声明的 Hyperscan 类型/常量 (对照 hs_common.h / hs_compile.h / hs_runtime.h) ----
typedef struct hs_database hs_database_t;
typedef struct hs_scratch hs_scratch_t;
typedef struct { char *message; int expression; } mrehs_compile_error_t;
typedef int (*mrehs_match_cb)(unsigned int id, unsigned long long from,
                              unsigned long long to, unsigned int flags, void *ctx);

#define MREHS_HS_SUCCESS        0
#define MREHS_HS_MODE_BLOCK     1
#define MREHS_HS_FLAG_SINGLEMATCH 8
#define MREHS_HS_FLAG_ALLOWEMPTY  16

// ---- dlsym 出来的函数指针类型 ----
typedef int (*fn_valid_platform)(void);
typedef const char* (*fn_version)(void);
typedef int (*fn_compile_multi)(const char *const *expressions, const unsigned int *flags,
                                const unsigned int *ids, unsigned int elements,
                                unsigned int mode, const void *platform,
                                hs_database_t **db, mrehs_compile_error_t **error);
typedef int (*fn_compile)(const char *expression, unsigned int flags, unsigned int mode,
                          const void *platform, hs_database_t **db, mrehs_compile_error_t **error);
typedef int (*fn_free_compile_error)(mrehs_compile_error_t *error);
typedef int (*fn_alloc_scratch)(const hs_database_t *db, hs_scratch_t **scratch);
typedef int (*fn_free_scratch)(hs_scratch_t *scratch);
typedef int (*fn_free_database)(hs_database_t *db);
typedef int (*fn_scan)(const hs_database_t *db, const char *data, unsigned int length,
                       unsigned int flags, hs_scratch_t *scratch, mrehs_match_cb onEvent, void *ctx);

static struct {
    void *handle;
    int   loaded; // -1 未尝试, 0 失败, 1 成功
    fn_valid_platform    valid_platform;
    fn_version           version;
    fn_compile_multi     compile_multi;
    fn_compile           compile;
    fn_free_compile_error free_compile_error;
    fn_alloc_scratch     alloc_scratch;
    fn_free_scratch      free_scratch;
    fn_free_database     free_database;
    fn_scan              scan;
} HS = { .loaded = -1 };

static int mrehs_hs_load(void) {
    // 测试/排障开关: 强制视为不可用 (用于验证优雅退化路径)。不写缓存, 便于动态切换。
    const char *dis = getenv("MINIREHS_HS_DISABLE");
    if (dis && dis[0] == '1') return 0;
    if (HS.loaded >= 0) return HS.loaded;
    void *h = NULL;
    const char *envp = getenv("MINIREHS_HS_LIB");
    if (envp && envp[0]) h = dlopen(envp, RTLD_NOW | RTLD_LOCAL);
    static const char *names[] = {
        "libhs.so.5", "libhs.so", "libhs.5.dylib", "libhs.dylib",
        "/opt/homebrew/opt/vectorscan/lib/libhs.dylib",
        "/usr/local/opt/vectorscan/lib/libhs.dylib",
        "/opt/homebrew/lib/libhs.dylib", "/usr/local/lib/libhs.dylib",
        "/usr/lib/x86_64-linux-gnu/libhs.so.5", "/usr/lib/libhs.so.5",
        "hs.dll", "libhs.dll", NULL,
    };
    for (int i = 0; !h && names[i]; i++) h = dlopen(names[i], RTLD_NOW | RTLD_LOCAL);
    if (!h) { HS.loaded = 0; return 0; }

    HS.handle = h;
    HS.valid_platform     = (fn_valid_platform)dlsym(h, "hs_valid_platform");
    HS.version            = (fn_version)dlsym(h, "hs_version");
    HS.compile_multi      = (fn_compile_multi)dlsym(h, "hs_compile_multi");
    HS.compile            = (fn_compile)dlsym(h, "hs_compile");
    HS.free_compile_error = (fn_free_compile_error)dlsym(h, "hs_free_compile_error");
    HS.alloc_scratch      = (fn_alloc_scratch)dlsym(h, "hs_alloc_scratch");
    HS.free_scratch       = (fn_free_scratch)dlsym(h, "hs_free_scratch");
    HS.free_database      = (fn_free_database)dlsym(h, "hs_free_database");
    HS.scan               = (fn_scan)dlsym(h, "hs_scan");

    if (!HS.valid_platform || !HS.compile_multi || !HS.compile || !HS.alloc_scratch ||
        !HS.free_scratch || !HS.free_database || !HS.scan) {
        dlclose(h); HS.handle = NULL; HS.loaded = 0; return 0;
    }
    if (HS.valid_platform() != MREHS_HS_SUCCESS) { // CPU 不满足 (如缺 SSSE3)
        dlclose(h); HS.handle = NULL; HS.loaded = 0; return 0;
    }
    HS.loaded = 1;
    return 1;
}

static int mrehs_hs_available(void) { return mrehs_hs_load(); }

static const char *mrehs_hs_version(void) {
    if (!mrehs_hs_load() || !HS.version) return "unknown";
    return HS.version();
}

// 单条表达式探测: 该正则能否被 Vectorscan 编译 (用于编译期分区)。
static int mrehs_hs_accepts(const char *expr) {
    if (!mrehs_hs_load()) return 0;
    hs_database_t *db = NULL;
    mrehs_compile_error_t *err = NULL;
    unsigned int flags = MREHS_HS_FLAG_SINGLEMATCH | MREHS_HS_FLAG_ALLOWEMPTY;
    int rc = HS.compile(expr, flags, MREHS_HS_MODE_BLOCK, NULL, &db, &err);
    if (rc == MREHS_HS_SUCCESS) {
        if (db) HS.free_database(db);
        return 1;
    }
    if (err) HS.free_compile_error(err);
    return 0;
}

// 多模式编译: 全部用 SINGLEMATCH|ALLOWEMPTY (大小写/dotall/multiline 由表达式内联 flag 表达)。
static hs_database_t *mrehs_hs_build(const char *const *exprs, const unsigned int *ids,
                                     unsigned int n) {
    if (!mrehs_hs_load() || n == 0) return NULL;
    unsigned int *flags = (unsigned int *)malloc(sizeof(unsigned int) * n);
    if (!flags) return NULL;
    for (unsigned int i = 0; i < n; i++)
        flags[i] = MREHS_HS_FLAG_SINGLEMATCH | MREHS_HS_FLAG_ALLOWEMPTY;
    hs_database_t *db = NULL;
    mrehs_compile_error_t *err = NULL;
    int rc = HS.compile_multi(exprs, flags, ids, n, MREHS_HS_MODE_BLOCK, NULL, &db, &err);
    free(flags);
    if (rc != MREHS_HS_SUCCESS) {
        if (err) HS.free_compile_error(err);
        return NULL;
    }
    return db;
}

static hs_scratch_t *mrehs_hs_alloc_scratch(hs_database_t *db) {
    hs_scratch_t *s = NULL;
    if (!HS.alloc_scratch || HS.alloc_scratch(db, &s) != MREHS_HS_SUCCESS) return NULL;
    return s;
}

// match 回调: 把 (id, to) 写入上下文数组 (SINGLEMATCH 下每条正则至多一次, 不会溢出)。
typedef struct { int32_t *ids; int64_t *tos; int count; int cap; } mrehs_ctx;

static int mrehs_cb(unsigned int id, unsigned long long from, unsigned long long to,
                    unsigned int flags, void *ctxp) {
    (void)from; (void)flags;
    mrehs_ctx *c = (mrehs_ctx *)ctxp;
    if (c->count < c->cap) {
        c->ids[c->count] = (int32_t)id;
        c->tos[c->count] = (int64_t)to;
    }
    c->count++;
    return 0; // 继续扫描 (不提前终止)
}

// 扫描一条数据, 命中写入 ids/tos, 返回命中条数。data 可为长度 0 (传入非空指针即可)。
static int mrehs_hs_scan(hs_database_t *db, hs_scratch_t *scratch,
                         const char *data, unsigned int len,
                         int32_t *ids, int64_t *tos, int cap) {
    mrehs_ctx c; c.ids = ids; c.tos = tos; c.count = 0; c.cap = cap;
    HS.scan(db, data, len, 0, scratch, mrehs_cb, &c);
    return c.count;
}

static void mrehs_hs_free_db(hs_database_t *db) { if (db && HS.free_database) HS.free_database(db); }
static void mrehs_hs_free_scratch(hs_scratch_t *s) { if (s && HS.free_scratch) HS.free_scratch(s); }
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/yaklang/yaklang/common/utils"
)

// vectorscanAvailable 报告运行时是否能加载到 libhs 且 CPU 平台受支持。
func vectorscanAvailable() bool { return C.mrehs_hs_available() == 1 }

// hsVersion 返回已加载的 Vectorscan 版本串 (不可用时为 "unknown")。
func hsVersion() string { return C.GoString(C.mrehs_hs_version()) }

// newVectorscanBackend 在 Vectorscan 可用时返回后端, 否则返回 nil (调用方退化为引擎)。
func newVectorscanBackend() backendImpl {
	if !vectorscanAvailable() {
		return nil
	}
	return &vectorscanBackend{}
}

type vectorscanBackend struct{}

func (b *vectorscanBackend) kind() BackendKind { return BackendVectorscan }
func (b *vectorscanBackend) tier() int         { return 1 } // 单一 SIMD 自动机, 最快
func (b *vectorscanBackend) simd() bool        { return true }

func (b *vectorscanBackend) compile(patterns []*compiledPattern, cfg *config) (compiledDB, error) {
	db := &vectorscanDB{}

	// 编译期分区: 逐条探测 Vectorscan 能否编译。能 -> 进单一多模式自动机; 不能 (backref/
	// lookaround 等) -> 进 fallback, 用其原有 verifier 逐条做存在性判定。
	var (
		exprs []string
		ids   []uint32
	)
	for _, cp := range patterns {
		cexpr := C.CString(cp.expr)
		ok := C.mrehs_hs_accepts(cexpr) == 1
		C.free(unsafe.Pointer(cexpr))
		if ok {
			db.hsidToCP = append(db.hsidToCP, cp)
			exprs = append(exprs, cp.expr)
			ids = append(ids, uint32(len(db.hsidToCP)-1)) // hs id = 在 hsidToCP 中的下标
		} else {
			db.fallback = append(db.fallback, cp)
		}
	}

	if len(exprs) > 0 {
		handle := hsBuildMulti(exprs, ids)
		if handle == nil {
			// 多模式编译失败 (极少: 资源上限等): 退化为把这些也并入 fallback, 保正确性。
			cfg.logger.Warnf("minirehs: vectorscan hs_compile_multi failed for %d patterns; routing them to per-pattern fallback", len(exprs))
			db.fallback = append(db.fallback, db.hsidToCP...)
			db.hsidToCP = nil
		} else {
			db.handle = handle
			db.numHS = len(db.hsidToCP)
		}
	}

	cfg.logger.Infof("minirehs: vectorscan backend ready (libhs %s): hs-patterns=%d fallback=%d",
		hsVersion(), db.numHS, len(db.fallback))
	return db, nil
}

// hsBuildMulti 把 exprs/ids 编译为多模式库, 返回不透明 handle (失败为 nil)。
func hsBuildMulti(exprs []string, ids []uint32) unsafe.Pointer {
	n := len(exprs)
	cexprs := make([]*C.char, n)
	cids := make([]C.uint, n)
	for i := range exprs {
		cexprs[i] = C.CString(exprs[i])
		cids[i] = C.uint(ids[i])
	}
	defer func() {
		for _, c := range cexprs {
			C.free(unsafe.Pointer(c))
		}
	}()
	db := C.mrehs_hs_build(
		(**C.char)(unsafe.Pointer(&cexprs[0])),
		(*C.uint)(unsafe.Pointer(&cids[0])),
		C.uint(n),
	)
	return unsafe.Pointer(db)
}

// vectorscanDB 是 Vectorscan 后端的可扫描实例。
type vectorscanDB struct {
	handle   unsafe.Pointer     // *C.hs_database_t (nil 表示无 hs 子集)
	numHS    int                // 进入 hs 自动机的正则数 (= 命中缓冲上限)
	hsidToCP []*compiledPattern // hs id -> compiledPattern
	fallback []*compiledPattern // hs 无法编译者, 逐条存在性判定

	mu      sync.Mutex       // 保护 scratch 空闲表
	freeScr []unsafe.Pointer // 空闲 hs_scratch 列表
	allScr  []unsafe.Pointer // 全部分配过的 hs_scratch (close 时释放)
}

func (d *vectorscanDB) numAlwaysOn() int { return len(d.fallback) }

func (d *vectorscanDB) close() error {
	d.mu.Lock()
	for _, s := range d.allScr {
		C.mrehs_hs_free_scratch((*C.hs_scratch_t)(s))
	}
	d.allScr = nil
	d.freeScr = nil
	d.mu.Unlock()
	if d.handle != nil {
		C.mrehs_hs_free_db((*C.hs_database_t)(d.handle))
		d.handle = nil
	}
	return nil
}

// acquireScratch 取一个本 db 的 hs_scratch (空闲表复用, 不足则新分配)。
func (d *vectorscanDB) acquireScratch() unsafe.Pointer {
	d.mu.Lock()
	if n := len(d.freeScr); n > 0 {
		s := d.freeScr[n-1]
		d.freeScr = d.freeScr[:n-1]
		d.mu.Unlock()
		return s
	}
	d.mu.Unlock()
	s := unsafe.Pointer(C.mrehs_hs_alloc_scratch((*C.hs_database_t)(d.handle)))
	if s != nil {
		d.mu.Lock()
		d.allScr = append(d.allScr, s)
		d.mu.Unlock()
	}
	return s
}

func (d *vectorscanDB) releaseScratch(s unsafe.Pointer) {
	if s == nil {
		return
	}
	d.mu.Lock()
	d.freeScr = append(d.freeScr, s)
	d.mu.Unlock()
}

var emptyByte = []byte{0}

func (d *vectorscanDB) scan(data []byte, sc *scratch, handler MatchHandler) (bool, error) {
	// 1) hs 子集: 单一 SIMD 自动机一次扫描, 批量取回命中。
	if d.handle != nil && d.numHS > 0 {
		if cap(sc.nativeIDs) < d.numHS {
			sc.nativeIDs = make([]int32, d.numHS)
			sc.nativeTo = make([]int64, d.numHS)
		}
		ids := sc.nativeIDs[:d.numHS]
		tos := sc.nativeTo[:d.numHS]

		dptr := (*C.char)(unsafe.Pointer(&emptyByte[0]))
		if len(data) > 0 {
			dptr = (*C.char)(unsafe.Pointer(&data[0]))
		}
		scr := d.acquireScratch()
		if scr == nil {
			return false, utils.Error("minirehs: vectorscan alloc scratch failed")
		}
		got := int(C.mrehs_hs_scan(
			(*C.hs_database_t)(d.handle),
			(*C.hs_scratch_t)(scr),
			dptr, C.uint(len(data)),
			(*C.int32_t)(unsafe.Pointer(&ids[0])),
			(*C.int64_t)(unsafe.Pointer(&tos[0])),
			C.int(d.numHS),
		))
		d.releaseScratch(scr)
		if got > d.numHS {
			got = d.numHS // SINGLEMATCH 下不应发生, 防御性夹紧
		}
		for i := 0; i < got; i++ {
			hsid := int(ids[i])
			if hsid < 0 || hsid >= len(d.hsidToCP) {
				continue
			}
			cp := d.hsidToCP[hsid]
			// 存在性命中 (无 SOM): From/To=-1, 与 regexp2-only 语义一致。
			if !handler(Match{ID: cp.id, From: -1, To: -1}) {
				return true, nil
			}
		}
	}

	// 2) fallback 子集 (backref/lookaround 等): 逐条做存在性判定。
	for _, cp := range d.fallback {
		if len(cp.v.findAll(data)) > 0 {
			if !handler(Match{ID: cp.id, From: -1, To: -1}) {
				return true, nil
			}
		}
	}
	return false, nil
}
