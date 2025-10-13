//go:build windows
// +build windows

package winpty

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// UTF16PtrToString 将 UTF16 指针转换为 Go 字符串
func UTF16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}

	var finalStr []uint16
	for {
		if *p == 0 {
			break
		}
		finalStr = append(finalStr, *p)
		p = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + unsafe.Sizeof(uint16(0))))
	}

	if len(finalStr) == 0 {
		return ""
	}

	return string(utf16.Decode(finalStr))
}

// UTF16PtrFromStringArray 将字符串数组转换为 UTF16 指针
func UTF16PtrFromStringArray(s []string) (*uint16, error) {
	if len(s) == 0 {
		// 返回一个只包含终止符的数组
		r := []uint16{0}
		return &r[0], nil
	}

	var r []uint16
	for _, ss := range s {
		a, err := syscall.UTF16FromString(ss)
		if err != nil {
			return nil, err
		}
		r = append(r, a...)
	}

	// 添加终止符
	r = append(r, 0)
	return &r[0], nil
}

// GetErrorMessage 获取错误消息
func GetErrorMessage(dll *WinptyDLL, errorPtr uintptr) string {
	if errorPtr == 0 {
		return "Unknown Error"
	}

	if dll == nil {
		return "WinPTY DLL not loaded"
	}

	msgPtr, _, _ := dll.ErrorMsg.Call(errorPtr)
	if msgPtr == 0 {
		return "Unknown Error"
	}

	return UTF16PtrToString((*uint16)(unsafe.Pointer(msgPtr)))
}
