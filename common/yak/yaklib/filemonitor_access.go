package yaklib

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var activeFileMonitors sync.Map

func registerActiveFileMonitor(fm *FileMonitor) {
	if fm != nil {
		activeFileMonitors.Store(fm, struct{}{})
	}
}

func unregisterActiveFileMonitor(fm *FileMonitor) {
	if fm != nil {
		activeFileMonitors.Delete(fm)
	}
}

func recordYakFileRead(path string) {
	recordYakFileAccess(FileOpRead, path)
}

func recordYakFileAccess(op, path string) {
	if path == "" {
		return
	}

	if absPath, err := filepath.Abs(path); err == nil {
		path = absPath
	}

	activeFileMonitors.Range(func(key, _ interface{}) bool {
		fm, ok := key.(*FileMonitor)
		if !ok || fm == nil {
			return true
		}
		fm.recordAccessEvent(op, path)
		return true
	})
}

func (fm *FileMonitor) recordAccessEvent(op, path string) {
	if fm == nil || fm.config == nil {
		return
	}
	if !fm.isWatchedPath(path) {
		return
	}
	if !fm.shouldMonitor(path) {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}
	fm.recordEvent(op, path, info.IsDir(), info)
}

func (fm *FileMonitor) isWatchedPath(path string) bool {
	if path == "" {
		return false
	}

	for base := range fm.monitors {
		if path == base || strings.HasPrefix(path, base+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
