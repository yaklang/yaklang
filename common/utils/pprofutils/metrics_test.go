package pprofutils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBasicMetrics(t *testing.T) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// 检查内存使用情况
	if m.Alloc == 0 {
		t.Fatal("内存分配不应为0")
	}

	if m.TotalAlloc == 0 {
		t.Fatal("总内存分配不应为0")
	}

	if m.HeapAlloc == 0 {
		t.Fatal("堆内存分配不应为0")
	}

	if m.HeapSys == 0 {
		t.Fatal("系统分配的堆内存不应为0")
	}
}

//
//func TestBasicMetrics_UA(t *testing.T) {
//	// 记录初始内存指标
//	start, _ := metricsForMem()
//
//	// 生成一些占用内存的数据
//	data := make([]string, 0)
//	for i := 0; i < 1000000; i++ {
//		data = append(data, "测试数据")
//	}
//
//	// 等待1秒
//	time.Sleep(time.Second)
//
//	// 记录结束时内存指标
//	end, _ := metricsForMem()
//
//	// 验证内存使用增加
//	if end.Alloc <= start.Alloc {
//		t.Error("期望内存分配增加，但没有增加")
//	}
//	// 估算内存增长范围
//	// 每个字符串"测试数据"占用3个中文字符,每个中文字符3字节,加上字符串头部信息
//	expectedMemPerString := 3*3 + 16                           // 大约25字节
//	expectedTotalMem := uint64(expectedMemPerString * 1000000) // 1百万个字符串
//
//	// 允许20%的误差范围作为下限
//	minExpectedMem := expectedTotalMem - expectedTotalMem/5
//
//	if end.HeapAlloc-start.HeapAlloc < minExpectedMem {
//		t.Errorf("堆内存分配增加低于预期,实际增加:%d,最小预期:%d", end.HeapAlloc-start.HeapAlloc, minExpectedMem)
//	}
//
//	// 确保数据没有被优化掉
//	if len(data) != 1000000 {
//		t.Error("数据长度不正确")
//	}
//	for _, element := range data {
//		fmt.Fprint(io.Discard, element)
//	}
//
//}
//
//func TestBasicMetrics_2(t *testing.T) {
//	// 记录初始内存指标
//	start, _ := metricsForMem()
//
//	// 生成一些占用内存的数据
//	data := make([]string, 0)
//	for i := 0; i < 1000000; i++ {
//		data = append(data, "测试数据")
//	}
//
//	// 等待1秒
//	time.Sleep(time.Second)
//
//	// 记录结束时内存指标
//	end, _ := metricsForMem()
//
//	// 验证内存使用增加
//	if end.Alloc <= start.Alloc {
//		t.Error("期望内存分配增加，但没有增加")
//	}
//
//	// 估算内存增长范围
//	// 每个字符串"测试数据"占用3个中文字符,每个中文字符3字节,加上字符串头部信息
//	expectedMemPerString := 3*3 + 16                           // 大约25字节
//	expectedTotalMem := uint64(expectedMemPerString * 1000000) // 1百万个字符串
//
//	// 允许20%的误差范围作为下限
//	minExpectedMem := expectedTotalMem - expectedTotalMem/5
//
//	if end.HeapAlloc-start.HeapAlloc < minExpectedMem {
//		t.Errorf("堆内存分配增加低于预期,实际增加:%d,最小预期:%d", end.HeapAlloc-start.HeapAlloc, minExpectedMem)
//	}
//
//	// 确保数据没有被优化掉
//	if len(data) != 1000000 {
//		t.Error("数据长度不正确")
//	}
//	for _, element := range data {
//		fmt.Fprint(io.Discard, element)
//	}
//
//}
//
//func TestBasicMetrics_3(t *testing.T) {
//	// 记录初始内存指标
//	start, _ := metricsForMem()
//
//	// 用于同步goroutine完成
//	done := make(chan struct{})
//
//	// 使用带缓冲的channel避免goroutine泄漏
//	allowExit := make(chan struct{}, 1)
//	defer func() {
//		allowExit <- struct{}{}
//	}()
//
//	go func() {
//		// 生成一些占用内存的数据
//		data := make([]string, 0, 1000000) // 预分配容量以减少内存分配
//		for i := 0; i < 1000000; i++ {
//			data = append(data, "测试数据")
//		}
//
//		// 确保数据不会被优化掉
//		for _, element := range data {
//			fmt.Fprint(io.Discard, element)
//		}
//
//		// 等待1秒让内存分配充分体现
//		time.Sleep(time.Second)
//
//		// 通知主goroutine完成
//		close(done)
//
//		// 添加超时控制
//		select {
//		case <-allowExit:
//			return
//		case <-time.After(5 * time.Second):
//			// 超时后退出,避免goroutine永久阻塞
//			return
//		}
//	}()
//
//	// 等待goroutine完成,添加超时控制
//	select {
//	case <-done:
//		// 正常完成
//	case <-time.After(10 * time.Second):
//		t.Fatal("测试执行超时")
//	}
//
//	// 记录结束时内存指标
//	end, _ := metricsForMem()
//
//	// 验证内存使用增加
//	if end.Alloc <= start.Alloc {
//		t.Error("期望内存分配增加，但没有增加")
//	}
//
//	// 估算内存增长范围
//	// 每个字符串"测试数据"占用3个中文字符,每个中文字符3字节,加上字符串头部信息
//	expectedMemPerString := 3*3 + 16                           // 大约25字节
//	expectedTotalMem := uint64(expectedMemPerString * 1000000) // 1百万个字符串
//
//	// 允许20%的误差范围作为下限
//	minExpectedMem := expectedTotalMem - expectedTotalMem/5
//
//	if end.HeapAlloc-start.HeapAlloc < minExpectedMem {
//		t.Errorf("堆内存分配增加低于预期,实际增加:%d,最小预期:%d", end.HeapAlloc-start.HeapAlloc, minExpectedMem)
//	}
//}

// 测试CPU监控器的基本功能
func TestNewCPUMonitor(t *testing.T) {
	yakit.InitialDatabase()
	checked := false

	db := consts.GetGormProfileDatabase()
	yakit.SetKey(db, consts.PPROFILEAUTOANALYZE_KEY, "true")
	defer yakit.DelKey(db, consts.PPROFILEAUTOANALYZE_KEY)
	AddCPUProfileCallback(func(stats []FunctionStat) {
		for _, i := range stats {
			fmt.Println(i.Dump())
			if strings.Contains(i.Dump(), `pprofutils/metrics_test.go`) {
				checked = true
			}
		}
	})

	go func() {
		// 模拟CPU密集型任务持续2s
		start := time.Now()
		for time.Since(start) < 2*time.Second {
			for i := 0; i < 1000; i++ {
				math.Pow(float64(i), 2)
				math.Sqrt(float64(i))
				math.Sin(float64(i))
				math.Cos(float64(i))
			}
		}
	}()

	time.Sleep(time.Second * 3)
	if !checked {
		t.Fatal("CPU监控器未触发")
	}
}
