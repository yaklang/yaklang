package trace

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

var Log = log.GetLogger("ssa2llvmTrace").SetLevel("disable").SetOutput(os.Stderr)

func SetEnabled(v bool) {
	if v {
		Log.SetLevel("info")
		return
	}
	Log.SetLevel("disable")
}

func PrintWorkDir(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	Log.Infof("WORK=%s", dir)
}

func PrintCmd(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}

	var b strings.Builder
	if strings.TrimSpace(cmd.Dir) != "" {
		b.WriteString("cd ")
		b.WriteString(quoteArg(cmd.Dir))
		b.WriteString(" && ")
	}
	if cmd.Path != "" {
		b.WriteString(quoteArg(cmd.Path))
	} else if len(cmd.Args) > 0 {
		b.WriteString(quoteArg(cmd.Args[0]))
	}
	for i := 1; i < len(cmd.Args); i++ {
		b.WriteByte(' ')
		b.WriteString(quoteArg(cmd.Args[i]))
	}
	Log.Infof("%s", strings.TrimSpace(b.String()))
}

func Printf(format string, args ...any) {
	format = strings.TrimSpace(format)
	if format == "" {
		return
	}
	Log.Infof(format, args...)
}

// PrintLink prints a synthetic command line for an in-process tool invocation
// (e.g. the in-process lld link or TargetMachine emit), so -x output clearly
// shows what ran even though no subprocess was spawned. prog is the tool name
// (e.g. "ld.lld (in-process)"); args are its arguments, quoted like PrintCmd.
func PrintLink(prog string, args []string) {
	prog = strings.TrimSpace(prog)
	if prog == "" {
		return
	}
	var b strings.Builder
	b.WriteString(quoteArg(prog))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(quoteArg(a))
	}
	Log.Infof("%s", strings.TrimSpace(b.String()))
}

func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	// Keep common args readable; quote when whitespace or shell-sensitive chars exist.
	if strings.IndexFunc(s, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '"', '\'', '\\', '$', '`', '!', '&', '|', ';', '<', '>', '(', ')', '{', '}', '[', ']', '*', '?':
			return true
		default:
			return false
		}
	}) == -1 {
		return s
	}
	return strconv.Quote(s)
}
