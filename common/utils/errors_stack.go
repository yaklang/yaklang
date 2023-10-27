package utils

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	depth = 32
)

var (
	currentAbsPath string = getCurrentAbsPathByExecutable()
	skipNum        int    = 3
	skipBottomNum  int    = 2
)

// Frame represents a program counter inside a stack frame.
// For historical reasons if Frame is interpreted as a uintptr
// its value represents the program counter + 1.
type Frame uintptr

// pc returns the program counter for this frame;
// multiple frames may have the same PC value.
func (f Frame) pc() uintptr { return uintptr(f) - 1 }

// file returns the full path to the file that contains the
// function for this Frame's pc.
func (f Frame) file() string {
	fn := runtime.FuncForPC(f.pc())
	if fn == nil {
		return ""
	}
	file, _ := fn.FileLine(f.pc())
	return file
}

// line returns the line number of source code of the
// function for this Frame's pc.
func (f Frame) line() int {
	fn := runtime.FuncForPC(f.pc())
	if fn == nil {
		return 0
	}
	_, line := fn.FileLine(f.pc())
	return line
}

// name returns the name of this function, if known.
func (f Frame) name() string {
	fn := runtime.FuncForPC(f.pc())
	if fn == nil {
		return ""
	}
	return fn.Name()
}

// Format formats the frame according to the fmt.Formatter interface.
//
//	%s    source file
//	%d    source line
//	%n    function name
//	%v    equivalent to %s:%d
//
// Format accepts flags that alter the printing of some verbs, as follows:
//
//	%+s   function name and path of source file relative to the compile time
//	      GOPATH separated by \n\t (<funcname>\n\t<path>)
//	%+v   equivalent to %+s:%d
func (f Frame) Format(s fmt.State, verb rune) {
	switch verb {
	case 's':
		switch {
		case s.Flag('+'):
			if f == 0 {
				break
			}
			io.WriteString(s, f.name())
			io.WriteString(s, "\n\t")
			io.WriteString(s, filename(f.file()))
		default:
			io.WriteString(s, filename(path.Base(f.file())))
		}
	case 'd':
		io.WriteString(s, strconv.Itoa(f.line()))
	case 'n':
		io.WriteString(s, funcname(f.name()))
	case 'v':
		f.Format(s, 's')
		io.WriteString(s, ":")
		f.Format(s, 'd')
	}
}

// MarshalText formats a stacktrace Frame as a text string. The output is the
// same as that of fmt.Sprintf("%+v", f), but without newlines or tabs.
func (f Frame) MarshalText() ([]byte, error) {
	name := f.name()
	if name == "" {
		return []byte(name), nil
	}
	return []byte(fmt.Sprintf("%s %s:%d", name, filename(f.file()), f.line())), nil
}

// StackTrace is stack of Frames from innermost (newest) to outermost (oldest).
type StackTrace []Frame

// Format formats the stack of Frames according to the fmt.Formatter interface.
//
//	%s	lists source files for each Frame in the stack
//	%v	lists the source file and line number for each Frame in the stack
//
// Format accepts flags that alter the printing of some verbs, as follows:
//
//	%+v   Prints filename, function, and line number for each Frame in the stack.
func (st StackTrace) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		switch {
		case s.Flag('+'):
			for _, f := range st {
				if f == 0 {
					fmt.Fprintf(s, "\n--------------------------------------------------")
					continue
				}
				io.WriteString(s, "\n")
				f.Format(s, verb)
			}
		case s.Flag('#'):
			fmt.Fprintf(s, "%#v", []Frame(st))
		default:
			st.formatSlice(s, verb)
		}
	case 's':
		st.formatSlice(s, verb)
	}
}

// formatSlice will format this StackTrace into the given buffer as a slice of
// Frame, only valid when called with '%s' or '%v'.
func (st StackTrace) formatSlice(s fmt.State, verb rune) {
	io.WriteString(s, "[")
	for i, f := range st {
		if i > 0 {
			io.WriteString(s, " ")
		}
		f.Format(s, verb)
	}
	io.WriteString(s, "]")
}

// stack represents a stack of program counters.
type stack struct {
	st    []uintptr
	hashs map[[20]byte]struct{}
}

func (s *stack) Format(st fmt.State, verb rune) {
	switch verb {
	case 'v':
		switch {
		case st.Flag('+'):
			for _, pc := range s.st {
				if pc == 0 {
					fmt.Fprintf(st, "\n--------------------------------------------------")
					continue
				}
				f := Frame(pc)
				fmt.Fprintf(st, "\n%+v", f)
			}
		}
	}
}

func (s *stack) StackTrace() StackTrace {
	vs := *s
	lenOfStack := len(vs.st)

	f := make([]Frame, lenOfStack)
	for i := 0; i < lenOfStack; i++ {
		f[i] = Frame(vs.st[i])
	}
	return f
}

func callers() *stack {
	var pcs [depth]uintptr
	n := runtime.Callers(skipNum, pcs[:])
	if n-skipBottomNum < 0 {
		return &stack{hashs: make(map[[20]byte]struct{})}
	} else {
		return &stack{st: pcs[0 : n-skipBottomNum], hashs: make(map[[20]byte]struct{})}
	}
}

func (st *stack) appendCurrentFrame() {
	pc, file, line, ok := runtime.Caller(skipNum - 1)
	if ok && st.hash(file, line) {
		st.st = append([]uintptr{pc}, st.st...)
	}
}

func (st *stack) appendEmptyFrame() {
	st.st = append(st.st, 0)
}

func (st *stack) appendStack(otherStack *stack) {
	st.st = append(st.st, otherStack.st...)
}

func (st *stack) hash(file string, line int) bool {
	var ok bool
	s := fmt.Sprintf("%s:%d", file, line)
	raw := sha1.Sum(UnsafeStringToBytes(s))
	if _, ok = st.hashs[raw]; !ok {
		st.hashs[raw] = struct{}{}
	}
	return ok
}

// SetSkipFrameNum set the number of frames to skip, default is 3
func SetSkipFrameNum(skip int) {
	skipNum = skip
}

// SetSkipBottomFrameNum set the number of frames to skip from bottom, default is 2
func SetSkipBottomFrameNum(skip int) {
	skipBottomNum = skip
}

// SetCurrentAbsPath set absolute path as current project path, ff you pass a string parameter, use this string as the absolute path of the project
func SetCurrentAbsPath(path ...string) {
	if len(path) == 0 {
		dir := getCurrentAbsPathByExecutable()
		tmpDir, _ := filepath.EvalSymlinks(os.TempDir())
		if strings.Contains(dir, tmpDir) {
			dir = getCurrentAbsPathByCaller()
		}
		currentAbsPath = dir
	} else {
		currentAbsPath = path[0]
	}

}

func getCurrentAbsPathByExecutable() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	res, _ := filepath.EvalSymlinks(filepath.Dir(exePath))
	return res
}

func getCurrentAbsPathByCaller() string {
	var abPath string
	_, filename, _, ok := runtime.Caller(2)
	if ok {
		abPath = path.Dir(filename)
	}
	return abPath
}

// filename removes the path prefix component of a file name reported by func.file().
func filename(path string) string {
	relPath, err := filepath.Rel(currentAbsPath, path)
	if err != nil || strings.Contains(relPath, "../") || strings.Contains(relPath, "..\\") {
		return path
	}
	return relPath
}

// funcname removes the path prefix component of a function's name reported by func.Name().
func funcname(name string) string {
	i := strings.LastIndex(name, "/")
	name = name[i+1:]
	i = strings.Index(name, ".")
	return name[i+1:]
}
