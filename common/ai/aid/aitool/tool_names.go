package aitool

import "strings"

// BashToolName is the built-in shell tool backed by bash.yak. Guard code uses
// it instead of repeating the literal in every package.
const BashToolName = "bash"

// IsBashShell reports whether a shell parameter selects the bash grammar. An
// empty value follows bash.yak's bash-first default.
func IsBashShell(shell string) bool {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "", BashToolName, BashToolName + ".exe":
		return true
	default:
		return false
	}
}
