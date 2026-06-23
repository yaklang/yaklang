package embed

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

var ErrUnsupportedPrunedRuntime = errors.New("unsupported pruned ssa2llvm runtime dependency")

const yaklangModulePath = "github.com/yaklang/yaklang"

type YaklibDependency struct {
	Module  string
	Methods []string
}

type PrunedRuntimeDependencies struct {
	Yaklib []YaklibDependency
}

func ValidatePrunedRuntimeDependencies(deps []YaklibDependency) error {
	unsupported := UnsupportedPrunedRuntimeDependencies(deps)
	if len(unsupported) == 0 {
		return nil
	}
	dep := unsupported[0]
	method := ""
	if len(dep.Methods) > 0 {
		method = dep.Methods[0]
	}
	return formatUnsupportedPrunedRuntimeDependency(dep.Module, method)
}

func UnsupportedPrunedRuntimeDependencies(deps []YaklibDependency) []YaklibDependency {
	out := make([]YaklibDependency, 0)
	for _, dep := range normalizeYaklibDependencies(deps) {
		methods := make([]string, 0, len(dep.Methods))
		for _, method := range dep.Methods {
			if !isPrunedRuntimeDependencySupported(dep.Module, method) {
				methods = append(methods, method)
			}
		}
		if len(methods) > 0 {
			out = append(out, YaklibDependency{
				Module:  dep.Module,
				Methods: methods,
			})
		}
	}
	return out
}

func BuildPrunedRuntimeArchiveFromLocalSource(buildDir string, deps []YaklibDependency) (archivePath string, gcLibDir string, err error) {
	return BuildPrunedRuntimeArchiveFromLocalSourceWithDeps(buildDir, PrunedRuntimeDependencies{Yaklib: deps})
}

func BuildPrunedRuntimeArchiveFromLocalSourceWithDeps(buildDir string, deps PrunedRuntimeDependencies) (archivePath string, gcLibDir string, err error) {
	buildDir = strings.TrimSpace(buildDir)
	if buildDir == "" {
		return "", "", fmt.Errorf("build pruned runtime archive failed: empty buildDir")
	}
	root, err := localGoModuleRoot()
	if err != nil {
		return "", "", err
	}
	srcDir := root
	runtimeDir := filepath.Join(root, "common", "yak", "ssa2llvm", "runtime", "runtime_go")
	if err := ensureLocalLibgc(runtimeDir); err != nil {
		return "", "", err
	}
	return BuildPrunedRuntimeArchiveFromSourceTreeWithDeps(buildDir, srcDir, deps)
}

func BuildPrunedRuntimeArchiveFromSourceTree(buildDir, srcDir string, deps []YaklibDependency) (archivePath string, gcLibDir string, err error) {
	return BuildPrunedRuntimeArchiveFromSourceTreeWithDeps(buildDir, srcDir, PrunedRuntimeDependencies{Yaklib: deps})
}

func BuildPrunedRuntimeArchiveFromSourceTreeWithDeps(buildDir, srcDir string, deps PrunedRuntimeDependencies) (archivePath string, gcLibDir string, err error) {
	buildDir = strings.TrimSpace(buildDir)
	srcDir = strings.TrimSpace(srcDir)
	if buildDir == "" {
		return "", "", fmt.Errorf("build pruned runtime archive failed: empty buildDir")
	}
	if srcDir == "" {
		return "", "", fmt.Errorf("build pruned runtime archive failed: empty srcDir")
	}

	gcLibDir = filepath.Join(srcDir, "common", "yak", "ssa2llvm", "runtime", "runtime_go", "libs")
	if _, statErr := os.Stat(filepath.Join(gcLibDir, "libgc.a")); statErr != nil {
		return "", "", fmt.Errorf("build pruned runtime archive failed: libgc.a not found under %s", gcLibDir)
	}

	runtimeDir := filepath.Join(srcDir, "common", "yak", "ssa2llvm", "runtime", "runtime_go")
	generatedPath := filepath.Join(buildDir, "runtime_imports_generated.go")
	canonicalGeneratedPath := filepath.Join(runtimeDir, "runtime_imports_generated.go")
	if err := writePrunedRuntimeImportsToFile(generatedPath, deps.Yaklib); err != nil {
		return "", "", err
	}
	overlayPath, err := writeBuildOverlay(buildDir, canonicalGeneratedPath, generatedPath)
	if err != nil {
		return "", "", err
	}
	defer os.Remove(generatedPath)
	defer os.Remove(overlayPath)

	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", "", fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	archivePath = filepath.Join(buildDir, "libyak.a")
	cmd := exec.Command(goPath, "build",
		"-trimpath",
		"-ldflags=-s -w",
		"-buildmode=c-archive",
		"-overlay", overlayPath,
		"-o", archivePath,
		"./common/yak/ssa2llvm/runtime/runtime_go",
	)
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=1",
		"GOWORK=off",
	)
	trace.PrintCmd(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("build pruned runtime archive failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(archivePath); err != nil {
		return "", "", fmt.Errorf("build pruned runtime archive failed: output libyak.a missing: %v", err)
	}
	if _, err := writeRuntimeLinkArgsFile(buildDir, srcDir, overlayPath); err != nil {
		return "", "", err
	}
	return archivePath, gcLibDir, nil
}

func localGoModuleRoot() (string, error) {
	var candidates []string
	if root := strings.TrimSpace(os.Getenv("YAKLANG_SOURCE_ROOT")); root != "" {
		candidates = append(candidates, root)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok && filepath.IsAbs(file) {
		candidates = append(candidates, filepath.Dir(file))
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		if root, err := findYaklangModuleRoot(abs); err == nil {
			return root, nil
		}
	}
	return "", fmt.Errorf("build pruned runtime archive failed: current directory is not inside a Go module")
}

func findYaklangModuleRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		modPath := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(modPath)
		if err == nil {
			if strings.Contains(string(data), "module "+yaklangModulePath) {
				return dir, nil
			}
			return "", fmt.Errorf("go.mod at %s is not %s", modPath, yaklangModulePath)
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", modPath, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", start)
		}
		dir = parent
	}
}

func copyLocalLibgc(runtimeDir string) error {
	libgc, err := resolveLibgcArchive()
	if err != nil {
		return err
	}
	dstDir := filepath.Join(runtimeDir, "libs")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("build pruned runtime archive failed: mkdir %s: %w", dstDir, err)
	}
	return copyFile(libgc, filepath.Join(dstDir, "libgc.a"))
}

func ensureLocalLibgc(runtimeDir string) error {
	libgcPath := filepath.Join(runtimeDir, "libs", "libgc.a")
	if info, err := os.Stat(libgcPath); err == nil && !info.IsDir() && info.Size() > 0 {
		return nil
	}
	return copyLocalLibgc(runtimeDir)
}

func resolveLibgcArchive() (string, error) {
	tools := []string{"cc", "gcc", "clang"}
	var lastErr error
	for _, tool := range tools {
		p, err := exec.LookPath(tool)
		if err != nil {
			lastErr = err
			continue
		}
		cmd := exec.Command(p, "-print-file-name=libgc.a")
		out, err := cmd.CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("%s -print-file-name=libgc.a failed: %v\n%s", tool, err, out)
			continue
		}
		path := strings.TrimSpace(string(out))
		if path == "" || path == "libgc.a" {
			lastErr = fmt.Errorf("%s did not resolve libgc.a: %q", tool, path)
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
		lastErr = fmt.Errorf("libgc.a not found at %q", path)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("libgc.a not found")
	}
	return "", fmt.Errorf("build pruned runtime archive failed: %w", lastErr)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("build pruned runtime archive failed: open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("build pruned runtime archive failed: create %s: %w", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("build pruned runtime archive failed: copy %s to %s: %w", src, dst, err)
	}
	return out.Close()
}

type goImportSpec struct {
	Alias string
	Path  string
}

type globalExportRef struct {
	TableExpr string
	Key       string
}

func writePrunedRuntimeImports(runtimeDir string, deps []YaklibDependency) error {
	return writePrunedRuntimeImportsToFile(filepath.Join(runtimeDir, "runtime_imports_generated.go"), deps)
}

func writePrunedRuntimeImportsToFile(outputPath string, deps []YaklibDependency) error {
	imports := map[string]goImportSpec{}
	modules := map[string]map[string]string{}
	globals := map[string]string{}
	globalTables := map[string]ModuleImportSpec{}
	registeredGlobals := map[string]globalExportRef{}

	for _, dep := range normalizeYaklibDependencies(deps) {
		module := dep.Module
		methods := dep.Methods

		if module == "" {
			for _, method := range methods {
				if expr, ok := prunedBuiltinGlobalExportExpr(method); ok {
					globals[method] = expr
					continue
				}
				if spec, ok := globalBuiltinImportSpecs[method]; ok {
					imports[spec.ImportAlias] = goImportSpec{
						Alias: spec.ImportAlias,
						Path:  spec.GoImportPath,
					}
					globalTables[spec.ExportExpr] = spec
					continue
				}
				if tableExpr, importSpec, ok := lookupRegisteredGlobalExport(method); ok {
					imports[importSpec.ImportAlias] = goImportSpec{
						Alias: importSpec.ImportAlias,
						Path:  importSpec.GoImportPath,
					}
					registeredGlobals[method] = globalExportRef{
						TableExpr: tableExpr,
						Key:       method,
					}
					continue
				}
				return formatUnsupportedPrunedRuntimeDependency(module, method)
			}
			continue
		}

		spec, ok := LookupModuleSpec(module)
		if !ok {
			return formatUnsupportedPrunedRuntimeDependency(module, "")
		}

		imports[spec.ImportAlias] = goImportSpec{
			Alias: spec.ImportAlias,
			Path:  spec.GoImportPath,
		}
		modules[module] = setModuleExport(modules[module], "*", spec.ExportExpr)
		_ = methods
	}

	var b strings.Builder
	b.WriteString("package main\n\n")
	if len(imports) > 0 {
		b.WriteString("import (\n")
		keys := make([]string, 0, len(imports))
		for key := range imports {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			spec := imports[key]
			b.WriteString(fmt.Sprintf("\t%s %q\n", spec.Alias, spec.Path))
		}
		b.WriteString(")\n\n")
	}
	b.WriteString("func init() {\n")
	if len(globalTables) > 0 {
		for _, spec := range globalTables {
			b.WriteString(fmt.Sprintf("\truntimeRegisterYaklibGlobals(%s)\n", spec.ExportExpr))
		}
	}
	if len(globals) > 0 || len(registeredGlobals) > 0 {
		names := make([]string, 0, len(globals)+len(registeredGlobals))
		for name := range globals {
			names = append(names, name)
		}
		for name := range registeredGlobals {
			names = append(names, name)
		}
		sort.Strings(names)
		b.WriteString("\truntimeRegisterYaklibGlobals(map[string]any{\n")
		for _, name := range names {
			if expr, ok := globals[name]; ok {
				b.WriteString(fmt.Sprintf("\t\t%q: %s,\n", name, expr))
				continue
			}
			ref := registeredGlobals[name]
			b.WriteString(fmt.Sprintf("\t\t%q: %s[%q],\n", name, ref.TableExpr, ref.Key))
		}
		b.WriteString("\t})\n")
	}
	moduleNames := make([]string, 0, len(modules))
	for name := range modules {
		moduleNames = append(moduleNames, name)
	}
	sort.Strings(moduleNames)
	for _, module := range moduleNames {
		exports := modules[module]
		if whole, ok := exports["*"]; ok {
			b.WriteString(fmt.Sprintf("\truntimeRegisterYaklibModule(%q, %s)\n", module, whole))
			continue
		}
		methods := make([]string, 0, len(exports))
		for method := range exports {
			methods = append(methods, method)
		}
		sort.Strings(methods)
		b.WriteString(fmt.Sprintf("\truntimeRegisterYaklibModule(%q, map[string]any{\n", module))
		for _, method := range methods {
			b.WriteString(fmt.Sprintf("\t\t%q: %s,\n", method, exports[method]))
		}
		b.WriteString("\t})\n")
	}
	b.WriteString("}\n")

	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return fmt.Errorf("build pruned runtime archive failed: empty generated imports path")
	}
	src := []byte(b.String())
	if formatted, err := format.Source(src); err == nil {
		src = formatted
	}
	return os.WriteFile(outputPath, src, 0o644)
}

func writeBuildOverlay(buildDir, canonicalPath, generatedPath string) (string, error) {
	canonicalPath, err := filepath.Abs(canonicalPath)
	if err != nil {
		return "", fmt.Errorf("build pruned runtime archive failed: abs canonical overlay path: %w", err)
	}
	generatedPath, err = filepath.Abs(generatedPath)
	if err != nil {
		return "", fmt.Errorf("build pruned runtime archive failed: abs generated overlay path: %w", err)
	}
	payload := map[string]map[string]string{
		"Replace": {
			canonicalPath: generatedPath,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("build pruned runtime archive failed: marshal overlay: %w", err)
	}
	overlayPath := filepath.Join(buildDir, "runtime-overlay.json")
	if err := os.WriteFile(overlayPath, data, 0o644); err != nil {
		return "", fmt.Errorf("build pruned runtime archive failed: write overlay %s: %w", overlayPath, err)
	}
	return overlayPath, nil
}

func writeRuntimeLinkArgsFile(buildDir, srcDir, overlayPath string) (string, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	overlayPath = strings.TrimSpace(overlayPath)
	if overlayPath == "" {
		return "", fmt.Errorf("build pruned runtime archive failed: empty overlay path for link args")
	}

	args := []string{"list", "-deps", "-overlay", overlayPath}
	args = append(args, "-f", "{{if .CgoLDFLAGS}}{{range .CgoLDFLAGS}}{{printf \"%s\\n\" .}}{{end}}{{end}}", "./common/yak/ssa2llvm/runtime/runtime_go")
	cmd := exec.Command(goPath, args...)
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=1",
		"GOWORK=off",
	)
	trace.PrintCmd(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build pruned runtime archive failed: collect runtime link args: %v\n%s", err, out)
	}

	linkFlagsPath := filepath.Join(buildDir, "libyak.linkflags")
	if err := os.WriteFile(linkFlagsPath, out, 0o644); err != nil {
		return "", fmt.Errorf("build pruned runtime archive failed: write %s: %v", linkFlagsPath, err)
	}
	return linkFlagsPath, nil
}

func setModuleExport(exports map[string]string, name, expr string) map[string]string {
	if exports == nil {
		exports = make(map[string]string)
	}
	exports[name] = expr
	return exports
}

func isPrunedRuntimeDependencySupported(module, method string) bool {
	module = strings.TrimSpace(module)
	method = strings.TrimSpace(method)
	if module == "" {
		if method == "" {
			return false
		}
		if globalBuiltinNames[method] {
			return true
		}
		if _, ok := prunedBuiltinGlobalExportExpr(method); ok {
			return true
		}
		if _, ok := globalBuiltinImportSpecs[method]; ok {
			return true
		}
		_, _, ok := lookupRegisteredGlobalExport(method)
		return ok
	}
	_, ok := LookupModuleSpec(module)
	return ok
}

func formatUnsupportedPrunedRuntimeDependency(module, method string) error {
	module = strings.TrimSpace(module)
	method = strings.TrimSpace(method)
	switch {
	case module == "" && method != "":
		return fmt.Errorf("%w: global.%s", ErrUnsupportedPrunedRuntime, method)
	case module != "" && method != "":
		return fmt.Errorf("%w: %s.%s", ErrUnsupportedPrunedRuntime, module, method)
	case module != "":
		return fmt.Errorf("%w: %s", ErrUnsupportedPrunedRuntime, module)
	default:
		return fmt.Errorf("%w: empty dependency", ErrUnsupportedPrunedRuntime)
	}
}

func prunedBuiltinGlobalExportExpr(method string) (string, bool) {
	switch method {
	case "len":
		return "runtimeYakBuiltinLen", true
	case "cap":
		return "runtimeYakBuiltinCap", true
	}
	return "", false
}

func normalizeYaklibDependencies(deps []YaklibDependency) []YaklibDependency {
	merged := make(map[string]map[string]struct{})
	for _, dep := range deps {
		module := strings.TrimSpace(dep.Module)
		methods := merged[module]
		if methods == nil {
			methods = make(map[string]struct{})
			merged[module] = methods
		}
		for _, method := range dep.Methods {
			method = strings.TrimSpace(method)
			if method == "" {
				continue
			}
			methods[method] = struct{}{}
		}
	}
	out := make([]YaklibDependency, 0, len(merged))
	for module, set := range merged {
		methods := make([]string, 0, len(set))
		for method := range set {
			methods = append(methods, method)
		}
		sort.Strings(methods)
		out = append(out, YaklibDependency{Module: module, Methods: methods})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Module < out[j].Module })
	return out
}
