package yaklib

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kataras/golog"
	"github.com/yaklang/yaklang/common/log"

	"strings"
	"sync"
)

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

type yakLogger struct {
	logger   *log.Logger
	Info     logFunc
	Debug    logFunc
	Warn     logFunc
	Error    logFunc
	SetLevel func(string) *golog.Logger
}

func CreateYakLogger(yakFile string) *yakLogger {
	var logger *log.Logger
	loggerRaw, ok := _logs.Load(_fixYakModName(yakFile))
	if !ok {
		logger = log.GetLogger(_fixYakModName(yakFile))
		logger.SetOutput(os.Stdout)
		logger.Level = log.DefaultLogger.Level
		_logs.Store(_fixYakModName(yakFile), logger)
	} else {
		logger = loggerRaw.(*log.Logger)
	}

	res := &yakLogger{logger: logger}
	res.Info = logger.Infof
	res.Debug = logger.Debugf
	res.Warn = logger.Warnf
	res.Error = logger.Errorf
	res.SetLevel = logger.SetLevel
	return res
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
