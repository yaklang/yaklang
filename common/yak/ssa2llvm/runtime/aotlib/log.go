package aotlib

import (
	"fmt"
	"os"
)

func logPrintf(level string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "[%s] ", level)
	_, _ = fmt.Fprintln(os.Stderr, args...)
}

func LogInfo(args ...interface{})  { logPrintf("info", args...) }
func LogInfoF(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "[info] "+format+"\n", args...)
}
func LogWarn(args ...interface{})  { logPrintf("warn", args...) }
func LogError(args ...interface{}) { logPrintf("error", args...) }
func LogDebug(args ...interface{}) { logPrintf("debug", args...) }

// LogExports mirrors the log module's export table (AOT-supported subset).
var LogExports = map[string]any{
	"info":  LogInfo,
	"Info":  LogInfoF,
	"warn":  LogWarn,
	"error": LogError,
	"debug": LogDebug,
}
