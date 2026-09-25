package ssadb

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	yaklog "github.com/yaklang/yaklang/common/log"
)

func TestSQLFileLogKeepsFailuresAndOmitsFastMisses(t *testing.T) {
	var process bytes.Buffer
	previous := log
	log = yaklog.GetLogger("sql-log-regression").SetLevel("debug")
	log.SetOutput(&process)
	t.Cleanup(func() { log = previous })
	dir := t.TempDir()
	stop, err := StartSQLFileLog(dir)
	require.NoError(t, err)
	t.Cleanup(stop)
	logNativeSQL("fast lookup miss", time.Millisecond, sql.ErrNoRows)
	logNativeSQL("fast success", time.Millisecond, nil)
	logNativeSQL("slow select", time.Second, nil)
	logNativeSQL("timed out select", time.Second, context.DeadlineExceeded)
	ssadbSQLLogger{}.Print("sql", "source", time.Millisecond, "gorm select", nil, 1)
	data, err := os.ReadFile(filepath.Join(dir, "db.log"))
	require.NoError(t, err)
	output := string(data)
	require.NotContains(t, output, "fast lookup miss")
	require.NotContains(t, output, "fast success")
	require.Contains(t, output, "status=context deadline exceeded")
	for _, query := range []string{"slow select", "timed out select", "gorm select"} {
		require.Equal(t, 1, strings.Count(output, query))
		require.NotContains(t, process.String(), query, "SQL must not be duplicated into process log")
	}
	require.Contains(t, process.String(), "native query failed")
}

func TestSQLTextDoesNotReachInfoLog(t *testing.T) {
	var process bytes.Buffer
	previous := log
	log = yaklog.GetLogger("sql-info-regression").SetLevel("info")
	log.SetOutput(&process)
	t.Cleanup(func() { log = previous })
	ssadbSQLLogger{}.Print("sql", "source", time.Second, "SELECT private_data", nil, 1)
	logNativeSQL("SELECT secret", time.Second, context.DeadlineExceeded)
	require.NotContains(t, process.String(), "SELECT")
	require.Contains(t, process.String(), "native query failed")
	process.Reset()
	ssadbSQLLogger{}.Print("log", "source", gorm.ErrRecordNotFound)
	require.Empty(t, process.String())
	ssadbSQLLogger{}.Print("log", "source", errors.New("database is locked"))
	require.Contains(t, process.String(), "GORM operation failed: database is locked")
	require.NotContains(t, process.String(), "SELECT")
}
