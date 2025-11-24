// Package pprofutils provides utilities for runtime profiling of Go applications.
// It supports both CPU and memory profiling with customizable thresholds and callbacks.
//
// Example usage:
//
//	pprofutils.AddCPUProfileCallback(func(stats []FunctionStat) {
//	    // Handle CPU profile data
//	})
//
//	pprofutils.AddMemProfileCallback(func(metrics MemMetrics) {
//	    // Handle memory metrics
//	})
package pprofutils

import (
	"bytes"
	"fmt"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
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
		result.SystemMemoryUsage = float64(m.HeapSys) / float64(m.SystemMemory)
	}

	// 计算占系统可用内存百分比
	if m.AvailableMemory > 0 {
		result.AvailableMemoryUsage = float64(m.HeapSys) / float64(m.AvailableMemory)
	}

	return result
}

var (
	memInfoOnce sync.Once
	memInfo     *mem.VirtualMemoryStat
	memInfoErr  error
)

func metricsForMem() (MemMetrics, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 只执行一次获取系统内存信息
	memInfoOnce.Do(func() {
		memInfo, memInfoErr = mem.VirtualMemory()
		if memInfoErr != nil {
			memInfoErr = fmt.Errorf("failed to get system memory info: %v", memInfoErr)
		}
	})

	if memInfoErr != nil {
		return MemMetrics{}, memInfoErr
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
var memThreshold = 0.7

// 回调函数列表和锁
var (
	cpuCallbackMutex sync.RWMutex
	cpuCallbacks     []func(stats []FunctionStat)
	memCallbackMutex sync.RWMutex
	memCallbacks     []func(metrics []FunctionStat)
)

// 添加CPU分析回调函数
func AddCPUProfileCallback(callback func(stats []FunctionStat)) {
	cpuCallbackMutex.Lock()
	defer cpuCallbackMutex.Unlock()
	cpuCallbacks = append(cpuCallbacks, callback)
}

// 添加内存分析回调函数
func AddMemProfileCallback(callback func(metrics []FunctionStat)) {
	memCallbackMutex.Lock()
	defer memCallbackMutex.Unlock()
	memCallbacks = append(memCallbacks, callback)
}

// 清空CPU分析回调函数
func ClearCPUProfileCallbacks() {
	cpuCallbackMutex.Lock()
	defer cpuCallbackMutex.Unlock()
	cpuCallbacks = nil
}

// 清空内存分析回调函数
func ClearMemProfileCallbacks() {
	memCallbackMutex.Lock()
	defer memCallbackMutex.Unlock()
	memCallbacks = nil
}

// 执行所有CPU回调函数
func executeCPUCallbacks(stats []FunctionStat) {
	cpuCallbackMutex.RLock()
	defer cpuCallbackMutex.RUnlock()

	for _, callback := range cpuCallbacks {
		callback(stats)
	}
}

// 执行所有内存回调函数
func executeMemCallbacks(metrics []FunctionStat) {
	memCallbackMutex.RLock()
	defer memCallbackMutex.RUnlock()

	for _, callback := range memCallbacks {
		callback(metrics)
	}
}

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		exportReport := func(prefix string, stats []FunctionStat) {
			msg := DumpFunctionStats(stats)
			path := filepath.Join(consts.GetDefaultYakitPprofDir(), fmt.Sprintf("%s-%s.txt", prefix, utils.DatetimePretty2()))
			err := os.WriteFile(path, []byte(msg), 0644)
			if err != nil {
				log.Errorf("write auto analyze report failed: %s", err)
				return
			}
			log.Infof("auto analyze report exported to: %s", path)
		}
		AddCPUProfileCallback(func(stats []FunctionStat) {
			log.Infof("High CPU usage detected, top consuming function: %v", stats[0].Dump())
			exportReport("cpu", stats)
		})
		AddMemProfileCallback(func(stats []FunctionStat) {
			log.Infof("High memory usage detected, top consuming function: %v", stats[0].Dump())
			exportReport("mem", stats)
		})

		db := consts.GetGormProfileDatabase()
		// CPU分析协程
		go func() {
			// 获取CPU核心数
			numCPU := runtime.NumCPU()
			cpuPprofileOnce.Do(func() {
				var buf = bytes.NewBuffer(nil)
				for {
					time.Sleep(time.Second)
					if yakit.GetKey(db, consts.PPROFILEAUTOANALYZE_KEY) != "true" {
						continue
					}
					cpuCallbackMutex.RLock()
					if len(cpuCallbacks) == 0 {
						cpuCallbackMutex.RUnlock()
						continue
					}
					cpuCallbackMutex.RUnlock()

					start := time.Now()
					buf.Reset()
					err := pprof.StartCPUProfile(buf)
					if err != nil {
						randInt := rand.Intn(10) + 1
						fmt.Printf("CPU Profile error: %v, retry after %d seconds\n", err, randInt)
						time.Sleep(time.Duration(randInt) * time.Second)
						continue
					}
					time.Sleep(time.Second)
					pprof.StopCPUProfile()
					stats, err := AutoAnalyzeRaw(buf)
					if err != nil && len(stats) == 0 {
						log.Debugf("finished pprofiling for cpu duration: %v, profile: %v", time.Now().Sub(start), utils.ByteSize(uint64(buf.Len())))
						continue
					}

					// 获取CPU使用率
					cpuPercent := stats[0].Percent
					// 如果CPU使用率超过单核100%,则需要除以CPU核心数来获得平均使用率
					if cpuPercent > 1.0 {
						cpuPercent = cpuPercent / float64(numCPU)
					}

					if cpuPercent >= cpuThreshold {
						// 执行回调函数
						if len(stats) > 0 {
							log.Infof("High CPU usage detected (%v%%), top consuming function: %v", cpuPercent, stats[0].Dump())
						}
						executeCPUCallbacks(stats)
					}
				}
			})
		}()

		// 内存分析协程
		go func() {
			var buf = bytes.NewBuffer(nil)
			for {
				time.Sleep(time.Second)
				if yakit.GetKey(db, consts.PPROFILEAUTOANALYZE_KEY) != "true" {
					continue
				}
				memCallbackMutex.RLock()
				if len(memCallbacks) == 0 {
					memCallbackMutex.RUnlock()
					continue
				}
				memCallbackMutex.RUnlock()

				buf.Reset()
				pprof.WriteHeapProfile(buf)
				stats, err := AutoAnalyzeRaw(buf)
				if err != nil {
					log.Debugf("memory analyze failed, reason: %v", err)
					continue
				}

				metrics, err := metricsForMem()
				if err != nil {
					log.Debugf("memory metrics generating failed, reason: %v", err)
					continue
				}

				// 只有当内存使用率超过阈值时才触发回调
				percent := metrics.Percent()
				if percent.SystemMemoryUsage >= memThreshold {
					// 执行内存回调函数
					executeMemCallbacks(stats)
				}
			}
		}()
		return nil
	}, "register-profile")
}
