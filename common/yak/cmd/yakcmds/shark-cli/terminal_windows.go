//go:build windows

package sharkcli

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func prepareTerminal(in, out *os.File) (func(), error) {
	input := windows.Handle(in.Fd())
	var inputMode uint32
	if err := windows.GetConsoleMode(input, &inputMode); err != nil {
		return nil, fmt.Errorf("read terminal input mode: %w", err)
	}
	handle := windows.Handle(out.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return nil, fmt.Errorf("read terminal output mode: %w", err)
	}
	inputCP, err := windows.GetConsoleCP()
	if err != nil {
		return nil, fmt.Errorf("read terminal input code page: %w", err)
	}
	outputCP, err := windows.GetConsoleOutputCP()
	if err != nil {
		return nil, fmt.Errorf("read terminal output code page: %w", err)
	}
	restore := func() {
		windows.SetConsoleMode(input, inputMode)
		windows.SetConsoleMode(handle, mode)
		windows.SetConsoleCP(inputCP)
		windows.SetConsoleOutputCP(outputCP)
	}
	// MakeRaw configures stdin only. stdout needs VT processing explicitly;
	// UTF-8 also keeps CJK code pages from changing Unicode glyph widths.
	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return nil, fmt.Errorf("enable terminal VT output: %w", err)
	}
	// Legacy conhost Quick Edit intercepts pointer selection before the TUI
	// can receive mouse reports. ENABLE_EXTENDED_FLAGS makes this bit writable.
	if err := windows.SetConsoleMode(input, (inputMode|windows.ENABLE_EXTENDED_FLAGS)&^windows.ENABLE_QUICK_EDIT_MODE); err != nil {
		restore()
		return nil, fmt.Errorf("disable terminal Quick Edit: %w", err)
	}
	if err := windows.SetConsoleCP(65001); err != nil {
		restore()
		return nil, fmt.Errorf("enable terminal UTF-8 input: %w", err)
	}
	if err := windows.SetConsoleOutputCP(65001); err != nil {
		restore()
		return nil, fmt.Errorf("enable terminal UTF-8 output: %w", err)
	}
	return restore, nil
}
