package asynchelper

import (
	"testing"
	"time"
)

func TestLogModesWithMock(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, format)
	}

	helper := NewAsyncPerformanceHelper(1*time.Second, 1*time.Second, "test", false)
	helper.mockLog = mock
	helper.Log("info log")
	if len(logs) != 1 || logs[0] != "info log" {
		t.Errorf("expected info log, got %v", logs)
	}

	logs = nil
	helper.errorLogMode = true
	helper.Log("error log")
	if len(logs) != 1 || logs[0] != "error log" {
		t.Errorf("expected error log, got %v", logs)
	}
}

func TestCheckLastAndLogWithMock(t *testing.T) {
	var logs []string
	mock := func(format string, args ...any) {
		logs = append(logs, format)
	}

	helper := NewAsyncPerformanceHelper(1*time.Millisecond, 1*time.Millisecond, "test", false)
	helper.mockLog = mock
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

	helper := NewAsyncPerformanceHelper(1*time.Millisecond, 1*time.Millisecond, "test", false)
	helper.mockLog = mock
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

	helper := NewAsyncPerformanceHelper(1*time.Millisecond, 1*time.Millisecond, "test", false)
	helper.mockLog = mock
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

	helper := NewAsyncPerformanceHelper(1*time.Millisecond, 1*time.Millisecond, "test", false)
	helper.mockLog = mock
	helper.CheckLastMarkAndLog(1*time.Millisecond, "verbose")
	// Should not log via mockLog, but error via log.Errorf (not captured here)
	if len(logs) != 0 {
		t.Errorf("expected no log, got %v", logs)
	}
}
