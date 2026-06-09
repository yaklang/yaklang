package diagnostics

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// logLineWriter routes live TRACE lines into the project logger.
type logLineWriter struct{}

func (logLineWriter) Write(p []byte) (int, error) {
	if s := strings.TrimRight(string(p), "\n"); s != "" {
		log.Info(s)
	}
	return len(p), nil
}

// applyDefaultOutput enables nested TRACE + measurement on the standard logger.
func applyDefaultOutput(rec *Recorder) {
	if rec == nil {
		return
	}
	rec.SetNested(true)
	rec.SetNestedLog(true, 0, nil)
}
