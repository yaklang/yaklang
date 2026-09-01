package bruteutils_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
)

// TestSentinelPasswordNeverLeaks 用哨兵密码驱动全部内置协议的
// 结果字符串、错误信息与日志，任何一处出现哨兵明文即失败。
//
// 协议实现连接不可达地址（127.0.0.1:1），重点覆盖：
//   - 结果对象与字符串表示
//   - 错误链文本
//   - 全局日志输出（log.SetOutput 捕获）
func TestSentinelPasswordNeverLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: 跳过 30 协议全量扫描")
	}
	sentinel := "SENTINEL-P@ss!中文🔐$(echo)"

	// 捕获日志
	var logBuf syncBuffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(nil)

	types := bruteutils.GetBuildinAvailableBruteType()
	var mu sync.Mutex
	leaks := []string{}

	check := func(name, text string) {
		if strings.Contains(text, sentinel) {
			mu.Lock()
			leaks = append(leaks, fmt.Sprintf("%s: %s", name, text))
			mu.Unlock()
		}
	}

	for _, typ := range types {
		handler, err := bruteutils.GetBruteFuncByType(typ)
		if err != nil {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					check(typ+"-panic", fmt.Sprintf("%v", r))
				}
			}()
			item := &bruteutils.BruteItem{
				Type:     typ,
				Target:   "127.0.0.1:1", // 立即连接拒绝
				Username: "sentinel-user",
				Password: sentinel,
				Context:  context.Background(),
			}
			res := handler(item)
			if res == nil {
				return
			}
			check(typ+"-result-string", res.String())
			check(typ+"-result-fmt", fmt.Sprintf("%+v", res))
			check(typ+"-extra", string(res.ExtraInfo))
			check(typ+"-item-string", item.String())
		}()
	}

	time.Sleep(200 * time.Millisecond) // 等待异步日志落盘
	check("global-log", logBuf.String())

	if len(leaks) > 0 {
		t.Fatalf("sentinel password leaked in %d places:\n%s", len(leaks), strings.Join(leaks, "\n"))
	}
}

// TestSentinelPasswordNeverLeaksInStream 用哨兵密码跑一遍流式调度器，
// 扫描全部结果与日志。
func TestSentinelPasswordNeverLeaksInStream(t *testing.T) {
	sentinel := "STREAM-SENTINEL-P@ss!"

	var logBuf syncBuffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(nil)

	util, err := bruteutils.NewMultiTargetBruteUtilEx(
		bruteutils.WithBruteCallback(func(item *bruteutils.BruteItem) *bruteutils.BruteItemResult {
			return item.Result()
		}),
		bruteutils.WithTargetsConcurrent(4),
		bruteutils.WithTargetTasksConcurrent(2),
	)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	leaks := 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = util.StreamBruteContext(ctx, "ssh",
		[]string{"127.0.0.1:1"},
		[]string{"user"},
		[]string{sentinel},
		func(res *bruteutils.BruteItemResult) {
			mu.Lock()
			defer mu.Unlock()
			if strings.Contains(res.String(), sentinel) || strings.Contains(fmt.Sprintf("%v", res), sentinel) {
				leaks++
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if strings.Contains(logBuf.String(), sentinel) {
		t.Fatalf("sentinel leaked into logs: %s", truncate(logBuf.String(), 200))
	}
	if leaks > 0 {
		t.Fatalf("sentinel leaked into %d results", leaks)
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
