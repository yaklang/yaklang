package aireactdeps

import (
	"io"
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/log"
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
	originalStdin *os.File
	prevented     bool

	// 分流相关
	divertedReader io.Reader
	divertedPipe   *os.File
	divertedWriter *os.File

	// 活跃的reader管道列表，用于在Prevent时关闭
	activeReaders []*os.File
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
			prevented:     false,
		}
	})
	return stdinManagerInstance
}

// GetDefaultReader 获取默认的reader
func (sm *StdinManager) GetDefaultReader() io.Reader {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.prevented {
		// 如果被阻止，返回空的reader
		return &emptyReader{}
	}

	return sm.originalStdin
}

// PreventDefault 阻止默认的stdin读取，返回可以读取stdin数据的reader
func (sm *StdinManager) PreventDefault() io.Reader {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	log.Info("start to fetch stdin in prevent default stdin manager")
	defer log.Info("end to fetch stdin in prevent default stdin manager")

	if sm.prevented {
		// 如果已经被阻止，返回现有的分流reader
		log.Info("stdin already prevented, returning existing diverted reader")
		return sm.divertedReader
	}

	log.Info("preventing stdin access")

	// 关闭所有活跃的reader管道，强制中断正在阻塞的读取
	log.Info("closing all active reader pipes")
	for _, pipe := range sm.activeReaders {
		if pipe != nil {
			pipe.Close()
		}
	}
	sm.activeReaders = nil

	log.Info("creating pipe for stdin data diversion")
	// 创建管道用于分流stdin数据
	r, w, err := os.Pipe()
	if err != nil {
		log.Info("failed to create pipe, returning original stdin")
		// 如果创建管道失败，返回原始stdin
		return sm.originalStdin
	}

	log.Info("setting up diverted pipe and reader")
	sm.divertedPipe = r
	sm.divertedWriter = w
	sm.divertedReader = r

	log.Info("starting data forwarding goroutine")
	// 启动数据转发goroutine
	go sm.forwardStdinData()

	log.Info("replacing os.Stdin with write end of pipe")
	// 替换os.Stdin为写入端，这样其他读取os.Stdin的代码会被阻塞
	os.Stdin = w
	sm.prevented = true

	return sm.divertedReader
}

// RecoverDefault 恢复默认的stdin读取
func (sm *StdinManager) RecoverDefault() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.prevented {
		return
	}

	// 恢复原始的stdin
	os.Stdin = sm.originalStdin
	sm.prevented = false

	// 关闭分流管道
	if sm.divertedWriter != nil {
		sm.divertedWriter.Close()
		sm.divertedWriter = nil
	}
	if sm.divertedPipe != nil {
		sm.divertedPipe.Close()
		sm.divertedPipe = nil
	}
	sm.divertedReader = nil

	log.Info("stdin access recovered")
}

// forwardStdinData 将原始stdin的数据转发到分流reader
func (sm *StdinManager) forwardStdinData() {
	buffer := make([]byte, 1)
	for {
		n, err := sm.originalStdin.Read(buffer)
		if err != nil {
			break
		}
		if n > 0 {
			sm.divertedWriter.Write(buffer[:n])
		}
	}
}

// forwardToReaderPipe 将原始stdin的数据转发到指定的reader管道
func (sm *StdinManager) forwardToReaderPipe(writer *os.File) {
	defer writer.Close()

	buffer := make([]byte, 1)
	for {
		// 检查是否被阻止
		sm.mu.RLock()
		prevented := sm.prevented
		sm.mu.RUnlock()

		if prevented {
			// 如果被阻止，停止转发
			break
		}

		n, err := sm.originalStdin.Read(buffer)
		if err != nil {
			break
		}
		if n > 0 {
			_, writeErr := writer.Write(buffer[:n])
			if writeErr != nil {
				// 写入失败，可能管道被关闭了
				break
			}
		}
	}
}

// IsPrevented 检查是否被阻止
func (sm *StdinManager) IsPrevented() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.prevented
}

// emptyReader 空的reader，总是返回EOF
type emptyReader struct{}

func (e *emptyReader) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

// RegisterReader 注册一个背景reader，返回一个基于管道的控制器
func (sm *StdinManager) RegisterReader() *ReaderController {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 创建一个管道用于该reader
	r, w, err := os.Pipe()
	if err != nil {
		log.Errorf("Failed to create pipe for reader: %v", err)
		// 如果创建管道失败，返回一个使用原始stdin的控制器
		return &ReaderController{
			manager:    sm,
			readerPipe: sm.originalStdin,
		}
	}

	// 如果当前已经被阻止，直接返回一个被阻止的控制器
	if sm.prevented {
		r.Close()
		w.Close()
		return &ReaderController{
			manager:    sm,
			readerPipe: nil, // nil表示被阻止
		}
	}

	// 添加到活跃reader列表
	sm.activeReaders = append(sm.activeReaders, r)

	// 启动数据转发goroutine
	go sm.forwardToReaderPipe(w)

	return &ReaderController{
		manager:    sm,
		readerPipe: r,
	}
}

// ReaderController 背景reader的同步控制器（基于管道）
type ReaderController struct {
	manager    *StdinManager
	readerPipe *os.File // 该reader专用的管道
}

// WaitForSignals 检查是否应该继续处理数据
func (rc *ReaderController) WaitForSignals() bool {
	rc.manager.mu.RLock()
	prevented := rc.manager.prevented
	rc.manager.mu.RUnlock()

	// 如果被阻止，返回false表示应该暂停
	return !prevented
}

// ReadLine 从专用管道逐字节读取一行数据
func (rc *ReaderController) ReadLine() ([]byte, error) {
	// 如果管道为nil，说明被阻止了
	if rc.readerPipe == nil {
		return nil, io.EOF
	}

	var line []byte
	buffer := make([]byte, 1)

	for {
		n, err := rc.readerPipe.Read(buffer)
		if err != nil {
			// 如果是管道关闭错误，静默返回EOF，避免大量错误日志
			if err == io.EOF || err.Error() == "file already closed" {
				return nil, io.EOF
			}
			return nil, err
		}

		if n > 0 {
			// 如果遇到换行符，结束读取
			if buffer[0] == '\n' {
				break
			}
			// 跳过回车符
			if buffer[0] != '\r' {
				line = append(line, buffer[0])
			}
		}
	}

	return line, nil
}

// Close 关闭操作（简化版无需操作）
func (rc *ReaderController) Close() error {
	return nil
}

// Unregister 注销reader（简化版无需操作）
func (rc *ReaderController) Unregister() {
	// 简化版无需操作
}

// Reactivate 重新激活被阻止的ReaderController（在RecoverDefault后调用）
func (rc *ReaderController) Reactivate() error {
	rc.manager.mu.Lock()
	defer rc.manager.mu.Unlock()

	// 如果已经有活跃的管道，无需重新激活
	if rc.readerPipe != nil {
		return nil
	}

	// 如果仍然被阻止，无法重新激活
	if rc.manager.prevented {
		return nil
	}

	// 创建新的管道
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}

	// 设置新的管道
	rc.readerPipe = r
	rc.manager.activeReaders = append(rc.manager.activeReaders, r)

	// 启动数据转发goroutine
	go rc.manager.forwardToReaderPipe(w)

	return nil
}
