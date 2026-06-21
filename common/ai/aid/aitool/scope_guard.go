package aitool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Environment variable names used by code_security_audit (and potentially other
// focus loops) to declare the allowed filesystem scope for AI tools.
const (
	EnvAuditTargetPath = "YAK_AI_AUDIT_TARGET_PATH"
	EnvAuditWorkDir    = "YAK_AI_AUDIT_WORK_DIR"
)

// isPathUnderRoots reports whether p is equal to one of the roots or contained
// within one of them. It uses filepath.Rel so that partial prefix matches are
// avoided.
func isPathUnderRoots(p string, roots []string) bool {
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if abs == rootAbs {
			return true
		}
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

// allowedAuditRoots returns the list of paths that AI tools are allowed to
// touch when a focus loop has declared a scope. An empty slice means "no
// restriction".
func allowedAuditRoots() []string {
	var explicit []string
	for _, env := range []string{EnvAuditTargetPath, EnvAuditWorkDir} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			explicit = append(explicit, v)
		}
	}
	// No explicit scope declared: remain permissive so other AI flows are not
	// affected by the audit scope guard.
	if len(explicit) == 0 {
		return nil
	}

	var roots []string
	roots = append(roots, explicit...)
	// Always allow common temp directories so caching/intermediate files work.
	for _, tmp := range []string{os.TempDir(), "/tmp", "/var/tmp"} {
		if tmp != "" {
			roots = append(roots, tmp)
		}
	}
	// Allow standard system binary directories so absolute paths to common
	// executables (e.g. /usr/bin/python3) do not trigger false positives.
	roots = append(roots, "/bin", "/sbin", "/usr/bin", "/usr/sbin",
		"/usr/local/bin", "/usr/local/sbin", "/opt")
	return roots
}

// pathParamNames lists the parameter names that commonly carry filesystem paths
// in built-in tools.
var pathParamNames = []string{
	"file", "path", "dir", "dirname", "src", "dst", "source", "target",
}

// extractCommandAbsolutePaths extracts absolute filesystem paths from a shell
// command string. It handles simple quoting and whitespace splitting. This is
// intentionally best-effort; it is meant to catch obviously out-of-scope paths
// like `grep -rl ... /home/user/other-project`.
var absPathRe = regexp.MustCompile("(?:^|[\\s\"'])(/[^\\s\"'<>|&;\\$\\`\\(\\)]+)")

func extractCommandAbsolutePaths(cmd string) []string {
	var paths []string
	for _, m := range absPathRe.FindAllStringSubmatch(cmd, -1) {
		p := strings.TrimSpace(m[1])
		if p == "" {
			continue
		}
		// Trim trailing punctuation that may have been captured.
		p = strings.TrimRight(p, ":;,.!?")
		paths = append(paths, p)
	}
	return paths
}

// CheckToolScope verifies that a tool invocation stays within the declared
// audit scope. It is a no-op when no scope has been declared.
func CheckToolScope(toolName string, params map[string]any) error {
	roots := allowedAuditRoots()
	if len(roots) == 0 {
		return nil
	}

	// Filesystem tools with explicit path parameters.
	for _, name := range pathParamNames {
		raw, ok := params[name]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok || s == "" {
			continue
		}
		if filepath.IsAbs(s) && !isPathUnderRoots(s, roots) {
			return fmt.Errorf(
				"scope guard: tool %q parameter %q references absolute path %q which is outside the allowed audit scope (%v). "+
					"All file operations must stay under the target project or the audit working directory.",
				toolName, name, s, roots[:len(roots)-7])
		}
	}

	// Shell tools: inspect the command string for absolute paths.
	if toolName == "bash" || toolName == "cmd" || toolName == "powershell" {
		raw, ok := params["command"]
		if !ok {
			return nil
		}
		cmd, ok := raw.(string)
		if !ok || cmd == "" {
			return nil
		}
		for _, p := range extractCommandAbsolutePaths(cmd) {
			if !isPathUnderRoots(p, roots) {
				return fmt.Errorf(
					"scope guard: tool %q command references absolute path %q which is outside the allowed audit scope (%v). "+
						"Use relative paths or paths under the target project only.",
					toolName, p, roots[:len(roots)-7])
			}
		}
	}

	return nil
}
