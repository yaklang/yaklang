//go:build windows

package sharkcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// Exercise real Windows console handles rather than mocking mode/code-page
// calls. ConPTY keeps this independent of the developer's active shell.
func TestWindowsTerminalSetupInConPTY(t *testing.T) {
	inR, inW, err := os.Pipe()
	require.NoError(t, err)
	defer inR.Close()
	defer inW.Close()
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	defer outR.Close()
	defer outW.Close()
	var console windows.Handle
	err = windows.CreatePseudoConsole(windows.Coord{X: 120, Y: 32}, windows.Handle(inR.Fd()), windows.Handle(outW.Fd()), 0, &console)
	if err == windows.ERROR_PROC_NOT_FOUND {
		t.Skip("ConPTY needs Windows 10 1809 or later")
	}
	require.NoError(t, err)
	defer windows.ClosePseudoConsole(console)
	go io.Copy(io.Discard, outR)
	attributes, err := windows.NewProcThreadAttributeList(1)
	require.NoError(t, err)
	defer attributes.Delete()
	require.NoError(t, attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(console), unsafe.Sizeof(console)))
	startup := windows.StartupInfoEx{
		StartupInfo:             windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))},
		ProcThreadAttributeList: attributes.List(),
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	program, err := windows.UTF16PtrFromString(executable)
	require.NoError(t, err)
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{executable, "-test.run=^TestWindowsTerminalConsoleChild$"}))
	require.NoError(t, err)
	report := filepath.Join(t.TempDir(), "console.txt")
	t.Setenv("YAK_SHARK_CONSOLE_REPORT", report)
	var process windows.ProcessInformation
	require.NoError(t, windows.CreateProcess(program, command, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &process))
	defer windows.CloseHandle(process.Thread)
	defer windows.CloseHandle(process.Process)
	defer windows.TerminateProcess(process.Process, 1)
	status, err := windows.WaitForSingleObject(process.Process, 15000)
	require.NoError(t, err)
	require.Equal(t, uint32(windows.WAIT_OBJECT_0), status, "console child timed out")
	var code uint32
	require.NoError(t, windows.GetExitCodeProcess(process.Process, &code))
	data, err := os.ReadFile(report)
	require.NoError(t, err)
	require.Zero(t, code, string(data))
	require.Equal(t, "CP936 -> UTF-8; VT output; glyph columns; raw input; restored", string(data))
}

func TestWindowsTerminalConsoleChild(t *testing.T) {
	report := os.Getenv("YAK_SHARK_CONSOLE_REPORT")
	if report == "" {
		t.Skip("ConPTY child only")
	}
	result := "console check failed"
	defer func() { _ = os.WriteFile(report, []byte(result), 0600) }()
	// The test runner's stdio may be redirected by CI. Open the newly attached
	// pseudo console instead of inheriting those redirected parent handles.
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	require.NoError(t, err)
	defer in.Close()
	outFile, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	require.NoError(t, err)
	defer outFile.Close()
	os.Stdin, os.Stdout = in, outFile
	require.NoError(t, windows.SetConsoleCP(936))
	require.NoError(t, windows.SetConsoleOutputCP(936))
	out := windows.Handle(os.Stdout.Fd())
	var oldMode uint32
	require.NoError(t, windows.GetConsoleMode(out, &oldMode))
	oldMode &^= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	require.NoError(t, windows.SetConsoleMode(out, oldMode))
	inputHandle := windows.Handle(os.Stdin.Fd())
	var originalInput uint32
	require.NoError(t, windows.GetConsoleMode(inputHandle, &originalInput))
	originalInput |= windows.ENABLE_EXTENDED_FLAGS | windows.ENABLE_QUICK_EDIT_MODE
	require.NoError(t, windows.SetConsoleMode(inputHandle, originalInput))
	restore, err := prepareTerminal(os.Stdin, os.Stdout)
	require.NoError(t, err)
	defer restore()
	inputCP, err := windows.GetConsoleCP()
	require.NoError(t, err)
	require.Equal(t, uint32(65001), inputCP)
	outputCP, err := windows.GetConsoleOutputCP()
	require.NoError(t, err)
	require.Equal(t, uint32(65001), outputCP)
	var mode uint32
	require.NoError(t, windows.GetConsoleMode(out, &mode))
	require.NotZero(t, mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	const glyphs = "╭─╮ · → 中文"
	_, err = fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H"+glyphs)
	require.NoError(t, err)
	var info windows.ConsoleScreenBufferInfo
	require.NoError(t, windows.GetConsoleScreenBufferInfo(out, &info))
	require.Equal(t, int16(terminalWidth.StringWidth(glyphs)), info.CursorPosition.X)
	var inputMode uint32
	require.NoError(t, windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &inputMode))
	require.Zero(t, inputMode&windows.ENABLE_QUICK_EDIT_MODE)
	raw, err := term.MakeRaw(int(os.Stdin.Fd()))
	require.NoError(t, err)
	require.NoError(t, term.Restore(int(os.Stdin.Fd()), raw))
	var restoredInput uint32
	require.NoError(t, windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &restoredInput))
	require.Equal(t, inputMode, restoredInput)
	restore()
	require.NoError(t, windows.GetConsoleMode(inputHandle, &restoredInput))
	require.Equal(t, originalInput, restoredInput)
	inputCP, err = windows.GetConsoleCP()
	require.NoError(t, err)
	require.Equal(t, uint32(936), inputCP)
	outputCP, err = windows.GetConsoleOutputCP()
	require.NoError(t, err)
	require.Equal(t, uint32(936), outputCP)
	require.NoError(t, windows.GetConsoleMode(out, &mode))
	require.Equal(t, oldMode, mode)
	result = "CP936 -> UTF-8; VT output; glyph columns; raw input; restored"
}
