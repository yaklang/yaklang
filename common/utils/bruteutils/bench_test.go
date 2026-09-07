package bruteutils_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/utils/bruteutils"
)

// 本文件基准对比旧调度路径（BruteUtil.Feed 全量物化 + Run）
// 与新流式调度器（StreamBruteContext）。
//
// 运行：go test -bench=. -benchmem -count=10 ./common/utils/bruteutils/ | benchstat -
// （benchstat 需要至少 10 次采样）

var benchSink int

// BenchmarkSchedulerStreamNew 新流式调度器。
func BenchmarkSchedulerStreamNew(b *testing.B) {
	targets := genStrings(2, "bench-t%d.example:22")
	users := genStrings(50, "user%d")
	passes := genStrings(100, "pass%d")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		util, err := bruteutils.NewMultiTargetBruteUtilEx(
			bruteutils.WithBruteCallback(func(item *bruteutils.BruteItem) *bruteutils.BruteItemResult {
				benchSink++
				return item.Result()
			}),
			bruteutils.WithTargetsConcurrent(64),
			bruteutils.WithTargetTasksConcurrent(4),
		)
		if err != nil {
			b.Fatal(err)
		}
		if err := util.StreamBruteContext(context.Background(), "ssh", targets, users, passes, func(res *bruteutils.BruteItemResult) {
			benchSink++
		}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSchedulerFeedLegacy 旧调度路径（Feed 全量物化 + Run）。
func BenchmarkSchedulerFeedLegacy(b *testing.B) {
	targets := genStrings(2, "bench-t%d.example:22")
	users := genStrings(50, "user%d")
	passes := genStrings(100, "pass%d")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		util, err := bruteutils.NewMultiTargetBruteUtilEx(
			bruteutils.WithBruteCallback(func(item *bruteutils.BruteItem) *bruteutils.BruteItemResult {
				benchSink++
				return item.Result()
			}),
			bruteutils.WithTargetsConcurrent(64),
			bruteutils.WithTargetTasksConcurrent(4),
		)
		if err != nil {
			b.Fatal(err)
		}
		// 旧路径：物化全部组合（等价于旧 BruteItemStreamWithContext + Feed）
		count := 0
		for _, t := range targets {
			for _, p := range passes {
				for _, u := range users {
					util.Feed(&bruteutils.BruteItem{Type: "ssh", Target: t, Username: u, Password: p})
					count++
				}
			}
		}
		if err := util.RunWithContext(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func genStrings(n int, pattern string) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf(pattern, i)
	}
	return out
}
