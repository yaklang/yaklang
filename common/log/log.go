package log

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/kataras/golog"
)

// LogWriter is an io.Writer that writes to a golog.Logger.
// It's useful for redirecting the output of other processes (like ffmpeg)
// into the application's logging system.
type LogWriter struct {
	level golog.Level
}

// NewLogWriter creates a new LogWriter.
func NewLogWriter(level golog.Level) *LogWriter {
	return &LogWriter{level: level}
}

// Write implements the io.Writer interface. It logs the provided byte slice
// as a string message at the configured log level.
func (lw *LogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		DefaultLogger.Log(lw.level, msg)
	}
	return len(p), nil
}

func init() {
	SetConfig(NewDefaultConfig())
}

var (
	lock           = sync.Mutex{}
	loggers        = make(map[string]*Logger)
	frameIgnored   = regexp.MustCompile(`(?)(github.com/kataras/golog)|(palm/common/log/log.go)|(log.go)`)
	ErrUnknowLevel = errors.New("unknown log level")
)

const (
	DebugLevel = golog.DebugLevel
	InfoLevel  = golog.InfoLevel
	WarnLevel  = golog.WarnLevel
	ErrorLevel = golog.ErrorLevel
	FatalLevel = golog.FatalLevel
	PanicLevel = golog.FatalLevel
	TraceLevel = golog.DebugLevel
)

type Logger struct {
	*golog.Logger
	vmRuntimeInfoGetter func(infoType string) (any, error)
	name                string
}

const IGNOREFLAG = `[IGNORE]`

func formatter(l *golog.Log, name string, line int) bool {
	if l == nil {
		return true
	}
	if strings.Contains(l.Message, IGNOREFLAG) {
		return true
	}

	if strings.HasSuffix(strings.ToLower(name), ".yak") {
		name = name[:len(name)-4]
		if line == -1 {
			l.Message = fmt.Sprintf("[%v] %v", name, l.Message)
		} else if os.Getenv("YAK_IN_TERMINAL_MODE") != "" {
			if ret := len([]rune(name)); ret > 20 {
				nameBuf := append(([]rune(name))[:14], '.', '.')
				nameBuf = append(nameBuf, []rune(name)[ret-4:]...)
				name = string(nameBuf)
			}
			l.Message = fmt.Sprintf("[%v:%v] %v", name, line, l.Message)
		} else {
			l.Message = fmt.Sprintf("[%v:%v] %v", name, line, l.Message)
		}
		return false
	}

	file := "???"
	line = 0
	pc := make([]uintptr, 64)
	n := runtime.Callers(3, pc)
	if n != 0 {
		pc = pc[:n]
		frames := runtime.CallersFrames(pc)

		for {
			frame, more := frames.Next()
			if !frameIgnored.MatchString(frame.File) {
				file = frame.File
				line = frame.Line
				break
			}
			if !more {
				break
			}
		}
	}

	slices := strings.Split(file, "/")
	file = slices[len(slices)-1]
	if strings.HasSuffix(file, ".go") {
		file = file[:len(file)-3]
	}

	if name != "default" && name != "" {
		l.Message = fmt.Sprintf("[%s:%s:%d] %s", name, file, line, l.Message)
	} else {
		l.Message = fmt.Sprintf("[%s:%d] %s", file, line, l.Message)
	}

	return false
}

// GetLogger Return New Logger
func GetLogger(name string) *Logger {
	lock.Lock()
	defer lock.Unlock()
	logger, exists := loggers[name]
	if exists {
		return logger
	} else {
		logger = &Logger{
			Logger: golog.New(),
			name:   name,
		}
		logger.Handle(func(l *golog.Log) bool {
			line := -1
			if logger.vmRuntimeInfoGetter != nil {
				l, err := logger.vmRuntimeInfoGetter("line")
				if err != nil {
					return false
				}
				line = l.(int)
			}
			return formatter(l, name, line)
		})
		//logger.SetTimeFormat("2006-01-02 15:04:05 -0700")
		logger.SetTimeFormat("2006-01-02 15:04:05")
		logger.SetLevel(GetConfig().Level)
		loggers[name] = logger
		return logger
	}
}
func (l *Logger) SetName(name string) {
	l.name = name
}
func (l *Logger) SetVMRuntimeInfoGetter(f func(infoType string) (any, error)) {
	l.vmRuntimeInfoGetter = f
}
func CheckLogDir(dir string) error {
	if dir == "" {
		return nil
	} else {
		testFilepath := path.Join(dir, "test-log-dir.test")
		defer os.Remove(testFilepath)
		return ioutil.WriteFile(testFilepath, []byte("test log file"), 0640)
	}
}

var DefaultLogger = GetLogger("default")

// Print prints a log message without levels and colors.
func Print(v ...interface{}) {
	DefaultLogger.Print(v...)
}

// Printf formats according to a format specifier and writes to `Printer#Output` without levels and colors.
func Printf(format string, args ...interface{}) {
	DefaultLogger.Printf(format, args...)
}

// Println prints a log message without levels and colors.
// It adds a new line at the end, it overrides the `NewLine` option.
func Println(v ...interface{}) {
	DefaultLogger.Println(v...)
}

// Fatal `os.Exit(1)` exit no matter the level of the logger.
// If the logger's level is fatal, error, warn, info or debug
// then it will print the log message too.
func Fatal(v ...interface{}) {
	DefaultLogger.Fatal(v...)
}

// Fatalf will `os.Exit(1)` no matter the level of the logger.
// If the logger's level is fatal, error, warn, info or debug
// then it will print the log message too.
func Fatalf(format string, args ...interface{}) {
	DefaultLogger.Fatalf(format, args...)
}

// Error will print only when logger's Level is error, warn, info or debug.
func Error(v ...interface{}) {
	DefaultLogger.Error(v...)
}

// Errorf will print only when logger's Level is error, warn, info or debug.
func Errorf(format string, args ...interface{}) {
	DefaultLogger.Errorf(format, args...)
}

// Warn will print when logger's Level is warn, info or debug.
func Warn(v ...interface{}) {
	DefaultLogger.Warn(v...)
}

// Warnf will print when logger's Level is warn, info or debug.
func Warnf(format string, args ...interface{}) {
	DefaultLogger.Warnf(format, args...)
}

// Info will print when logger's Level is info or debug.
func Info(v ...interface{}) {
	DefaultLogger.Info(v...)
}

// Infof will print when logger's Level is info or debug.
func Infof(format string, args ...interface{}) {
	DefaultLogger.Infof(format, args...)
}

var _onceLog = new(sync.Map)

func OnceInfoLog(key string, fmtStr string, args ...interface{}) {
	if key == "" {
		Infof(fmtStr, args...)
		return
	}
	raw, ok := _onceLog.Load(key)
	if !ok {
		raw = new(sync.Once)
		_onceLog.Store(key, raw)
	}
	raw.(*sync.Once).Do(func() {
		Infof(fmtStr, args...)
	})
}

// Debug will print when logger's Level is debug.
func Debug(v ...interface{}) {
	DefaultLogger.Debug(v...)
}

// Debugf will print when logger's Level is debug.
func Debugf(format string, args ...interface{}) {
	DefaultLogger.Debugf(format, args...)
}

// Trace is named after Debug
var (
	Trace  = Debug
	Tracef = Debugf
)

func SetLevel(level golog.Level) {
	lock.Lock()
	defer lock.Unlock()
	DefaultLogger.Level = level
	for _, l := range loggers {
		l.Level = level
	}
}

func GetLevel() golog.Level {
	return DefaultLogger.Level
}

func SetOutput(w io.Writer) {
	lock.Lock()
	defer lock.Unlock()
	DefaultLogger.SetOutput(w)
	for _, l := range loggers {
		l.SetOutput(w)
	}
}

func ParseLevel(raw string) (golog.Level, error) {
	disable := golog.Levels[golog.DisableLevel]
	if disable.Name == raw {
		return golog.DisableLevel, nil
	}
	for _, s := range disable.AlternativeNames {
		if raw == s {
			return golog.DisableLevel, nil
		}
	}
	level := golog.ParseLevel(raw)
	if level == golog.DisableLevel {
		return level, ErrUnknowLevel
	}
	return level, nil
}

func Warningf(raw string, args ...interface{}) {
	Warnf(raw, args...)
}
