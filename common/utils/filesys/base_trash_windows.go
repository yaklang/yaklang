//go:build windows

package filesys

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	FO_DELETE          = 3
	FOF_NOCONFIRMATION = 16
	FOF_ALLOWUNDO      = 64
)

var shell32Dll = windows.NewLazySystemDLL("Shell32.dll")

var shFileOperationWProc = shell32Dll.NewProc("SHFileOperationW")

type _ShFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uintptr
	pFrom                 uintptr
	pTo                   uintptr
	fileOpFlags           uintptr
	fAnyOperationsAborted uintptr
	hNameMappings         uintptr
	lpszProgressTitle     uintptr
}

func (*LocalFs) Throw(filenames ...string) error {
	pFromData := make([]uint16, 0, 256)
	for _, fn := range filenames {
		u, err := windows.UTF16FromString(fn)
		if err != nil {
			return err
		}
		pFromData = append(pFromData, u...)
	}
	pFromData = append(pFromData, 0)

	title := []uint16{0, 0}

	param := &_ShFileOpStruct{
		wFunc:             FO_DELETE,
		pFrom:             uintptr(unsafe.Pointer(&pFromData[0])),
		fileOpFlags:       (FOF_ALLOWUNDO | FOF_NOCONFIRMATION),
		lpszProgressTitle: uintptr(unsafe.Pointer(&title[0])),
	}

	_, _, err := shFileOperationWProc.Call(uintptr(unsafe.Pointer(param)))
	if strings.Contains(err.Error(), "successfully") {
		return nil 
	}
	return err
}
