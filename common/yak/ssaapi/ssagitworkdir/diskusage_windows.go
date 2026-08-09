//go:build windows

package ssagitworkdir

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func platformDiskUsage(path string) (*diskUsageStat, error) {
	pathPointer, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var freeBytesAvailable uint64
	var totalBytes uint64
	var totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(
		pathPointer,
		&freeBytesAvailable,
		&totalBytes,
		&totalFreeBytes,
	); err != nil {
		return nil, err
	}
	return &diskUsageStat{Free: freeBytesAvailable}, nil
}
