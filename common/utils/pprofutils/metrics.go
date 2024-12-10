package pprofutils

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type MemMetrics struct {
	// 当前分配的内存大小(字节)
	// 这个值表示当前正在使用的内存量,会随着内存分配和垃圾回收而变化
	Alloc uint64

	// 程序启动后累计分配的总内存(字节)
	// 这个值只会增加,不会因为垃圾回收而减少
	TotalAlloc uint64

	// 当前堆内存分配量(字节)
	// 与Alloc类似,但只统计堆内存,不包括栈内存
	HeapAlloc uint64

	// 从系统申请的堆内存总量(字节)
	// 这个值表示系统实际为堆分配的内存,可能会大于HeapAlloc
	// 因为包含了预留和未使用的内存
	HeapSys uint64

	// 系统总物理内存
	SystemMemory uint64

	// 系统可用物理内存
	AvailableMemory uint64
}

type MemMetricsPercent struct {
	// 程序占用系统总物理内存的百分比
	SystemMemoryUsage float64

	// 程序占用系统可用物理内存的百分比
	AvailableMemoryUsage float64
}

// Percent 计算程序内存占用系统物理内存的百分比
func (m MemMetrics) Percent() MemMetricsPercent {
	var result MemMetricsPercent

	// 计算占系统总物理内存百分比
	if m.SystemMemory > 0 {
		result.SystemMemoryUsage = float64(m.HeapSys) / float64(m.SystemMemory) * 100
	}

	// 计算占系统可用内存百分比
	if m.AvailableMemory > 0 {
		result.AvailableMemoryUsage = float64(m.HeapSys) / float64(m.AvailableMemory) * 100
	}

	return result
}
func metricsForMem() (MemMetrics, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取系统内存信息
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return MemMetrics{}, fmt.Errorf("failed to get system memory info: %v", err)
	}

	return MemMetrics{
		Alloc:           m.Alloc,
		TotalAlloc:      m.TotalAlloc,
		HeapAlloc:       m.HeapAlloc,
		HeapSys:         m.HeapSys,
		SystemMemory:    memInfo.Total,
		AvailableMemory: memInfo.Available,
	}, nil
}

var cpuPprofileOnce sync.Once
var cpuThreshold = 0.8

// 回调函数列表和锁
var (
	cpuCallbackMutex sync.RWMutex
	cpuCallbacks     []func(stats []FunctionStat)
)

// 添加CPU分析回调函数
func AddCPUProfileCallback(callback func(stats []FunctionStat)) {
	cpuCallbackMutex.Lock()
	defer cpuCallbackMutex.Unlock()
	cpuCallbacks = append(cpuCallbacks, callback)
}

// 清空CPU分析回调函数
func ClearCPUProfileCallbacks() {
	cpuCallbackMutex.Lock()
	defer cpuCallbackMutex.Unlock()
	cpuCallbacks = nil
}

// 执行所有回调函数
func executeCPUCallbacks(stats []FunctionStat) {
	cpuCallbackMutex.RLock()
	defer cpuCallbackMutex.RUnlock()

	for _, callback := range cpuCallbacks {
		callback(stats)
	}
}

func init() {
	go func() {
		cpuPprofileOnce.Do(func() {
			for {
				time.Sleep(time.Second)

				cpuCallbackMutex.RLock()
				if len(cpuCallbacks) == 0 {
					cpuCallbackMutex.RUnlock()
					continue
				}
				cpuCallbackMutex.RUnlock()

				var buf bytes.Buffer
				start := time.Now()
				err := pprof.StartCPUProfile(&buf)
				if err != nil {
					randInt := rand.Intn(10) + 1
					fmt.Printf("CPU Profile error: %v, retry after %d seconds\n", err, randInt)
					time.Sleep(time.Duration(randInt) * time.Second)
					continue
				}
				time.Sleep(time.Second)
				pprof.StopCPUProfile()
				stats, err := AutoAnalyzeRaw(&buf)
				if err != nil && len(stats) == 0 {
					log.Debugf("finished pprofiling for cpu duration: %v, profile: %v", time.Now().Sub(start), utils.ByteSize(uint64(buf.Len())))
					continue
				}

				// 执行回调函数
				executeCPUCallbacks(stats)
			}
		})
	}()
}
