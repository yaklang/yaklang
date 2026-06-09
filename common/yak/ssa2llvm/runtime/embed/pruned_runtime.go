package embed

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

var ErrUnsupportedPrunedRuntime = errors.New("unsupported pruned ssa2llvm runtime dependency")

type YaklibDependency struct {
	Module  string
	Methods []string
}

func BuildPrunedRuntimeArchiveFromEmbeddedSource(buildDir string, deps []YaklibDependency) (archivePath string, gcLibDir string, err error) {
	srcDir := filepath.Join(buildDir, "ssa2llvm-stdlib-src")
	if _, err := ExtractRuntimeSourceToDir(srcDir); err != nil {
		return "", "", err
	}
	return BuildPrunedRuntimeArchiveFromSourceTree(buildDir, srcDir, deps)
}

func BuildPrunedRuntimeArchiveFromLocalSource(buildDir string, deps []YaklibDependency) (archivePath string, gcLibDir string, err error) {
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
	return BuildPrunedRuntimeArchiveFromSourceTree(buildDir, srcDir, deps)
}

func BuildPrunedRuntimeArchiveFromSourceTree(buildDir, srcDir string, deps []YaklibDependency) (archivePath string, gcLibDir string, err error) {
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
	if err := writePrunedRuntimeImports(runtimeDir, deps); err != nil {
		return "", "", err
	}

	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", "", fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	archivePath = filepath.Join(buildDir, "libyak.a")
	cmd := exec.Command(goPath, "build",
		"-trimpath",
		"-tags=ssa2llvm_pruned_runtime",
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
	if _, err := writeRuntimeLinkArgsFileWithTags(buildDir, srcDir, "ssa2llvm_pruned_runtime"); err != nil {
		return "", "", err
	}
	return archivePath, gcLibDir, nil
}

func localGoModuleRoot() (string, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("go toolchain not found in PATH: %w", err)
	}
	cmd := exec.Command(goPath, "env", "GOMOD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build pruned runtime archive failed: go env GOMOD: %v\n%s", err, out)
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" || modPath == os.DevNull {
		return "", fmt.Errorf("build pruned runtime archive failed: current directory is not inside a Go module")
	}
	return filepath.Dir(modPath), nil
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
	imports := map[string]goImportSpec{}
	modules := map[string]map[string]string{}
	globals := map[string]string{}

	for _, dep := range normalizeYaklibDependencies(deps) {
		module := dep.Module
		methods := dep.Methods
		switch module {
		case "":
			for _, method := range methods {
				expr, ok := prunedBuiltinGlobalExportExpr(method)
				if !ok {
					return fmt.Errorf("%w: global.%s", ErrUnsupportedPrunedRuntime, method)
				}
				globals[method] = expr
			}
		case "codec":
			for _, method := range methods {
				expr, ok := prunedCodecExportExpr(method)
				if !ok {
					return fmt.Errorf("%w: codec.%s", ErrUnsupportedPrunedRuntime, method)
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
		default:
			return fmt.Errorf("%w: %s", ErrUnsupportedPrunedRuntime, module)
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
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func setModuleExport(exports map[string]string, name, expr string) map[string]string {
	if exports == nil {
		exports = make(map[string]string)
	}
	exports[name] = expr
	return exports
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
