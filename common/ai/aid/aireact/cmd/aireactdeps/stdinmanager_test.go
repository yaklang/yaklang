package aireactdeps

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

// TestStdinManagerSingleton 测试单例模式
func TestStdinManagerSingleton(t *testing.T) {
	// 重置单例实例以确保测试的独立性
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager1 := NewStdinManager()
	manager2 := NewStdinManager()

	if manager1 != manager2 {
		t.Error("StdinManager should be singleton, but got different instances")
	}

	if manager1 == nil {
		t.Error("NewStdinManager should return non-nil instance")
	}
}

// TestStdinManagerConcurrency 测试并发安全
func TestStdinManagerConcurrency(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	var wg sync.WaitGroup
	instances := make([]*StdinManager, 10)

	// 并发创建10个实例
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			instances[index] = NewStdinManager()
		}(i)
	}

	wg.Wait()

	// 检查所有实例是否相同
	for i := 1; i < 10; i++ {
		if instances[0] != instances[i] {
			t.Error("Concurrent access should return same instance")
		}
	}
}

// TestStdinManagerBasicFunctionality 测试基本功能
func TestStdinManagerBasicFunctionality(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	// 测试初始状态
	if manager.IsPrevented() {
		t.Error("Initial state should not be prevented")
	}

	// 测试GetDefaultReader
	reader := manager.GetDefaultReader()
	if reader == nil {
		t.Error("GetDefaultReader should return non-nil reader")
	}

	// 测试PreventDefault
	manager.PreventDefault()
	if !manager.IsPrevented() {
		t.Error("After PreventDefault, IsPrevented should return true")
	}

	// 测试RecoverDefault
	manager.RecoverDefault()
	if manager.IsPrevented() {
		t.Error("After RecoverDefault, IsPrevented should return false")
	}
}

// TestStdinManagerTDDScenario 测试TDD场景
func TestStdinManagerTDDScenario(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	// 按照TDD示例测试
	manager := NewStdinManager()

	defaultReader := manager.GetDefaultReader()
	if defaultReader == nil {
		t.Error("GetDefaultReader should return valid reader")
	}

	// 模拟TDD场景中的使用方式
	manager.PreventDefault()
	defer manager.RecoverDefault()

	if !manager.IsPrevented() {
		t.Error("Should be prevented after PreventDefault")
	}

	// defer会在函数结束时执行RecoverDefault
}

// TestStdinManagerPreventRecoverMultipleTimes 测试多次调用PreventDefault和RecoverDefault
func TestStdinManagerPreventRecoverMultipleTimes(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	// 多次调用PreventDefault
	manager.PreventDefault()
	manager.PreventDefault()
	if !manager.IsPrevented() {
		t.Error("Should still be prevented after multiple PreventDefault calls")
	}

	// 多次调用RecoverDefault
	manager.RecoverDefault()
	manager.RecoverDefault()
	if manager.IsPrevented() {
		t.Error("Should not be prevented after multiple RecoverDefault calls")
	}
}

// TestStdinManagerMultipleGoroutineReading 测试多个goroutine同时读取stdin的场景
func TestStdinManagerMultipleGoroutineReading(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	// 创建模拟的stdin数据
	mockStdin := &mockReader{
		data: []byte("test input line 1\ntest input line 2\ntest input line 3\n"),
	}

	// 设置mock reader作为默认reader
	manager.mu.Lock()
	manager.defaultReader = mockStdin
	manager.mu.Unlock()

	var wg sync.WaitGroup
	var defaultGoroutineBlocked bool
	var otherGoroutineRead bool
	var mu sync.Mutex

	// 模拟默认的持续读取goroutine（类似SetupSignalHandler）
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 3; i++ {
			// 检查是否被阻止
			if manager.IsPrevented() {
				mu.Lock()
				defaultGoroutineBlocked = true
				mu.Unlock()
				// 等待恢复
				for manager.IsPrevented() {
					time.Sleep(10 * time.Millisecond)
				}
			}

			// 尝试读取
			defaultReader := manager.GetDefaultReader()
			if mockReader, ok := defaultReader.(*mockReader); ok && !mockReader.exhausted {
				line, err := mockReader.ReadLine()
				if err == nil && len(line) > 0 {
					t.Logf("Default goroutine read: %s", string(line))
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// 模拟其他需要临时读取stdin的goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(100 * time.Millisecond) // 让默认goroutine先运行

		// 阻止默认读取
		manager.PreventDefault()
		defer manager.RecoverDefault()

		// 进行自己的读取
		if mockReader, ok := manager.defaultReader.(*mockReader); ok && !mockReader.exhausted {
			line, err := mockReader.ReadLine()
			if err == nil && len(line) > 0 {
				mu.Lock()
				otherGoroutineRead = true
				mu.Unlock()
				t.Logf("Other goroutine read: %s", string(line))
			}
		}

		time.Sleep(100 * time.Millisecond) // 模拟处理时间
	}()

	wg.Wait()

	// 验证测试结果
	mu.Lock()
	defer mu.Unlock()

	if !defaultGoroutineBlocked {
		t.Error("Default goroutine should have been blocked when other goroutine was reading")
	}

	if !otherGoroutineRead {
		t.Error("Other goroutine should have successfully read from stdin")
	}

	if manager.IsPrevented() {
		t.Error("Manager should not be prevented after other goroutine finished")
	}
}

// mockReader 模拟stdin读取器
type mockReader struct {
	data      []byte
	pos       int
	exhausted bool
	mu        sync.Mutex
}

func (mr *mockReader) Read(p []byte) (n int, err error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.exhausted || mr.pos >= len(mr.data) {
		mr.exhausted = true
		return 0, io.EOF
	}

	n = copy(p, mr.data[mr.pos:])
	mr.pos += n

	if mr.pos >= len(mr.data) {
		mr.exhausted = true
	}

	return n, nil
}

func (mr *mockReader) ReadLine() ([]byte, error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.exhausted || mr.pos >= len(mr.data) {
		mr.exhausted = true
		return nil, io.EOF
	}

	// 查找下一个换行符
	start := mr.pos
	for mr.pos < len(mr.data) && mr.data[mr.pos] != '\n' {
		mr.pos++
	}

	if mr.pos < len(mr.data) && mr.data[mr.pos] == '\n' {
		line := mr.data[start:mr.pos]
		mr.pos++ // 跳过换行符
		return line, nil
	}

	// 没有找到换行符，返回剩余数据
	if start < len(mr.data) {
		line := mr.data[start:]
		mr.pos = len(mr.data)
		mr.exhausted = true
		return line, nil
	}

	mr.exhausted = true
	return nil, io.EOF
}

// TestStdinManagerRealWorldScenario 测试真实世界的使用场景
// 模拟SetupSignalHandler中的持续读取goroutine和临时需要读取stdin的其他goroutine
func TestStdinManagerRealWorldScenario(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()

	// 创建模拟的stdin数据，包含用户输入和命令
	mockStdin := &mockReader{
		data: []byte("user input 1\nmenu command\nuser input 2\nexit\n"),
	}

	// 设置mock reader作为默认reader
	manager.mu.Lock()
	manager.defaultReader = mockStdin
	manager.mu.Unlock()

	var wg sync.WaitGroup
	var defaultReads []string
	var interactiveReads []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 模拟SetupSignalHandler中的持续读取goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		defaultReader := manager.GetDefaultReader()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 检查是否被阻止
				if manager.IsPrevented() {
					time.Sleep(10 * time.Millisecond)
					continue
				}

				// 尝试读取一行
				if mockReader, ok := defaultReader.(*mockReader); ok && !mockReader.exhausted {
					line, err := mockReader.ReadLine()
					if err == nil && len(line) > 0 {
						mu.Lock()
						defaultReads = append(defaultReads, string(line))
						mu.Unlock()
						t.Logf("Default handler read: %s", string(line))
					}
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// 模拟交互式菜单或其他需要临时读取stdin的goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(100 * time.Millisecond) // 让默认goroutine先运行一会儿

		// 阻止默认读取，进行交互式操作
		manager.PreventDefault()
		defer manager.RecoverDefault()

		t.Log("Interactive mode started, default reading prevented")

		// 模拟交互式读取
		if mockReader, ok := manager.defaultReader.(*mockReader); ok && !mockReader.exhausted {
			line, err := mockReader.ReadLine()
			if err == nil && len(line) > 0 {
				mu.Lock()
				interactiveReads = append(interactiveReads, string(line))
				mu.Unlock()
				t.Logf("Interactive mode read: %s", string(line))
			}
		}

		time.Sleep(200 * time.Millisecond) // 模拟交互处理时间
		t.Log("Interactive mode ended, default reading restored")
	}()

	wg.Wait()

	// 验证测试结果
	mu.Lock()
	defer mu.Unlock()

	if len(defaultReads) == 0 {
		t.Error("Default goroutine should have read some data")
	}

	if len(interactiveReads) == 0 {
		t.Error("Interactive goroutine should have read some data")
	}

	// 验证读取的数据不重复（每行只被一个goroutine读取）
	allReads := make(map[string]bool)
	for _, read := range defaultReads {
		if allReads[read] {
			t.Errorf("Duplicate read detected: %s", read)
		}
		allReads[read] = true
	}

	for _, read := range interactiveReads {
		if allReads[read] {
			t.Errorf("Duplicate read detected: %s", read)
		}
		allReads[read] = true
	}

	if manager.IsPrevented() {
		t.Error("Manager should not be prevented after test completion")
	}

	t.Logf("Default reads: %v", defaultReads)
	t.Logf("Interactive reads: %v", interactiveReads)
}

// TestStdinManagerCoordinatedReading 测试协调读取机制
func TestStdinManagerCoordinatedReading(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()
	defer manager.Stop()

	// 给协调器一点时间启动
	time.Sleep(50 * time.Millisecond)

	// 创建模拟的stdin数据
	mockStdin := &mockReader{
		data: []byte("line1\nline2\nline3\nline4\n"),
	}

	// 替换原始stdin为mock
	manager.mu.Lock()
	manager.originalStdin = mockStdin
	manager.mu.Unlock()

	// 重新启动协调器以使用新的mock stdin
	manager.Stop()
	time.Sleep(10 * time.Millisecond)
	manager.stopChan = make(chan struct{})
	go manager.startStdinCoordinator()

	var wg sync.WaitGroup
	var reads1, reads2 []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 第一个goroutine使用协调读取器
	wg.Add(1)
	go func() {
		defer wg.Done()

		reader := manager.GetCoordinatedReader()
		buffer := make([]byte, 1024)

		for {
			select {
			case <-ctx.Done():
				t.Log("Reader1 context done")
				return
			default:
				if manager.IsPrevented() {
					time.Sleep(10 * time.Millisecond)
					continue
				}

				t.Log("Reader1 attempting to read...")
				n, err := reader.Read(buffer)
				if err != nil {
					if err != io.EOF {
						t.Logf("Reader1 error: %v", err)
					}
					return
				}

				if n > 0 {
					data := string(buffer[:n])
					mu.Lock()
					reads1 = append(reads1, data)
					mu.Unlock()
					t.Logf("Reader1 read: %q", data)
				} else {
					t.Log("Reader1 read 0 bytes")
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	// 第二个goroutine也使用协调读取器
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(200 * time.Millisecond) // 稍后启动

		// 阻止默认读取
		manager.PreventDefault()
		defer manager.RecoverDefault()

		t.Log("Reader2 started, prevented default reading")

		reader := manager.GetCoordinatedReader()
		buffer := make([]byte, 1024)

		for i := 0; i < 2; i++ {
			t.Logf("Reader2 attempting read %d...", i+1)
			n, err := reader.Read(buffer)
			if err != nil {
				if err != io.EOF {
					t.Logf("Reader2 error: %v", err)
				}
				break
			}

			if n > 0 {
				data := string(buffer[:n])
				mu.Lock()
				reads2 = append(reads2, data)
				mu.Unlock()
				t.Logf("Reader2 read: %q", data)
			} else {
				t.Log("Reader2 read 0 bytes")
			}
			time.Sleep(100 * time.Millisecond)
		}

		t.Log("Reader2 finished")
	}()

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	t.Logf("Reader1 reads: %v", reads1)
	t.Logf("Reader2 reads: %v", reads2)

	// 验证至少有一个reader读到了数据
	if len(reads1) == 0 && len(reads2) == 0 {
		t.Error("At least one reader should have read some data")
	}
}

// TestStdinManagerPracticalUsage 测试实际使用场景
func TestStdinManagerPracticalUsage(t *testing.T) {
	// 重置单例实例
	stdinManagerInstance = nil
	stdinManagerOnce = sync.Once{}

	manager := NewStdinManager()
	defer manager.Stop()

	// 创建模拟的stdin数据，包含多行输入
	mockStdin := &mockReader{
		data: []byte("normal input 1\ninteractive command\nnormal input 2\nmenu selection\nnormal input 3\n"),
	}

	// 替换原始stdin
	time.Sleep(50 * time.Millisecond)
	manager.mu.Lock()
	manager.originalStdin = mockStdin
	manager.mu.Unlock()

	// 重启协调器
	manager.Stop()
	time.Sleep(10 * time.Millisecond)
	manager.stopChan = make(chan struct{})
	go manager.startStdinCoordinator()

	var wg sync.WaitGroup
	var defaultInputs []string
	var interactiveInputs []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 模拟默认的输入处理器（类似SetupSignalHandler）
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 检查是否被其他goroutine阻止
				if manager.IsPrevented() {
					time.Sleep(20 * time.Millisecond)
					continue
				}

				// 使用协调读取一行（只在未被阻止时）
				line, err := manager.ReadLineWhenNotPrevented()
				if err != nil {
					if err != io.EOF {
						t.Logf("Default handler error: %v", err)
					}
					return
				}

				if len(line) > 0 {
					input := string(line)
					mu.Lock()
					defaultInputs = append(defaultInputs, input)
					mu.Unlock()
					t.Logf("Default handler processed: %q", input)
				}

				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	// 模拟交互式菜单或其他需要临时接管stdin的goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(300 * time.Millisecond) // 让默认处理器先运行

		// 阻止默认处理器，开始交互式处理
		manager.PreventDefault()
		defer manager.RecoverDefault()

		t.Log("Interactive mode started - default processing blocked")

		// 读取交互式输入
		for i := 0; i < 2; i++ {
			line, err := manager.ReadLineWithCoordination()
			if err != nil {
				if err != io.EOF {
					t.Logf("Interactive handler error: %v", err)
				}
				break
			}

			if len(line) > 0 {
				input := string(line)
				mu.Lock()
				interactiveInputs = append(interactiveInputs, input)
				mu.Unlock()
				t.Logf("Interactive handler processed: %q", input)
			}

			time.Sleep(100 * time.Millisecond)
		}

		t.Log("Interactive mode ended - default processing restored")
	}()

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	t.Logf("Default inputs: %v", defaultInputs)
	t.Logf("Interactive inputs: %v", interactiveInputs)

	// 验证两种处理器都读取了数据
	if len(defaultInputs) == 0 {
		t.Error("Default handler should have processed some inputs")
	}

	if len(interactiveInputs) == 0 {
		t.Error("Interactive handler should have processed some inputs")
	}

	// 验证没有重复处理（每行只被一个处理器处理）
	allInputs := make(map[string]bool)
	for _, input := range defaultInputs {
		if allInputs[input] {
			t.Errorf("Duplicate processing detected: %q", input)
		}
		allInputs[input] = true
	}

	for _, input := range interactiveInputs {
		if allInputs[input] {
			t.Errorf("Duplicate processing detected: %q", input)
		}
		allInputs[input] = true
	}
}

// readLineFromReader 从reader中读取一行，类似utils.ReadLine
func readLineFromReader(reader io.Reader) ([]byte, error) {
	var line []byte
	buf := make([]byte, 1)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			return line, err
		}

		if n > 0 {
			if buf[0] == '\n' {
				return line, nil
			}
			line = append(line, buf[0])
		}
	}
}
