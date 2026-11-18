package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/reify"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

// Set REIFY_TEST_OUTPUT_DIR to keep compiler outputs as reproducible evidence.
// The normal Go test temporary directory remains the portable default.
func languageOutputDir(t *testing.T) string {
	t.Helper()
	root := os.Getenv("REIFY_TEST_OUTPUT_DIR")
	if root == "" {
		return t.TempDir()
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "language-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifacts: %s", dir)
	return dir
}

func compileLanguageExample(t *testing.T, tool string, args, environment []string) {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s is not installed", tool)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Env = append(os.Environ(), environment...)
	t.Logf("%s %s %s", strings.Join(environment, " "), tool, strings.Join(args, " "))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, output)
	}
}

func TestGoBinariesIdentifyRecoverAndLift(t *testing.T) {
	for _, tc := range []struct {
		os, arch string
		format   core.Format
		stripped bool
	}{
		{"linux", "amd64", core.FormatELF, false},
		{"linux", "amd64", core.FormatELF, true},
		{"windows", "amd64", core.FormatPE, true},
		{"darwin", "arm64", core.FormatMachO, true},
	} {
		t.Run(fmt.Sprintf("%s-%s-stripped-%v", tc.os, tc.arch, tc.stripped), func(t *testing.T) {
			binary := filepath.Join(languageOutputDir(t), "go_hello")
			args := []string{"build", "-gcflags=-N -l", "-o", binary}
			if tc.stripped {
				args = append(args, "-ldflags=-s -w")
			}
			args = append(args, "examples/go_hello.go")
			compileLanguageExample(t, "go", args, []string{"CGO_ENABLED=0", "GOOS=" + tc.os, "GOARCH=" + tc.arch})
			ctx := context.Background()
			a, _, err := reify.Inspect(ctx, binary)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if a.Format != tc.format || a.Language == nil || a.Language.Language != core.LanguageGo || a.Language.Confidence != core.ConfidenceCertain {
				t.Fatalf("format=%s report=%+v", a.Format, a.Language)
			}
			s, ok := a.Image.SymbolByName("main.Add")
			if !ok || s.Source != symbol.SourcePCLNTab || s.Kind != symbol.SymbolFunction || s.Size == 0 {
				t.Fatalf("recovered symbol = %+v, present=%v; report=%+v", s, ok, a.Language)
			}
			if tc.stripped {
				// PE can retain an empty section called .symtab after -s. Check
				// the container loader's actual symbol table, not section names.
				file, err := os.Open(binary)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				stat, err := file.Stat()
				if err != nil {
					t.Fatal(err)
				}
				reader, err := core.NewByteReader(binary, file, stat.Size())
				if err != nil {
					t.Fatal(err)
				}
				container, ok := loader.Default.Loader(tc.format)
				if !ok {
					t.Fatalf("missing loader for %s", tc.format)
				}
				raw, err := container.Load(ctx, reader)
				if err != nil {
					t.Fatal(err)
				}
				if _, found := raw.Image.SymbolByName("main.Add"); found {
					t.Fatal("stripped fixture still has a native main.Add symbol")
				}
			}
			result, err := reify.Lift(ctx, binary, reify.WithFunction("main.Add"), reify.WithMode(reify.ModeAll))
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if result.Functions != 1 || result.IR.Functions[0].Facts == nil || result.Assembly == "" || result.SSAIR == "" {
				t.Fatalf("incomplete lift: %+v", result)
			}
			if !strings.Contains(result.Source, "main.Add(") || !strings.Contains(result.Source, "arg0 + arg1") {
				t.Fatalf("Go ABI scalar result not recovered:\n%s", result.Source)
			}
			if err := os.WriteFile(binary+".source.txt", []byte(result.Source), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: language=%s symbol=%s source=%s address=0x%x functions=%d\n%s", tc.format, a.Language.Language, s.Name, s.Source, s.Address, result.Functions, result.Source)
		})
	}
}

func TestRustBinaryIdentifyDemangleAndLift(t *testing.T) {
	binary := filepath.Join(languageOutputDir(t), "rust_hello")
	compileLanguageExample(t, "rustc", []string{
		"--edition=2021", "-C", "opt-level=0", "-C", "overflow-checks=off", "-C", "symbol-mangling-version=v0",
		"-o", binary, "examples/rust_hello.rs",
	}, nil)
	ctx := context.Background()
	a, _, err := reify.Inspect(ctx, binary)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.Language == nil || a.Language.Language != core.LanguageRust || a.Language.Confidence != core.ConfidenceCertain {
		t.Fatalf("report = %+v", a.Language)
	}
	s, ok := a.Image.SymbolByName("rust_hello::add")
	if !ok || s.Mangled == "" || s.Source != symbol.SourceRustDemangle {
		t.Fatalf("Rust symbol = %+v, present=%v", s, ok)
	}
	// Both the display name and the original linker name select the same body.
	for _, name := range []string{s.Name, s.Mangled} {
		result, err := reify.Lift(ctx, binary, reify.WithFunction(name), reify.WithMode(reify.ModeAll))
		if err != nil {
			t.Fatal(err)
		}
		if result.Functions != 1 || result.Assembly == "" || result.SSAIR == "" || !strings.Contains(result.Source, "rust_hello::add(") || !strings.Contains(result.Source, "arg0 + arg1") {
			result.Close()
			t.Fatalf("incomplete Rust lift:\n%s", result.Source)
		}
		if err := os.WriteFile(binary+".source.txt", []byte(result.Source), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("language=%s mangled=%s name=%s functions=%d\n%s", a.Language.Language, s.Mangled, s.Name, result.Functions, result.Source)
		result.Close()
	}
	result, err := reify.Lift(ctx, binary, reify.WithFunction("rust_hello::main"), reify.WithMode(reify.ModeSource))
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	if !strings.Contains(result.Source, "rust_hello::add(") {
		t.Fatalf("direct call target was not demangled:\n%s", result.Source)
	}
}
