package loglite

import (
	"fmt"
	"os"
)

// Yakit exports used by ssa2llvm AOT binaries. Kept in a standalone package so
// pruned runtime builds do not link the full yaklib/yakit dependency graph.

func YakitInfo(format string, items ...interface{}) {
	yakitStderrLog("info", format, items...)
}

func YakitWarn(format string, items ...interface{}) {
	yakitStderrLog("warn", format, items...)
}

func YakitDebug(format string, items ...interface{}) {
	yakitStderrLog("debug", format, items...)
}

func YakitError(format string, items ...interface{}) {
	yakitStderrLog("error", format, items...)
}

func YakitCode(value interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "[code] %v\n", value)
}

func AutoInitYakit(...interface{}) {}

func YakitSetProgress(f float64) {}

func YakitSetProgressEx(id string, f float64) {}

func YakitFile(path string, extra ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "[file] %s\n", path)
}

func YakitText(text string) {
	_, _ = fmt.Fprintf(os.Stderr, "[text] %s\n", text)
}

func yakitStderrLog(level, format string, items ...interface{}) {
	msg := format
	if len(items) > 0 {
		msg = fmt.Sprintf(format, items...)
	}
	_, _ = fmt.Fprintf(os.Stderr, "[yakit][%s] %s\n", level, msg)
}

var YakitExports = map[string]interface{}{
	"Info":          YakitInfo,
	"Warn":          YakitWarn,
	"Debug":         YakitDebug,
	"Error":         YakitError,
	"Code":          YakitCode,
	"Text":          YakitText,
	"File":          YakitFile,
	"SetProgress":   YakitSetProgress,
	"SetProgressEx": YakitSetProgressEx,
	"AutoInitYakit": AutoInitYakit,
}
