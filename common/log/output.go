package log

import (
	"io"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/kataras/golog"
	"github.com/kataras/pio"
	"github.com/kataras/pio/terminal"
)

var (
	// Capture the original descriptors during package initialization. Runtime
	// stdout attachment swaps os.Stdout, so logger creation must not read it.
	logPrototype     = golog.New()
	logInitialWriter = logPrototype.Printer.Output
	logCurrentOutput = &logOutputTarget{
		destination: newLogDestination(logInitialWriter),
		terminal:    logPrototype.Printer.IsTerminal,
	}
	logWriterLocks = struct {
		sync.Mutex
		writers map[io.Writer]*logWriterLock
	}{writers: make(map[io.Writer]*logWriterLock)}
)

type logDestination struct {
	writer     io.Writer
	comparable bool
	mu         sync.Mutex
}

type logWriterLock struct {
	mu         sync.Mutex
	references int
}

func newLogDestination(w io.Writer) *logDestination {
	return &logDestination{writer: w, comparable: w == nil || reflect.ValueOf(w).Comparable()}
}

// Share the lock while writes to the same writer are running or waiting, even
// when output switches away and back. Do not retain completed destinations.
// A lock for one writer must not prevent that writer forwarding to another
// logger with a different destination.
func (d *logDestination) lockWriter() func() {
	if !d.comparable {
		d.mu.Lock()
		return d.mu.Unlock
	}
	logWriterLocks.Lock()
	lock := logWriterLocks.writers[d.writer]
	if lock == nil {
		lock = new(logWriterLock)
		logWriterLocks.writers[d.writer] = lock
	}
	lock.references++
	logWriterLocks.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		logWriterLocks.Lock()
		lock.references--
		if lock.references == 0 {
			delete(logWriterLocks.writers, d.writer)
		}
		logWriterLocks.Unlock()
	}
}

type logOutputTarget struct {
	destination *logDestination
	terminal    bool
}

// Keep Printer.Output and its hijacker fixed after publishing a logger. pio
// reads those fields without its SetOutput lock, so replace the destination
// behind this writer instead of reconfiguring a live Printer.
type logOutput struct {
	target   atomic.Pointer[logOutputTarget]
	recordMu sync.Mutex
	record   *logOutputRecord
}

type logOutputRecord struct {
	target  *logOutputTarget
	body    []byte
	newLine bool
	finish  func()
}

func newLogOutputTarget(w io.Writer) *logOutputTarget {
	return &logOutputTarget{destination: newLogDestination(w), terminal: terminal.IsTerminal(w)}
}

func (o *logOutput) Write(data []byte) (int, error) {
	// pio keeps its Printer mutex across hijack, body and newline. Identify
	// its stored body by slice identity, so unrelated Output.Write calls keep
	// their normal writer behavior instead of consuming a pending record.
	o.recordMu.Lock()
	record := o.record
	if record != nil && sameLogBuffer(data, record.body) {
		o.record = nil
		o.recordMu.Unlock()
		return o.writeRecord(record, data)
	}
	if record != nil && record.finish != nil && sameLogBuffer(data, pio.NewLine) {
		o.record = nil
		o.recordMu.Unlock()
		defer record.finish()
		return writeLogDestination(record.target, data)
	}
	o.recordMu.Unlock()
	target := o.target.Load()
	if target == nil || target.destination.writer == nil {
		return len(data), nil
	}
	finish := target.destination.lockWriter()
	defer finish()
	return writeLogDestination(target, data)
}

func sameLogBuffer(a, b []byte) bool {
	return len(a) > 0 && len(a) == len(b) && &a[0] == &b[0]
}

func writeLogDestination(target *logOutputTarget, data []byte) (int, error) {
	if target == nil || target.destination.writer == nil {
		return len(data), nil
	}
	return target.destination.writer.Write(data)
}

func (o *logOutput) writeRecord(record *logOutputRecord, data []byte) (int, error) {
	finish := func() {}
	if record.target != nil && record.target.destination.writer != nil {
		finish = record.target.destination.lockWriter()
	}
	defer func() {
		if finish != nil {
			finish()
		}
	}()
	n, err := writeLogDestination(record.target, data)
	if err == nil && record.newLine {
		// pio holds its Printer mutex from hijack through both writes. The
		// following Write(NewLine) therefore completes this same record. Keep
		// its destination locked so another logger cannot split the line.
		record.body = nil
		record.finish = finish
		o.recordMu.Lock()
		o.record = record
		o.recordMu.Unlock()
		finish = nil
	}
	return n, err
}

func (o *logOutput) setRecord(record *logOutputRecord) {
	o.recordMu.Lock()
	previous := o.record
	o.record = record
	o.recordMu.Unlock()
	if previous != nil && previous.finish != nil {
		// A caller of Printer.WriteTo may choose not to append a newline even
		// for a Log that requests one. Its Printer mutex is now available, so
		// no write from that earlier record is still in progress.
		previous.finish()
	}
}

func newGologLogger() *golog.Logger {
	logger := logPrototype.Clone()
	logger.Printer = pio.NewPrinter("", logInitialWriter).EnableDirectOutput()
	return logger
}

func (o *logOutput) hijack(ctx *pio.Ctx) {
	entry, ok := ctx.Value.(*golog.Log)
	if !ok {
		o.setRecord(nil)
		ctx.Next()
		return
	}
	target := o.target.Load()
	line := golog.GetTextForLevel(entry.Level, target != nil && target.terminal)
	if line != "" {
		line += " "
	}
	if timestamp := entry.FormatTime(); timestamp != "" {
		line += timestamp + " "
	}
	line += entry.Message
	data := append([]byte(nil), entry.Logger.Prefix...)
	data = append(data, line...)
	o.setRecord(&logOutputRecord{target: target, body: data, newLine: entry.NewLine})
	ctx.Store(data, nil)
	ctx.Next()
}

func (l *Logger) SetOutput(w io.Writer) *Logger {
	l.output.target.Store(newLogOutputTarget(w))
	return l
}

func (l *Logger) SetTerminal(enabled bool) {
	for {
		old := l.output.target.Load()
		if old == nil {
			return
		}
		updated := &logOutputTarget{destination: old.destination, terminal: enabled}
		if l.output.target.CompareAndSwap(old, updated) {
			return
		}
	}
}
