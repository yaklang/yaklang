package linkprep

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

// RewriteArchives applies manifest symbol renames to each static archive using
// ar + objcopy. Returns new archive paths in the same order as inputs.
func RewriteArchives(inputs []string, manifest map[string]string, workDir string, traceEnabled bool) (outputs []string, cleanup func(), err error) {
	if len(manifest) == 0 {
		return inputs, func() {}, nil
	}
	if strings.TrimSpace(workDir) == "" {
		return nil, nil, fmt.Errorf("linkprep: empty workDir")
	}
	baseDir := filepath.Join(workDir, "linkprep-archives")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("linkprep: mkdir: %w", err)
	}

	objcopy := findObjcopy()
	arTool := findAr()
	if objcopy == "" {
		return nil, nil, fmt.Errorf("linkprep: llvm-objcopy/objcopy not found in PATH")
	}
	if arTool == "" {
		return nil, nil, fmt.Errorf("linkprep: llvm-ar/ar not found in PATH")
	}

	outputs = make([]string, 0, len(inputs))
	for i, inPath := range inputs {
		inPath = filepath.Clean(inPath)
		if _, stat := os.Stat(inPath); stat != nil {
			return nil, cleanupAll(baseDir), fmt.Errorf("linkprep: stat %q: %w", inPath, stat)
		}
		sub := filepath.Join(baseDir, fmt.Sprintf("a%d", i))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return nil, cleanupAll(baseDir), err
		}
		outArc := filepath.Join(sub, filepath.Base(inPath))
		if err := rewriteOneArchive(inPath, outArc, sub, manifest, objcopy, arTool, traceEnabled); err != nil {
			return nil, cleanupAll(baseDir), fmt.Errorf("linkprep: rewrite %q: %w", inPath, err)
		}
		copyLinkflagsIfPresent(filepath.Dir(inPath), sub)
		outputs = append(outputs, outArc)
	}

	return outputs, cleanupAll(baseDir), nil
}

func cleanupAll(dir string) func() {
	return func() { _ = os.RemoveAll(dir) }
}

func rewriteOneArchive(inPath, outArc, extractDir string, manifest map[string]string, objcopy, arTool string, traceEnabled bool) error {
	// Extract members into extractDir
	cmd := exec.Command(arTool, "x", inPath)
	cmd.Dir = extractDir
	traceMaybe(cmd, traceEnabled)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ar x: %w\n%s", err, out)
	}

	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return err
	}
	var objects []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".o") {
			objects = append(objects, name)
		}
	}
	sort.Strings(objects)
	if len(objects) == 0 {
		return fmt.Errorf("no .o members in %s", inPath)
	}

	for _, name := range objects {
		objPath := filepath.Join(extractDir, name)
		args, err := objcopyArgsForObject(objPath, manifest, objcopy)
		if err != nil {
			return err
		}
		if len(args) == 0 {
			continue
		}
		tmp := objPath + ".tmp.o"
		run := append(append([]string{}, args...), objPath, tmp)
		c := exec.Command(objcopy, run...)
		traceMaybe(c, traceEnabled)
		out, err := c.CombinedOutput()
		if err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("objcopy %s: %w\n%s", name, err, out)
		}
		if err := os.Rename(tmp, objPath); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}

	// Repack
	_ = os.Remove(outArc)
	args := append([]string{"rcs", outArc}, objects...)
	cmd = exec.Command(arTool, args...)
	cmd.Dir = extractDir
	traceMaybe(cmd, traceEnabled)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ar rcs: %w\n%s", err, out)
	}
	return nil
}

func objcopyArgsForObject(objPath string, manifest map[string]string, objcopy string) ([]string, error) {
	nm := findNm()
	if nm == "" {
		return nil, fmt.Errorf("linkprep: nm/llvm-nm not found in PATH")
	}
	// List globals including undefined references so callers (e.g. C stubs) are
	// rewritten in lockstep with definitions in other archive members.
	cmd := exec.Command(nm, "-g", objPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nm %s: %w", objPath, err)
	}
	present := parseNmSymbolNames(string(out))
	var args []string
	for old, newName := range manifest {
		if newName == "" || old == newName {
			continue
		}
		if present[old] {
			args = append(args, "--redefine-sym="+old+"="+newName)
		}
	}
	sort.Strings(args)
	return args, nil
}

// parseNmSymbolNames returns every global symbol name mentioned in nm -g output
// (defined or undefined) for one object file.
func parseNmSymbolNames(nmOut string) map[string]bool {
	m := make(map[string]bool)
	for _, line := range strings.Split(nmOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, ":") && !strings.Contains(line, " ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var sym string
		switch {
		case len(fields) >= 3 && nmLooksLikeHex(fields[0]) && len(fields[1]) == 1:
			// e.g. 0000000000000000 T foo  or 0000000000000000 U bar
			sym = fields[len(fields)-1]
		case len(fields) == 2 && len(fields[0]) == 1 && nmSymbolTypeLetter(fields[0]):
			// e.g. "U yak_internal_release_shadow" (undefined external)
			sym = fields[1]
		case len(fields) >= 2 && len(fields[1]) == 1 && !nmLooksLikeHex(fields[0]):
			// POSIX nm -P style: name type value ...
			sym = fields[0]
		case len(fields) >= 3:
			sym = fields[len(fields)-1]
		default:
			continue
		}
		if sym != "" {
			m[sym] = true
		}
	}
	return m
}

func parseNmDefined(nmOut string) map[string]bool {
	m := make(map[string]bool)
	for _, line := range strings.Split(nmOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// `nm` on archives prints member headers like `go.o:` (no spaces).
		if strings.HasSuffix(line, ":") && !strings.Contains(line, " ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Common layouts:
		//   <addr> <type> <sym>  (GNU / LLVM on ELF)
		//   <sym> <type> <value> ...  (POSIX `nm -P`, if used)
		if len(fields) >= 3 && nmLooksLikeHex(fields[0]) && len(fields[1]) == 1 && nmDefinedType(fields[1]) {
			sym := fields[len(fields)-1]
			if sym != "" {
				m[sym] = true
			}
			continue
		}
		if len(fields) >= 2 && len(fields[1]) == 1 && nmDefinedType(fields[1]) && !nmLooksLikeHex(fields[0]) {
			m[fields[0]] = true
			continue
		}
		// Fallback: last token (older parser), require a defined-type column before it.
		if len(fields) < 3 {
			continue
		}
		typeTok := fields[len(fields)-2]
		if len(typeTok) != 1 || !nmDefinedType(typeTok) {
			continue
		}
		sym := fields[len(fields)-1]
		if sym != "" {
			m[sym] = true
		}
	}
	return m
}

func nmLooksLikeHex(s string) bool {
	if len(s) < 4 {
		return false
	}
	for _, r := range s {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			continue
		}
		return false
	}
	return true
}

func nmDefinedType(s string) bool {
	if len(s) != 1 {
		return false
	}
	switch s {
	case "T", "t", "D", "d", "B", "b", "R", "r", "W", "w", "A", "a", "N", "n", "V", "v", "C", "c":
		return true
	default:
		return false
	}
}

// nmSymbolTypeLetter matches a one-character nm symbol type (including undefined "U").
func nmSymbolTypeLetter(s string) bool {
	if len(s) != 1 {
		return false
	}
	switch s {
	case "U", "u", "T", "t", "D", "d", "B", "b", "R", "r", "W", "w", "A", "a", "N", "n", "V", "v", "C", "c":
		return true
	default:
		return false
	}
}

func copyLinkflagsIfPresent(srcDir, dstDir string) {
	p := filepath.Join(srcDir, "libyak.linkflags")
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dstDir, "libyak.linkflags"), data, 0o644)
}

func findObjcopy() string {
	for _, t := range []string{"llvm-objcopy", "objcopy"} {
		if p, err := exec.LookPath(t); err == nil {
			return p
		}
	}
	return ""
}

func findAr() string {
	for _, t := range []string{"llvm-ar", "ar"} {
		if p, err := exec.LookPath(t); err == nil {
			return p
		}
	}
	return ""
}

func findNm() string {
	for _, t := range []string{"llvm-nm", "nm"} {
		if p, err := exec.LookPath(t); err == nil {
			return p
		}
	}
	return ""
}

func traceMaybe(cmd *exec.Cmd, enabled bool) {
	if enabled {
		trace.PrintCmd(cmd)
	}
}

// CopyFile is a small helper for tests.
func CopyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
