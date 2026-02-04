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
	"github.com/yaklang/yaklang/common/yak/c2ssa"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
)

// TestGRPCMUSTPASS_GoProjectASTPerformance 测试大项目的AST解析性能
// 测试指定目录下的所有.go文件的AST解析时间
//
// 使用方法:
//  1. 在代码中修改 testDir 变量，设置为要测试的目录绝对路径
//  2. 运行测试: go test -v -count=1 -run TestGRPCMUSTPASS_GoProjectASTPerformance ./common/yakgrpc
//
// 如果不设置 testDir，默认测试 common/yak 目录
func TestGRPCMUSTPASS_GoProjectASTPerformance(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in github actions")
	}

	// ========== 请在此处设置要测试的目录绝对路径 ==========
	// 例如: testDir := "C:\\Users\\username\\work\\project"
	// 或者: testDir := "/home/username/work/project"
	testDir := ""
	// ====================================================

	if testDir == "" {
		// 如果没有设置，使用默认目录
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatalf("无法获取当前文件路径")
		}

		// 从当前测试文件位置向上查找项目根目录
		projectRoot := filepath.Dir(filename)   // common/yakgrpc
		projectRoot = filepath.Dir(projectRoot) // common
		projectRoot = filepath.Dir(projectRoot) // yaklang (项目根目录)

		// 默认测试目录: common/yak
		testDir = filepath.Join(projectRoot, "common", "yak")
	}

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
	fmt.Printf("%-60s %12s %15s\n", "相对路径", "文件大小", "解析时间")
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

		// 获取相对于测试目录的路径
		relPath, err := filepath.Rel(testDir, path)
		if err != nil {
			// 如果获取相对路径失败，使用绝对路径
			relPath = path
		}

		// 读取文件内容
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("%-60s %12s %15s (警告: 无法读取文件)\n", relPath, "N/A", "N/A")
			return nil
		}

		fileSize := int64(len(content))

		// 测量AST解析时间
		start := time.Now()
		ast, err := builder.ParseAST(string(content), cache)
		duration := time.Since(start)

		// 检查解析时间是否超过1分钟，如果超过则在路径前添加 @@ 标记
		displayPath := relPath
		if duration >= time.Minute {
			displayPath = "@@" + relPath
		}

		// 立即输出结果，不缓存
		if err != nil {
			fmt.Printf("%-60s %12s %15s (错误: %s)\n", displayPath, formatSize(fileSize), duration, err.Error())
			errorCount++
		} else {
			fmt.Printf("%-60s %12s %15s\n", displayPath, formatSize(fileSize), duration)
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

// TestGRPCMUSTPASS_CProjectASTPerformance 测试大项目的C语言AST解析性能
// 测试指定目录下的所有.c文件的AST解析时间
//
// 使用方法:
//  1. 在代码中修改 testDir 变量，设置为要测试的目录绝对路径
//  2. 运行测试: go test -v -count=1 -run TestGRPCMUSTPASS_CProjectASTPerformance ./common/yakgrpc
//
// 如果不设置 testDir，默认测试 common/yak/antlr4c 目录
func TestGRPCMUSTPASS_CProjectASTPerformance(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in github actions")
	}

	// ========== 请在此处设置要测试的目录绝对路径 ==========
	// 例如: testDir := "C:\\Users\\username\\work\\cve-project"
	// 或者: testDir := "/home/username/work/cve-project"
	testDir := ""
	// ====================================================

	if testDir == "" {
		// 如果没有设置，使用默认目录
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatalf("无法获取当前文件路径")
		}

		// 从当前测试文件位置向上查找项目根目录
		projectRoot := filepath.Dir(filename)   // common/yakgrpc
		projectRoot = filepath.Dir(projectRoot) // common
		projectRoot = filepath.Dir(projectRoot) // yaklang (项目根目录)

		// 默认测试目录: common/yak/antlr4c
		testDir = filepath.Join(projectRoot, "common", "yak", "antlr4c")
	}

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
	builder := c2ssa.CreateBuilder()
	cache := builder.GetAntlrCache()

	var totalFiles int
	var totalSize int64
	var totalTime time.Duration
	var errorCount int

	// 打印表头
	fmt.Printf("%-60s %12s %15s\n", "相对路径", "文件大小", "解析时间")
	fmt.Printf("%s\n", "----------------------------------------------------------------------------------------")

	// 遍历目录下所有.c文件，直接输出结果
	err = filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 只处理.c文件
		if d.IsDir() || filepath.Ext(path) != ".c" {
			return nil
		}

		// 获取相对于测试目录的路径
		relPath, err := filepath.Rel(testDir, path)
		if err != nil {
			// 如果获取相对路径失败，使用绝对路径
			relPath = path
		}

		// 读取文件内容
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("%-60s %12s %15s (警告: 无法读取文件)\n", relPath, "N/A", "N/A")
			return nil
		}

		fileSize := int64(len(content))

		// 预处理（宏扩展）
		preprocessedContent, preprocessErr := c2ssa.PreprocessCSource(string(content))
		if preprocessErr != nil {
			// 如果预处理失败，使用原始内容并记录警告
			preprocessedContent = string(content)
		}

		// 测量AST解析时间
		start := time.Now()
		ast, err := builder.ParseAST(preprocessedContent, cache)
		duration := time.Since(start)

		// 检查解析时间是否超过1分钟，如果超过则在路径前添加 @@ 标记
		displayPath := relPath
		if duration >= time.Minute {
			displayPath = "@@" + relPath
		}

		// 立即输出结果，不缓存
		if err != nil {
			fmt.Printf("%-60s %12s %15s (错误: %s)\n", displayPath, formatSize(fileSize), duration, err.Error())
			errorCount++
		} else {
			fmt.Printf("%-60s %12s %15s\n", displayPath, formatSize(fileSize), duration)
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
	fmt.Printf("\n========== C语言AST解析性能测试结果 ==========\n")
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
