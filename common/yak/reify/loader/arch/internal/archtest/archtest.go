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

	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa/pass"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	ssapass "github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
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
	// Data lists the sections the code reads, so a fixture can resolve a load
	// from a literal pool or a string table. The bytes are appended to the same
	// reader the code comes from.
	Data []Section `json:"data"`
	// LowIR is the exact text the region must produce, minus the fact header.
	LowIR []string `json:"lowir"`
	// FrameRemoved is the number of instructions the stack-frame pass removes.
	FrameRemoved *int `json:"frame_removed"`
	// CanaryRemoved is the number of instructions the canary pass removes.
	CanaryRemoved *int `json:"canary_removed"`
	// Params is the parameter count the converter binds from the ABI facts.
	Params *int `json:"params"`
	// Source lists substrings the recovered C source must contain. It is how a
	// fixture asserts what the code means: a load from a data section has to
	// become the string, and a call has to name its callee.
	Source []string `json:"source"`
	// Diagnostics lists the diagnostic codes the parse must report, in order.
	Diagnostics []string `json:"diagnostics"`
}

// Section is one readable section a fixture places in the image. Bytes is a hex
// byte string, or C-style text with escape sequences.
type Section struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Bytes   string `json:"bytes"`
	// Text marks Bytes as literal text instead of hex, so a fixture can write
	// "hello\0" directly.
	Text bool `json:"text"`
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
		if len(testCase.Source) > 0 {
			fresh := parseFixture(t, config, testCase)
			artifact := buildArtifact(t, config, testCase, mustAddress(t, testCase.Address), mustDecodeCode(t, testCase.Code))
			checkSource(t, testCase, fresh, artifact)
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
	// The reader holds the code first, then every data section. A section records
	// the offset its bytes landed at, so a load from its address reads the
	// fixture bytes and the string renderer can name them.
	buffer := make([]byte, 0, len(code)+64)
	buffer = append(buffer, code...)
	image := &core.Image{Sections: []core.Section{{
		Name:           ".text",
		FileSize:       uint64(len(code)),
		VirtualAddress: address,
		MemorySize:     uint64(len(code)),
	}}}
	for _, section := range testCase.Data {
		bytes := mustSectionBytes(t, section)
		offset := uint64(len(buffer))
		buffer = append(buffer, bytes...)
		image.Sections = append(image.Sections, core.Section{
			Name:           section.Name,
			FileOffset:     offset,
			FileSize:       uint64(len(bytes)),
			VirtualAddress: mustAddress(t, section.Address),
			MemorySize:     uint64(len(bytes)),
		})
	}
	reader, err := core.NewByteReader(testCase.Name, strings.NewReader(string(buffer)), int64(len(buffer)))
	if err != nil {
		t.Fatal(err)
	}
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

// mustSectionBytes decodes one section body.
func mustSectionBytes(t *testing.T, section Section) []byte {
	t.Helper()
	if section.Text {
		return []byte(unescape(section.Bytes))
	}
	decoded, err := hex.DecodeString(strings.Join(strings.Fields(section.Bytes), ""))
	if err != nil {
		t.Fatalf("section %q: %v", section.Name, err)
	}
	return decoded
}

// unescape resolves the C escapes a fixture writer uses for a string table.
func unescape(text string) string {
	replacer := strings.NewReplacer(`\0`, "\x00", `\n`, "\n", `\t`, "\t", `\r`, "\r")
	return replacer.Replace(text)
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

// checkSource renders the region as C and requires every expected substring. A
// substring keeps the fixture readable: the rest of the expression is free to
// change, while the meaning it asserts stays fixed.
func checkSource(t *testing.T, testCase Fixture, result *asm2lowir.Result, artifact *core.Artifact) {
	t.Helper()
	converted, err := lowir2ssa.Convert(context.Background(), result.IR)
	if err != nil {
		t.Fatal(err)
	}
	// A fixture that reads a data section asserts what the program means, so it
	// runs the same preparation the API runs: the SSA passes replace an address
	// with the string it points at, and the renderer then prints the literal.
	stringAt := func(address uint64) (string, bool) { return artifact.StringAt(address, 128) }
	ssapass.ApplyDefault(converted.Program, stringAt)
	ssa2source.AnnotateArtifact(converted.Program, artifact)
	source, err := ssa2source.Render(context.Background(), converted.Program)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range testCase.Source {
		if !strings.Contains(source, want) {
			t.Fatalf("source lacks %q:\n%s", want, source)
		}
	}
}

// countParams converts the region and returns the parameter count of the first
// function, which is what the ABI facts decide.
func countParams(t *testing.T, ir *core.LowIR) int {
	t.Helper()
	converted, err := lowir2ssa.Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	function, ok := omap.GetByIndex(converted.Program.Funcs, 0)
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
