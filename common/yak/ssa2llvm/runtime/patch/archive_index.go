package patch

// Reading the ar symbol index, so a script that needs a module the embedded
// libyak.a was not built with fails with that sentence instead of a linker
// crash.
//
// The in-process lld does not survive reporting an undefined symbol: it takes
// the whole compiler process down with SIGSEGV inside cgo, so the Go traceback
// the user sees names lld's cgo entry point and nothing about the actual
// problem. The only way to produce a readable message is to notice the
// situation before handing anything to lld.
//
// The check reads the archive's symbol index (the leading "/" member) rather
// than the 200 MB of objects behind it: it is the same table the linker
// resolves against, and it is a few tens of KB.

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// registerModulePrefix is the exported entry point genfull emits per module,
// and what the compiled script calls to bring a module in.
const registerModulePrefix = "yak_register_module_"

const (
	arMagic      = "!<arch>\n"
	arHeaderSize = 60
	arSizeOffset = 48
	arSizeLen    = 10
)

// MissingModuleRegistrations reports the yaklib modules objPath calls into that
// archivePath cannot provide. Asking the object rather than the compiler's
// used-module list keeps the check exact: it sees the same undefined symbols
// lld would, so it neither misses a module nor flags the split-only section
// groups (shared, ssafront) that never have a registration function.
//
// An empty result means every module resolves — or that the archive carries no
// symbol index, in which case the check is skipped rather than guessed at.
func MissingModuleRegistrations(objPath, archivePath string) ([]string, error) {
	needed, err := requestedModules(objPath)
	if err != nil || len(needed) == 0 {
		return nil, err
	}
	available, err := readArchiveModuleIndex(archivePath)
	if err != nil || available == nil {
		return nil, err
	}
	var missing []string
	for _, m := range needed {
		if !available[m] {
			missing = append(missing, m)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

// requestedModules lists the modules an object expects to be registered, by
// reading the undefined yak_register_module_* symbols it references.
func requestedModules(objPath string) ([]string, error) {
	f, err := elf.Open(objPath)
	if err != nil {
		return nil, fmt.Errorf("open object: %w", err)
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		// An object with no symbol table references nothing to check.
		return nil, nil
	}
	var out []string
	for _, sym := range syms {
		if sym.Section == elf.SHN_UNDEF && strings.HasPrefix(sym.Name, registerModulePrefix) {
			out = append(out, strings.TrimPrefix(sym.Name, registerModulePrefix))
		}
	}
	return out, nil
}

// ArchiveModules lists the yaklib modules the archive can register, for error
// messages. It returns nil when the archive has no symbol index.
func ArchiveModules(archivePath string) ([]string, error) {
	available, err := readArchiveModuleIndex(archivePath)
	if err != nil || available == nil {
		return nil, err
	}
	out := make([]string, 0, len(available))
	for m := range available {
		out = append(out, m)
	}
	sort.Strings(out)
	return out, nil
}

// readArchiveModuleIndex returns the module names named by the archive's
// symbol index, or nil when there is no index member.
func readArchiveModuleIndex(archivePath string) (map[string]bool, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	header := make([]byte, len(arMagic)+arHeaderSize)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, fmt.Errorf("read archive header: %w", err)
	}
	if string(header[:len(arMagic)]) != arMagic {
		return nil, fmt.Errorf("not a GNU ar archive: %s", archivePath)
	}
	member := header[len(arMagic):]
	// The GNU symbol index is the first member and is named "/". Anything
	// else means this archive was built without one.
	if strings.TrimRight(string(member[:16]), " ") != "/" {
		return nil, nil
	}
	size, err := parseARSize(member)
	if err != nil {
		return nil, err
	}
	index := make([]byte, size)
	if _, err := io.ReadFull(f, index); err != nil {
		return nil, fmt.Errorf("read archive symbol index: %w", err)
	}
	// Layout: big-endian symbol count, one big-endian member offset per
	// symbol, then that many NUL-terminated names.
	if len(index) < 4 {
		return nil, fmt.Errorf("archive symbol index is truncated")
	}
	count := int(binary.BigEndian.Uint32(index))
	namesAt := 4 + count*4
	if namesAt > len(index) {
		return nil, fmt.Errorf("archive symbol index claims %d symbols but is %d bytes", count, len(index))
	}

	modules := map[string]bool{}
	for _, name := range strings.Split(string(index[namesAt:]), "\x00") {
		// Each module also has a _cgoexp_<hash>_-prefixed wrapper; matching
		// on the suffix would count those twice, so anchor at the start.
		if strings.HasPrefix(name, registerModulePrefix) {
			modules[strings.TrimPrefix(name, registerModulePrefix)] = true
		}
	}
	return modules, nil
}

// parseARSize reads the decimal ar_size field of an ar member header.
func parseARSize(header []byte) (int, error) {
	field := strings.TrimRight(string(header[arSizeOffset:arSizeOffset+arSizeLen]), " ")
	size := 0
	for _, c := range field {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad ar member size %q", field)
		}
		size = size*10 + int(c-'0')
	}
	return size, nil
}
