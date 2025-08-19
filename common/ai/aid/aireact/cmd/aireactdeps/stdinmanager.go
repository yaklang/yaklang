package aireactdeps

import (
	"bufio"
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
	originalStdin *os.File
	prevented     bool

	// 分流相关
	divertedReader io.Reader
	divertedPipe   *os.File
	divertedWriter *os.File

	// 同步信号
	paused        bool                   // 是否暂停状态
	pauseCond     *sync.Cond             // 暂停条件变量
	activeReaders int                    // 活跃的reader数量
	bufioReaders  []*ClosableBufioReader // 所有活跃的bufio readers
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
			paused:        false,
		}
		stdinManagerInstance.pauseCond = sync.NewCond(&stdinManagerInstance.mu)
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

	if sm.prevented {
		// 如果已经被阻止，返回现有的分流reader
		return sm.divertedReader
	}

	// 暂停所有活跃的reader并立即替换它们的stdin源
	if sm.activeReaders > 0 {
		sm.paused = true
		// 创建一个立即返回EOF的空文件作为新的stdin源
		emptyPipe, _, _ := os.Pipe()
		emptyPipe.Close() // 立即关闭，使其返回EOF

		// 强制替换所有bufio readers的源为空管道
		sm.replaceAllBufioReadersSource(emptyPipe)
	}

	// 创建管道用于分流stdin数据
	r, w, err := os.Pipe()
	if err != nil {
		// 如果创建管道失败，返回原始stdin
		return sm.originalStdin
	}

	sm.divertedPipe = r
	sm.divertedWriter = w
	sm.divertedReader = r

	// 启动数据转发goroutine
	go sm.forwardStdinData()

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

	// 恢复所有活跃的reader
	if sm.activeReaders > 0 {
		sm.paused = false
		// 重新创建所有bufio readers
		sm.recreateAllBufioReaders()
		sm.pauseCond.Broadcast() // 唤醒所有等待的reader
	}
}

// forwardStdinData 将原始stdin的数据转发到分流reader
func (sm *StdinManager) forwardStdinData() {
	buffer := make([]byte, 1)
	for {
		sm.mu.RLock()
		if !sm.prevented || sm.divertedWriter == nil {
			sm.mu.RUnlock()
			return
		}
		writer := sm.divertedWriter
		sm.mu.RUnlock()

		n, err := sm.originalStdin.Read(buffer)
		if err != nil {
			return
		}

		if n > 0 {
			_, writeErr := writer.Write(buffer[:n])
			if writeErr != nil {
				return
			}
		}
	}
}

// IsPrevented 检查是否已经阻止了默认读取
func (sm *StdinManager) IsPrevented() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.prevented
}

// emptyReader 空读取器，用于在被阻止时返回EOF
type emptyReader struct{}

func (er *emptyReader) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

// ClosableBufioReader 可关闭的bufio Reader
type ClosableBufioReader struct {
	reader   *bufio.Reader
	closer   io.Closer
	closed   bool
	closedMu sync.RWMutex
}

// NewClosableBufioReader 创建可关闭的bufio Reader
func NewClosableBufioReader(r io.ReadCloser) *ClosableBufioReader {
	return &ClosableBufioReader{
		reader: bufio.NewReader(r),
		closer: r,
		closed: false,
	}
}

// NewClosableBufioReaderFromFile 从*os.File创建可关闭的bufio Reader
func NewClosableBufioReaderFromFile(f *os.File) *ClosableBufioReader {
	return &ClosableBufioReader{
		reader: bufio.NewReader(f),
		closer: nil, // 不关闭原始文件
		closed: false,
	}
}

// ReadLine 读取一行，支持主动关闭中断
func (cbr *ClosableBufioReader) ReadLine() ([]byte, error) {
	cbr.closedMu.RLock()
	if cbr.closed {
		cbr.closedMu.RUnlock()
		return nil, io.EOF
	}
	cbr.closedMu.RUnlock()

	return cbr.reader.ReadBytes('\n')
}

// Close 关闭reader，中断正在进行的读取操作
func (cbr *ClosableBufioReader) Close() error {
	cbr.closedMu.Lock()
	defer cbr.closedMu.Unlock()

	if cbr.closed {
		return nil
	}

	cbr.closed = true
	if cbr.closer != nil {
		return cbr.closer.Close()
	}
	return nil
}

// IsClosed 检查是否已关闭
func (cbr *ClosableBufioReader) IsClosed() bool {
	cbr.closedMu.RLock()
	defer cbr.closedMu.RUnlock()
	return cbr.closed
}

// RegisterReader 注册一个背景reader，返回一个同步控制器
func (sm *StdinManager) RegisterReader() *ReaderController {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.activeReaders++

	// 创建可关闭的bufio reader
	bufioReader := NewClosableBufioReaderFromFile(sm.originalStdin)

	// 添加到跟踪列表
	sm.bufioReaders = append(sm.bufioReaders, bufioReader)

	return &ReaderController{
		manager:     sm,
		bufioReader: bufioReader,
	}
}

// ReaderController 背景reader的同步控制器
type ReaderController struct {
	manager     *StdinManager
	bufioReader *ClosableBufioReader
}

// WaitForSignals 等待暂停/恢复信号，返回true表示应该继续，false表示应该暂停
func (rc *ReaderController) WaitForSignals() bool {
	rc.manager.pauseCond.L.Lock()
	defer rc.manager.pauseCond.L.Unlock()

	// 如果被暂停，等待恢复信号
	for rc.manager.paused {
		rc.manager.pauseCond.Wait()
	}

	return true
}

// ReadLine 使用bufio读取一行，支持主动关闭中断（包含强制同步检查）
func (rc *ReaderController) ReadLine() ([]byte, error) {
	// 强制检查是否被暂停，如果被暂停立即返回EOF
	rc.manager.pauseCond.L.Lock()
	if rc.manager.paused {
		rc.manager.pauseCond.L.Unlock()
		return nil, io.EOF
	}
	rc.manager.pauseCond.L.Unlock()

	// 使用bufio读取
	line, err := rc.bufioReader.ReadLine()
	if err != nil {
		return nil, err
	}

	// 去掉换行符
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}

	return line, nil
}

// ReadLineWithSync 使用bufio读取一行，包含同步检查
func (rc *ReaderController) ReadLineWithSync() ([]byte, error) {
	// 检查是否被暂停
	rc.manager.pauseCond.L.Lock()
	for rc.manager.paused {
		rc.manager.pauseCond.Wait()
	}
	rc.manager.pauseCond.L.Unlock()

	return rc.ReadLine()
}

// Close 关闭bufio reader，中断正在进行的读取
func (rc *ReaderController) Close() error {
	if rc.bufioReader != nil {
		return rc.bufioReader.Close()
	}
	return nil
}

// closeAllBufioReaders 关闭所有活跃的bufio readers
func (sm *StdinManager) closeAllBufioReaders() {
	for _, reader := range sm.bufioReaders {
		if reader != nil {
			reader.Close()
		}
	}
}

// replaceAllBufioReadersSource 替换所有bufio readers的数据源
func (sm *StdinManager) replaceAllBufioReadersSource(newSource *os.File) {
	for _, reader := range sm.bufioReaders {
		if reader != nil {
			// 强制设置新的数据源
			reader.reader = bufio.NewReader(newSource)
		}
	}
}

// recreateAllBufioReaders 重新创建所有bufio readers
func (sm *StdinManager) recreateAllBufioReaders() {
	for i, reader := range sm.bufioReaders {
		if reader != nil {
			// 关闭旧的reader
			reader.Close()
			// 创建新的reader
			sm.bufioReaders[i] = NewClosableBufioReaderFromFile(sm.originalStdin)
		}
	}
}

// Unregister 注销reader
func (rc *ReaderController) Unregister() {
	rc.manager.mu.Lock()
	defer rc.manager.mu.Unlock()
	rc.manager.activeReaders--

	// 从跟踪列表中移除
	for i, reader := range rc.manager.bufioReaders {
		if reader == rc.bufioReader {
			rc.manager.bufioReaders = append(rc.manager.bufioReaders[:i], rc.manager.bufioReaders[i+1:]...)
			break
		}
	}

	// 关闭bufio reader
	if rc.bufioReader != nil {
		rc.bufioReader.Close()
	}
}
