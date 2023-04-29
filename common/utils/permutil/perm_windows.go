//go:build windows
// +build windows

package permutil

//func RunMeElevated() bool {
//	verb := "runas"
//	exe, _ := os.Executable()
//	cwd, _ := os.Getwd()
//	args := strings.Join(os.Args[1:], " ")
//
//	verbPtr, _ := syscall.UTF16PtrFromString(verb)
//	exePtr, _ := syscall.UTF16PtrFromString(exe)
//	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
//	argPtr, _ := syscall.UTF16PtrFromString(args)
//
//	var showCmd int32 = 1 //SW_NORMAL
//
//	err := windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
//	if err != nil {
//		log.Error("require administrator failed: %s", err)
//		return false
//	}
//	return true
//}
