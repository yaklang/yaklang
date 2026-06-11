package ssaproject

import (
	"strings"
	"time"
)

const compileTimestampLayout = "2006-01-02 15:04:05"

// BaseProjectNameFromProgramName strips compile timestamp suffix from program_name.
// e.g. "demo(2026-06-11 16:43:50)" -> "demo"
func BaseProjectNameFromProgramName(programName string) string {
	programName = strings.TrimSpace(programName)
	if programName == "" {
		return programName
	}
	idx := strings.LastIndex(programName, "(")
	if idx <= 0 || !strings.HasSuffix(programName, ")") {
		return programName
	}
	suffix := programName[idx+1 : len(programName)-1]
	if _, err := time.Parse(compileTimestampLayout, suffix); err != nil {
		return programName
	}
	return strings.TrimSpace(programName[:idx])
}
