package minirehs

import (
	"os"
	"strconv"
	"testing"
)

// diffIters 返回随机差分测试的默认迭代数.
//
// 默认 3000: 仍远超各测试的 tested 下限护栏 (1000), 统计覆盖充分, 但相比历史 20000 砍 ~85%,
// 让 minirehs 全量回归从 ~480s 降到可接受区间, 研发流程不被阻塞.
//
// 全量覆盖可通过环境变量 MINIREHS_DIFF_ITERS 恢复 (CI / 发布前回归):
//
//	MINIREHS_DIFF_ITERS=20000 go test ./common/minirehs/
//
// testing.Short() 时进一步降到 500 (仅冒烟, 不依赖环境变量).
func diffIters(tb testing.TB, fallback int) int {
	if testing.Short() {
		return 500
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
// 真实语料共 1332 条 / 5.2MB; 400 条已覆盖 HTTP 请求/响应/JSON/表单/二进制等多样形态,
// 足以在差分/一致性测试中暴露问题, 同时把 oracle 逐条扫描的耗时砍 ~70%.
// 全量语料用 MINIREHS_FULL_CORPUS=1 恢复.
const corpusDefaultLimit = 400

// fullCorpusRequested 报告是否要求加载全量真实语料 (CI / 发布回归).
func fullCorpusRequested() bool {
	return os.Getenv("MINIREHS_FULL_CORPUS") == "1"
}
