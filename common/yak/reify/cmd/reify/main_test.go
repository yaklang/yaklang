package main

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/reify"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func runCLI(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCLIHelpAndUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		code, out, errOut := runCLI(args...)
		if code != 0 || !strings.Contains(out, "COMMANDS:") || !strings.Contains(out, "loaders") || errOut != "" {
			t.Fatalf("help %v: %d %q %q", args, code, out, errOut)
		}
	}
	for _, args := range [][]string{{"analyze"}, {"inspect"}, {"check"}, {"lift"}, {"loaders", "extra"}, {"parsers", "extra"}, {"inspect", "--unknown"}, {"inspect", "a", "b"}, {"lift", "a", "b"}, {"lift", "--mode", "bogus", "a"}, {"lift", "--mode", "1asm", "a"}} {
		code, _, errOut := runCLI(args...)
		if code != 2 || errOut == "" {
			t.Fatalf("usage %v: %d %q", args, code, errOut)
		}
	}
	if code, _, _ := runCLI("inspect", "--help"); code != 0 {
		t.Fatalf("subcommand help exited %d", code)
	}
}

func TestCLIListsLoadersAndParsers(t *testing.T) {
	code, out, errOut := runCLI("loaders", "--json")
	if code != 0 || errOut != "" {
		t.Fatalf("loaders: %d %s", code, errOut)
	}
	var loaders []core.LoaderInfo
	if err := json.Unmarshal([]byte(out), &loaders); err != nil {
		t.Fatal(err)
	}
	if len(loaders) != 6 {
		t.Fatalf("unexpected loaders: %+v", loaders)
	}
	seen := map[core.Format]bool{}
	for _, loader := range loaders {
		seen[loader.Kind] = true
	}
	for _, kind := range []core.Format{core.FormatArchive, core.FormatCOFF, core.FormatELF,
		core.FormatMachO, core.FormatMZ, core.FormatPE} {
		if !seen[kind] {
			t.Fatalf("loader %s not listed: %+v", kind, loaders)
		}
	}
	if code, out, _ := runCLI("loaders"); code != 0 || !strings.Contains(out, "ELF") {
		t.Fatalf("text loaders: %d %q", code, out)
	}
	code, out, errOut = runCLI("parsers", "--json")
	if code != 0 || errOut != "" {
		t.Fatalf("parsers: %d %s", code, errOut)
	}
	var parsers []core.ParserInfo
	if err := json.Unmarshal([]byte(out), &parsers); err != nil {
		t.Fatal(err)
	}
	if len(parsers) != 13 || parsers[0].Kind != core.Arch386 {
		t.Fatalf("unexpected parsers: %+v", parsers)
	}
	for _, parser := range parsers {
		if !parser.Available {
			t.Fatalf("unexpected parser availability: %+v", parser)
		}
	}
	if code, out, _ := runCLI("parsers"); code != 0 || !strings.Contains(out, "available") {
		t.Fatalf("text parsers: %d %q", code, out)
	}
}

func TestCLIInspectsExecutableWithoutRunningIt(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI("inspect", "--json", path)
	if code != 0 || errOut != "" {
		t.Fatalf("inspect: %d %s", code, errOut)
	}
	var report struct {
		Scope    string           `json:"scope"`
		Artifact *core.Artifact   `json:"artifact"`
		Parser   *core.ParserInfo `json:"parser"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Scope != "metadata-only" || report.Artifact == nil || report.Artifact.Image == nil || report.Parser == nil {
		t.Fatalf("bad report: %+v", report)
	}
	artifact := report.Artifact
	if artifact.Origin.Name != path || artifact.Format != core.FormatELF || artifact.Target.Architecture == core.ArchUnknown {
		t.Fatalf("bad artifact: %+v", artifact)
	}
	if err := core.ValidateArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	if report.Parser.Kind != artifact.Target.Architecture || !report.Parser.Available {
		t.Fatalf("bad parser report: %+v", report.Parser)
	}

	code, out, errOut = runCLI("check", "--json", path)
	if code != 0 || errOut != "" {
		t.Fatalf("check: %d %s", code, errOut)
	}
	var checked struct {
		Scope string              `json:"scope"`
		Check *loader.CheckResult `json:"check"`
	}
	if err := json.Unmarshal([]byte(out), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.Scope != "metadata-only" || checked.Check == nil || checked.Check.Parser == nil || checked.Check.Sections == 0 {
		t.Fatalf("bad check: %+v", checked)
	}
	for _, command := range []string{"inspect", "check"} {
		code, text, errOut := runCLI(command, path)
		if code != 0 || errOut != "" || !strings.Contains(text, "SSA") || !strings.Contains(text, "Parser:") {
			t.Fatalf("text %s: %d %q %q", command, code, text, errOut)
		}
	}
}

func TestCLIRejectsUnsupportedAndMalformed(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"script.py", []byte("print('not executed')")},
		{"Example.class", []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 61}},
		{"broken.elf", []byte{0x7f, 'E', 'L', 'F'}},
		{"fat.macho", []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 2}},
	} {
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, tc.data, 0600); err != nil {
			t.Fatal(err)
		}
		code, out, errOut := runCLI("check", "--json", path)
		if code != 1 || out != "" || errOut == "" {
			t.Fatalf("%s: %d %q %q", tc.name, code, out, errOut)
		}
	}
	if code, _, _ := runCLI("inspect", dir); code != 1 {
		t.Fatalf("directory accepted: %d", code)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestCLIWriterErrorAndCancellation(t *testing.T) {
	var errOut bytes.Buffer
	if code := run(context.Background(), []string{"loaders", "--json"}, errorWriter{}, &errOut); code != 1 || !strings.Contains(errOut.String(), "write failed") {
		t.Fatalf("write error: %d %q", code, errOut.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	errOut.Reset()
	if code := run(ctx, []string{"inspect", "missing"}, io.Discard, &errOut); code != 1 || !strings.Contains(errOut.String(), "context canceled") {
		t.Fatalf("cancellation: %d %q", code, errOut.String())
	}
}

func TestCLILiftsAMD64Region(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Skipf("test binary is not ELF: %v", err)
	}
	defer file.Close()
	if file.Machine != elf.EM_X86_64 {
		t.Skipf("test binary is not x86-64")
	}
	text := file.Section(".text")
	if text == nil || text.Size == 0 {
		t.Skip("test binary has no .text")
	}
	size := uint64(64)
	if size > text.Size {
		size = text.Size
	}
	code, out, errOut := runCLI("lift", "--json", "--mode", "lowir", "--address", strconv.FormatUint(text.Addr, 10), "--size", strconv.FormatUint(size, 10), path)
	if code != 0 || errOut != "" {
		t.Fatalf("lift: %d %s", code, errOut)
	}
	var report struct {
		Scope       string            `json:"scope"`
		Region      core.Region       `json:"region"`
		IR          *core.LowIR       `json:"ir"`
		Diagnostics []core.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Scope != "amd64-lowir" || report.IR == nil || report.IR.Target.Architecture != core.ArchAMD64 {
		t.Fatalf("bad lift report: %+v", report)
	}
	if report.Region.Address != text.Addr || report.Region.Size != size || len(report.IR.Functions) == 0 || len(report.IR.Functions[0].Blocks) == 0 {
		t.Fatalf("bad region or blocks: %+v", report)
	}
}

func TestCLILiftsBinTrue(t *testing.T) {
	path := "/bin/true"
	file, err := elf.Open(path)
	if err != nil {
		t.Skipf("%s unavailable: %v", path, err)
	}
	defer file.Close()
	if file.Machine != elf.EM_X86_64 {
		t.Skipf("%s is not x86-64", path)
	}
	code, out, errOut := runCLI("lift", "--json", "--mode", "lowir", "--address", strconv.FormatUint(file.Entry, 10), "--size", "16", path)
	if code != 0 || errOut != "" {
		t.Fatalf("lift /bin/true: %d %s", code, errOut)
	}
	var report struct {
		IR *core.LowIR `json:"ir"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.IR == nil || len(report.IR.Functions) == 0 || len(report.IR.Functions[0].Blocks) == 0 || report.IR.Target.Architecture != core.ArchAMD64 {
		t.Fatalf("bad /bin/true lift: %+v", report)
	}

	code, out, errOut = runCLI("lift", path, "--json", "--mode", "lowir", "--address", strconv.FormatUint(file.Entry, 10), "--size", "16")
	if code != 0 || errOut != "" {
		t.Fatalf("lift with flags after file: %d %s", code, errOut)
	}
	report.IR = nil
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.IR == nil || len(report.IR.Functions) == 0 || len(report.IR.Functions[0].Blocks) == 0 {
		t.Fatalf("bad /bin/true lift after file: %+v", report)
	}
}

func TestCLILiftNeedsAddressAndSizeTogether(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCLI("lift", "--address", "1", path); code != 1 {
		t.Fatalf("missing size accepted: %d", code)
	}
	if code, _, _ := runCLI("lift", "--size", "1", path); code != 1 {
		t.Fatalf("missing address accepted: %d", code)
	}
	if code, _, _ := runCLI("lift", "--section", ".missing", path); code != 1 {
		t.Fatalf("missing section accepted: %d", code)
	}
}

func TestCLILiftsAssembly(t *testing.T) {
	path := "/bin/true"
	file, err := elf.Open(path)
	if err != nil {
		t.Skipf("%s unavailable: %v", path, err)
	}
	defer file.Close()
	if file.Machine != elf.EM_X86_64 {
		t.Skipf("%s is not x86-64", path)
	}
	code, out, errOut := runCLI("lift", "--mode", "asm", "--address", strconv.FormatUint(file.Entry, 10), "--size", "16", path)
	if code != 0 || errOut != "" {
		t.Fatalf("lift asm: %d %s", code, errOut)
	}
	for _, fragment := range []string{"Scope: amd64 assembly", "Region: address=", "Instructions:", "0x"} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("asm output lacks %q: %s", fragment, out)
		}
	}

	code, out, errOut = runCLI("lift", "--json", "--mode", "asm", "--address", strconv.FormatUint(file.Entry, 10), "--size", "16", path)
	if code != 0 || errOut != "" {
		t.Fatalf("lift asm json: %d %s", code, errOut)
	}
	var report struct {
		Scope        string `json:"scope"`
		Mode         string `json:"mode"`
		Assembly     string `json:"assembly"`
		Instructions int    `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Scope != "amd64-assembly" || report.Mode != "asm" || report.Assembly == "" || report.Instructions == 0 {
		t.Fatalf("bad asm report: %+v", report)
	}
}

func TestCLILiftsSSAIRAndSource(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, tc := range []struct {
		mode     string
		scope    string
		fragment string
	}{
		{"ssair", "Scope: amd64 SSair", "ret"},
		{"source", "Scope: amd64 source", "sub_0x"},
	} {
		code, out, errOut := runCLI("lift", "--mode", tc.mode, "--address", strconv.FormatUint(file.Entry, 10), "--size", "48", path)
		if code != 0 || errOut != "" {
			t.Fatalf("mode %s: %d %q", tc.mode, code, errOut)
		}
		if !strings.Contains(out, tc.scope) || !strings.Contains(out, tc.fragment) {
			t.Fatalf("mode %s output lacks %q/%q: %s", tc.mode, tc.scope, tc.fragment, out)
		}
	}
}

func TestCLILiftsSSAIRAndSourceJSON(t *testing.T) {
	path := "/bin/true"
	file, err := elf.Open(path)
	if err != nil {
		t.Skipf("%s unavailable: %v", path, err)
	}
	defer file.Close()
	if file.Machine != elf.EM_X86_64 {
		t.Skipf("%s is not x86-64", path)
	}
	for _, tc := range []struct {
		mode     string
		field    string
		scope    string
		fragment string
	}{
		{"ssair", "ssair", "amd64-ssair", "ret"},
		{"source", "source", "amd64-source", "sub_0x"},
	} {
		code, out, errOut := runCLI("lift", "--json", "--mode", tc.mode, "--address", strconv.FormatUint(file.Entry, 10), "--size", "48", path)
		if code != 0 || errOut != "" {
			t.Fatalf("mode %s: %d %s", tc.mode, code, errOut)
		}
		var report struct {
			Scope       string            `json:"scope"`
			SSAIR       string            `json:"ssair"`
			Source      string            `json:"source"`
			IR          *core.LowIR       `json:"ir"`
			Diagnostics []core.Diagnostic `json:"diagnostics"`
		}
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatal(err)
		}
		body := report.SSAIR
		if tc.field == "source" {
			body = report.Source
		}
		if report.Scope != tc.scope || report.IR == nil || !strings.Contains(body, tc.fragment) {
			t.Fatalf("bad %s report: %+v", tc.mode, report)
		}
	}
}

func TestCLIDecodesCString(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "string.c")
	if err := os.WriteFile(source, []byte("const char greeting[] = \"reify-string\";\nint main(void) { return greeting[0]; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "string")
	compile := exec.Command("gcc", "-O0", "-fno-pie", "-no-pie", "-fno-stack-protector", "-o", binary, source)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Skipf("cannot build fixture: %v\n%s", err, output)
	}
	code, out, errOut := runCLI("strings", "--at", "greeting", binary)
	if code != 0 || errOut != "" {
		t.Fatalf("strings: %d %s", code, errOut)
	}
	if !strings.Contains(out, `Value: "reify-string"`) || !strings.Contains(out, "nul-terminated") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	code, out, errOut = runCLI("strings", "--json", "--at", "greeting", binary)
	if code != 0 || errOut != "" {
		t.Fatalf("strings --json: %d %s", code, errOut)
	}
	var report struct {
		Address uint64 `json:"address"`
		Layout  string `json:"layout"`
		Value   string `json:"value"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Value != "reify-string" || report.Layout != "nul-terminated" || report.Address == 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestCLILiftsControlFlowExample(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	directory := t.TempDir()
	binary := filepath.Join(directory, "control_flow")
	source := filepath.Join("..", "..", "test", "examples", "control_flow.c")
	compile := exec.Command("gcc", "-O0", "-fno-pie", "-no-pie", "-fno-stack-protector", "-o", binary, source)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Skipf("cannot build example: %v\n%s", err, output)
	}
	file, err := elf.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	symbols, err := file.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	find := func(name string) *elf.Symbol {
		for index := range symbols {
			if symbols[index].Name == name {
				return &symbols[index]
			}
		}
		return nil
	}
	lift := func(symbol *elf.Symbol) string {
		if symbol == nil || symbol.Size == 0 {
			t.Skip("symbol is unavailable")
		}
		code, out, errOut := runCLI("lift", "--json", "--mode", "source",
			"--address", strconv.FormatUint(symbol.Value, 10),
			"--size", strconv.FormatUint(symbol.Size, 10),
			binary)
		if code != 0 || errOut != "" {
			t.Fatalf("lift %s: %d %s", symbol.Name, code, errOut)
		}
		var report struct {
			Source string `json:"source"`
		}
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatal(err)
		}
		return report.Source
	}

	classify := lift(find("classify"))
	for _, fragment := range []string{"classify(", "if (", "return 0x1", "return 0x0", "return 0xffffffff"} {
		if !strings.Contains(classify, fragment) {
			t.Fatalf("classify lacks %q:\n%s", fragment, classify)
		}
	}
	for _, rejected := range []string{"goto ", "phi("} {
		if strings.Contains(classify, rejected) {
			t.Fatalf("classify keeps %q:\n%s", rejected, classify)
		}
	}
	accumulate := lift(find("accumulate"))
	for _, fragment := range []string{"classify(", "while (", "<= 0x4", "return "} {
		if !strings.Contains(accumulate, fragment) {
			t.Fatalf("accumulate lacks %q:\n%s", fragment, accumulate)
		}
	}
	for _, source := range []string{classify, accumulate} {
		for _, rejected := range []string{"goto ", "phi("} {
			if strings.Contains(source, rejected) {
				t.Fatalf("source keeps %q:\n%s", rejected, source)
			}
		}
	}
}

func TestCLILiftsIntelSyntax(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	code, out, errOut := runCLI("lift", "--mode", "asm", "--asm-syntax", "intel",
		"--address", strconv.FormatUint(file.Entry, 10), "--size", "16", path)
	if code != 0 || errOut != "" {
		t.Fatalf("lift Intel asm: %d %s", code, errOut)
	}
	if !strings.Contains(out, "Scope: amd64 assembly") {
		t.Fatalf("unexpected Intel header: %s", out)
	}
	if strings.Contains(out, "%rsp") || strings.Contains(out, "%rbp") {
		t.Fatalf("Intel syntax contains AT&T registers: %s", out)
	}
}

func TestCLILiftsDefaultTextSection(t *testing.T) {
	path := "/bin/true"
	file, err := elf.Open(path)
	if err != nil {
		t.Skipf("%s unavailable: %v", path, err)
	}
	_ = file.Close()
	if file.Machine != elf.EM_X86_64 {
		t.Skipf("%s is not x86-64", path)
	}
	code, out, errOut := runCLI("lift", path)
	if code != 0 || errOut != "" {
		t.Fatalf("lift default section: %d %s", code, errOut)
	}
	for _, fragment := range []string{"Scope: amd64 assembly", "Region: address=", "Instructions:", "Diagnostics:"} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("lift output lacks %q: %s", fragment, out)
		}
	}
	code, out, errOut = runCLI("lift", "--mode", "lowir", path)
	if code != 0 || errOut != "" {
		t.Fatalf("lift lowir default section: %d %s", code, errOut)
	}
	for _, fragment := range []string{"Scope: amd64 LowIR", "Functions:", "Blocks:", "target:"} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("lowir output lacks %q", fragment)
		}
	}
	if code, out, errOut := runCLI("lift", "--mode", "lowir", "--section", ".text", path); code != 0 || errOut != "" || !strings.Contains(out, "Blocks:") {
		t.Fatalf("lift lowir .text: %d %s %s", code, errOut, out)
	}
}

func TestParseLiftModeAndAsmSyntaxAliases(t *testing.T) {
	for raw, want := range map[string]reify.Mode{
		"asm":    reify.ModeASM,
		"lowir":  reify.ModeLowIR,
		"ssair":  reify.ModeSSAIR,
		"source": reify.ModeSource,
		"all":    reify.ModeAll,
	} {
		got, err := reify.ParseMode(raw)
		if err != nil || got != want {
			t.Fatalf("ParseMode(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for raw, want := range map[string]core.AsmSyntax{
		"att": core.AsmSyntaxATT, "gnu": core.AsmSyntaxATT,
		"intel": core.AsmSyntaxIntel, "": core.AsmSyntaxIntel,
	} {
		got, err := reify.ParseAsmSyntax(raw)
		if err != nil || got != want {
			t.Fatalf("ParseAsmSyntax(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := reify.ParseAsmSyntax("nasm"); err == nil {
		t.Fatal("unknown assembly syntax was accepted")
	}
}

func TestCLILiftsAllMode(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	code, out, errOut := runCLI("lift", "--mode", "all", "--address", strconv.FormatUint(file.Entry, 10), "--size", "16", path)
	if code != 0 || errOut != "" {
		t.Fatalf("lift all: %d %s", code, errOut)
	}
	for _, marker := range []string{"===== 1asm =====", "===== 2lowir =====", "===== 3ssair =====", "===== 4source ====="} {
		if !strings.Contains(out, marker) {
			t.Fatalf("all output lacks %q:\n%s", marker, out)
		}
	}
}

func TestCLISaveReplaceKeepsOldProgramOnFailure(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "tiny.c")
	require.NoError(t, os.WriteFile(source, []byte(`
int helper(int x) { return x + 1; }
int main(void) { return helper(1); }
`), 0o644))
	binary := filepath.Join(directory, "tiny")
	build := exec.Command("gcc", "-O0", "-fno-pie", "-no-pie", "-fno-stack-protector", "-o", binary, source)
	if output, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build tiny program: %v\n%s", err, output)
	}

	db, cleanup := isolatedCLIDB(t)
	defer cleanup()
	name := "cli-replace-" + uuid.NewString()
	code, out, errOut := runCLI("save", "--name", name, "--section", ".text", binary)
	require.Zero(t, code, "save failed: %s\n%s", errOut, out)
	oldCodes, oldMax := programCodeStats(t, db, name)
	require.NotZero(t, oldCodes)

	corrupt := filepath.Join(directory, "corrupt.bin")
	require.NoError(t, os.WriteFile(corrupt, []byte("not an executable"), 0o644))
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"invalid flag", []string{"save", "--name", name, "--replace", "--asm-syntax", "invalid", "--section", ".text", binary}},
		{"missing file", []string{"save", "--name", name, "--replace", "--section", ".text", filepath.Join(directory, "missing")}},
		{"invalid file", []string{"save", "--name", name, "--replace", "--section", ".text", corrupt}},
		{"unknown SSA pass", []string{"save", "--name", name, "--replace", "--ssa-pass", "unknown-pass", "--section", ".text", binary}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runCLI(tc.args...)
			require.NotZero(t, code, "failure case succeeded")
			require.NotEmpty(t, errOut)
			codes, maxID := programCodeStats(t, db, name)
			require.Equal(t, oldCodes, codes, "old program rows changed")
			require.Equal(t, oldMax, maxID, "old program was replaced")
			require.Zero(t, stagingProgramCount(t, db), "staging rows were left behind")
			_, err := ssadb.GetProgram(name, "")
			require.NoError(t, err, "old program disappeared")
		})
	}

	code, out, errOut = runCLI("save", "--name", name, "--replace", "--section", ".text", binary)
	require.Zero(t, code, "replace failed: %s\n%s", errOut, out)
	require.Contains(t, out, "Program: "+name)
	require.Zero(t, stagingProgramCount(t, db), "staging rows were left behind")
	codes, maxID := programCodeStats(t, db, name)
	require.NotZero(t, codes)
	require.NotEqual(t, oldMax, maxID, "replace did not write new instructions")
	_, err := ssadb.GetProgram(name, "")
	require.NoError(t, err)
	code, out, errOut = runCLI("audit", "--program", name, "helper(* as $args) as $call")
	require.Zero(t, code, "audit after replace failed: %s\n%s", errOut, out)
	require.Contains(t, out, "helper")
}

func isolatedCLIDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	database, err := consts.CreateSSAProjectDatabase(consts.SQLiteExtend, filepath.Join(t.TempDir(), "ssa.db"))
	require.NoError(t, err)
	oldSSA := ssadb.GetDB()
	oldProject := consts.GetGormSSAProjectDataBase()
	ssadb.SetDB(database)
	consts.SetGormSSAProjectDatabase(database)
	return database, func() {
		ssadb.SetDB(oldSSA)
		consts.SetGormSSAProjectDatabase(oldProject)
		_ = database.Close()
	}
}

func programCodeStats(t *testing.T, db *gorm.DB, program string) (int, int64) {
	t.Helper()
	var count int
	var maxRowID int64
	require.NoError(t, db.Raw(`SELECT count(*), COALESCE(MAX(id), 0) FROM "`+ssadb.TableIrCodes+`" WHERE program_name = ?`, program).Row().Scan(&count, &maxRowID))
	return count, maxRowID
}

func stagingProgramCount(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.Raw(`SELECT count(*) FROM "`+ssadb.TableIrPrograms+`" WHERE program_name LIKE 'reify-staging-%'`).Row().Scan(&count))
	return count
}
