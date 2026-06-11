package embed

import (
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

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

var ErrUnsupportedPrunedRuntime = errors.New("unsupported pruned ssa2llvm runtime dependency")

const yaklangModulePath = "github.com/yaklang/yaklang"

type YaklibDependency struct {
	Module  string
	Methods []string
}

type PrunedRuntimeDependencies struct {
	Yaklib          []YaklibDependency
	RuntimeDispatch []abi.FuncID
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
	registry, _ := scriptEngineRegistryFromLocalSource()
	out := make([]YaklibDependency, 0)
	for _, dep := range normalizeYaklibDependencies(deps) {
		methods := make([]string, 0, len(dep.Methods))
		for _, method := range dep.Methods {
			if !isPrunedRuntimeDependencySupportedWithRegistry(registry, dep.Module, method) {
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

func BuildPrunedRuntimeArchiveFromEmbeddedSource(buildDir string, deps []YaklibDependency) (archivePath string, gcLibDir string, err error) {
	return BuildPrunedRuntimeArchiveFromEmbeddedSourceWithDeps(buildDir, PrunedRuntimeDependencies{Yaklib: deps})
}

func BuildPrunedRuntimeArchiveFromEmbeddedSourceWithDeps(buildDir string, deps PrunedRuntimeDependencies) (archivePath string, gcLibDir string, err error) {
	srcDir := filepath.Join(buildDir, "ssa2llvm-stdlib-src")
	if _, err := ExtractRuntimeSourceToDir(srcDir); err != nil {
		return "", "", err
	}
	return BuildPrunedRuntimeArchiveFromSourceTreeWithDeps(buildDir, srcDir, deps)
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
	srcDir := filepath.Join(buildDir, "ssa2llvm-stdlib-src")
	_ = os.RemoveAll(srcDir)
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", "", fmt.Errorf("go toolchain not found in PATH: %w", err)
	}
	cmd := exec.Command(goPath, "run", "./common/utils/gomodsrc/cmd",
		"--pkg", "./common/yak/ssa2llvm/runtime/runtime_go",
		"--dst", srcDir,
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	trace.PrintCmd(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("build pruned runtime archive failed: gomodsrc: %v\n%s", err, out)
	}
	if err := copyLocalLibgc(srcDir); err != nil {
		return "", "", err
	}
	if err := copyPrunedRuntimeBuildTagSources(root, srcDir); err != nil {
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
	if err := writePrunedRuntimeImports(runtimeDir, deps.Yaklib); err != nil {
		return "", "", err
	}

	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", "", fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	archivePath = filepath.Join(buildDir, "libyak.a")
	buildTags := prunedRuntimeBuildTags(deps)
	cmd := exec.Command(goPath, "build",
		"-trimpath",
		"-tags="+strings.Join(buildTags, ","),
		"-ldflags=-s -w",
		"-buildmode=c-archive",
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
	if _, err := writeRuntimeLinkArgsFileWithTags(buildDir, srcDir, buildTags...); err != nil {
		return "", "", err
	}
	return archivePath, gcLibDir, nil
}

func prunedRuntimeBuildTags(deps PrunedRuntimeDependencies) []string {
	tags := []string{"ssa2llvm_pruned_runtime"}
	if prunedRuntimeNeedsModule(deps.Yaklib, "cli") {
		tags = append(tags, "ssa2llvm_runtime_cli")
	}
	if prunedRuntimeNeedsPoc(deps.RuntimeDispatch) {
		tags = append(tags, "ssa2llvm_runtime_poc")
	}
	if prunedRuntimeNeedsModule(deps.Yaklib, "yakit") {
		tags = append(tags, "ssa2llvm_runtime_yakit")
	}
	return tags
}

func prunedRuntimeNeedsPoc(ids []abi.FuncID) bool {
	for _, id := range ids {
		switch id {
		case abi.IDPocTimeout, abi.IDPocGet, abi.IDPocGetHTTPPacketBody:
			return true
		}
	}
	return false
}

func prunedRuntimeNeedsModule(deps []YaklibDependency, module string) bool {
	module = strings.TrimSpace(module)
	if module == "" {
		return false
	}
	for _, dep := range deps {
		if strings.TrimSpace(dep.Module) == module {
			return true
		}
	}
	return false
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

func copyLocalLibgc(srcDir string) error {
	libgc, err := resolveLibgcArchive()
	if err != nil {
		return err
	}
	dstDir := filepath.Join(srcDir, "common", "yak", "ssa2llvm", "runtime", "runtime_go", "libs")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("build pruned runtime archive failed: mkdir %s: %w", dstDir, err)
	}
	return copyFile(libgc, filepath.Join(dstDir, "libgc.a"))
}

func copyPrunedRuntimeBuildTagSources(root, srcDir string) error {
	files := []string{
		filepath.Join("common", "yak", "ssa2llvm", "runtime", "runtime_go", "runtime_yaklib_builtins_pruned.go"),
		filepath.Join("common", "yak", "ssa2llvm", "runtime", "runtime_go", "runtime_yaklib_lookup_pruned.go"),
		filepath.Join("common", "yak", "ssa2llvm", "runtime", "runtime_go", "runtime_yaklib_yakit_pruned.go"),
		filepath.Join("common", "yak", "ssa2llvm", "runtime", "runtime_go", "runtime_sync_pruned.go"),
	}
	for _, rel := range files {
		src := filepath.Join(root, rel)
		dst := filepath.Join(srcDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("build pruned runtime archive failed: mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
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

func writePrunedRuntimeImports(runtimeDir string, deps []YaklibDependency) error {
	registry, registryErr := scriptEngineRegistryForRuntimeDir(runtimeDir)
	imports := map[string]goImportSpec{}
	modules := map[string]map[string]string{}
	globals := map[string]string{}
	globalTables := map[string]scriptEngineExport{}

	for _, dep := range normalizeYaklibDependencies(deps) {
		module := dep.Module
		methods := dep.Methods
		switch module {
		case "":
			for _, method := range methods {
				expr, ok := prunedBuiltinGlobalExportExpr(method)
				if ok {
					globals[method] = expr
					continue
				}
				export, ok := registry.globalForMethod(method)
				if !ok {
					return formatUnsupportedPrunedRuntimeDependency(module, method)
				}
				if err := addExportImports(imports, export); err != nil {
					return err
				}
				globalTables[export.Expr] = export
			}
		case "codec":
			for _, method := range methods {
				expr, ok := prunedCodecExportExpr(method)
				if !ok {
					return formatUnsupportedPrunedRuntimeDependency(module, method)
				}
				imports["codec"] = goImportSpec{Alias: "codec", Path: "github.com/yaklang/yaklang/common/yak/yaklib/codec"}
				modules[module] = setModuleExport(modules[module], method, expr)
			}
		case "cli":
			imports["cli"] = goImportSpec{Alias: "cli", Path: "github.com/yaklang/yaklang/common/utils/cli"}
			modules[module] = setModuleExport(modules[module], "*", "cli.CliExports")
		case "poc":
			imports["poc"] = goImportSpec{Alias: "poc", Path: "github.com/yaklang/yaklang/common/utils/lowhttp/poc"}
			modules[module] = setModuleExport(modules[module], "*", "poc.PoCExports")
		case "http":
			imports["yakhttp"] = goImportSpec{Alias: "yakhttp", Path: "github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"}
			modules[module] = setModuleExport(modules[module], "*", "yakhttp.HttpExports")
		case "yakit":
			modules[module] = setModuleExport(modules[module], "*", "runtimePrunedYakitExports()")
		default:
			if registryErr != nil {
				return fmt.Errorf("%w: %s: %v", ErrUnsupportedPrunedRuntime, module, registryErr)
			}
			export, ok := registry.module(module)
			if !ok {
				return formatUnsupportedPrunedRuntimeDependency(module, "")
			}
			if err := addExportImports(imports, export); err != nil {
				return err
			}
			modules[module] = setModuleExport(modules[module], "*", export.Expr)
		}
	}

	var b strings.Builder
	b.WriteString("//go:build ssa2llvm_pruned_runtime\n\n")
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
		for _, expr := range orderedGlobalTableExprs(registry, globalTables) {
			b.WriteString(fmt.Sprintf("\truntimeRegisterYaklibGlobals(%s)\n", expr))
		}
	}
	if len(globals) > 0 {
		names := make([]string, 0, len(globals))
		for name := range globals {
			names = append(names, name)
		}
		sort.Strings(names)
		b.WriteString("\truntimeRegisterYaklibGlobals(map[string]any{\n")
		for _, name := range names {
			b.WriteString(fmt.Sprintf("\t\t%q: %s,\n", name, globals[name]))
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

	path := filepath.Join(runtimeDir, "runtime_imports_generated.go")
	src := []byte(b.String())
	if formatted, err := format.Source(src); err == nil {
		src = formatted
	}
	return os.WriteFile(path, src, 0o644)
}

func addExportImports(imports map[string]goImportSpec, export scriptEngineExport) error {
	for _, spec := range export.Imports {
		if err := addImportSpec(imports, spec); err != nil {
			return err
		}
	}
	return nil
}

func addImportSpec(imports map[string]goImportSpec, spec goImportSpec) error {
	if spec.Alias == "" || spec.Path == "" {
		return nil
	}
	existing, ok := imports[spec.Alias]
	if ok && existing.Path != spec.Path {
		return fmt.Errorf("build pruned runtime archive failed: import alias %s maps to both %s and %s", spec.Alias, existing.Path, spec.Path)
	}
	imports[spec.Alias] = spec
	return nil
}

func orderedGlobalTableExprs(registry *scriptEngineLibRegistry, selected map[string]scriptEngineExport) []string {
	out := make([]string, 0, len(selected))
	seen := make(map[string]struct{}, len(selected))
	if registry != nil {
		for _, export := range registry.Globals {
			if _, ok := selected[export.Expr]; !ok {
				continue
			}
			out = append(out, export.Expr)
			seen[export.Expr] = struct{}{}
		}
	}
	extras := make([]string, 0)
	for expr := range selected {
		if _, ok := seen[expr]; ok {
			continue
		}
		extras = append(extras, expr)
	}
	sort.Strings(extras)
	return append(out, extras...)
}

func setModuleExport(exports map[string]string, name, expr string) map[string]string {
	if exports == nil {
		exports = make(map[string]string)
	}
	exports[name] = expr
	return exports
}

func isPrunedRuntimeDependencySupported(module, method string) bool {
	registry, _ := scriptEngineRegistryFromLocalSource()
	return isPrunedRuntimeDependencySupportedWithRegistry(registry, module, method)
}

func isPrunedRuntimeDependencySupportedWithRegistry(registry *scriptEngineLibRegistry, module, method string) bool {
	module = strings.TrimSpace(module)
	method = strings.TrimSpace(method)
	if method == "" {
		return false
	}
	switch module {
	case "":
		if _, ok := prunedBuiltinGlobalExportExpr(method); ok {
			return true
		}
		_, ok := registry.globalForMethod(method)
		return ok
	case "codec":
		_, ok := prunedCodecExportExpr(method)
		return ok
	case "cli", "poc", "http", "yakit":
		return true
	default:
		_, ok := registry.module(module)
		return ok
	}
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

func prunedCodecExportExpr(method string) (string, bool) {
	switch method {
	case "EncodeToHex":
		return "codec.EncodeToHex", true
	case "DecodeHex":
		return "codec.DecodeHex", true
	case "EncodeBase64":
		return "codec.EncodeBase64", true
	case "DecodeBase64":
		return "codec.DecodeBase64", true
	case "EncodeBase32":
		return "codec.EncodeBase32", true
	case "DecodeBase32":
		return "codec.DecodeBase32", true
	case "EncodeBase64Url":
		return "codec.EncodeBase64Url", true
	case "DecodeBase64Url":
		return "codec.DecodeBase64Url", true
	case "Sha1":
		return "codec.Sha1", true
	case "Sha224":
		return "codec.Sha224", true
	case "Sha256":
		return "codec.Sha256", true
	case "Sha384":
		return "codec.Sha384", true
	case "Sha512":
		return "codec.Sha512", true
	case "Md5":
		return "codec.Md5", true
	case "EncodeUrl", "EscapeQueryUrl", "EscapeUrl":
		return "codec.QueryEscape", true
	case "DecodeUrl", "UnescapeQueryUrl":
		return "codec.QueryUnescape", true
	case "EscapePathUrl":
		return "codec.PathEscape", true
	case "UnescapePathUrl":
		return "codec.PathUnescape", true
	case "DoubleEncodeUrl":
		return "codec.DoubleEncodeUrl", true
	case "DoubleDecodeUrl":
		return "codec.DoubleDecodeUrl", true
	case "EncodeHtml":
		return "codec.EncodeHtmlEntity", true
	case "EncodeHtmlHex":
		return "codec.EncodeHtmlEntityHex", true
	case "EscapeHtml":
		return "codec.EscapeHtmlString", true
	case "DecodeHtml":
		return "codec.UnescapeHtmlString", true
	case "EncodeChunked":
		return "codec.HTTPChunkedEncode", true
	case "DecodeChunked":
		return "codec.HTTPChunkedDecode", true
	case "StrconvQuote":
		return "codec.StrConvQuote", true
	case "StrconvUnquote":
		return "codec.StrConvUnquote", true
	}
	return "", false
}
