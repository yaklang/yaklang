package log

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kataras/golog"
	"github.com/kataras/pio"
	"github.com/stretchr/testify/require"
)

func newOutputTestLogger(w io.Writer) *Logger {
	logger := &Logger{Logger: newGologLogger()}
	logger.output.target.Store(newLogOutputTarget(w))
	logger.Printer.SetOutput(&logger.output)
	logger.Printer.Hijack(logger.output.hijack)
	logger.SetTimeFormat("")
	logger.SetLevel("debug")
	return logger
}

type outputSwitchWriter struct {
	bytes.Buffer
	switchOutput func()
}

func (w *outputSwitchWriter) Write(data []byte) (int, error) {
	if w.switchOutput != nil {
		switchOutput := w.switchOutput
		w.switchOutput = nil
		switchOutput()
	}
	return w.Buffer.Write(data)
}

func TestLoggerOutputRecordKeepsDestination(t *testing.T) {
	var first outputSwitchWriter
	var second bytes.Buffer
	logger := newOutputTestLogger(&first)
	first.switchOutput = func() { logger.SetOutput(&second) }
	logger.Println("first")
	require.Equal(t, "first\n", first.String())
	require.Empty(t, second.String())
	logger.Println("second")
	require.Equal(t, "second\n", second.String())

	// Ordinary writes reach the output. Like golog's original Printer, an
	// untyped value has no marshaler and is skipped.
	n, err := logger.Printer.Output.Write([]byte("raw"))
	require.NoError(t, err)
	require.Equal(t, 3, n)
	_, err = logger.Printer.Output.Write(pio.NewLine)
	require.NoError(t, err)
	_, err = logger.Printer.Println("plain")
	require.ErrorIs(t, err, pio.ErrSkipped)
	require.Equal(t, "second\nraw\n", second.String())
}

type outputValueWriter struct {
	value  any
	output *bytes.Buffer
}

func (w outputValueWriter) Write(data []byte) (int, error) { return w.output.Write(data) }

func TestLoggerOutputAcceptsNonComparableWriter(t *testing.T) {
	var output bytes.Buffer
	// The struct type is comparable, but its dynamic interface member is not.
	logger := newOutputTestLogger(outputValueWriter{value: []byte("value"), output: &output})
	logger.Println("value writer")
	require.Equal(t, "value writer\n", output.String())
}

type outputBlockingWriter struct {
	bytes.Buffer
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *outputBlockingWriter) Write(data []byte) (int, error) {
	w.once.Do(func() {
		close(w.entered)
		<-w.release
	})
	return w.Buffer.Write(data)
}

func TestLoggerOutputKeepsConcurrentRawWrite(t *testing.T) {
	writer := &outputBlockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	logger := newOutputTestLogger(writer)
	printed := make(chan struct{})
	go func() {
		logger.Println("record")
		close(printed)
	}()
	<-writer.entered
	started := make(chan struct{})
	written := make(chan error, 1)
	go func() {
		close(started)
		_, err := logger.Printer.Output.Write([]byte("raw"))
		written <- err
	}()
	<-started
	close(writer.release)
	select {
	case <-printed:
	case <-time.After(3 * time.Second):
		t.Fatal("record could not complete")
	}
	select {
	case err := <-written:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("ordinary Output.Write could not complete")
	}
	require.Equal(t, "record\nraw", writer.String())
}

func TestLoggerOutputPreservesPrintHandlers(t *testing.T) {
	var output bytes.Buffer
	logger := newOutputTestLogger(&output)
	require.Same(t, logger, logger.SetLevel("info").SetOutput(&output))
	logger.SetPrefix("prefix ")
	logger.NewLine = false
	var lines []bool
	logger.Handle(func(entry *golog.Log) bool {
		lines = append(lines, entry.NewLine)
		return false
	})
	var results []pio.PrintResult
	logger.Printer.Handle(func(result pio.PrintResult) {
		result.Contents = append([]byte(nil), result.Contents...)
		results = append(results, result)
	})
	logger.Print("one")
	logger.Println("two")
	logger.Info("three")
	logger.NewLine = true
	logger.Info("four")
	level := golog.GetTextForLevel(InfoLevel, false) + " "
	expected := []string{"prefix one", "prefix two", "prefix " + level + "three", "prefix " + level + "four"}
	require.Equal(t, []bool{false, true, false, true}, lines)
	require.Len(t, results, len(expected))
	for index, text := range expected {
		require.NoError(t, results[index].Error)
		require.Equal(t, len(text), results[index].Written)
		require.Equal(t, text, string(results[index].Contents), "PrintResult must exclude the newline")
	}
	require.Equal(t, expected[0]+expected[1]+"\n"+expected[2]+expected[3]+"\n", output.String())

	logger.Handle(func(entry *golog.Log) bool { return entry.Message == "hidden" })
	logger.Println("hidden")
	require.Len(t, results, len(expected), "handled messages must not reach Printer handlers")
}

type outputFailureWriter struct {
	bytes.Buffer
	calls   int
	errorAt int
	panicAt int
	shortAt int
	err     error
}

func (w *outputFailureWriter) Write(data []byte) (int, error) {
	w.calls++
	if w.calls == w.panicAt {
		panic("output panic")
	}
	if w.calls == w.errorAt {
		return 0, w.err
	}
	if w.calls == w.shortAt && len(data) > 0 {
		return w.Buffer.Write(data[:len(data)-1])
	}
	return w.Buffer.Write(data)
}

func TestLoggerOutputReleasesFailedRecord(t *testing.T) {
	for _, testcase := range []struct {
		name    string
		errorAt int
		panicAt int
		shortAt int
	}{
		{name: "body error", errorAt: 1},
		{name: "newline error", errorAt: 2},
		{name: "body panic", panicAt: 1},
		{name: "newline panic", panicAt: 2},
		{name: "short body", shortAt: 1},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			failure := errors.New("output failure")
			writer := &outputFailureWriter{errorAt: testcase.errorAt, panicAt: testcase.panicAt, shortAt: testcase.shortAt, err: failure}
			logger := newOutputTestLogger(writer)
			var results []pio.PrintResult
			logger.Printer.Handle(func(result pio.PrintResult) { results = append(results, result) })
			write := func() {
				_, err := logger.Printer.Println(&golog.Log{Logger: logger.Logger, Level: InfoLevel, Message: "first", NewLine: true})
				if testcase.errorAt == 1 {
					require.ErrorIs(t, err, failure)
				} else {
					require.NoError(t, err, "pio intentionally ignores newline errors and short write counts")
				}
			}
			if testcase.panicAt != 0 {
				require.PanicsWithValue(t, "output panic", write)
				require.Empty(t, results)
			} else {
				write()
				require.Len(t, results, 1)
				if testcase.errorAt == 1 {
					require.ErrorIs(t, results[0].Error, failure)
					require.Equal(t, -1, results[0].Written)
				} else {
					require.NoError(t, results[0].Error)
					require.Equal(t, len(golog.GetTextForLevel(InfoLevel, false)+" first"), results[0].Written)
				}
			}
			completed := make(chan struct{})
			go func() {
				logger.Println("after")
				close(completed)
			}()
			select {
			case <-completed:
			case <-time.After(3 * time.Second):
				t.Fatal("failed record retained its writer lock")
			}
			require.True(t, strings.HasSuffix(writer.String(), "after\n"))
		})
	}
}

type outputYieldWriter struct{ bytes.Buffer }

func (w *outputYieldWriter) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	runtime.Gosched()
	return n, err
}

func TestLoggerOutputSerializesSharedWriterAcrossSwitches(t *testing.T) {
	var first, second outputYieldWriter
	const workers, messages = 4, 32
	loggers := []*Logger{newOutputTestLogger(&first), newOutputTestLogger(&first)}
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(workers + 1)
	for worker := 0; worker < workers; worker++ {
		go func(worker int) {
			defer wait.Done()
			<-start
			for message := 0; message < messages; message++ {
				loggers[worker%len(loggers)].Println(fmt.Sprintf("%d:%d", worker, message))
			}
		}(worker)
	}
	go func() {
		defer wait.Done()
		<-start
		for index := 0; index < workers*messages; index++ {
			writer := io.Writer(&first)
			if index%2 == 0 {
				writer = &second
			}
			for _, logger := range loggers {
				logger.SetOutput(writer)
			}
			runtime.Gosched()
		}
	}()
	close(start)
	wait.Wait()
	lines := strings.Split(strings.TrimSuffix(first.String()+second.String(), "\n"), "\n")
	require.Len(t, lines, workers*messages)
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		require.False(t, seen[line], "duplicate record %q", line)
		seen[line] = true
	}
	for worker := 0; worker < workers; worker++ {
		for message := 0; message < messages; message++ {
			require.True(t, seen[fmt.Sprintf("%d:%d", worker, message)], "record was split or lost")
		}
	}
}
