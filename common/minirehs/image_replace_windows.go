//go:build windows

package minirehs

import (
	"errors"
	"io"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func transientImageLockError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

// Go's ordinary file reader omits FILE_SHARE_DELETE on Windows. Cache readers
// allow replacement while their handle still reads the old complete file.
func readImageFile(path string) ([]byte, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > 0x7fffffff {
		return nil, ErrCorruptImage
	}
	b := make([]byte, int(info.Size()))
	n, err := io.ReadFull(f, b)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = nil
	}
	return b[:n], err
}

// Replace in one filesystem operation instead of unlinking the old cache. A
// failed replacement keeps the previous complete file available to readers.
func replaceImageFile(from, to string) error {
	f, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	// FileRenameInfoEx's POSIX flag keeps old reader handles valid while new
	// opens see the new file. MoveFileEx alone can leave a name pending deletion
	// under sustained reads, even with FILE_SHARE_DELETE.
	// https://learn.microsoft.com/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information
	h, err := windows.CreateFile(f, windows.DELETE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	name, err := windows.UTF16FromString(to)
	if err != nil {
		_ = windows.CloseHandle(h)
		return err
	}
	type renameInfo struct {
		flags      uint32
		root       windows.Handle
		nameLength uint32
		name       [1]uint16
	}
	var layout renameInfo
	buf := make([]byte, int(unsafe.Offsetof(layout.name))+len(name)*2)
	info := (*renameInfo)(unsafe.Pointer(&buf[0]))
	info.flags = 0x1 | 0x2 // REPLACE_IF_EXISTS | POSIX_SEMANTICS
	info.nameLength = uint32((len(name) - 1) * 2)
	copy(unsafe.Slice(&info.name[0], len(name)), name)
	err = windows.SetFileInformationByHandle(h, windows.FileRenameInfoEx, &buf[0], uint32(len(buf)))
	_ = windows.CloseHandle(h)
	if err == nil {
		return nil
	}
	if !errors.Is(err, windows.ERROR_INVALID_PARAMETER) && !errors.Is(err, windows.ERROR_NOT_SUPPORTED) && !errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
		return err
	}
	// NTFS may briefly leave the replaced name pending deletion while readers
	// close the old handle. Bounded retries also cover indexers/antivirus sharing
	// the cache. Never unlink the previous complete image on a failed replace.
	for i := 0; i < 32; i++ {
		err = windows.MoveFileEx(f, t, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return err
}
