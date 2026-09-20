package ssadb

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/gorm"
)

const nativeSQLLogMin = 100 * time.Millisecond

var (
	sqlLogMu   sync.Mutex
	sqlLogFile *os.File
)

// StartSQLFileLog appends GORM SQL (and slow/failed native reads) to
// {debugDir}/db.log. SQL text is written once, to this diagnostic file;
// without a file it is available only at DEBUG level in the process log.
func StartSQLFileLog(debugDir string) (func(), error) {
	debugDir = strings.TrimSpace(debugDir)
	if debugDir == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(debugDir, "db.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	sqlLogMu.Lock()
	prev := sqlLogFile
	sqlLogFile = f
	sqlLogMu.Unlock()
	if prev != nil && prev != f {
		_ = prev.Close()
	}
	header := fmt.Sprintf("# ssadb SQL log started %s\n", time.Now().Format(time.RFC3339))
	_, _ = f.WriteString(header)
	log.Infof("[ssadb] SQL log: %s", path)
	return func() {
		sqlLogMu.Lock()
		if sqlLogFile == f {
			sqlLogFile = nil
		}
		sqlLogMu.Unlock()
		_ = f.Close()
	}, nil
}

func sqlLogEnabled() bool {
	sqlLogMu.Lock()
	defer sqlLogMu.Unlock()
	return sqlLogFile != nil
}

func attachSQLLogger(db *gorm.DB) {
	if db == nil || !sqlLogEnabled() {
		return
	}
	db.SetLogger(ssadbSQLLogger{})
	db.LogMode(true)
}

type ssadbSQLLogger struct{}

func (ssadbSQLLogger) Print(values ...interface{}) {
	line := formatGormSQLLog(values...)
	if line == "" {
		return
	}
	if !writeSQLLogLine(line) {
		log.Debugf("[ssadb] %s", line)
	}
	// Do not leak SQL/arguments at INFO/WARN/ERROR. Keep a visible indication
	// of failures even when SQL diagnostics are disabled.
	level, _ := values[0].(string)
	if level == "error" {
		log.Errorf("[ssadb] GORM operation failed; details in SQL debug log")
	}
}

func formatGormSQLLog(values ...interface{}) string {
	if len(values) == 0 {
		return ""
	}
	level, _ := values[0].(string)
	switch level {
	case "sql":
		if len(values) < 6 {
			return fmt.Sprint(values...)
		}
		dur, _ := values[2].(time.Duration)
		query := fmt.Sprint(values[3])
		return fmt.Sprintf("%s gorm [%s] %s", time.Now().Format("2006-01-02 15:04:05.000"), dur, query)
	case "error", "log":
		return fmt.Sprintf("%s gorm-%s %v", time.Now().Format("2006-01-02 15:04:05.000"), level, values[1:])
	default:
		return fmt.Sprintf("%s gorm %v", time.Now().Format("2006-01-02 15:04:05.000"), values)
	}
}

func writeSQLLogLine(line string) bool {
	line = strings.TrimRight(line, "\n")
	if line == "" {
		return false
	}
	sqlLogMu.Lock()
	defer sqlLogMu.Unlock()
	f := sqlLogFile
	if f == nil {
		return false
	}
	_, _ = f.WriteString(line + "\n")
	return true
}

// logNativeSQL records native IR reads. Fast successful reads are omitted so a
// hadoop-scale scan does not fill db.log; slow or failed statements are kept.
func logNativeSQL(query string, d time.Duration, err error) {
	miss := errors.Is(err, sql.ErrNoRows)
	if (err == nil || miss) && d < nativeSQLLogMin {
		return
	}
	if err != nil && !miss {
		log.Errorf("[ssadb] native query failed after %s (cancellation=%t); details in SQL debug log", d, nativeContextErr(err))
	}
	status := "ok"
	if err != nil {
		status = err.Error()
	}
	line := fmt.Sprintf("%s native [%s] %s status=%s", time.Now().Format("2006-01-02 15:04:05.000"), d, query, status)
	if !writeSQLLogLine(line) {
		log.Debugf("[ssadb] %s", line)
	}
}
