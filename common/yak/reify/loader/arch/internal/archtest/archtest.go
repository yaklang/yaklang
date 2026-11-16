// Package archtest runs the fixture files every architecture package ships.
//
// One fixture is one JSON object: bytes in, expectations out. A file holds an
// array of them. The harness is shared so a new architecture only supplies its
// parser, its target and a testdata directory; it does not copy a driver.
package archtest

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa/pass"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Fixture is one case. Every expectation is optional; a case asserts only the
// fields it sets, so a decode case does not have to repeat a frame count.
type Fixture struct {
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Symbols []Symbol `json:"symbols"`
	// Code is one hex byte string per line with an assembly comment, for
	// example "01 00 80 e2 // add r0, r0, #1".
	Code []string `json:"code"`
	// LowIR is the exact text the region must produce, minus the fact header.
	LowIR []string `json:"lowir"`
	// FrameRemoved is the number of instructions the stack-frame pass removes.
	FrameRemoved *int `json:"frame_removed"`
	// CanaryRemoved is the number of instructions the canary pass removes.
	CanaryRemoved *int `json:"canary_removed"`
	// Params is the parameter count the converter binds from the ABI facts.
	Params *int `json:"params"`
	// Diagnostics lists the diagnostic codes the parse must report, in order.
	Diagnostics []string `json:"diagnostics"`
}

// Symbol is one function symbol the fixture places in the image.
type Symbol struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Config wires one architecture package to the harness.
type Config struct {
	// FS holds the testdata directory.
	FS fs.FS
	// Dir is the directory inside FS that holds the JSON files.
	Dir string
	// Parser decodes the fixtures.
	Parser core.Parser
	// Target is the target the fixtures decode as.
	Target core.Target
	// Facts overrides the facts the parser reports. It is only needed when the
	// fixture exercises a fact the parser does not emit yet.
	Facts *core.ArchFacts
}

// Run executes every JSON file under Config.Dir.
func Run(t *testing.T, config Config) {
	t.Helper()
	entries, err := fs.ReadDir(config.FS, config.Dir)
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := config.Dir + "/" + entry.Name()
		data, err := fs.ReadFile(config.FS, path)
		if err != nil {
			t.Fatal(err)
		}
		var cases []Fixture
		if err := json.Unmarshal(data, &cases); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if len(cases) == 0 {
			t.Fatalf("%s: no cases", path)
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		t.Run(name, func(t *testing.T) {
			for _, testCase := range cases {
				runCase(t, config, testCase)
			}
		})
		ran++
	}
	if ran == 0 {
		t.Fatalf("%s: no fixture files", config.Dir)
	}
}

func runCase(t *testing.T, config Config, testCase Fixture) {
	t.Helper()
	t.Run(testCase.Name, func(t *testing.T) {
		result := parseFixture(t, config, testCase)
		checkDiagnostics(t, testCase, result.Diagnostics)
		if len(testCase.LowIR) > 0 {
			checkLowIR(t, testCase, result)
		}
		// Each count starts from its own parse. The two passes interact — the
		// canary spill sits in the frame the other pass removes — so measuring
		// them on one shared program would make each number depend on the other.
		if testCase.FrameRemoved != nil {
			fresh := parseFixture(t, config, testCase)
			checkCount(t, "frame_removed", *testCase.FrameRemoved, removeFrames(fresh.IR))
		}
		if testCase.CanaryRemoved != nil {
			fresh := parseFixture(t, config, testCase)
			checkCount(t, "canary_removed", *testCase.CanaryRemoved, removeCanaries(fresh.IR))
		}
		if testCase.Params != nil {
			fresh := parseFixture(t, config, testCase)
			checkCount(t, "params", *testCase.Params, countParams(t, fresh.IR))
		}
	})
}

// parseFixture decodes one fixture and applies the configured facts.
func parseFixture(t *testing.T, config Config, testCase Fixture) *asm2lowir.Result {
	t.Helper()
	address := mustAddress(t, testCase.Address)
	code := mustDecodeCode(t, testCase.Code)
	artifact := buildArtifact(t, config, testCase, address, code)
	options := []asm2lowir.Option{}
	if len(testCase.Symbols) > 0 {
		options = append(options, asm2lowir.WithAll())
	} else {
		options = append(options, asm2lowir.WithRegion(core.Region{Address: address, Size: uint64(len(code))}), asm2lowir.WithExact())
	}
	result, err := asm2lowir.Parse(context.Background(), artifact, options...)
	if err != nil {
		t.Fatal(err)
	}
	if config.Facts != nil {
		result.IR.Frame = config.Facts.Frame
		result.IR.Registers = config.Facts.Registers
		result.IR.Canary = config.Facts.Canary
	}
	return result
}

// buildArtifact wraps the fixture bytes in the smallest artifact the parser
// accepts: one executable section plus the fixture symbols.
func buildArtifact(t *testing.T, config Config, testCase Fixture, address uint64, code []byte) *core.Artifact {
	t.Helper()
	reader, err := core.NewByteReader(testCase.Name, strings.NewReader(string(code)), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	image := &core.Image{Sections: []core.Section{{
		Name:           ".text",
		FileSize:       uint64(len(code)),
		VirtualAddress: address,
		MemorySize:     uint64(len(code)),
	}}}
	for _, item := range testCase.Symbols {
		image.Symbols = append(image.Symbols, sym.Symbol{
			Name:    item.Name,
			Address: mustAddress(t, item.Address),
			Kind:    sym.SymbolFunction,
		})
	}
	return &core.Artifact{
		Origin: core.Origin{Name: testCase.Name, Size: int64(len(code))},
		Kind:   core.KindNative,
		Format: core.FormatELF,
		Target: config.Target,
		Image:  image,
		Reader: reader,
		Parser: config.Parser,
		Hints:  &core.BoundaryHints{ExecSections: []core.Region{{Address: address, Size: uint64(len(code))}}},
	}
}

func checkLowIR(t *testing.T, testCase Fixture, result *asm2lowir.Result) {
	t.Helper()
	got := strings.TrimRight(withoutFacts(lowirtext.Format(result.IR)), "\n")
	want := strings.TrimRight(withoutFacts(strings.Join(testCase.LowIR, "\n")), "\n")
	if got != want {
		t.Fatalf("LowIR mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func checkDiagnostics(t *testing.T, testCase Fixture, diagnostics []core.Diagnostic) {
	t.Helper()
	if testCase.Diagnostics == nil {
		return
	}
	got := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		got = append(got, diagnostic.Code)
	}
	if strings.Join(got, ",") != strings.Join(testCase.Diagnostics, ",") {
		t.Fatalf("diagnostics = %v, want %v", got, testCase.Diagnostics)
	}
}

func checkCount(t *testing.T, field string, want, got int) {
	t.Helper()
	if want != got {
		t.Fatalf("%s = %d, want %d", field, got, want)
	}
}

// removeFrames runs the frame pass over every function and returns the number of
// removed instructions.
func removeFrames(ir *core.LowIR) int {
	removed := 0
	for index := range ir.Functions {
		removed += pass.RemoveFrame(&ir.Functions[index], ir.Facts())
	}
	return removed
}

// removeCanaries runs the canary pass over every function.
func removeCanaries(ir *core.LowIR) int {
	removed := 0
	for index := range ir.Functions {
		removed += pass.RemoveStackCanary(&ir.Functions[index], ir.Facts())
	}
	return removed
}

// countParams converts the region and returns the parameter count of the first
// function, which is what the ABI facts decide.
func countParams(t *testing.T, ir *core.LowIR) int {
	t.Helper()
	converted, err := lowir2ssa.Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	function, ok := converted.Program.Funcs.GetByIndex(0)
	if !ok {
		return 0
	}
	return len(function.Params)
}

// withoutFacts removes the architecture fact header. A fixture describes what one
// encoding produces; the facts are asserted by their own fields.
func withoutFacts(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "frame: ") || strings.HasPrefix(line, "registers: ") || strings.HasPrefix(line, "canary: ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func mustDecodeCode(t *testing.T, lines []string) []byte {
	t.Helper()
	var code []byte
	for _, line := range lines {
		bytesPart, comment, hasComment := strings.Cut(line, "//")
		if !hasComment || strings.TrimSpace(comment) == "" {
			t.Fatalf("line without an assembly comment: %q", line)
		}
		decoded, err := hex.DecodeString(strings.Join(strings.Fields(bytesPart), ""))
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		code = append(code, decoded...)
	}
	if len(code) == 0 {
		t.Fatal("no bytes")
	}
	return code
}

func mustAddress(t *testing.T, text string) uint64 {
	t.Helper()
	if text == "" {
		return 0
	}
	value, err := strconv.ParseUint(text, 0, 64)
	if err != nil {
		t.Fatalf("bad address %q: %v", text, err)
	}
	return value
}
