package asynchelper

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/omap"
	"testing"
	"time"
)

func TestLogModesWithMock(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, format)
	}

	helper := NewAsyncPerformanceHelper("test", withMockLog(mock))
	helper.Logf("info log")
	if len(logs) != 1 || logs[0] != "info log" {
		t.Errorf("expected info log, got %v", logs)
	}

	logs = nil
	helper.errorLogMode = true
	helper.Logf("error log")
	if len(logs) != 1 || logs[0] != "error log" {
		t.Errorf("expected error log, got %v", logs)
	}
}

func TestCheckLastAndLogWithMock(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, format)
	}

	helper := NewAsyncPerformanceHelper("test", withMockLog(mock))
	helper.MarkNow()
	time.Sleep(2 * time.Millisecond)
	helper.CheckLastMarkAndLog(1*time.Millisecond, "verbose")
	if len(logs) != 1 {
		t.Errorf("expected one log, got %v", logs)
	}
}

func TestCheckMarkAndLogWithMock(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, format)
	}

	helper := NewAsyncPerformanceHelper("test", withMockLog(mock))
	mark := helper.MarkNow()
	time.Sleep(2 * time.Millisecond)
	helper.CheckMarkAndLog(mark, 1*time.Millisecond, "verbose")
	if len(logs) != 1 {
		t.Errorf("expected one log, got %v", logs)
	}
}

func TestCheckMarkNotFoundWithMock(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, format)
	}

	helper := NewAsyncPerformanceHelper("test", withMockLog(mock))
	helper.CheckMarkAndLog("notfound", 1*time.Millisecond, "verbose")
	// Should not log via mockLog, but error via log.Errorf (not captured here)
	if len(logs) != 0 {
		t.Errorf("expected no log, got %v", logs)
	}
}

func TestCheckLastNotFoundWithMock(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, format)
	}

	helper := NewAsyncPerformanceHelper("test", withMockLog(mock))
	helper.CheckLastMarkAndLog(1*time.Millisecond, "verbose")
	// Should not log via mockLog, but error via log.Errorf (not captured here)
	if len(logs) != 0 {
		t.Errorf("expected no log, got %v", logs)
	}
}

func TestDumpStatusDurations(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	helper := NewAsyncPerformanceHelper("test", withMockLog(mock))
	helper.statusToDuration = omap.NewOrderedMap[string, []time.Duration](make(map[string][]time.Duration))
	helper.statusToDuration.Set("ready", []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		5 * time.Millisecond,
		15 * time.Millisecond,
	})

	helper.DumpStatusDurations("ready")
	contentOk := false
	expectedTop1 := "20ms"
	expectedTop2 := "15ms"
	expectedTop3 := "10ms"
	expectedAvg := "12.5ms"
	expectedCount := "4"
	for _, log := range logs {
		if log != "" &&
			contains(log, "Durations for [test] status [ready]") &&
			contains(log, "Top1") &&
			contains(log, "Avg") &&
			contains(log, "Count") &&
			contains(log, expectedTop1) &&
			contains(log, expectedTop2) &&
			contains(log, expectedTop3) &&
			contains(log, expectedAvg) &&
			contains(log, expectedCount) {
			contentOk = true
		}
	}

	if !contentOk {
		t.Errorf("DumpStatusDurations table missing expected content or values, got: %v", logs)
	}
}

func TestDumpAllStatus(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	helper := NewAsyncPerformanceHelper("test", withMockLog(mock))
	helper.statusToDuration = omap.NewOrderedMap[string, []time.Duration](make(map[string][]time.Duration))
	helper.statusToDuration.Set("ready", []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
	})
	helper.statusToDuration.Set("done", []time.Duration{
		5 * time.Millisecond,
		15 * time.Millisecond,
		25 * time.Millisecond,
	})

	helper.DumpAllStatus()
	contentOk := false
	// ready: total=30ms, count=2, avg=15ms
	// done: total=45ms, count=3, avg=15ms
	for _, log := range logs {
		if log != "" &&
			contains(log, "All Status Summary for [test]") &&
			contains(log, "ready") &&
			contains(log, "done") &&
			contains(log, "Total") &&
			contains(log, "Count") &&
			contains(log, "Average") &&
			contains(log, "30ms") &&
			contains(log, "2") &&
			contains(log, "15ms") &&
			contains(log, "45ms") &&
			contains(log, "3") {
			contentOk = true
		}
	}
	if !contentOk {
		t.Errorf("DumpAllStatus table missing expected content or values, got: %v", logs)
	}
}

// contains returns true if substr is in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (s[0:len(substr)] == substr || contains(s[1:], substr))))
}
