package aireactdeps

import (
	"io"
	"os"
	"sync"
)

/*
TDD

manager := NewStdinManager()

defaultReader := manager.GetDefaultReader()
go func(){
	newLine := utils.ReadLine(defaultReader)
	... handle it
}()

manager.PreventDefault()
defer manager.RecoverDefault()

// use stdin as normal ...
*/

// StdinManager 管理stdin的读取，支持暂时阻止默认stdin读取
type StdinManager struct {
	mu            sync.RWMutex
	originalStdin io.Reader
	defaultReader io.Reader
	prevented     bool

	// 新增字段用于协调stdin读取
	stdinChan chan []byte
	errorChan chan error
	stopChan  chan struct{}
}

var (
	stdinManagerInstance *StdinManager
	stdinManagerOnce     sync.Once
)

// NewStdinManager 返回StdinManager的单例实例
func NewStdinManager() *StdinManager {
	stdinManagerOnce.Do(func() {
		stdinManagerInstance = &StdinManager{
			originalStdin: os.Stdin,
			defaultReader: os.Stdin,
			prevented:     false,
			stdinChan:     make(chan []byte, 10),
			errorChan:     make(chan error, 10),
			stopChan:      make(chan struct{}),
		}
		// 启动stdin读取协调器
		go stdinManagerInstance.startStdinCoordinator()
	})
	return stdinManagerInstance
}

// GetDefaultReader 获取默认的reader
func (sm *StdinManager) GetDefaultReader() io.Reader {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.defaultReader
}

// ReadLineWithCoordination 协调读取一行，类似utils.ReadLine但支持协调
func (sm *StdinManager) ReadLineWithCoordination() ([]byte, error) {
	return sm.readLineFromCoordinator()
}

// ReadLineWhenNotPrevented 只在未被阻止时读取一行
func (sm *StdinManager) ReadLineWhenNotPrevented() ([]byte, error) {
	// 如果被阻止，返回错误
	if sm.IsPrevented() {
		return nil, io.EOF
	}

	return sm.readLineFromCoordinator()
}

// readLineFromCoordinator 从协调器读取一行
func (sm *StdinManager) readLineFromCoordinator() ([]byte, error) {
	var line []byte

	for {
		select {
		case data := <-sm.stdinChan:
			for _, b := range data {
				if b == '\n' {
					return line, nil
				}
				line = append(line, b)
			}
		case err := <-sm.errorChan:
			return line, err
		}
	}
}

// PreventDefault 阻止默认的stdin读取
func (sm *StdinManager) PreventDefault() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if !sm.prevented {
		sm.prevented = true
		// 这里可以根据需要实现具体的阻止逻辑
		// 比如替换os.Stdin为一个空的reader
	}
}

// RecoverDefault 恢复默认的stdin读取
func (sm *StdinManager) RecoverDefault() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.prevented {
		sm.prevented = false
		// 恢复原始的stdin（如果是*os.File类型）
		if file, ok := sm.originalStdin.(*os.File); ok {
			os.Stdin = file
		}
	}
}

// IsPrevented 检查是否已经阻止了默认读取
func (sm *StdinManager) IsPrevented() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.prevented
}

// startStdinCoordinator 启动stdin读取协调器
func (sm *StdinManager) startStdinCoordinator() {
	buffer := make([]byte, 1024)
	for {
		select {
		case <-sm.stopChan:
			return
		default:
			// 从原始stdin读取数据
			n, err := sm.originalStdin.Read(buffer)
			if err != nil {
				select {
				case sm.errorChan <- err:
				case <-sm.stopChan:
					return
				}
				continue
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buffer[:n])
				select {
				case sm.stdinChan <- data:
				case <-sm.stopChan:
					return
				}
			}
		}
	}
}

// CoordinatedReader 协调读取器，避免多个goroutine争抢stdin
type CoordinatedReader struct {
	manager *StdinManager
	buffer  []byte
	pos     int
}

// newCoordinatedReader 创建协调读取器
func (sm *StdinManager) newCoordinatedReader() *CoordinatedReader {
	return &CoordinatedReader{
		manager: sm,
		buffer:  make([]byte, 0),
		pos:     0,
	}
}

// Read 实现io.Reader接口
func (cr *CoordinatedReader) Read(p []byte) (n int, err error) {
	// 如果缓冲区中有数据，先返回缓冲区数据
	if cr.pos < len(cr.buffer) {
		n = copy(p, cr.buffer[cr.pos:])
		cr.pos += n
		return n, nil
	}

	// 从协调器获取新数据
	select {
	case data := <-cr.manager.stdinChan:
		cr.buffer = append(cr.buffer[:0], data...)
		cr.pos = 0
		n = copy(p, cr.buffer)
		cr.pos += n
		return n, nil
	case err := <-cr.manager.errorChan:
		return 0, err
	}
}

// GetCoordinatedReader 获取协调读取器
func (sm *StdinManager) GetCoordinatedReader() io.Reader {
	return sm.newCoordinatedReader()
}

// Stop 停止stdin协调器
func (sm *StdinManager) Stop() {
	close(sm.stopChan)
}
