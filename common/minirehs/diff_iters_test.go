package minirehs

import (
	"os"
	"strconv"
	"testing"
)

// diffIters 返回随机差分测试的默认迭代数.
//
// 默认 fallback (3000): 仍远超各测试的 tested 下限护栏 (1000), 统计覆盖充分, 但相比历史 20000 砍 ~85%,
// 让 minirehs 全量回归从 ~480s 降到可接受区间, 研发流程不被阻塞.
//
// 全量覆盖可通过环境变量 MINIREHS_DIFF_ITERS 恢复 (CI / 发布前回归):
//
//	MINIREHS_DIFF_ITERS=20000 go test ./common/minirehs/
//
// testing.Short() 时降到 3500: reverse-eligible 通过率最低 (~31%), 需 ~3500 才能稳定过 1000 护栏;
// 差分本身很快 (数千 iter 仅几十毫秒), -short 主要靠 corpus 裁剪 + 诊断跳过来提速, 非靠削弱差分.
func diffIters(tb testing.TB, fallback int) int {
	if testing.Short() {
		return 3500
	}
	if v := os.Getenv("MINIREHS_DIFF_ITERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// defaultDiffIters: 随机差分测试的默认强度 (研发默认).
// 历史值为 20000; 3000 经验上 tested(NFA) 稳定落在 1200~1500 区间, 远超 1000 下限.
const defaultDiffIters = 3000

// corpusDefaultLimit: loadCorpus 在快速回归模式下的记录截断上限.
// 真实语料共 1332 条 / 5.2MB; 差分/一致性测试的目的是发现 NFA 与 oracle 的分歧,
// 80 条已覆盖 HTTP 请求/响应/JSON/表单/二进制等多形态, 足以暴露问题,
// 同时把 stdlib oracle 逐条扫描 (最慢后端) 的耗时砍到可接受范围.
// 全量语料用 MINIREHS_FULL_CORPUS=1 恢复 (CI / 发布回归).
const corpusDefaultLimit = 80

// fullCorpusRequested 报告是否要求加载全量真实语料 (CI / 发布回归).
func fullCorpusRequested() bool {
	return os.Getenv("MINIREHS_FULL_CORPUS") == "1"
}

// diagEnabled 报告是否启用诊断/分析类测试 (mvs_*_diag_test.go).
// 这些测试输出 t.Logf 统计用于开发期分析, 非正确性断言, 默认跳过以免拖慢常规回归.
// 显式开启: MINIREHS_DIAG=1 go test ./common/minirehs/
func diagEnabled() bool {
	return os.Getenv("MINIREHS_DIAG") == "1"
}

// requireDiag 在诊断测试开头调用, 默认环境跳过; MINIREHS_DIAG=1 时才执行.
func requireDiag(tb testing.TB) {
	tb.Helper()
	if !diagEnabled() {
		tb.Skip("diagnostic test skipped (set MINIREHS_DIAG=1 to enable)")
	}
}
