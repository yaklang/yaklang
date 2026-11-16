package bash2ssa

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// wrapperCommandIndex returns the index of the command a wrapper executes, or
// -1 when the wrapper has no statically identifiable command. The option
// skipping is deliberately conservative: an unknown option is treated as a
// flag, which is good enough to surface the common `sudo rm -rf /` shape.
func wrapperCommandIndex(name string, args []*syntax.Word) int {
	if len(args) == 0 {
		return -1
	}
	i := 1

	switch name {
	case "env":
		for i < len(args) {
			token, ok := staticWord(args[i])
			if !ok {
				return i
			}
			if token == "--" {
				return i + 1
			}
			if !strings.HasPrefix(token, "-") || token == "-" {
				if strings.Contains(token, "=") {
					i++
					continue
				}
				return i
			}
			if envOptionTakesValue(token) {
				i += 2
			} else {
				i++
			}
		}
		return -1

	case "timeout":
		for i < len(args) {
			token, ok := staticWord(args[i])
			if !ok {
				return i
			}
			if token == "--" {
				i++
				break
			}
			if !strings.HasPrefix(token, "-") || token == "-" {
				break
			}
			if timeoutOptionTakesValue(token) {
				i += 2
			} else {
				i++
			}
		}
		// The first positional argument is the duration.
		if i < len(args) {
			i++
		}
		return i

	case "sudo", "nice", "ionice", "nohup", "exec", "command", "builtin", "stdbuf":
		for i < len(args) {
			token, ok := staticWord(args[i])
			if !ok {
				return i
			}
			if token == "--" {
				return i + 1
			}
			if !strings.HasPrefix(token, "-") || token == "-" {
				return i
			}
			if wrapperOptionTakesValue(name, token) {
				i += 2
			} else {
				i++
			}
		}
		return -1
	}
	return -1
}

func xargsCommandIndex(args []*syntax.Word) int {
	i := 1
	for i < len(args) {
		token, ok := staticWord(args[i])
		if !ok {
			return i
		}
		if token == "--" {
			return i + 1
		}
		if !strings.HasPrefix(token, "-") || token == "-" {
			return i
		}
		if xargsOptionTakesValue(token) {
			i += 2
		} else {
			i++
		}
	}
	return -1
}

// findExecCommand locates the command inside `find ... -exec cmd ... ;`.
func findExecCommand(args []*syntax.Word) (int, int) {
	for i := 1; i < len(args); i++ {
		token, ok := staticWord(args[i])
		if !ok {
			continue
		}
		switch token {
		case "-exec", "-execdir", "-ok", "-okdir":
			start := i + 1
			for j := start; j < len(args); j++ {
				end, ok := staticWord(args[j])
				if !ok {
					continue
				}
				if end == ";" || end == `\;` || end == "+" {
					return start, j
				}
			}
			return start, len(args)
		}
	}
	return -1, -1
}

func hasInlineValue(token string) bool {
	return strings.Contains(token, "=")
}

func envOptionTakesValue(token string) bool {
	if hasInlineValue(token) {
		return false
	}
	switch token {
	case "-u", "-C", "-S", "--unset", "--chdir", "--split-string":
		return true
	}
	return false
}

func timeoutOptionTakesValue(token string) bool {
	if hasInlineValue(token) {
		return false
	}
	switch token {
	case "-s", "-k", "--signal", "--kill-after":
		return true
	}
	return false
}

func wrapperOptionTakesValue(name, token string) bool {
	if hasInlineValue(token) {
		return false
	}
	switch name {
	case "sudo":
		switch token {
		case "-u", "-g", "-p", "-C", "-h", "-r", "-t", "-U", "-T",
			"--user", "--group", "--prompt", "--close-from", "--host",
			"--role", "--type", "--other-user", "--command-timeout":
			return true
		}
	case "nice":
		switch token {
		case "-n", "--adjustment":
			return true
		}
	case "ionice":
		switch token {
		case "-c", "-n", "-p", "--class", "--classdata", "--pid":
			return true
		}
	case "stdbuf":
		switch token {
		case "-i", "-o", "-e", "--input", "--output", "--error":
			return true
		}
	case "exec":
		return token == "-a"
	}
	return false
}

func xargsOptionTakesValue(token string) bool {
	if hasInlineValue(token) {
		return false
	}
	switch token {
	case "-I", "-L", "-n", "-P", "-s", "-d", "-E", "-a",
		"--replace", "--max-lines", "--max-args", "--max-procs",
		"--max-chars", "--delimiter", "--eof", "--arg-file":
		return true
	}
	return false
}
