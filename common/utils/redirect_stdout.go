package utils

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
)

var (
	// Global state for stdout/stderr mirroring
	mirrorMu             sync.Mutex
	mirrorRefCount       int32 // atomic reference count
	globalStdoutMirror   *fanoutMirror
	globalStderrMirror   *fanoutMirror
	originStdout         *os.File
	originStderr         *os.File
	isInCached           = NewBool(false)
	cachedLog            *CircularQueue
	attachOutputCallback = new(sync.Map)
)

func GetCachedLog() (res []string) {
	if cachedLog == nil {
		return nil
	}
	for _, e := range cachedLog.GetElements() {
		res = append(res, e.(string))
	}
	return
}

func StartCacheLog(ctx context.Context, n int) {
	cachedLog = NewCircularQueue(n)
	if isInCached.IsSet() {
		return
	}
	isInCached.Set()
	go func() {
		if err := HandleStdout(ctx, func(s string) {
			cachedLog.Push(s)
		}); err != nil {
			log.Error(err)
		}
		isInCached.UnSet()
	}()
}

func HandleStdoutBackgroundForTest(handle func(string)) (func(), func(), error) {
	ctx := context.Background()
	var l int32 = 0xffff - 0xfe00
	n := rand.Int31n(l)
	msg := string([]rune{n + 0xfe00})
	endCh := make(chan struct{})
	endFlagMsg := fmt.Sprintf("%s", msg)
	sendEndMsg := func() {
		println(endFlagMsg)
	}
	checkEndMsg := func(s string) {
		if strings.Contains(s, msg) {
			select {
			case endCh <- struct{}{}:
			default:
			}
		}
	}
	waitEnd := func() {
		select {
		case <-endCh:
		case <-time.After(time.Second * 3):
		}
	}
	startCh := make(chan struct{})
	once := sync.Once{}
	var err error
	go func() {
		err = HandleStdout(ctx, func(s string) {
			once.Do(func() {
				select {
				case startCh <- struct{}{}:
				default:
				}
			})
			handle(s)
			checkEndMsg(s)
		})
		once.Do(func() {
			select {
			case startCh <- struct{}{}:
			default:
			}
		})
	}()
	for i := 0; i < 10; i++ {
		select {
		case <-startCh:
			return sendEndMsg, waitEnd, err
		case <-time.After(100 * time.Millisecond):
			fmt.Println("waiting for mirror stdout start signal...")
		}
	}
	return nil, nil, Errorf("wait for mirror stdout start signal timeout")
}

// fanoutMirror reads from a pipe and fans out to multiple callbacks.
// It uses reference counting to manage lifecycle - only stops when all subscribers are gone.
type fanoutMirror struct {
	name       string   // "stdout" or "stderr" for debugging
	original   *os.File // Original stdout/stderr to write to
	pipeReader *os.File // Read end of the pipe
	pipeWriter *os.File // Write end of the pipe (assigned to os.Stdout/Stderr)
	wg         sync.WaitGroup
	stopped    bool
	mu         sync.Mutex
}

func newFanoutMirror(name string, original *os.File) (*fanoutMirror, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, Errorf("failed to create %s pipe: %v", name, err)
	}

	m := &fanoutMirror{
		name:       name,
		original:   original,
		pipeReader: reader,
		pipeWriter: writer,
	}

	m.start()
	return m, nil
}

func (m *fanoutMirror) start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				m.original.Write([]byte(fmt.Sprintf("[%s-mirror] recovered from panic: %v\n", m.name, r)))
			}
		}()

		scanner := bufio.NewScanner(m.pipeReader)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			m.processLine(line)
		}

		if err := scanner.Err(); err != nil {
			errMsg := err.Error()
			if !strings.Contains(errMsg, "file already closed") &&
				!strings.Contains(errMsg, "bad file descriptor") {
				m.original.Write([]byte(fmt.Sprintf("[%s-mirror] scanner error: %v\n", m.name, err)))
			}
		}
	}()
}

func (m *fanoutMirror) processLine(line string) {
	// ALWAYS write to original first
	m.original.Write([]byte(line + "\n"))

	// Fan out to all registered callbacks
	attachOutputCallback.Range(func(key, value any) bool {
		if callback, ok := value.(func(string)); ok && callback != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Silently recover from callback panic
					}
				}()
				callback(line)
			}()
		}
		return true
	})
}

func (m *fanoutMirror) Writer() *os.File {
	return m.pipeWriter
}

func (m *fanoutMirror) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	m.mu.Unlock()

	m.pipeWriter.Close()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		m.pipeReader.Close()
		select {
		case <-done:
		case <-time.After(2500 * time.Millisecond):
		}
	}
}

// initMirrors initializes global mirrors if not already done.
// Returns true if this call initialized the mirrors (first caller).
func initMirrors() (bool, error) {
	mirrorMu.Lock()
	defer mirrorMu.Unlock()

	newCount := atomic.AddInt32(&mirrorRefCount, 1)
	if newCount > 1 {
		// Already initialized by another caller
		return false, nil
	}

	// First caller - save originals and create mirrors
	originStdout = os.Stdout
	originStderr = os.Stderr

	var err error
	globalStdoutMirror, err = newFanoutMirror("stdout", originStdout)
	if err != nil {
		atomic.AddInt32(&mirrorRefCount, -1)
		return false, err
	}

	globalStderrMirror, err = newFanoutMirror("stderr", originStderr)
	if err != nil {
		globalStdoutMirror.Stop()
		globalStdoutMirror = nil
		atomic.AddInt32(&mirrorRefCount, -1)
		return false, err
	}

	// Replace stdout/stderr with pipe writers
	os.Stdout = globalStdoutMirror.Writer()
	os.Stderr = globalStderrMirror.Writer()
	log.SetOutput(globalStdoutMirror.Writer())
	log.DefaultLogger.Printer.IsTerminal = true

	return true, nil
}

// releaseMirrors decrements reference count and cleans up if last caller.
func releaseMirrors() {
	mirrorMu.Lock()
	defer mirrorMu.Unlock()

	newCount := atomic.AddInt32(&mirrorRefCount, -1)
	if newCount > 0 {
		// Other callers still active
		return
	}

	// Last caller - restore originals and stop mirrors
	if originStdout != nil {
		os.Stdout = originStdout
		os.Stderr = originStderr
		log.SetOutput(originStdout)
	}

	if globalStdoutMirror != nil {
		globalStdoutMirror.Stop()
		globalStdoutMirror = nil
	}
	if globalStderrMirror != nil {
		globalStderrMirror.Stop()
		globalStderrMirror = nil
	}
}

// HandleStdout mirrors stdout/stderr to a callback function.
//
// SAFETY GUARANTEES:
// 1. All output is ALWAYS printed to the original stdout/stderr first
// 2. Callback panics are recovered and don't affect output
// 3. Uses reference counting - only restores when ALL callers are done
// 4. Multiple concurrent callers are supported (fanout mode)
//
// This function replaces os.Stdout and os.Stderr with pipe writers,
// and fans out output to all registered callbacks.
func HandleStdout(ctx context.Context, handle func(string)) error {
	// Register callback with unique ID
	callbackID := uuid.New().String()
	attachOutputCallback.Store(callbackID, func(result string) {
		defer func() {
			if err := recover(); err != nil {
				// Silently recover
			}
		}()
		handle(result)
	})

	// Initialize mirrors (or increment ref count)
	_, err := initMirrors()
	if err != nil {
		attachOutputCallback.Delete(callbackID)
		return err
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Unregister callback
	attachOutputCallback.Delete(callbackID)

	// Release mirrors (decrement ref count, cleanup if last)
	releaseMirrors()

	return nil
}
