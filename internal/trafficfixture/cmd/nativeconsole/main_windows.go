package main

import (
	"bytes"
	"encoding/hex"
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

// Console byte panes may pad offsets to six or eight hexadecimal digits.
// Match a complete zero-offset token, not a prefix of another width/value.
func nativeBytePaneTitleColumn(line string) int {
	at := strings.Index(line, "╭─ 3 ")
	if at < 0 {
		return -1
	}
	return len(utf16.Encode([]rune(line[:at+len("╭─ ")])))
}

func nativeHexOffsetColumn(line []uint16, titleX int) (int, int) {
	for x := max(0, titleX-4); x <= titleX+2; x++ {
		if x > 0 && x <= len(line) && line[x-1] != ' ' && line[x-1] != '\t' && line[x-1] != '│' && line[x-1] != '║' {
			continue // Never accept the six/eight-digit suffix of a longer token.
		}
		for _, width := range []int{6, 8} {
			if x+width >= len(line) || line[x+width] != ' ' {
				continue
			}
			zero := true
			for _, r := range line[x : x+width] {
				if r != '0' {
					zero = false
					break
				}
			}
			if zero {
				return x, width
			}
		}
	}
	return -1, 0
}

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
	// Publish completion only after the cell JSON has been written and closed.
	// A reader observing the newly created JSON file may otherwise race its write.
	fail(os.WriteFile(path+".ready", []byte("complete\n"), 0600))
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
			if e == nil && len(b) != 0 {
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
			if _, e := os.Stat(filepath.Join(dir, name+".json.ready")); e == nil {
				break
			}
			if time.Now().After(until) {
				panic("snapshot timeout")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	// A protocol word in the chart or old VT output is not proof that the
	// current query, selected row and detail pane agree. Strict checks inspect
	// fresh native cell snapshots, after the actual packet window has drained.
	strictProtocol := os.Getenv("SHARK_QA_EXPECT_PROTOCOL")
	strictQuery := os.Getenv("SHARK_QA_EXPECT_QUERY")
	if strictQuery == "" && os.Args[5] != "-" {
		strictQuery = os.Args[5]
	}
	strictPackets := os.Getenv("SHARK_QA_EXPECT_PACKETS")
	waitShot := func(name string, match func(snapshot) bool) {
		until := time.Now().Add(45 * time.Second)
		for i := 0; ; i++ {
			candidate := fmt.Sprintf("%s-wait-%03d", name, i)
			shot(candidate)
			data, e := os.ReadFile(filepath.Join(dir, candidate+".json"))
			fail(e)
			var state snapshot
			fail(json.Unmarshal(data, &state))
			if match(state) {
				fail(os.WriteFile(filepath.Join(dir, name+".json"), data, 0600))
				return
			}
			if time.Now().After(until) {
				panic("current console state did not satisfy " + name)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	ready := func(s snapshot) bool {
		if len(s.Rows) < 2 || !strings.Contains(s.Rows[0], " EOF ") {
			return false
		}
		if strictPackets != "" && !regexp.MustCompile(`buffer\s+`+regexp.QuoteMeta(strictPackets)+`\s*/`).MatchString(strings.Join(s.Rows, "\n")) {
			return false
		}
		return true
	}
	matching := func(s snapshot, detail bool) bool {
		if !ready(s) || strictQuery != "" && (!strings.Contains(s.Rows[1], strictQuery) || strings.Contains(s.Rows[1], "▏")) {
			return false
		}
		frame := ""
		protocolWords := strings.Fields(strictProtocol)
		for _, row := range s.Rows {
			f := strings.Fields(row)
			if len(f) >= 3+len(protocolWords) && strings.HasPrefix(f[1], "›") && strings.Join(f[3:3+len(protocolWords)], " ") == strictProtocol {
				frame = strings.TrimPrefix(f[1], "›")
				break
			}
		}
		if frame == "" {
			return false
		}
		if !detail {
			return true
		}
		for _, row := range s.Rows {
			if strings.Contains(row, "2 "+strictProtocol+" FIELDS") && strings.Contains(row, "#"+frame) {
				return true
			}
		}
		return false
	}
	waitFor("Y A K  S H A R K")
	waitFor("EOF")
	if strictProtocol != "" {
		waitShot("00-capture-ready", ready)
	}
	// Query '-' preserves the CLI's initial display-filter for native QA.
	if os.Args[5] != "-" {
		send("/" + os.Args[5] + "\r")
	}
	if strictProtocol != "" {
		waitShot("00-filter-ready", func(s snapshot) bool { return matching(s, false) })
	}
	send("1g\r")
	time.Sleep(time.Second)
	send("d")
	if strictProtocol != "" {
		waitShot("01-protocol-fields", func(s snapshot) bool { return matching(s, true) })
	} else {
		shot("01-protocol-fields")
	}
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
	if os.Getenv("SHARK_QA_EXPECT_ADAPTIVE") == "1" {
		loadShot := func(name string) snapshot {
			shot(name)
			data, err := os.ReadFile(filepath.Join(dir, name+".json"))
			fail(err)
			var state snapshot
			fail(json.Unmarshal(data, &state))
			return state
		}
		tabs := func(state snapshot) (int, int, int) {
			for y, row := range state.Rows {
				xf, xb := strings.Index(row, "[2 Fields]"), strings.Index(row, "[3 Bytes]")
				if xf >= 0 && xb >= 0 {
					return y, xf, xb
				}
			}
			panic("adaptive tabs not present in fresh native cells")
		}
		send("2")
		before := loadShot("01-adaptive-fields-before")
		y, xf, xb := tabs(before)
		if before.Height-3-(y+1) < 24 {
			panic("sparse result did not release enough native rows for deep inspection")
		}
		selected := before.Attributes[y][xf+1]
		if selected == before.Attributes[y][xb+1] {
			panic("native field-tab selection is not visually distinct")
		}
		click := func(x, y int) {
			send(fmt.Sprintf("\x1b[<0;%d;%dM\x1b[<0;%d;%dm", x+1, y+1, x+1, y+1))
		}
		click(xb+2, y)
		byteView := loadShot("01-adaptive-bytes-click")
		by, bf, bb := tabs(byteView)
		if by != y || byteView.Attributes[by][bb+1] != selected || byteView.Attributes[by][bf+1] == selected {
			panic("mouse click missed the rendered adaptive Bytes tab")
		}
		click(xf+2, y)
		fields := loadShot("01-adaptive-fields-click")
		fy, ff, fb := tabs(fields)
		if fy != y || fields.Attributes[fy][ff+1] != selected || fields.Attributes[fy][fb+1] == selected || !matching(fields, true) {
			panic("mouse click missed the rendered adaptive Fields tab or changed the inspected packet")
		}
		if name := os.Getenv("SHARK_QA_SELECT_FIELD"); name != "" {
			fieldY := -1
			value := os.Getenv("SHARK_QA_SELECT_FIELD_VALUE")
			occurrence := 0
			if word := os.Getenv("SHARK_QA_SELECT_FIELD_OCCURRENCE"); word != "" {
				n, err := strconv.Atoi(word)
				fail(err)
				if n < 0 || n > 64 {
					panic("field occurrence outside bounded QA range")
				}
				occurrence = n
			}
			seen := 0
			for y, row := range fields.Rows {
				if strings.Contains(row, name+": ") && (value == "" || strings.Contains(row, name+": "+value)) {
					if seen < occurrence {
						seen++
						continue
					}
					fieldY = y
					break
				}
			}
			if fieldY < 0 {
				panic("requested field is not visible in the actual native pane")
			}
			click(8, fieldY)
			loadShot("01-selected-field")
		}
		if name := os.Getenv("SHARK_QA_BYTE_FIELD"); name != "" {
			want, err := hex.DecodeString(os.Getenv("SHARK_QA_BYTE_HEX"))
			fail(err)
			if len(want) == 0 || len(want) > 256 || fields.Width != 180 {
				panic("literal byte-value QA requires 1..256 bytes and the native 180-column viewport")
			}
			valueStart, readUntil := 0, len(want)
			mappedRange := os.Getenv("SHARK_QA_BYTE_RANGE")
			if mappedRange != "" {
				parts := strings.Split(mappedRange, ":")
				if len(parts) != 2 {
					panic("mapped byte QA requires an explicit start:end PDU interval")
				}
				valueStart, err = strconv.Atoi(parts[0])
				fail(err)
				readUntil, err = strconv.Atoi(parts[1])
				fail(err)
				if valueStart < 0 || readUntil-valueStart != len(want) || readUntil > 4096 {
					panic("mapped PDU interval does not match the independent field bytes")
				}
			}
			occurrence := 0
			if value := os.Getenv("SHARK_QA_BYTE_OCCURRENCE"); value != "" {
				occurrence, err = strconv.Atoi(value)
				fail(err)
				if occurrence < 0 || occurrence > 64 {
					panic("byte field occurrence outside bounded QA range")
				}
			}
			fieldY := -1
			seen := 0
			for row, text := range fields.Rows {
				if strings.Contains(text, name+": ") {
					if seen < occurrence {
						seen++
						continue
					}
					if !strings.Contains(text, fmt.Sprintf("%d bytes", len(want))) || !strings.Contains(text, "HEX") {
						panic("native binary field did not retain its literal length and hex representation")
					}
					fieldY = row
					break
				}
			}
			if fieldY < 0 {
				panic("requested literal byte field is not visible in the actual native field pane")
			}
			click(8, fieldY)
			state := loadShot("01-selected-byte-field")
			headerY, rightX := -1, -1
			for row, text := range state.Rows {
				if mappedRange != "" {
					if at := nativeBytePaneTitleColumn(text); at >= 0 && strings.Contains(text, "PDU") && strings.Contains(text, fmt.Sprintf("0x%x–0x%x · %d bytes", valueStart, readUntil-1, len(want))) {
						headerY, rightX = row, at
						break
					}
					continue
				}
				if at := strings.Index(text, "3 Extracted field value"); at >= 0 && strings.Contains(text, "frame offsets unavailable") {
					headerY, rightX = row, len(utf16.Encode([]rune(text[:at])))
					break
				}
			}
			if headerY < 0 {
				panic("selected field does not expose its expected extracted-value or PDU byte scope")
			}
			// The box title has its own padding. Locate the zero-offset hex
			// column in the actual first data row instead of reusing title X.
			line := utf16.Encode([]rune(state.Rows[headerY+1]))
			dataX, offsetDigits := nativeHexOffsetColumn(line, rightX)
			if dataX < 0 {
				panic("selected extracted bytes do not start at an observable zero-offset row")
			}
			rightX = dataX
			var observed []byte
			for _, text := range state.Rows[headerY+1:] {
				cells := utf16.Encode([]rune(text))
				if rightX >= len(cells) {
					continue
				}
				words := strings.Fields(string(utf16.Decode(cells[rightX:])))
				if len(words) < 2 || len(words[0]) != offsetDigits {
					continue
				}
				offset, err := strconv.ParseUint(words[0], 16, 32)
				if err != nil || int(offset) != len(observed) {
					continue
				}
				// The initial 180-column split uses sixteen octets per row.
				// Stop before the ASCII preview, even if it resembles hex.
				for _, word := range words[1:min(17, len(words))] {
					if len(observed) == readUntil || len(word) != 2 {
						break
					}
					part, err := hex.DecodeString(word)
					fail(err)
					observed = append(observed, part...)
				}
				if len(observed) == readUntil {
					break
				}
			}
			if len(observed) != readUntil || !bytes.Equal(observed[valueStart:], want) {
				panic("selected native hex cells differ from the independent original-wire field value")
			}
		}
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
	if os.Getenv("SHARK_QA_NATIVE_WIRE") == "1" {
		send("ah")
		shot("03-native-wire-stream")
		send("a")
	}
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
