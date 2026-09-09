//go:build windows

package stdio

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func prepareOutput(output io.WriteCloser) (io.WriteCloser, func(), error) {
	f, ok := output.(*os.File)
	if !ok {
		return output, func() { _ = output.Close() }, nil
	}
	var duplicate windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, windows.Handle(f.Fd()), process,
		&duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, nil, err
	}
	// os.File.Close cancels pending Windows pipe I/O via CancelIoEx.
	writer := os.NewFile(uintptr(duplicate), "mcp-cancellable-output")
	return writer, func() { _ = writer.Close() }, nil
}
