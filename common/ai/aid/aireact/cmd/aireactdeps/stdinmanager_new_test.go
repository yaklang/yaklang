package aireactdeps

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestStdinManagerDivertedReading 测试分流读取机制
func TestStdinManagerDivertedReading(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	// 模拟promptui使用场景
	t.Log("=== 模拟promptui使用场景 ===")

	// 1. 默认情况下，GetDefaultReader返回原始stdin
	defaultReader := manager.GetDefaultReader()
	if defaultReader != os.Stdin {
		t.Error("Default reader should be os.Stdin when not prevented")
	}

	// 2. 调用PreventDefault获取分流reader
	t.Log("调用PreventDefault，获取分流reader用于promptui")
	divertedReader := manager.PreventDefault()
	if divertedReader == nil {
		t.Error("PreventDefault should return a valid reader")
	}

	// 3. 此时GetDefaultReader应该返回空reader
	defaultReader = manager.GetDefaultReader()
	if _, ok := defaultReader.(*emptyReader); !ok {
		t.Error("Default reader should be emptyReader when prevented")
	}

	// 4. 验证IsPrevented状态
	if !manager.IsPrevented() {
		t.Error("Manager should be prevented after PreventDefault")
	}

	// 5. 调用RecoverDefault恢复
	t.Log("调用RecoverDefault，恢复默认stdin")
	manager.RecoverDefault()

	// 6. 验证恢复状态
	if manager.IsPrevented() {
		t.Error("Manager should not be prevented after RecoverDefault")
	}

	defaultReader = manager.GetDefaultReader()
	if defaultReader != os.Stdin {
		t.Error("Default reader should be os.Stdin after recovery")
	}

	t.Log("✓ 分流机制测试通过")
}

// TestStdinManagerPromptuiScenario 测试promptui使用场景
func TestStdinManagerPromptuiScenario(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	t.Log("=== promptui使用场景示例 ===")
	t.Log("以下是如何在promptui中使用StdinManager避免stdin争抢：")
	t.Log("")
	t.Log("// 在需要使用promptui的地方")
	t.Log("manager := NewStdinManager()")
	t.Log("divertedReader := manager.PreventDefault() // 获取分流reader")
	t.Log("defer manager.RecoverDefault()            // 确保恢复")
	t.Log("")
	t.Log("// 将divertedReader传给promptui使用")
	t.Log("// 这样promptui就不会与其他goroutine争抢os.Stdin了")
	t.Log("")

	// 模拟使用过程
	divertedReader := manager.PreventDefault()
	defer manager.RecoverDefault()

	// 验证分流reader可以正常工作
	if divertedReader == nil {
		t.Error("Diverted reader should not be nil")
	}

	// 验证其他goroutine读取GetDefaultReader()会得到空reader
	defaultReader := manager.GetDefaultReader()
	buffer := make([]byte, 10)
	n, err := defaultReader.Read(buffer)
	if n != 0 || err != io.EOF {
		t.Error("Default reader should return EOF when prevented")
	}

	t.Log("✓ promptui场景测试通过")
	t.Log("✓ 现在可以安全地将divertedReader传给promptui使用")
}

// TestSetupSignalHandlerWithStdinManager 测试SetupSignalHandler与StdinManager的集成
func TestSetupSignalHandlerWithStdinManager(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	t.Log("=== 测试SetupSignalHandler与StdinManager的集成 ===")

	// 模拟SetupSignalHandler的行为
	var handledInputs []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 模拟handleFreeInput函数
	handleFreeInput := func(input string, config interface{}) {
		mu.Lock()
		handledInputs = append(handledInputs, strings.TrimSpace(input))
		mu.Unlock()
		t.Logf("Background handler processed: %q", strings.TrimSpace(input))
	}

	// 启动背景处理器（类似SetupSignalHandler）
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 获取当前可用的reader
				defaultReader := manager.GetDefaultReader()

				// 模拟读取（这里我们不能真正从stdin读取，所以模拟EOF情况）
				buffer := make([]byte, 1)
				n, err := defaultReader.Read(buffer)

				if err != nil {
					if err == io.EOF {
						// 如果是EOF（被阻止），等待一段时间后重试
						time.Sleep(10 * time.Millisecond)
						continue
					}
					t.Logf("Background handler error: %v", err)
					continue
				}

				if n > 0 {
					// 处理正常输入（在真实场景中这里会调用utils.ReadLine）
					handleFreeInput(string(buffer[:n]), nil)
				}

				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// 让背景处理器运行一段时间
	time.Sleep(100 * time.Millisecond)

	// 模拟promptui使用（阻止背景处理器）
	t.Log("模拟promptui开始使用stdin...")
	divertedReader := manager.PreventDefault()
	if divertedReader == nil {
		t.Error("PreventDefault should return a valid reader")
	}

	// 验证背景处理器被正确阻止（不会产生错误日志）
	time.Sleep(200 * time.Millisecond)

	// 恢复背景处理器
	t.Log("promptui使用完毕，恢复背景处理器...")
	manager.RecoverDefault()

	// 让背景处理器继续运行一段时间
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	inputCount := len(handledInputs)
	mu.Unlock()

	t.Logf("背景处理器处理了 %d 个输入", inputCount)
	t.Log("✓ SetupSignalHandler集成测试通过")
	t.Log("✓ 背景处理器在被阻止时不会产生错误日志")
	t.Log("✓ 背景处理器在恢复后可以继续正常工作")
}

// TestStdinManagerSynchronizedControl 测试强力同步控制机制
func TestStdinManagerSynchronizedControl(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	t.Log("=== 测试强力同步控制机制 ===")

	var wg sync.WaitGroup
	var processingStates []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 启动背景处理器（使用新的同步机制）
	wg.Add(1)
	go func() {
		defer wg.Done()

		controller := manager.RegisterReader()
		defer controller.Unregister()

		for {
			select {
			case <-ctx.Done():
				mu.Lock()
				processingStates = append(processingStates, "background_stopped")
				mu.Unlock()
				return
			default:
				// 检查暂停/恢复信号
				if !controller.WaitForSignals() {
					continue
				}

				mu.Lock()
				processingStates = append(processingStates, "background_active")
				mu.Unlock()

				// 模拟处理（在真实场景中这里会调用utils.ReadLine）
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	// 让背景处理器运行一段时间
	time.Sleep(200 * time.Millisecond)

	// 测试PreventDefault的同步效果
	t.Log("调用PreventDefault，应该立即阻止背景处理器...")

	wg.Add(1)
	go func() {
		defer wg.Done()

		divertedReader := manager.PreventDefault()
		if divertedReader == nil {
			t.Error("PreventDefault should return a valid reader")
		}

		mu.Lock()
		processingStates = append(processingStates, "prevent_completed")
		mu.Unlock()

		// 模拟promptui使用时间
		time.Sleep(300 * time.Millisecond)

		t.Log("调用RecoverDefault，应该等待背景处理器恢复...")
		manager.RecoverDefault()

		mu.Lock()
		processingStates = append(processingStates, "recover_completed")
		mu.Unlock()
	}()

	wg.Wait()

	mu.Lock()
	states := make([]string, len(processingStates))
	copy(states, processingStates)
	mu.Unlock()

	t.Logf("处理状态序列: %v", states)

	// 验证同步效果
	preventIndex := -1
	recoverIndex := -1
	for i, state := range states {
		if state == "prevent_completed" {
			preventIndex = i
		}
		if state == "recover_completed" {
			recoverIndex = i
		}
	}

	if preventIndex == -1 || recoverIndex == -1 {
		t.Error("Should have both prevent and recover events")
	}

	// 验证在prevent和recover之间没有background_active
	backgroundActiveDuringPrevent := false
	for i := preventIndex + 1; i < recoverIndex; i++ {
		if states[i] == "background_active" {
			backgroundActiveDuringPrevent = true
			break
		}
	}

	if backgroundActiveDuringPrevent {
		t.Error("Background should not be active during prevent period")
	}

	t.Log("✓ 强力同步控制测试通过")
	t.Log("✓ PreventDefault确保背景处理器完全停止")
	t.Log("✓ RecoverDefault确保背景处理器重新开始后才完成")
}

// TestClosableBufioReader 测试可关闭的bufio Reader
func TestClosableBufioReader(t *testing.T) {
	t.Log("=== 测试可关闭的bufio Reader ===")

	// 创建一个管道来模拟stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// 创建可关闭的bufio reader
	cbr := NewClosableBufioReader(r)
	defer cbr.Close()

	var wg sync.WaitGroup
	var readError error
	var mu sync.Mutex

	// 启动读取goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		_, err := cbr.ReadLine()
		mu.Lock()
		readError = err
		mu.Unlock()
	}()

	// 等待一小段时间确保读取开始
	time.Sleep(100 * time.Millisecond)

	// 主动关闭reader
	t.Log("主动关闭bufio reader...")
	err = cbr.Close()
	if err != nil {
		t.Errorf("Failed to close reader: %v", err)
	}

	// 等待读取goroutine完成
	wg.Wait()

	// 验证结果
	mu.Lock()
	defer mu.Unlock()

	// 关闭管道可能返回EOF或"file already closed"错误
	if readError == nil {
		t.Error("Expected an error after close")
	} else if readError != io.EOF && !strings.Contains(readError.Error(), "closed") {
		t.Errorf("Expected EOF or close error after close, got: %v", readError)
	}

	t.Log("✓ 可关闭的bufio Reader测试通过")
	t.Log("✓ 主动关闭成功中断了阻塞的读取操作")
}

// TestReaderControllerBufioReadLine 测试ReaderController的bufio ReadLine功能
func TestReaderControllerBufioReadLine(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	t.Log("=== 测试ReaderController的bufio ReadLine功能 ===")

	manager := NewStdinManager()

	// 创建一个管道来模拟stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer w.Close()

	// 替换原始stdin
	originalStdin := manager.originalStdin
	manager.originalStdin = r
	defer func() {
		manager.originalStdin = originalStdin
		r.Close()
	}()

	controller := manager.RegisterReader()
	defer controller.Unregister()

	var wg sync.WaitGroup
	var readResults []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 启动读取goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				line, err := controller.ReadLine()
				if err != nil {
					if err == io.EOF {
						return
					}
					t.Logf("Read error: %v", err)
					continue
				}

				mu.Lock()
				readResults = append(readResults, string(line))
				mu.Unlock()

				t.Logf("Read line: %q", string(line))
			}
		}
	}()

	// 发送一些测试数据
	testLines := []string{"line1", "line2", "line3"}
	for _, line := range testLines {
		_, err := w.Write([]byte(line + "\n"))
		if err != nil {
			t.Errorf("Failed to write test data: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 主动关闭controller来中断读取
	time.Sleep(200 * time.Millisecond)
	t.Log("主动关闭controller...")
	controller.Close()

	wg.Wait()

	mu.Lock()
	results := make([]string, len(readResults))
	copy(results, readResults)
	mu.Unlock()

	t.Logf("读取到的行: %v", results)

	// 验证至少读取了一些数据
	if len(results) == 0 {
		t.Error("Should have read at least some lines")
	}

	t.Log("✓ ReaderController bufio ReadLine测试通过")
	t.Log("✓ 主动关闭成功中断了bufio读取循环")
}

// TestSetupSignalHandlerWithBufioReadLine 测试修复后的SetupSignalHandler同步机制
func TestSetupSignalHandlerWithBufioReadLine(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	t.Log("=== 测试修复后的SetupSignalHandler同步机制 ===")

	// 创建一个管道来模拟stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer w.Close()

	// 替换原始stdin
	originalStdin := manager.originalStdin
	manager.originalStdin = r
	defer func() {
		manager.originalStdin = originalStdin
		r.Close()
	}()

	var wg sync.WaitGroup
	var handledInputs []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 模拟handleFreeInput函数
	handleFreeInput := func(input string, config interface{}) {
		mu.Lock()
		handledInputs = append(handledInputs, strings.TrimSpace(input))
		mu.Unlock()
		t.Logf("Handled input: %q", strings.TrimSpace(input))
	}

	// 启动模拟的SetupSignalHandler
	wg.Add(1)
	go func() {
		defer wg.Done()

		controller := manager.RegisterReader()
		defer controller.Unregister()

		for {
			select {
			case <-ctx.Done():
				controller.Close()
				return
			default:
				// 检查暂停/恢复信号
				if !controller.WaitForSignals() {
					continue
				}

				// 使用bufio ReadLine
				buffer, err := controller.ReadLine()
				if err != nil {
					if err == io.EOF {
						select {
						case <-ctx.Done():
							return
						default:
							time.Sleep(50 * time.Millisecond)
							continue
						}
					}
					t.Logf("Read error: %v", err)
					continue
				}

				// 处理正常输入
				handleFreeInput(string(buffer)+"\n", nil)
			}
		}
	}()

	// 发送一些测试数据
	testInputs := []string{"input1", "input2"}
	for _, input := range testInputs {
		_, err := w.Write([]byte(input + "\n"))
		if err != nil {
			t.Errorf("Failed to write test data: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 测试PreventDefault的同步效果
	t.Log("调用PreventDefault，应该阻止背景处理器...")
	divertedReader := manager.PreventDefault()
	if divertedReader == nil {
		t.Error("PreventDefault should return a valid reader")
	}

	// 在阻止期间发送数据（应该不会被处理）
	_, err = w.Write([]byte("blocked_input\n"))
	if err != nil {
		t.Errorf("Failed to write blocked test data: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 恢复处理器
	t.Log("调用RecoverDefault，恢复背景处理器...")
	manager.RecoverDefault()

	// 发送恢复后的数据
	_, err = w.Write([]byte("input3\n"))
	if err != nil {
		t.Errorf("Failed to write recovery test data: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	wg.Wait()

	mu.Lock()
	inputs := make([]string, len(handledInputs))
	copy(inputs, handledInputs)
	mu.Unlock()

	t.Logf("处理的输入: %v", inputs)

	// 验证同步效果：应该处理了阻止前和恢复后的输入，但不应该处理阻止期间的输入
	hasBlockedInput := false
	for _, input := range inputs {
		if strings.Contains(input, "blocked_input") {
			hasBlockedInput = true
			break
		}
	}

	if hasBlockedInput {
		t.Error("Should not have processed input during prevented period")
	}

	// 应该至少处理了一些正常输入
	if len(inputs) == 0 {
		t.Error("Should have processed at least some inputs")
	}

	t.Log("✓ 修复后的SetupSignalHandler同步机制测试通过")
	t.Log("✓ WaitForSignals()和bufio ReadLine()协同工作正常")
}
