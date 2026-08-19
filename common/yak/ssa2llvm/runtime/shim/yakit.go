package shim

import (
	"fmt"
	"os"
)

// YakitExports are lightweight yakit stubs for ssa2llvm AOT binaries.

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

func YakitOutput(value interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "[yakit][output] %v\n", value)
}

func YakitGetHomeTempDir() string {
	dir, err := os.MkdirTemp("", "yakit-tmp-*")
	if err != nil {
		return os.TempDir()
	}
	return dir
}

func YakitStatusCard(kind string, content interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "[yakit][status-card][%s] %v\n", kind, content)
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
	"Output":       YakitOutput,
	"GetHomeTempDir": YakitGetHomeTempDir,
	"StatusCard":   YakitStatusCard,
}
