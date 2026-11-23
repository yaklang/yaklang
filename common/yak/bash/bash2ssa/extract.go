package bash2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"

	"mvdan.cc/sh/v3/syntax"
)

const (
	// maxExtractShellBytes bounds AST work for one tool command. Larger input
	// falls back to the caller's raw-string path instead of stalling a call.
	maxExtractShellBytes = 64 * 1024
	maxExtractDepth      = 3
)

// ShellCommand is one simple command found in a shell source string. Text is
// the exact source span; EffectiveText is the command after inert/privilege
// wrappers (sudo, timeout, env, xargs, find -exec, ...) are removed. Nested
// commands come from static sh -c/eval payloads and must satisfy allow rules
// independently.
type ShellCommand struct {
	Text          string
	EffectiveText string
	Wrapper       string
	Nested        bool
}

// ExtractShellCommands parses src with the same mvdan/sh AST and wrapper tables
// used by the bash SSA frontend. It returns every simple command in source
// order, including static payloads reachable through sh -c, eval, xargs and
// find -exec.
func ExtractShellCommands(src string) ([]ShellCommand, error) {
	if len(src) > maxExtractShellBytes {
		return nil, utils.Errorf("bash command too large: %d bytes", len(src))
	}
	return extractShellCommands(src, 0)
}

func extractShellCommands(src string, depth int) ([]ShellCommand, error) {
	if depth > maxExtractDepth {
		return nil, nil
	}
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		return nil, utils.Wrap(err, "bash parse failed")
	}

	stmtByCall := make(map[*syntax.CallExpr]*syntax.Stmt)
	syntax.Walk(file, func(node syntax.Node) bool {
		stmt, ok := node.(*syntax.Stmt)
		if !ok || stmt == nil {
			return true
		}
		if call, ok := stmt.Cmd.(*syntax.CallExpr); ok && call != nil {
			stmtByCall[call] = stmt
		}
		return true
	})

	var commands []ShellCommand
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || call == nil || len(call.Args) == 0 {
			return true
		}
		commands = append(commands, extractCallCommands(src, call, stmtByCall[call], depth)...)
		return true
	})
	return commands, nil
}

func extractCallCommands(src string, call *syntax.CallExpr, stmt *syntax.Stmt, depth int) []ShellCommand {
	text := sourceRange(src, call.Pos(), call.End())
	stmtEnd := call.End()
	if stmt != nil {
		// Stmt includes redirections/background markers, which are part of the
		// permission rule's textual match even though CallExpr does not own them.
		text = sourceRange(src, stmt.Pos(), stmt.End())
		stmtEnd = stmt.End()
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	command := ShellCommand{Text: text}
	if len(call.Assigns) > 0 {
		// FOO=bar rm ...: the assignment is part of the source text but not the
		// effective command used by deny/ask rules.
		command.EffectiveText = sourceRange(src, call.Args[0].Pos(), stmtEnd)
	}

	args := call.Args
	var wrappers []string
	for len(args) > 0 {
		name, ok := staticWord(args[0])
		if !ok {
			break
		}
		base := commandBase(name)

		if base == "command" || base == "builtin" {
			if len(args) > 1 {
				if value, ok := staticWord(args[1]); ok && (value == "-v" || value == "-V") {
					break
				}
			}
		}

		switch base {
		case "sudo", "env", "timeout", "nice", "ionice", "nohup", "exec", "command", "builtin", "stdbuf":
			start := wrapperCommandIndex(base, args)
			if start <= 0 || start >= len(args) {
				break
			}
			wrappers = append(wrappers, base)
			args = args[start:]
			command.EffectiveText = sourceRange(src, args[0].Pos(), stmtEnd)
			continue
		case "xargs":
			start := xargsCommandIndex(args)
			if start <= 0 || start >= len(args) {
				break
			}
			wrappers = append(wrappers, "xargs")
			args = args[start:]
			command.EffectiveText = sourceRange(src, args[0].Pos(), stmtEnd)
			continue
		case "find":
			execStart, execEnd := findExecCommand(args)
			if execStart <= 0 || execStart >= execEnd || execEnd > len(args) {
				break
			}
			wrappers = append(wrappers, "find -exec")
			args = args[execStart:execEnd]
			command.EffectiveText = sourceRange(src, args[0].Pos(), args[len(args)-1].End())
			continue
		}
		break
	}
	if len(wrappers) > 0 {
		command.Wrapper = strings.Join(wrappers, " ")
	}

	var nested []ShellCommand
	if len(args) > 0 {
		if name, ok := staticWord(args[0]); ok {
			switch commandBase(name) {
			case "eval":
				if payload, ok := staticWordsText(args[1:]); ok && strings.TrimSpace(payload) != "" {
					nested = extractNestedCommands(payload, depth+1, "eval")
				}
			case "sh", "bash", "zsh", "dash", "ksh", "ash", "su":
				if index := commandFlagIndex(args, "-c"); index >= 0 && index+1 < len(args) {
					if payload, ok := staticWord(args[index+1]); ok && strings.TrimSpace(payload) != "" {
						nested = extractNestedCommands(payload, depth+1, commandBase(name))
					}
				}
			}
		}
	}

	return append([]ShellCommand{command}, nested...)
}

func extractNestedCommands(payload string, depth int, wrapper string) []ShellCommand {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}
	nested, err := extractShellCommands(payload, depth)
	if err != nil || len(nested) == 0 {
		return []ShellCommand{{Text: payload, Nested: true, Wrapper: wrapper}}
	}
	for i := range nested {
		nested[i].Nested = true
		if nested[i].Wrapper == "" {
			nested[i].Wrapper = wrapper
		}
	}
	return nested
}

func sourceRange(src string, start, end syntax.Pos) string {
	if !start.IsValid() || !end.IsValid() {
		return ""
	}
	left := int(start.Offset())
	right := int(end.Offset())
	if left < 0 || right < left || right > len(src) {
		return ""
	}
	return strings.TrimSpace(src[left:right])
}

func staticWordsText(words []*syntax.Word) (string, bool) {
	parts := make([]string, 0, len(words))
	for _, word := range words {
		value, ok := staticWord(word)
		if !ok {
			return "", false
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, " "), true
}
