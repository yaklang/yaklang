//go:build windows
// +build windows

package healthinfo

import (
	"context"
	"syscall"
	"unsafe"
)

func DiskUsageWithContext(ctx context.Context, path string) (*UsageStat, error) {
	kernel32, err := syscall.LoadLibrary("Kernel32.dll")
	if err != nil {
		return nil, err
	}
	defer syscall.FreeLibrary(kernel32)
	GetDiskFreeSpaceEx, err := syscall.GetProcAddress(syscall.Handle(kernel32), "GetDiskFreeSpaceExW")

	if err != nil {
		return nil, err
	}

	lpFreeBytesAvailable := int64(0)
	lpTotalNumberOfBytes := int64(0)
	lpTotalNumberOfFreeBytes := int64(0)
	_, _, err = syscall.Syscall6(uintptr(GetDiskFreeSpaceEx), 4,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("C:"))),
		uintptr(unsafe.Pointer(&lpFreeBytesAvailable)),
		uintptr(unsafe.Pointer(&lpTotalNumberOfBytes)),
		uintptr(unsafe.Pointer(&lpTotalNumberOfFreeBytes)), 0, 0)
	if err != nil {
		return nil, err
	}
	ret := &UsageStat{
		Path:        unescapeFstab(path),
		Total:       uint64(lpTotalNumberOfBytes),
		Free:        uint64(lpTotalNumberOfFreeBytes),
		Used:        uint64(lpTotalNumberOfBytes) - uint64(lpTotalNumberOfFreeBytes),
		UsedPercent: 0, // Placeholder value since Windows doesn't provide this information
		InodesTotal: 0, // Placeholder value since Windows doesn't provide this information
		InodesFree:  0, // Placeholder value since Windows doesn't provide this information
		InodesUsed:  0, // Placeholder value since Windows doesn't provide this information
	}

	return ret, nil
}
