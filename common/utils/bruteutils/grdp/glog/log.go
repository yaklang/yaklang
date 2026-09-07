package glog

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
)

// 并发说明：grdp 的异步读回调（StartReadBytes 派生的 goroutine）会与
// 下一次 newRDPClient 的 SetLevel/SetLogger 并发执行，历史上裸全局变量
// 读写构成 data race（连续爆破两次目标即可触发）。level 用原子值、
// logger 用锁保护读取，SetPrefix/SetOutput 仍在 mu 内串行。
var (
	loggerPtr atomic.Pointer[log.Logger]
	levelVal  atomic.Int32
	mu        sync.Mutex
)

type LEVEL int32

const (
	DEBUG LEVEL = iota
	INFO
	WARN
	ERROR
	NONE
)

func SetLogger(l *log.Logger) {
	l.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	loggerPtr.Store(l)
}

func SetLevel(l LEVEL) {
	levelVal.Store(int32(l))
}

func currentLevel() LEVEL { return LEVEL(levelVal.Load()) }

func checkLogger() *log.Logger {
	lg := loggerPtr.Load()
	if lg == nil && currentLevel() != NONE {
		panic("logger not inited")
	}
	return lg
}

func write(prefix string, lg *log.Logger, msg string) {
	if lg == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	lg.SetPrefix(prefix)
	lg.Output(3, msg)
}

func Debug(v ...interface{}) {
	if checkLogger() != nil && currentLevel() <= DEBUG {
		write("[DEBUG]", loggerPtr.Load(), fmt.Sprintln(v...))
	}
}
func Debugf(f string, v ...interface{}) {
	if checkLogger() != nil && currentLevel() <= DEBUG {
		write("[DEBUG]", loggerPtr.Load(), fmt.Sprintln(fmt.Sprintf(f, v...)))
	}
}
func Info(v ...interface{}) {
	if checkLogger() != nil && currentLevel() <= INFO {
		write("[INFO]", loggerPtr.Load(), fmt.Sprintln(v...))
	}
}
func Infof(f string, v ...interface{}) {
	if checkLogger() != nil && currentLevel() <= INFO {
		write("[INFO]", loggerPtr.Load(), fmt.Sprintln(fmt.Sprintf(f, v...)))
	}
}
func Warn(v ...interface{}) {
	if checkLogger() != nil && currentLevel() <= WARN {
		write("[WARN]", loggerPtr.Load(), fmt.Sprintln(v...))
	}
}

func Error(v ...interface{}) {
	if checkLogger() != nil && currentLevel() <= ERROR {
		write("[ERROR]", loggerPtr.Load(), fmt.Sprintln(v...))
	}
}

func Errorf(f string, v ...interface{}) {
	if checkLogger() != nil && currentLevel() <= ERROR {
		write("[ERROR]", loggerPtr.Load(), fmt.Sprintln(fmt.Sprintf(f, v...)))
	}
}
