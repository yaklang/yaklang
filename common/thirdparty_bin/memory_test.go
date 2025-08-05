package thirdparty_bin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// generateMemoryProfile 生成内存profile文件
func generateMemoryProfile(outputDir string) (string, error) {
	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %v", err)
	}

	// 生成文件名，包含时间戳
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(outputDir, fmt.Sprintf("memory_profile_%s.pprof", timestamp))

	// 创建profile文件
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create profile file: %v", err)
	}
	defer file.Close()

	// 强制垃圾回收以获得更准确的内存信息
	runtime.GC()
	runtime.GC()

	// 写入内存profile
	if err := pprof.WriteHeapProfile(file); err != nil {
		return "", fmt.Errorf("failed to write heap profile: %v", err)
	}

	return filename, nil
}

// generateGoroutineProfile 生成goroutine profile文件
func generateGoroutineProfile(outputDir string) (string, error) {
	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %v", err)
	}

	// 生成文件名，包含时间戳
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(outputDir, fmt.Sprintf("goroutine_profile_%s.pprof", timestamp))

	// 创建profile文件
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create profile file: %v", err)
	}
	defer file.Close()

	// 写入goroutine profile
	if err := pprof.Lookup("goroutine").WriteTo(file, 0); err != nil {
		return "", fmt.Errorf("failed to write goroutine profile: %v", err)
	}

	return filename, nil
}

// generateAllProfiles 生成所有相关的profile文件
func generateAllProfiles(outputDir string) error {
	fmt.Printf("Generating debug profiles to: %s\n", outputDir)

	// 生成内存profile
	memProfile, err := generateMemoryProfile(outputDir)
	if err != nil {
		return fmt.Errorf("failed to generate memory profile: %v", err)
	}
	fmt.Printf("✓ Memory profile: %s\n", memProfile)

	// 生成goroutine profile
	goroutineProfile, err := generateGoroutineProfile(outputDir)
	if err != nil {
		return fmt.Errorf("failed to generate goroutine profile: %v", err)
	}
	fmt.Printf("✓ Goroutine profile: %s\n", goroutineProfile)

	// 生成详细的内存统计信息
	memStatsFile := filepath.Join(outputDir, fmt.Sprintf("memstats_%s.txt", time.Now().Format("20060102_150405")))
	if err := writeMemoryStats(memStatsFile); err != nil {
		return fmt.Errorf("failed to write memory stats: %v", err)
	}
	fmt.Printf("✓ Memory stats: %s\n", memStatsFile)

	return nil
}

// writeMemoryStats 写入详细的内存统计信息
func writeMemoryStats(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	fmt.Fprintf(file, "=== Memory Statistics ===\n")
	fmt.Fprintf(file, "Timestamp: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "\n=== General Statistics ===\n")
	fmt.Fprintf(file, "Alloc (current heap objects): %d bytes (%.2f MB)\n", memStats.Alloc, float64(memStats.Alloc)/1024/1024)
	fmt.Fprintf(file, "TotalAlloc (cumulative): %d bytes (%.2f MB)\n", memStats.TotalAlloc, float64(memStats.TotalAlloc)/1024/1024)
	fmt.Fprintf(file, "Sys (total system memory): %d bytes (%.2f MB)\n", memStats.Sys, float64(memStats.Sys)/1024/1024)
	fmt.Fprintf(file, "Lookups: %d\n", memStats.Lookups)
	fmt.Fprintf(file, "Mallocs: %d\n", memStats.Mallocs)
	fmt.Fprintf(file, "Frees: %d\n", memStats.Frees)

	fmt.Fprintf(file, "\n=== Heap Statistics ===\n")
	fmt.Fprintf(file, "HeapAlloc: %d bytes (%.2f MB)\n", memStats.HeapAlloc, float64(memStats.HeapAlloc)/1024/1024)
	fmt.Fprintf(file, "HeapSys: %d bytes (%.2f MB)\n", memStats.HeapSys, float64(memStats.HeapSys)/1024/1024)
	fmt.Fprintf(file, "HeapIdle: %d bytes (%.2f MB)\n", memStats.HeapIdle, float64(memStats.HeapIdle)/1024/1024)
	fmt.Fprintf(file, "HeapInuse: %d bytes (%.2f MB)\n", memStats.HeapInuse, float64(memStats.HeapInuse)/1024/1024)
	fmt.Fprintf(file, "HeapReleased: %d bytes (%.2f MB)\n", memStats.HeapReleased, float64(memStats.HeapReleased)/1024/1024)
	fmt.Fprintf(file, "HeapObjects: %d\n", memStats.HeapObjects)

	fmt.Fprintf(file, "\n=== Stack Statistics ===\n")
	fmt.Fprintf(file, "StackInuse: %d bytes (%.2f MB)\n", memStats.StackInuse, float64(memStats.StackInuse)/1024/1024)
	fmt.Fprintf(file, "StackSys: %d bytes (%.2f MB)\n", memStats.StackSys, float64(memStats.StackSys)/1024/1024)

	fmt.Fprintf(file, "\n=== GC Statistics ===\n")
	fmt.Fprintf(file, "NumGC: %d\n", memStats.NumGC)
	fmt.Fprintf(file, "NumForcedGC: %d\n", memStats.NumForcedGC)
	fmt.Fprintf(file, "GCCPUFraction: %f\n", memStats.GCCPUFraction)

	fmt.Fprintf(file, "\n=== Goroutine Count ===\n")
	fmt.Fprintf(file, "NumGoroutine: %d\n", runtime.NumGoroutine())

	return nil
}

// TestInstallMemoryUsage 测试安装过程中的内存占用
func TestInstallMemoryUsage(t *testing.T) {
	// 创建临时目录用于测试
	tempDir := t.TempDir()
	installDir := filepath.Join(tempDir, "install")
	downloadDir := filepath.Join(tempDir, "download")
	profileDir := filepath.Join(tempDir, "profiles") // pprof文件输出目录

	// 确保目录存在
	os.MkdirAll(installDir, 0755)
	os.MkdirAll(downloadDir, 0755)
	os.MkdirAll(profileDir, 0755)

	// 打印profile目录位置
	fmt.Printf("Profile output directory: %s\n", profileDir)

	// 获取初始内存占用
	runtime.GC()                       // 强制垃圾回收
	runtime.GC()                       // 再次垃圾回收确保清理
	time.Sleep(100 * time.Millisecond) // 等待GC完成

	var initialMemStats runtime.MemStats
	runtime.ReadMemStats(&initialMemStats)
	initialMemoryMB := float64(initialMemStats.Alloc) / 1024 / 1024

	fmt.Printf("Initial memory usage: %.2f MB\n", initialMemoryMB)

	// 获取vulinbox配置
	config, err := LoadConfigFromEmbedded()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	var vulinboxBinary *BinaryDescriptor
	for _, binary := range config.Binaries {
		if binary.Name == "vulinbox" {
			vulinboxBinary = binary
			break
		}
	}

	if vulinboxBinary == nil {
		t.Fatalf("vulinbox binary not found in config")
	}

	// 创建安装器
	installer := NewInstaller(installDir, downloadDir)

	// 创建带取消功能的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 内存监控完成标志
	memoryMonitorDone := make(chan bool)
	memoryTestFailed := make(chan string, 1)

	// 启动内存监控协程
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				memoryMonitorDone <- true
				return
			case <-memoryMonitorDone:
				return
			case <-ticker.C:
				runtime.GC() // 执行垃圾回收以获得更准确的内存使用情况

				var currentMemStats runtime.MemStats
				runtime.ReadMemStats(&currentMemStats)
				currentMemoryMB := float64(currentMemStats.Alloc) / 1024 / 1024
				memoryIncreaseMB := currentMemoryMB - initialMemoryMB

				fmt.Printf("Current memory: %.2f MB, Increase: %.2f MB, Goroutines: %d\n",
					currentMemoryMB, memoryIncreaseMB, runtime.NumGoroutine())

				// 检查内存增长是否超过50MB
				if memoryIncreaseMB > 50.0 {
					fmt.Printf("⚠️  Memory limit exceeded! Generating debug profiles...\n")

					// 生成所有profile文件
					if err := generateAllProfiles(profileDir); err != nil {
						fmt.Printf("❌ Failed to generate profiles: %v\n", err)
					} else {
						fmt.Printf("✅ Debug profiles generated successfully\n")
						fmt.Printf("📁 Profile directory: %s\n", profileDir)
						fmt.Printf("🔍 To analyze memory usage:\n")
						fmt.Printf("   go tool pprof %s/memory_profile_*.pprof\n", profileDir)
						fmt.Printf("   go tool pprof %s/goroutine_profile_*.pprof\n", profileDir)
					}

					memoryTestFailed <- fmt.Sprintf("Memory usage increased by %.2f MB (> 50 MB limit). Debug profiles saved to: %s", memoryIncreaseMB, profileDir)
					return
				}
			}
		}
	}()

	// 配置安装选项
	options := &InstallOptions{
		Force:   true,
		Context: ctx,
		Progress: func(progress float64, downloaded, total int64, message string) {
			fmt.Printf("Progress: %.1f%% (%d/%d bytes) - %s\n", progress*100, downloaded, total, message)
		},
	}

	// 开始安装
	fmt.Println("Starting vulinbox installation...")
	installStart := time.Now()

	// 在单独的协程中执行安装
	installDone := make(chan error, 1)
	go func() {
		err := installer.Install(vulinboxBinary, options)
		installDone <- err
	}()

	// 等待安装完成或超时
	select {
	case err := <-installDone:
		// 安装完成，停止内存监控
		close(memoryMonitorDone)

		installDuration := time.Since(installStart)
		fmt.Printf("Installation completed in %v\n", installDuration)

		if err != nil {
			t.Errorf("Installation failed: %v", err)
		} else {
			fmt.Println("Installation successful!")
		}

	case failureMsg := <-memoryTestFailed:
		// 内存测试失败
		cancel() // 取消安装
		t.Fatalf("Memory test failed: %s", failureMsg)

	case <-ctx.Done():
		// 超时
		close(memoryMonitorDone)
		t.Fatalf("Installation timed out")
	}

	// 等待内存监控协程结束
	time.Sleep(100 * time.Millisecond)

	// 最终内存检查
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)
	finalMemoryMB := float64(finalMemStats.Alloc) / 1024 / 1024
	totalMemoryIncrease := finalMemoryMB - initialMemoryMB

	fmt.Printf("Final memory usage: %.2f MB\n", finalMemoryMB)
	fmt.Printf("Total memory increase: %.2f MB\n", totalMemoryIncrease)

	// 检查最终内存增长
	if totalMemoryIncrease > 50.0 {
		// 生成最终的profile文件
		fmt.Printf("⚠️  Final memory check failed! Generating final debug profiles...\n")
		if err := generateAllProfiles(profileDir); err != nil {
			fmt.Printf("❌ Failed to generate final profiles: %v\n", err)
		}
		t.Errorf("Final memory increase (%.2f MB) exceeds 50 MB limit. Debug profiles saved to: %s", totalMemoryIncrease, profileDir)
	}

	// 验证安装是否成功
	if installer.IsInstalled(vulinboxBinary) {
		fmt.Println("✓ Vulinbox is properly installed")

		// 清理安装的文件
		installPath := installer.GetInstallPath(vulinboxBinary)
		if err := os.Remove(installPath); err != nil {
			log.Warnf("Failed to cleanup installed file: %v", err)
		}
	} else {
		t.Error("Vulinbox installation verification failed")
	}
}

// TestMemoryUsageBaseline 测试基准内存占用
func TestMemoryUsageBaseline(t *testing.T) {
	// 多次测量基准内存占用
	var measurements []float64

	for i := 0; i < 5; i++ {
		runtime.GC()
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		memoryMB := float64(memStats.Alloc) / 1024 / 1024
		measurements = append(measurements, memoryMB)

		fmt.Printf("Baseline measurement %d: %.2f MB\n", i+1, memoryMB)
		time.Sleep(500 * time.Millisecond)
	}

	// 计算平均值
	var sum float64
	for _, m := range measurements {
		sum += m
	}
	average := sum / float64(len(measurements))

	fmt.Printf("Average baseline memory: %.2f MB\n", average)
	fmt.Printf("Memory measurements: %v\n", measurements)

	// 检查基准内存是否稳定（变化不超过5MB）
	for _, m := range measurements {
		if abs(m-average) > 5.0 {
			t.Errorf("Memory baseline unstable: measurement %.2f MB deviates more than 5MB from average %.2f MB", m, average)
		}
	}
}

// abs 计算浮点数的绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
