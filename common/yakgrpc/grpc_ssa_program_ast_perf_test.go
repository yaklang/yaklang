package yakgrpc

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
)

// TestGRPCMUSTPASS_GoProjectASTPerformance 测试大项目的AST解析性能
// 测试 common/yak/go2ssa 目录下的所有.go文件的AST解析时间
//
// 运行测试时禁用缓存（强制每次运行）:
//   go test -v -count=1 -run TestGRPCMUSTPASS_GoProjectASTPerformance ./common/yakgrpc
func TestGRPCMUSTPASS_GoProjectASTPerformance(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in github actions")
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("无法获取当前文件路径")
	}

	// 从当前测试文件位置向上查找项目根目录
	projectRoot := filepath.Dir(filename)   // common/yakgrpc
	projectRoot = filepath.Dir(projectRoot) // common
	projectRoot = filepath.Dir(projectRoot) // yaklang (项目根目录)

	// 硬编码测试目录: common/yak/go2ssa
	testDir := filepath.Join(projectRoot, "common", "yak")

	// 检查目录是否存在
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("无法访问目录 %s: %v", testDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s 不是一个目录", testDir)
	}

	fmt.Printf("开始测试目录: %s\n\n", testDir)

	// 创建builder以获取AntlrCache
	builder := go2ssa.CreateBuilder()
	cache := builder.GetAntlrCache()

	var totalFiles int
	var totalSize int64
	var totalTime time.Duration
	var errorCount int

	// 打印表头
	fmt.Printf("%-60s %12s %15s\n", "文件名", "文件大小", "解析时间")
	fmt.Printf("%s\n", "----------------------------------------------------------------------------------------")

	// 遍历目录下所有.go文件，直接输出结果
	err = filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 只处理.go文件
		if d.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		// 读取文件内容
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("%-60s %12s %15s (警告: 无法读取文件)\n", filepath.Base(path), "N/A", "N/A")
			return nil
		}

		fileSize := int64(len(content))
		fileName := filepath.Base(path)

		// 测量AST解析时间
		start := time.Now()
		ast, err := builder.ParseAST(string(content), cache)
		duration := time.Since(start)

		// 立即输出结果，不缓存
		if err != nil {
			fmt.Printf("%-60s %12s %15s (错误: %s)\n", fileName, formatSize(fileSize), duration, err.Error())
			errorCount++
		} else {
			fmt.Printf("%-60s %12s %15s\n", fileName, formatSize(fileSize), duration)
			totalFiles++
			totalSize += fileSize
			totalTime += duration
		}

		_ = ast // 避免未使用变量警告
		return nil
	})

	if err != nil {
		t.Fatalf("遍历目录失败: %v", err)
	}

	// 打印汇总结果
	fmt.Printf("%s\n", "----------------------------------------------------------------------------------------")
	fmt.Printf("\n========== AST解析性能测试结果 ==========\n")
	fmt.Printf("测试目录: %s\n", testDir)
	fmt.Printf("总文件数: %d\n", totalFiles)
	if errorCount > 0 {
		fmt.Printf("错误文件数: %d\n", errorCount)
	}
	fmt.Printf("总大小: %s\n", formatSize(totalSize))
	fmt.Printf("总耗时: %v\n", totalTime)
	if totalFiles > 0 {
		fmt.Printf("平均耗时: %v\n", totalTime/time.Duration(totalFiles))
		fmt.Printf("平均速度: %.2f KB/s\n", float64(totalSize)/1024.0/totalTime.Seconds())
	}
	fmt.Printf("==========================================\n\n")
}

// formatSize 格式化文件大小，参考 ssaapi.Size 函数
func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	}
	sizeKB := float64(size) / 1024.0
	if sizeKB < 1024 {
		return fmt.Sprintf("%.2fKB", sizeKB)
	}
	sizeMB := sizeKB / 1024.0
	if sizeMB < 1024 {
		return fmt.Sprintf("%.2fMB", sizeMB)
	}
	sizeGB := sizeMB / 1024.0
	return fmt.Sprintf("%.2fGB", sizeGB)
}
