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

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

func runCLI(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCLIHelpAndUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		code, out, errOut := runCLI(args...)
		if code != 0 || !strings.Contains(out, "Scope: file headers") || errOut != "" {
			t.Fatalf("help %v: %d %q %q", args, code, out, errOut)
		}
	}
	for _, args := range [][]string{{"analyze"}, {"inspect"}, {"check"}, {"lift"}, {"loaders", "extra"}, {"parsers", "extra"}, {"inspect", "--unknown"}, {"inspect", "a", "b"}, {"lift", "a", "b"}, {"lift", "--mode", "bogus", "a"}} {
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
	if len(loaders) != 3 || loaders[0].Kind != core.FormatELF || loaders[1].Kind != core.FormatMachO || loaders[2].Kind != core.FormatPE {
		t.Fatalf("unexpected loaders: %+v", loaders)
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
	if len(parsers) != 10 || parsers[0].Kind != core.Arch386 {
		t.Fatalf("unexpected parsers: %+v", parsers)
	}
	for _, parser := range parsers {
		if parser.Available != (parser.Kind == core.ArchAMD64) {
			t.Fatalf("unexpected parser availability: %+v", parser)
		}
	}
	if code, out, _ := runCLI("parsers"); code != 0 || !strings.Contains(out, "unavailable") {
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

func TestCLILiftsLinkedExample(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	directory := t.TempDir()
	binary := filepath.Join(directory, "linked_list")
	source := filepath.Join("..", "..", "examples", "linked_list.c")
	compile := exec.Command("gcc", "-O0", "-fno-pie", "-no-pie", "-fstack-protector-strong", "-o", binary, source)
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
	symbol := func(name string) *elf.Symbol {
		for index := range symbols {
			if symbols[index].Name == name {
				return &symbols[index]
			}
		}
		return nil
	}
	liftSource := func(symbol *elf.Symbol) (string, []core.Diagnostic) {
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
			Source      string            `json:"source"`
			Diagnostics []core.Diagnostic `json:"diagnostics"`
		}
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(report.Source, "reify.") {
			t.Fatalf("source uses reify pseudo-functions:\n%s", report.Source)
		}
		if len(report.Diagnostics) != 0 {
			t.Fatalf("%s diagnostics: %+v", symbol.Name, report.Diagnostics)
		}
		return report.Source, report.Diagnostics
	}

	mainSource, _ := liftSource(symbol("main"))
	for _, fragment := range []string{
		"sum_points(", "make_node(", `printf("total=%d`, "free_list(", "if (", "return ",
	} {
		if !strings.Contains(mainSource, fragment) {
			t.Fatalf("main source lacks %q:\n%s", fragment, mainSource)
		}
	}
	makeNodeSource, _ := liftSource(symbol("make_node"))
	allCode, allSource, allErr := runCLI("lift", "--mode", "4source", binary)
	if allCode != 0 || allErr != "" {
		t.Fatalf("lift all linked: %d %s", allCode, allErr)
	}
	if got := callArgumentCount(allSource, "make_node"); got != 2 {
		t.Fatalf("make_node arity = %d:\n%s", got, allSource)
	}
	asmCode, asmText, asmErr := runCLI("lift", "--mode", "1asm", binary)
	if asmCode != 0 || asmErr != "" || !strings.Contains(asmText, "f3 0f 1e fa") || strings.Contains(asmText, "7f 45") {
		t.Fatalf("all-function assembly is wrong: %d %s\n%s", asmCode, asmErr, asmText)
	}
	if strings.Contains(allSource, "function free(") || strings.Contains(allSource, "function printf(") {
		t.Fatalf("PLT functions leaked into source:\n%s", allSource)
	}
	if got := callArgumentCount(allSource, "free_list"); got != 1 {
		t.Fatalf("free_list arity = %d:\n%s", got, allSource)
	}
	for _, fragment := range []string{"malloc(", "strncpy(", "const char * source", "strncpy(v", "source,", "return ", "NULL"} {
		if !strings.Contains(makeNodeSource, fragment) {
			t.Fatalf("make_node source lacks %q:\n%s", fragment, makeNodeSource)
		}
	}
	sumPointsSource, _ := liftSource(symbol("sum_points"))
	for _, fragment := range []string{"while (", "< (uint32_t)arg1", "*(uint32_t *)", "return (uint64_t)v"} {
		if !strings.Contains(sumPointsSource, fragment) {
			t.Fatalf("sum_points source lacks %q:\n%s", fragment, sumPointsSource)
		}
	}
	freeListSource, _ := liftSource(symbol("free_list"))
	for _, fragment := range []string{"while (", "free(", "*(void * *)"} {
		if !strings.Contains(freeListSource, fragment) {
			t.Fatalf("free_list source lacks %q:\n%s", fragment, freeListSource)
		}
	}
	if !strings.Contains(mainSource, "return 0x1;") {
		t.Fatalf("main source lacks the failure return:\n%s", mainSource)
	}
	for _, source := range []string{mainSource, makeNodeSource, sumPointsSource, freeListSource} {
		for _, rejected := range []string{"goto ", "phi(", "load:"} {
			if strings.Contains(source, rejected) {
				t.Fatalf("source keeps %q:\n%s", rejected, source)
			}
		}
	}
	if strings.Contains(allSource, "v730 = v760") {
		t.Fatalf("main keeps a forward alias assignment:\n%s", allSource)
	}
	if strings.Contains(allSource, "v506") {
		t.Fatalf("sum_points keeps frame side-effect value v506:\n%s", allSource)
	}
	freeListBody := allSource[strings.Index(allSource, "free_list("):]
	if end := strings.Index(freeListBody, "\n}"); end >= 0 {
		freeListBody = freeListBody[:end]
	}
	if !strings.Contains(freeListBody, "return;") || strings.Contains(freeListBody, "return *") {
		t.Fatalf("free_list is not void:\n%s", freeListBody)
	}
	freeSSAIR, errOut := runCLI2("lift", "--mode", "3ssair", "--func", "free_list", binary)
	if errOut != "" || !strings.Contains(freeSSAIR, "free_list <ptr64>") || strings.Contains(freeSSAIR, "arg1") {
		t.Fatalf("free_list SSAIR signature is wrong:\n%s", freeSSAIR)
	}
	for _, args := range [][]string{
		{"lift", "--mode", "4source", "--address", "0xsum_points", "--size", "0x5e", binary},
		{"lift", "--mode", "4source", "--func", "sum_points", binary},
	} {
		code, out, errOut := runCLI(args...)
		if code != 0 || errOut != "" || !strings.Contains(out, "sum_points(") {
			t.Fatalf("symbol selection %v: %d %s", args, code, errOut)
		}
	}
	if code, _, errOut := runCLI("lift", "--mode", "4source", "--address", "0xnope", "--size", "1", binary); code == 0 || !strings.Contains(errOut, "does not exist") {
		t.Fatalf("unknown symbol was accepted: %d %s", code, errOut)
	}
	if code, _, errOut := runCLI("lift", "--mode", "4source", "--func", "__TMC_END__", binary); code == 0 || !strings.Contains(errOut, "not a function") {
		t.Fatalf("object symbol was accepted: %d %s", code, errOut)
	}
}

func TestCLILiftsControlFlowExample(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	directory := t.TempDir()
	binary := filepath.Join(directory, "control_flow")
	source := filepath.Join("..", "..", "examples", "control_flow.c")
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
		if strings.Contains(report.Source, "reify.") {
			t.Fatalf("source uses reify pseudo-functions:\n%s", report.Source)
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
	code, out, errOut := runCLI("lift", "--mode", "1asm", "--asm-syntax", "intel",
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

func TestCLIKeepStackFrame(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	code, out, errOut := runCLI("lift", "--mode", "source", "--keep-stack-frame", "--address", strconv.FormatUint(file.Entry, 10), "--size", "48", path)
	if code != 0 || errOut != "" || !strings.Contains(out, "sub_0x") {
		t.Fatalf("keep stack frame: %d %q %q", code, out, errOut)
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
	for raw, want := range map[string]liftMode{
		"1asm": liftModeASM, "asm": liftModeASM,
		"2lowir": liftModeLowIR, "lowir": liftModeLowIR,
		"3ssair": liftModeSSAIR, "ssair": liftModeSSAIR,
		"4source": liftModeSource, "source": liftModeSource,
	} {
		got, err := parseLiftMode(raw)
		if err != nil || got != want {
			t.Fatalf("parseLiftMode(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for raw, want := range map[string]core.AsmSyntax{
		"att": core.AsmSyntaxATT, "gnu": core.AsmSyntaxATT,
		"intel": core.AsmSyntaxIntel, "": core.AsmSyntaxIntel,
	} {
		got, err := parseAsmSyntax(raw)
		if err != nil || got != want {
			t.Fatalf("parseAsmSyntax(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := parseAsmSyntax("nasm"); err == nil {
		t.Fatal("unknown assembly syntax was accepted")
	}
}

func callArgumentCount(text, name string) int {
	index := strings.Index(text, name+"(")
	if index < 0 {
		return -1
	}
	depth := 0
	commas := 0
	for _, r := range text[index+len(name):] {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return commas + 1
			}
		case ',':
			if depth == 1 {
				commas++
			}
		}
	}
	return -1
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

func runCLI2(args ...string) (string, string) {
	code, out, errOut := runCLI(args...)
	if code != 0 {
		return out, errOut
	}
	return out, errOut
}
