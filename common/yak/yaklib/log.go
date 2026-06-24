package yaklib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kataras/golog"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// setLevel 根据传入的字符串设置日志级别
// disable: 禁用所有日志, fatal: 致命错误, error: 错误, warning: 警告, info: 信息, debug: 调试
// 参数:
//   - i: 日志级别名称，如 "info"、"warning"、"error"、"debug"、"fatal"、"disable"
//
// Example:
// ```
// // 把全局日志级别设置为 fatal(仅副作用，无返回值)
// log.setLevel("fatal")
// ```
func setLogLevel(i interface{}) {
	l, err := log.ParseLevel(fmt.Sprint(i))
	if err != nil {
		log.Errorf("parse %v(loglevel) error: %v, default: warning", i, err)
		log.SetLevel(log.WarnLevel)
		return
	}
	log.SetLevel(l)

	_logs.Range(func(key, value interface{}) bool {
		value.(*log.Logger).SetLevel(fmt.Sprint(i))
		return true
	})
}

type logFunc func(fmtStr string, items ...interface{})

var _logs = new(sync.Map)

func _fixYakModName(name string) string {
	_, file := filepath.Split(name)
	if file == "" {
		return "__main__.yak"
	}
	if strings.HasSuffix(strings.ToLower(file), ".yak") {
		return file
	} else {
		return file + ".yak"
	}
}

type YakLogger struct {
	*log.Logger
	SetLevel func(string) *golog.Logger
}

func CreateYakLogger(yakFiles ...string) *YakLogger {
	var yakFile string
	if len(yakFiles) > 0 {
		yakFile = yakFiles[0]
	}
	var logger *log.Logger
	loggerRaw, ok := _logs.Load(_fixYakModName(yakFile))
	if !ok {
		logger = log.GetLogger(_fixYakModName(yakFile))
		logger.SetOutput(os.Stdout)
		logger.Level = log.DefaultLogger.Level
		logger.Printer.IsTerminal = true
		_logs.Store(_fixYakModName(yakFile), logger)
	} else {
		logger = loggerRaw.(*log.Logger)
	}

	res := &YakLogger{Logger: logger}
	res.SetLevel = logger.SetLevel
	return res
}

func (y *YakLogger) SetEngine(engine *antlr4yak.Engine) {
	y.Logger.SetVMRuntimeInfoGetter(func(infoType string) (res any, err error) {
		return engine.RuntimeInfo(infoType)
	})
}

// Info 以 info(信息)级别格式化输出一条日志，日志内容应使用英文
// 参数:
//   - format: 格式化字符串
//   - args: 与格式化字符串对应的参数
//
// Example:
// ```
// // 输出一条 info 日志(仅副作用，无返回值)
// log.Info("server started on port %d", 8080)
// ```
func _logInfo(format string, args ...interface{}) {
	log.Infof(format, args...)
}

// Debug 以 debug(调试)级别格式化输出一条日志，日志内容应使用英文
// 参数:
//   - format: 格式化字符串
//   - args: 与格式化字符串对应的参数
//
// Example:
// ```
// // 输出一条 debug 日志(仅副作用，无返回值)
// log.Debug("current value is %v", 123)
// ```
func _logDebug(format string, args ...interface{}) {
	log.Debugf(format, args...)
}

// Warn 以 warning(警告)级别格式化输出一条日志，日志内容应使用英文
// 参数:
//   - format: 格式化字符串
//   - args: 与格式化字符串对应的参数
//
// Example:
// ```
// // 输出一条 warning 日志(仅副作用，无返回值)
// log.Warn("disk usage is high: %d%%", 90)
// ```
func _logWarn(format string, args ...interface{}) {
	log.Warningf(format, args...)
}

// Error 以 error(错误)级别格式化输出一条日志，日志内容应使用英文
// 参数:
//   - format: 格式化字符串
//   - args: 与格式化字符串对应的参数
//
// Example:
// ```
// // 输出一条 error 日志(仅副作用，无返回值)
// log.Error("failed to connect: %s", "timeout")
// ```
func _logError(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

var LogExports = map[string]interface{}{
	"info":     _logInfo,
	"setLevel": setLogLevel,
	"debug":    _logDebug,
	"warn":     _logWarn,
	"error":    _logError,

	"Info":     _logInfo,
	"SetLevel": setLogLevel,
	"Debug":    _logDebug,
	"Warn":     _logWarn,
	"Error":    _logError,
}
