package ssalog

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

const envSSALogLevel = "YAK_SSA_LOG_LEVEL"

func resolveSSALogLevel() string {
	level := strings.TrimSpace(os.Getenv(envSSALogLevel))
	if level == "" {
		return "error"
	}
	return level
}

var (
	Log = func() *log.Logger {
		logger := log.GetLogger("ssaLog")
		logger.SetLevel(resolveSSALogLevel())
		return logger
	}()
)
