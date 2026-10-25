package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

type charInfo struct {
	Char uint16
	Attr uint16
}
type snapshot struct {
	Width, Height int
	Rows          []string
	Attributes    [][]uint16
}

var readConsoleOutput = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleOutputW")

func fail(err error) {
	if err != nil {
		panic(err)
	}
}
func saveScreen(path string, out *os.File) {
	var info windows.ConsoleScreenBufferInfo
	fail(windows.GetConsoleScreenBufferInfo(windows.Handle(out.Fd()), &info))
	w, h := int(info.Window.Right-info.Window.Left+1), int(info.Window.Bottom-info.Window.Top+1)
	data := make([]charInfo, w*h)
	size := windows.Coord{X: int16(w), Y: int16(h)}
	region := info.Window
	ok, _, err := readConsoleOutput.Call(out.Fd(), uintptr(unsafe.Pointer(&data[0])), uintptr(*(*uint32)(unsafe.Pointer(&size))), 0, uintptr(unsafe.Pointer(&region)))
	if ok == 0 {
		fail(err)
	}
	s := snapshot{Width: w, Height: h}
	for y := 0; y < h; y++ {
		chars := make([]uint16, w)
		attrs := make([]uint16, w)
		for x := 0; x < w; x++ {
			chars[x] = data[y*w+x].Char
			if chars[x] == 0 {
				chars[x] = ' '
			}
			attrs[x] = data[y*w+x].Attr
		}
		s.Rows = append(s.Rows, string(utf16.Decode(chars)))
		s.Attributes = append(s.Attributes, attrs)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	fail(err)
	fail(os.WriteFile(path, b, 0600))
}
func child() {
	dir, binary := os.Args[2], os.Args[3]
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	fail(err)
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	fail(err)
	// Reproduce a Chinese, light-themed Windows console before yak starts.
	fail(windows.SetConsoleCP(936))
	fail(windows.SetConsoleOutputCP(936))
	var mode uint32
	fail(windows.GetConsoleMode(windows.Handle(out.Fd()), &mode))
	fail(windows.SetConsoleMode(windows.Handle(out.Fd()), mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING))
	_, err = out.WriteString("\x1b[30;47m\x1b[2J\x1b[H")
	fail(err)
	args := []string{"shark", "-r", os.Args[4]}
	if os.Args[5] != "-" {
		args = append(args, "--tls-keylog", os.Args[5])
	}
	if len(os.Args) > 6 {
		args = append(args, os.Args[6:]...)
	}

	cmd := exec.Command(binary, args...)
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = out
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "NO_COLOR=") && !strings.HasPrefix(env, "TERM=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	fail(cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			cp, _ := windows.GetConsoleCP()
			ocp, _ := windows.GetConsoleOutputCP()
			b, _ := json.Marshal(map[string]any{"input_cp": cp, "output_cp": ocp, "error": fmt.Sprint(err)})
			_ = os.WriteFile(filepath.Join(dir, "restored.json"), b, 0600)
			fail(err)
			return
		case <-tick.C:
			b, e := os.ReadFile(filepath.Join(dir, "snapshot.request"))
			if e == nil {
				_ = os.Remove(filepath.Join(dir, "snapshot.request"))
				saveScreen(filepath.Join(dir, string(b)+".json"), out)
			}
		}
	}
}

type logBuffer struct {
	sync.Mutex
	b bytes.Buffer
}

func (l *logBuffer) Write(b []byte) (int, error) { l.Lock(); defer l.Unlock(); return l.b.Write(b) }
func (l *logBuffer) String() string              { l.Lock(); defer l.Unlock(); return l.b.String() }
func main() {
	if len(os.Args) > 1 && os.Args[1] == "view" {
		self, err := os.Executable()
		fail(err)
		cmd := exec.Command(self, append([]string{"view-child"}, os.Args[2:]...)...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
		fail(cmd.Start())
		fmt.Println("Console viewer PID", cmd.Process.Pid)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "view-child" {
		in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
		fail(err)
		out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
		fail(err)
		fail(windows.SetConsoleCP(936))
		fail(windows.SetConsoleOutputCP(936))
		var mode uint32
		fail(windows.GetConsoleMode(windows.Handle(out.Fd()), &mode))
		fail(windows.SetConsoleMode(windows.Handle(out.Fd()), mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING))
		_, err = out.WriteString("\x1b[30;47m\x1b[2J\x1b[H")
		fail(err)
		title, _ := windows.UTF16PtrFromString("Yak Shark Windows verification")
		windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleTitleW").Call(uintptr(unsafe.Pointer(title)))
		cmd := exec.Command(os.Args[2], "shark", "--pcap-file", os.Args[3])
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, out
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, "NO_COLOR=") && !strings.HasPrefix(env, "TERM=") {
				cmd.Env = append(cmd.Env, env)
			}
		}
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
		fail(cmd.Run())
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "child" {
		child()
		return
	}
	dir, err := filepath.Abs(os.Args[1])
	fail(err)
	binary, err := filepath.Abs(os.Args[2])
	fail(err)
	fail(os.MkdirAll(dir, 0700))
	if _, err := os.Stat(filepath.Join(dir, "01-protocol-fields.json")); err == nil {
		panic("native acceptance requires a fresh output directory")
	}
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "yak shark Windows test · 中文 → response")
	})}
	listener, err := net.Listen("tcp", server.Addr)
	fail(err)
	go server.Serve(listener)
	defer server.Close()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	fail(err)
	defer udp.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, a, e := udp.ReadFrom(buf)
			if e != nil {
				return
			}
			reply := append([]byte(nil), buf[:n]...)
			if n >= 12 {
				reply[2] = 0x81
				reply[3] = 0x80
			}
			_, _ = udp.WriteTo(reply, a)
		}
	}()
	inR, inW, err := os.Pipe()
	fail(err)
	defer inR.Close()
	defer inW.Close()
	outR, outW, err := os.Pipe()
	fail(err)
	defer outR.Close()
	defer outW.Close()
	var console windows.Handle
	fail(windows.CreatePseudoConsole(windows.Coord{X: 180, Y: 48}, windows.Handle(inR.Fd()), windows.Handle(outW.Fd()), 0, &console))
	defer windows.ClosePseudoConsole(console)
	var log logBuffer
	go io.Copy(&log, outR)
	attrs, err := windows.NewProcThreadAttributeList(1)
	fail(err)
	defer attrs.Delete()
	fail(attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(console), unsafe.Sizeof(console)))
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attrs.List()}
	self, err := os.Executable()
	fail(err)
	program, err := windows.UTF16PtrFromString(self)
	fail(err)
	childArgs := []string{self, "child", dir, binary, os.Args[3], os.Args[4]}
	if len(os.Args) > 7 {
		childArgs = append(childArgs, os.Args[7:]...)
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(childArgs))
	fail(err)
	var proc windows.ProcessInformation
	fail(windows.CreateProcess(program, command, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &proc))
	defer windows.CloseHandle(proc.Process)
	defer windows.CloseHandle(proc.Thread)
	defer windows.TerminateProcess(proc.Process, 1)
	ansi := regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
	plain := func() string { return ansi.ReplaceAllString(log.String(), "") }
	waitFor := func(text string) {
		limit := 15 * time.Second
		if text == "EOF" {
			limit = 90 * time.Second
		}
		until := time.Now().Add(limit)
		for !strings.Contains(plain(), text) {
			if time.Now().After(until) {
				_ = os.WriteFile(filepath.Join(dir, "terminal.vt"), []byte(log.String()), 0600)
				panic("timeout: " + text)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	send := func(s string) { _, e := inW.WriteString(s); fail(e); time.Sleep(350 * time.Millisecond) }
	shot := func(name string) {
		fail(os.WriteFile(filepath.Join(dir, "snapshot.request"), []byte(name), 0600))
		until := time.Now().Add(3 * time.Second)
		for {
			if _, e := os.Stat(filepath.Join(dir, name+".json")); e == nil {
				break
			}
			if time.Now().After(until) {
				panic("snapshot timeout")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	waitFor("Y A K  S H A R K")
	waitFor("EOF")
	// Query '-' preserves the CLI's initial display-filter for native QA.
	if os.Args[5] != "-" {
		send("/" + os.Args[5] + "\r")
	}
	send("1g\r")
	time.Sleep(time.Second)
	send("d")
	shot("01-protocol-fields")
	// Opt-in navigation of the real field pane. Deep protocol trees must be
	// checked where their fields actually render, not only in JSON exports.
	if value := os.Getenv("SHARK_QA_FIELD_DOWN"); value != "" {
		n, err := strconv.Atoi(value)
		fail(err)
		if n < 0 || n > 512 {
			panic("field QA navigation outside bounded range")
		}
		send("2")
		send(strings.Repeat("\x1b[B", n))
		shot("01-deep-protocol-fields")
	}
	// A pinned pre-EOF detail must not make an empty filtered list pass.
	// Other bounded/evicted packets may separately report an indeterminate
	// predicate; that does not erase the proven matching rows being inspected.
	firstScreen, err := os.ReadFile(filepath.Join(dir, "01-protocol-fields.json"))
	fail(err)
	visible := regexp.MustCompile(`· ([0-9]+(?:\.[0-9]+)?k?) visible`).FindStringSubmatch(string(firstScreen))
	if len(visible) != 2 || visible[1] == "0" {
		panic("no complete matching protocol rows after replay")
	}
	send("b")
	shot("02-first-byte-source")
	if os.Args[6] == "observed transport header" {
		firstSource, err := os.ReadFile(filepath.Join(dir, "02-first-byte-source.json"))
		fail(err)
		if !strings.Contains(string(firstSource), "header · network observation; logical offsets") {
			panic("observed transport header did not reach a selectable logical byte source")
		}
	}
	send("b")
	shot("02-byte-sources")
	send("4x")
	shot("03-conversation-messages")
	send("e")
	shot("04-association-evidence")
	fail(windows.ResizePseudoConsole(console, windows.Coord{X: 80, Y: 30}))
	time.Sleep(time.Second)
	shot("05-narrow")
	fail(windows.ResizePseudoConsole(console, windows.Coord{X: 180, Y: 48}))
	time.Sleep(time.Second)
	shot("06-restored")
	send("q")
	status, e := windows.WaitForSingleObject(proc.Process, 15000)
	fail(e)
	if status != windows.WAIT_OBJECT_0 {
		panic("quit timed out")
	}
	var exit uint32
	fail(windows.GetExitCodeProcess(proc.Process, &exit))
	if exit != 0 {
		panic(fmt.Sprint("exit ", exit))
	}
	fail(os.WriteFile(filepath.Join(dir, "terminal.vt"), []byte(log.String()), 0600))
	if !strings.Contains(plain(), os.Args[6]) {
		panic("missing protocol detail: " + os.Args[6])
	}
	fmt.Println("PASS: Windows CP936 + color; protocol fields; byte sources; conversation messages/evidence; resize; q; " + os.Args[6])
}
