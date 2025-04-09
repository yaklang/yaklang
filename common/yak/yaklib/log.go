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

// loglevel 根据传入的字符串设置日志级别
// disable: 禁用所有日志, fatal: 致命错误, error: 错误, warning: 警告, info: 信息, debug: 调试
// Example:
// ```
// loglevel("fatal")
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

var LogExports = map[string]interface{}{
	"info":     log.Infof,
	"setLevel": setLogLevel,
	"debug":    log.Debugf,
	"warn":     log.Warningf,
	"error":    log.Errorf,

	"Info":     log.Infof,
	"SetLevel": setLogLevel,
	"Debug":    log.Debugf,
	"Warn":     log.Warningf,
	"Error":    log.Errorf,
}
