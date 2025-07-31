package thirdparty_bin

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestProcessManagement(t *testing.T) {
	// 测试启动、停止和获取运行中的二进制程序

	// 创建一个简单的回调函数来处理输出
	outputReceived := make(chan string, 1)
	callback := func(reader io.Reader) {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				select {
				case outputReceived <- line:
				default:
				}
			}
		}
	}

	// 测试启动一个不存在的程序
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := Start(ctx, "nonexistent", []string{}, callback)
	if err == nil {
		t.Error("Expected error when starting nonexistent binary")
	}

	// 测试获取运行中的程序列表（应该为空）
	runningBinaries := GetRunningBinaries()
	if len(runningBinaries) != 0 {
		t.Errorf("Expected 0 running binaries, got %d", len(runningBinaries))
	}

	// 测试检查程序是否运行
	if IsRunning("nonexistent") {
		t.Error("Expected nonexistent binary to not be running")
	}

	// 测试停止不存在的程序
	err = Stop("nonexistent")
	if err == nil {
		t.Error("Expected error when stopping nonexistent binary")
	}

	t.Log("Process management tests completed successfully")
}

func TestProcessCallback(t *testing.T) {
	// 测试进程回调函数类型
	callbackCalled := false

	callback := func(reader io.Reader) {
		callbackCalled = true
		// 模拟读取一些数据
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "test") {
				t.Logf("Received output: %s", line)
			}
		}
	}

	// 测试回调函数是否可以正常调用
	testReader := strings.NewReader("test output\n")
	callback(testReader)

	if !callbackCalled {
		t.Error("Callback function was not called")
	}
}

func TestRunningProcessStruct(t *testing.T) {
	// 测试RunningProcess结构体
	process := &RunningProcess{
		Name:     "test",
		Cmd:      nil,
		Cancel:   nil,
		Callback: nil,
	}

	if process.Name != "test" {
		t.Errorf("Expected process name 'test', got '%s'", process.Name)
	}

	t.Log("RunningProcess struct test completed successfully")
}
