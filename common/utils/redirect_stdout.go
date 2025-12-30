package utils

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
)

var (
	isInAttached         = NewBool(false)
	isInCached           = NewBool(false)
	attachOutputCallback = new(sync.Map)
	cachedLog            *CircularQueue
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

// safeOutputMirror reads from a pipe and writes to both the original file and a callback.
// It guarantees that:
// 1. Output is ALWAYS written to the original file first (screen output never lost)
// 2. Callback panics are recovered and don't affect output
// 3. Clean shutdown on context cancellation
type safeOutputMirror struct {
	name       string       // "stdout" or "stderr" for debugging
	original   *os.File     // Original stdout/stderr to write to
	pipeReader *os.File     // Read end of the pipe
	pipeWriter *os.File     // Write end of the pipe (assigned to os.Stdout/Stderr)
	callback   func(string) // Callback to receive mirrored output
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stopped    bool
	mu         sync.Mutex
}

func newSafeOutputMirror(ctx context.Context, name string, original *os.File, callback func(string)) (*safeOutputMirror, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, Errorf("failed to create %s pipe: %v", name, err)
	}

	ctx, cancel := context.WithCancel(ctx)
	m := &safeOutputMirror{
		name:       name,
		original:   original,
		pipeReader: reader,
		pipeWriter: writer,
		callback:   callback,
		ctx:        ctx,
		cancel:     cancel,
	}

	m.start()
	return m, nil
}

func (m *safeOutputMirror) start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				// Write to original on panic - this should never happen but just in case
				m.original.Write([]byte(fmt.Sprintf("[%s-mirror] recovered from panic: %v\n", m.name, r)))
			}
		}()

		scanner := bufio.NewScanner(m.pipeReader)
		// Use a larger buffer to handle long lines
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024) // Max 1MB per line

		// Simple loop: scanner.Scan() will return false when pipeReader is closed by Stop()
		// We rely on Stop() to close the pipe rather than checking context in the loop,
		// because scanner.Scan() is blocking and won't respond to context cancellation.
		for scanner.Scan() {
			line := scanner.Text()
			m.processLine(line)
		}

		if err := scanner.Err(); err != nil {
			// Only log if not caused by pipe close
			errMsg := err.Error()
			if !strings.Contains(errMsg, "file already closed") &&
				!strings.Contains(errMsg, "bad file descriptor") {
				m.original.Write([]byte(fmt.Sprintf("[%s-mirror] scanner error: %v\n", m.name, err)))
			}
		}
	}()
}

func (m *safeOutputMirror) processLine(line string) {
	// ALWAYS write to original first - this is the safety guarantee
	// Even if callback fails, output is never lost
	m.original.Write([]byte(line + "\n"))

	// Then call callback safely
	if m.callback != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Log panic to original, not to the mirrored stream to avoid recursion
					m.original.Write([]byte(fmt.Sprintf("[%s-mirror] callback panic: %v\n", m.name, r)))
				}
			}()
			m.callback(line)
		}()
	}
}

func (m *safeOutputMirror) Writer() *os.File {
	return m.pipeWriter
}

func (m *safeOutputMirror) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	m.mu.Unlock()

	// Cancel context to signal goroutine to stop
	m.cancel()

	// Close writer first to signal EOF to reader
	m.pipeWriter.Close()

	// Wait for goroutine to finish
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Goroutine finished normally after pipeWriter closed
	case <-time.After(500 * time.Millisecond):
		// If goroutine hasn't finished after 500ms, force close reader
		// This ensures scanner.Scan() returns immediately
		m.pipeReader.Close()
		select {
		case <-done:
		case <-time.After(2500 * time.Millisecond):
			// Should never happen, but just in case
		}
	}
}

// HandleStdout mirrors stdout/stderr to a callback function.
//
// SAFETY GUARANTEES:
// 1. All output is ALWAYS printed to the original stdout/stderr first
// 2. Callback panics are recovered and don't affect output
// 3. Original stdout/stderr are always restored on exit (normal or panic)
// 4. Context cancellation properly cleans up all resources
//
// This function replaces os.Stdout and os.Stderr with pipe writers,
// and starts goroutines that read from those pipes, write to the
// original stdout/stderr, and call the callback function.
func HandleStdout(ctx context.Context, handle func(string)) error {
	// If already attached, just register a new callback
	if isInAttached.IsSet() {
		id := uuid.New().String()
		attachOutputCallback.Store(id, func(result string) {
			defer func() {
				if err := recover(); err != nil {
					// Silently recover
				}
			}()
			handle(result)
		})
		select {
		case <-ctx.Done():
			attachOutputCallback.Delete(id)
			return nil
		}
	} else {
		isInAttached.Set()
	}

	// Save original stdout/stderr - these will ALWAYS be used for output
	originStdout := os.Stdout
	originStderr := os.Stderr

	// Create a unified callback that also notifies all attached callbacks
	sendOutput := func(result string) {
		// Call the main handler
		func() {
			defer func() {
				if r := recover(); r != nil {
					originStdout.Write([]byte(fmt.Sprintf("[redirect_stdout] handle panic: %v\n", r)))
				}
			}()
			handle(result)
		}()

		// Call all attached callbacks
		attachOutputCallback.Range(func(key, value any) bool {
			if va, ok := value.(func(result string)); ok && va != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Silently recover from callback panic
						}
					}()
					va(result)
				}()
			}
			return true
		})
	}

	// Create safe mirrors for stdout and stderr
	stdoutMirror, err := newSafeOutputMirror(ctx, "stdout", originStdout, sendOutput)
	if err != nil {
		isInAttached.UnSet()
		return err
	}

	stderrMirror, err := newSafeOutputMirror(ctx, "stderr", originStderr, sendOutput)
	if err != nil {
		stdoutMirror.Stop()
		isInAttached.UnSet()
		return err
	}

	// Cleanup function - ALWAYS called via defer
	cleanup := func() {
		// CRITICAL: Restore original stdout/stderr FIRST before closing pipes.
		// This ensures that any goroutines still writing to os.Stdout/Stderr
		// will write to the original files instead of blocked pipes.
		os.Stdout = originStdout
		os.Stderr = originStderr
		log.SetOutput(originStdout)

		// Now it's safe to stop mirrors and close pipes
		// No more writes to the pipes since stdout/stderr are restored
		stdoutMirror.Stop()
		stderrMirror.Stop()

		// Unset attached flag
		isInAttached.UnSet()
	}

	// Ensure cleanup is ALWAYS called
	defer func() {
		if r := recover(); r != nil {
			// Restore on panic
			cleanup()
			originStdout.Write([]byte(fmt.Sprintf("[redirect_stdout] recovered from panic: %v\n", r)))
		} else {
			cleanup()
		}
	}()

	// Replace stdout/stderr with pipe writers
	os.Stdout = stdoutMirror.Writer()
	os.Stderr = stderrMirror.Writer()

	// Set log output to our stdout mirror
	log.SetOutput(stdoutMirror.Writer())
	log.DefaultLogger.Printer.IsTerminal = true

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}
