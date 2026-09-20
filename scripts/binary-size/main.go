// Command binary-size inventories a stripped Go ELF or Mach-O executable.
// It uses the runtime's pclntab, so release builds need no debug symbols.
package main

import (
	"debug/buildinfo"
	"debug/elf"
	"debug/gosym"
	"debug/macho"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type section struct {
	Name  string
	Bytes uint64
}
type component struct {
	Name      string
	CodeBytes uint64
	Functions int
}
type resource struct {
	Package, File string
	Bytes         int64
}
type dependency struct {
	ImportPath, Dir     string
	Imports, EmbedFiles []string
	Module              *struct{ Path string }
}
type report struct {
	Binary                   string
	FileBytes                int64
	GoVersion                string
	BuildSettings            map[string]string
	Sections                 []section
	SectionBytes             uint64
	OtherFileBytes           uint64
	PCLNSubtables            []section
	GoCodeBytes              uint64
	OtherCodeBytes           uint64
	Packages                 []component
	Subsystems               []component
	Modules                  []component
	EmbeddedResources        []resource
	EmbeddedInputBytes       int64
	SlimDependencyViolations []string `json:",omitempty"`
	Notes                    []string
}

var forbiddenSlimPrefixes = []string{
	"common/yak/c2ssa", "common/yak/antlr4c/parser",
	"common/yak/csharp/csharp2ssa", "common/yak/csharp/parser",
	"common/yak/go2ssa", "common/yak/antlr4go/parser",
	"common/yak/java/java2ssa", "common/yak/java/parser",
	"common/yak/php/php2ssa", "common/yak/php/parser",
	"common/yak/python/python2ssa", "common/yak/python/parser",
	"common/yak/typescript/ts2ssa", "common/yak/typescript/frontend",
	"common/yak/ssa_compile", "common/yak/syntaxflow_scan",
}

func main() {
	binaryPath := flag.String("binary", "", "stripped Go ELF/Mach-O executable (optional for dependency-only checks)")
	depsPath := flag.String("deps", "", "JSON stream from go list -deps -json with the same build tags")
	checkSlim := flag.Bool("check-slim", false, "fail if the dependency graph includes excluded SSA frontends/compiler commands")
	flag.Parse()
	if err := run(*binaryPath, *depsPath, *checkSlim); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(binaryPath, depsPath string, checkSlim bool) error {
	r := report{BuildSettings: map[string]string{}, Notes: []string{
		"SectionBytes + OtherFileBytes equals the executable's file size; zero-fill/BSS is excluded.",
		"Package CodeBytes counts Go function PC spans, including alignment, not total package footprint. Metadata and shared read-only data are reported separately; do not add embedded inputs to sections.",
		"EmbeddedInputBytes inventories go:embed files in the supplied dependency graph. Linker elimination/deduplication can change bytes retained in the executable; ordinary Go literals and generated tables are not included.",
		"A dependency-only check verifies imports, not runtime functionality. Use the same commit, toolchain, platform and tags for binary and dependency inputs.",
	}}
	if binaryPath != "" {
		var err error
		r, err = inspect(binaryPath, r)
		if err != nil {
			return err
		}
	}
	if depsPath != "" {
		deps, err := readDependencies(depsPath)
		if err != nil {
			return err
		}
		modulePaths := make(map[string]string, len(deps))
		for _, d := range deps {
			if d.Module != nil {
				modulePaths[d.ImportPath] = d.Module.Path
			}
			if forbiddenSlimPackage(d.ImportPath) {
				r.SlimDependencyViolations = append(r.SlimDependencyViolations, d.ImportPath)
			}
			for _, f := range d.EmbedFiles {
				info, err := os.Stat(filepath.Join(d.Dir, f))
				if err != nil {
					return err
				}
				r.EmbeddedResources = append(r.EmbeddedResources, resource{d.ImportPath, f, info.Size()})
				r.EmbeddedInputBytes += info.Size()
			}
		}
		modules := map[string]*component{}
		for _, p := range r.Packages {
			name := modulePaths[p.Name]
			if name == "" {
				name = "[standard library, main, or compiler-generated]"
			}
			if modules[name] == nil {
				modules[name] = &component{Name: name}
			}
			modules[name].CodeBytes += p.CodeBytes
			modules[name].Functions += p.Functions
		}
		r.Modules = sortedComponents(modules)
		sort.Strings(r.SlimDependencyViolations)
		sort.Slice(r.EmbeddedResources, func(i, j int) bool {
			a, b := r.EmbeddedResources[i], r.EmbeddedResources[j]
			if a.Bytes != b.Bytes {
				return a.Bytes > b.Bytes
			}
			return a.Package+"/"+a.File < b.Package+"/"+b.File
		})
	}
	if checkSlim && depsPath == "" {
		return fmt.Errorf("-check-slim requires -deps")
	}
	if binaryPath == "" && depsPath == "" {
		return fmt.Errorf("provide -binary and/or -deps")
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(r); err != nil {
		return err
	}
	if checkSlim && len(r.SlimDependencyViolations) > 0 {
		return fmt.Errorf("slim imports excluded compiler packages: %s", strings.Join(r.SlimDependencyViolations, ", "))
	}
	return nil
}

func inspect(name string, r report) (report, error) {
	info, err := os.Stat(name)
	if err != nil {
		return r, err
	}
	r.Binary = name
	r.FileBytes = info.Size()
	bi, err := buildinfo.ReadFile(name)
	if err != nil {
		return r, err
	}
	r.GoVersion = bi.GoVersion
	for _, s := range bi.Settings {
		r.BuildSettings[s.Key] = s.Value
	}
	var pcln []byte
	var textStart, textBytes uint64
	if f, err := macho.Open(name); err == nil {
		defer f.Close()
		for _, s := range f.Sections {
			// S_ZEROFILL, S_GB_ZEROFILL and S_THREAD_LOCAL_ZEROFILL have no file bytes.
			typ := s.Flags & 0xff
			if typ == 1 || typ == 12 || typ == 18 {
				continue
			}
			r.Sections = append(r.Sections, section{s.Seg + "/" + s.Name, s.Size})
			r.SectionBytes += s.Size
			if s.Name == "__gopclntab" {
				pcln, err = s.Data()
				if err != nil {
					return r, err
				}
			}
			if s.Seg == "__TEXT" && s.Name == "__text" {
				textStart, textBytes = s.Addr, s.Size
			}
		}
	} else if f, err := elf.Open(name); err == nil {
		defer f.Close()
		for _, s := range f.Sections {
			if s.Type == elf.SHT_NOBITS || s.Type == elf.SHT_NULL {
				continue
			}
			r.Sections = append(r.Sections, section{s.Name, s.FileSize})
			r.SectionBytes += s.FileSize
			if s.Name == ".gopclntab" {
				pcln, err = s.Data()
				if err != nil {
					return r, err
				}
			}
			if s.Name == ".text" {
				textStart, textBytes = s.Addr, s.Size
			}
		}
	} else {
		return r, fmt.Errorf("%s: expected an ELF or Mach-O executable", name)
	}
	if r.SectionBytes > uint64(r.FileBytes) {
		return r, fmt.Errorf("section accounting exceeds file size")
	}
	r.OtherFileBytes = uint64(r.FileBytes) - r.SectionBytes
	if len(pcln) == 0 || textStart == 0 {
		return r, fmt.Errorf("Go pclntab/text section not found")
	}
	table, err := gosym.NewTable(nil, gosym.NewLineTable(pcln, textStart))
	if err != nil {
		return r, err
	}
	if len(table.Funcs) == 0 {
		return r, fmt.Errorf("no Go functions decoded from pclntab")
	}
	pkgs := map[string]*component{}
	groups := map[string]*component{}
	for _, f := range table.Funcs {
		if f.End < f.Entry || f.Entry < textStart || f.End > textStart+textBytes {
			return r, fmt.Errorf("invalid function span for %s", f.Name)
		}
		n := f.End - f.Entry
		r.GoCodeBytes += n
		pkg := f.PackageName()
		if unescaped, err := url.PathUnescape(pkg); err == nil {
			pkg = unescaped
		}
		if pkg == "" {
			pkg = "[compiler-generated]"
		}
		add(pkgs, pkg, n)
		add(groups, subsystem(pkg), n)
	}
	if r.GoCodeBytes > textBytes {
		return r, fmt.Errorf("Go function spans exceed text section")
	}
	r.OtherCodeBytes = textBytes - r.GoCodeBytes
	r.Packages = sortedComponents(pkgs)
	r.Subsystems = sortedComponents(groups)
	r.PCLNSubtables = pclnSubtables(pcln)
	return r, nil
}

func add(m map[string]*component, name string, n uint64) {
	if m[name] == nil {
		m[name] = &component{Name: name}
	}
	m[name].CodeBytes += n
	m[name].Functions++
}
func sortedComponents(m map[string]*component) []component {
	out := make([]component, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CodeBytes != out[j].CodeBytes {
			return out[i].CodeBytes > out[j].CodeBytes
		}
		return out[i].Name < out[j].Name
	})
	return out
}
func subsystem(pkg string) string {
	const root = "github.com/yaklang/yaklang/"
	if strings.HasPrefix(pkg, root) {
		p := strings.TrimPrefix(pkg, root)
		parts := strings.Split(p, "/")
		if len(parts) >= 3 && parts[0] == "common" && parts[1] == "yak" {
			return strings.Join(parts[:3], "/")
		}
		if len(parts) >= 2 && parts[0] == "common" {
			return strings.Join(parts[:2], "/")
		}
		return parts[0]
	}
	if pkg == "[compiler-generated]" {
		return pkg
	}
	first := strings.Split(pkg, "/")[0]
	if !strings.Contains(first, ".") {
		return "[Go standard library/runtime]"
	}
	return "[third-party]"
}
func forbiddenSlimPackage(pkg string) bool {
	for _, p := range forbiddenSlimPrefixes {
		p = "github.com/yaklang/yaklang/" + p
		// c2ssa/preprocess is also used by the code-formatting RPC and does
		// not import the C compiler/parser. Exclude builder packages exactly;
		// the TypeScript frontend is a complete subtree of compiler packages.
		if pkg == p || strings.HasSuffix(p, "/frontend") && strings.HasPrefix(pkg, p+"/") {
			return true
		}
	}
	return false
}
func readDependencies(name string) ([]dependency, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	var out []dependency
	for {
		var p dependency
		err := d.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty dependency graph")
	}
	return out, nil
}

// The table format is Go-versioned. Only split the known Go 1.18/1.20+
// layout; an unknown layout still has an exact section-level byte count.
func pclnSubtables(data []byte) []section {
	if len(data) < 16 {
		return nil
	}
	var order binary.ByteOrder = binary.LittleEndian
	magic := order.Uint32(data)
	if magic != 0xfffffff0 && magic != 0xfffffff1 {
		order = binary.BigEndian
		magic = order.Uint32(data)
	}
	if magic != 0xfffffff0 && magic != 0xfffffff1 {
		return nil
	}
	ptr := int(data[7])
	if (ptr != 4 && ptr != 8) || len(data) < 8+8*ptr {
		return nil
	}
	offsets := []uint64{0}
	for word := 3; word <= 7; word++ {
		at := 8 + word*ptr
		var v uint64
		if ptr == 8 {
			v = order.Uint64(data[at:])
		} else {
			v = uint64(order.Uint32(data[at:]))
		}
		if v < offsets[len(offsets)-1] || v > uint64(len(data)) {
			return nil
		}
		offsets = append(offsets, v)
	}
	offsets = append(offsets, uint64(len(data)))
	names := []string{"header", "function_names", "compilation_units", "file_names", "pc_data", "function_records"}
	out := make([]section, len(names))
	for i, n := range names {
		out[i] = section{n, offsets[i+1] - offsets[i]}
	}
	return out
}
