package aibalance

import (
	"math/rand"
)

// resolveDelayRange 归一化（minSec, maxSec）为本次实际可用的随机区间。
// 兼容老语义：
//   - maxSec <= 0 且 minSec > 0 -> [minSec, 2*minSec]（老 N~2N）
//   - minSec <= 0 且 maxSec > 0 -> [0, maxSec]（用户配置 "0~5 随机延迟"）
//   - 两者都 <= 0               -> [0, 0]（不延迟）
//   - 否则保持 [minSec, maxSec]
//
// 关键词: resolveDelayRange N~M, 兼容老 N~2N, 0~M 语义
func resolveDelayRange(minSec, maxSec int64) (int64, int64) {
	if minSec < 0 {
		minSec = 0
	}
	if maxSec < 0 {
		maxSec = 0
	}
	if minSec == 0 && maxSec == 0 {
		return 0, 0
	}
	if maxSec == 0 && minSec > 0 {
		// 老语义 N~2N
		return minSec, 2 * minSec
	}
	if maxSec < minSec {
		maxSec = minSec
	}
	return minSec, maxSec
}

// computeJitterDelaySec 在 [minSec, maxSec] 之间均匀采样一个秒数（包含上下界）。
// 经过 resolveDelayRange 归一化后调用，保证 max >= min >= 0。
//
// 关键词: computeJitterDelaySec, 调用前延迟随机采样
func computeJitterDelaySec(minSec, maxSec int64) int64 {
	minSec, maxSec = resolveDelayRange(minSec, maxSec)
	if maxSec <= 0 {
		return 0
	}
	if maxSec == minSec {
		return minSec
	}
	span := maxSec - minSec + 1
	return minSec + rand.Int63n(span)
}
