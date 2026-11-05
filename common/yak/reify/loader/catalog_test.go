package loader

import (
	"bytes"
	"context"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/native"
)

func writeValue(t *testing.T, buffer *bytes.Buffer, value any) {
	t.Helper()
	if err := binary.Write(buffer, binary.LittleEndian, value); err != nil {
		t.Fatal(err)
	}
}

func minimalELF(t *testing.T) []byte {
	t.Helper()
	raw := make([]byte, 64)
	copy(raw, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(raw[16:], 2)  // ET_EXEC
	binary.LittleEndian.PutUint16(raw[18:], 62) // EM_X86_64
	binary.LittleEndian.PutUint32(raw[20:], 1)  // EV_CURRENT
	binary.LittleEndian.PutUint64(raw[24:], 0x401000)
	binary.LittleEndian.PutUint16(raw[52:], 64) // e_ehsize
	binary.LittleEndian.PutUint16(raw[58:], 64) // e_shentsize
	return raw
}

func minimalPE(t *testing.T) []byte {
	t.Helper()
	raw := make([]byte, 0x200)
	copy(raw, "MZ")
	binary.LittleEndian.PutUint32(raw[0x3c:], 0x80)
	optional := pe.OptionalHeader64{
		Magic: 0x20b, ImageBase: 0x140000000, AddressOfEntryPoint: 0x1000,
		SectionAlignment: 0x1000, FileAlignment: 0x200, SizeOfImage: 0x2000, SizeOfHeaders: 0x200,
		Subsystem: pe.IMAGE_SUBSYSTEM_WINDOWS_CUI, NumberOfRvaAndSizes: 16,
	}
	var buffer bytes.Buffer
	buffer.WriteString("PE\x00\x00")
	writeValue(t, &buffer, pe.FileHeader{
		Machine: pe.IMAGE_FILE_MACHINE_AMD64, SizeOfOptionalHeader: uint16(binary.Size(optional)), Characteristics: 0x0002,
	})
	writeValue(t, &buffer, optional)
	copy(raw[0x80:], buffer.Bytes())
	return raw
}

func minimalMachO(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writeValue(t, &buffer, macho.FileHeader{Magic: macho.Magic64, Cpu: macho.CpuArm64, Type: macho.TypeExec})
	writeValue(t, &buffer, uint32(0)) // reserved in the 64-bit header
	return buffer.Bytes()
}

func openFixture(t *testing.T, catalog *Catalog, name string, raw []byte) (*core.Artifact, error) {
	t.Helper()
	return catalog.Open(context.Background(), name, bytes.NewReader(raw), int64(len(raw)))
}

func TestDefaultCatalog(t *testing.T) {
	catalog := Default
	loaders := catalog.Loaders()
	if len(loaders) != 3 || loaders[0].Kind != core.FormatELF || loaders[1].Kind != core.FormatMachO || loaders[2].Kind != core.FormatPE {
		t.Fatalf("unexpected loaders: %+v", loaders)
	}
	parsers := catalog.Parsers()
	if len(parsers) != 10 || parsers[0].Kind != core.Arch386 {
		t.Fatalf("unexpected parsers: %+v", parsers)
	}
	for _, parser := range parsers {
		if parser.Available != (parser.Kind == core.ArchAMD64) {
			t.Fatalf("unexpected parser availability: %+v", parser)
		}
	}
	if _, ok := catalog.Loader(core.FormatELF); !ok {
		t.Fatal("ELF loader not registered")
	}
	if _, ok := catalog.Parser(core.ArchAMD64); !ok {
		t.Fatal("amd64 parser not registered")
	}
	if _, ok := catalog.Loader("cobol"); ok {
		t.Fatal("unregistered format returned a loader")
	}
}

type fakeLoader struct {
	kind  core.Format
	match string
}

func (l fakeLoader) Kind() core.Format        { return l.kind }
func (fakeLoader) Name() string               { return "fake" }
func (fakeLoader) Description() string        { return "test loader" }
func (l fakeLoader) Match(prefix []byte) bool { return bytes.HasPrefix(prefix, []byte(l.match)) }
func (l fakeLoader) Check(*core.ByteReader) (*core.CheckResult, error) {
	return &core.CheckResult{Kind: core.KindNative, Format: l.kind, Target: testTarget(core.ArchAMD64)}, nil
}
func (l fakeLoader) Load(context.Context, *core.ByteReader) (*core.Artifact, error) {
	return &core.Artifact{Kind: core.KindNative, Format: l.kind}, nil
}

type fakeParser struct {
	kind   core.Architecture
	usable bool
	calls  int
}

func (p fakeParser) Kind() core.Architecture { return p.kind }
func (fakeParser) Name() string              { return "fake" }
func (p fakeParser) Available() bool         { return p.usable }
func (fakeParser) Check(core.Target) error   { return nil }
func (p *fakeParser) Parse(_ context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	p.calls++
	return &core.ParseResult{
		IR: &core.LowIR{Target: request.Target, Functions: []core.Function{{
			ID:      1,
			Address: request.Address,
			Blocks:  []core.Block{{ID: 1, Address: request.Address}},
		}}},
	}, nil
}

func testTarget(arch core.Architecture) core.Target {
	return core.Target{OS: core.OSLinux, Architecture: arch, Bits: 64, ByteOrder: core.LittleEndian}
}

func TestNewCatalogValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		loaders  []core.Loader
		parsers  []core.Parser
		expected string
	}{
		{"nil loader", []core.Loader{nil}, nil, "nil loader"},
		{"empty loader kind", []core.Loader{fakeLoader{}}, nil, "format kind"},
		{"unknown loader kind", []core.Loader{fakeLoader{kind: core.FormatUnknown}}, nil, "format kind"},
		{"duplicate loader", []core.Loader{fakeLoader{kind: core.FormatELF}, fakeLoader{kind: core.FormatELF}}, nil, "duplicate loader"},
		{"nil parser", nil, []core.Parser{nil}, "nil parser"},
		{"empty parser kind", nil, []core.Parser{&fakeParser{}}, "architecture kind"},
		{"unknown parser kind", nil, []core.Parser{&fakeParser{kind: core.ArchUnknown}}, "architecture kind"},
		{"duplicate parser", nil, []core.Parser{&fakeParser{kind: core.ArchAMD64}, &fakeParser{kind: core.ArchAMD64}}, "duplicate parser"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCatalog(tc.loaders, tc.parsers)
			if err == nil || !strings.Contains(err.Error(), tc.expected) {
				t.Fatalf("wanted %q, got %v", tc.expected, err)
			}
		})
	}
	if _, err := NewCatalog(nil, nil); err != nil {
		t.Fatalf("empty catalog rejected: %v", err)
	}
}

func TestCatalogOpenFormatAlignment(t *testing.T) {
	for _, tc := range []struct {
		name       string
		raw        []byte
		format     core.Format
		loaderName string
		os         core.OperatingSystem
		arch       core.Architecture
		parser     string
		available  bool
		notes      int
	}{
		{"elf", minimalELF(t), core.FormatELF, "ELF", core.OSUnknown, core.ArchAMD64, "amd64", true, 0},
		{"pe", minimalPE(t), core.FormatPE, "PE", core.OSWindows, core.ArchAMD64, "amd64", true, 0},
		{"macho", minimalMachO(t), core.FormatMachO, "Mach-O", core.OSUnknown, core.ArchARM64, "arm64", false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := openFixture(t, Default, tc.name, tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.Format != tc.format || artifact.Loader.Name() != tc.loaderName {
				t.Fatalf("loader alignment lost: %+v", artifact)
			}
			if artifact.Target.OS != tc.os || artifact.Target.Architecture != tc.arch {
				t.Fatalf("unexpected target: %+v", artifact.Target)
			}
			if artifact.Parser == nil || artifact.Parser.Name() != tc.parser || artifact.Parser.Available() != tc.available {
				t.Fatalf("parser alignment lost: %+v", artifact.Parser)
			}
			if artifact.Reader == nil || artifact.Reader.Kind() != artifact.Target.ByteOrder {
				t.Fatalf("reader order not aligned: %+v", artifact.Reader)
			}
			if len(artifact.Diagnostics) != tc.notes {
				t.Fatalf("wanted %d diagnostics, got %+v", tc.notes, artifact.Diagnostics)
			}
			found := false
			for _, diagnostic := range artifact.Diagnostics {
				if diagnostic.Code == "decoder-unavailable" {
					found = true
				}
			}
			if !tc.available && !found {
				t.Fatalf("decoder status not reported: %+v", artifact.Diagnostics)
			}
		})
	}
}

func TestCatalogCheck(t *testing.T) {
	result, err := Default.Check(context.Background(), "fixture.elf", bytes.NewReader(minimalELF(t)), int64(len(minimalELF(t))))
	if err != nil {
		t.Fatal(err)
	}
	if result.Format != core.FormatELF || result.Target.Architecture != core.ArchAMD64 || result.Parser == nil || result.Parser.Name != "amd64" {
		t.Fatalf("unexpected check result: %+v", result)
	}
	if result.Target.ByteOrder != core.LittleEndian || result.Loader.Kind != core.FormatELF {
		t.Fatalf("check lost loader or reader facts: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Default.Check(ctx, "fixture.elf", bytes.NewReader(minimalELF(t)), 64); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestCatalogRejectsUnsupportedInput(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("plain text"), {0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 61}} {
		if _, err := openFixture(t, Default, "input", raw); !errors.Is(err, ErrUnsupportedFormat) {
			t.Fatalf("expected unsupported format, got %v", err)
		}
	}
	if _, err := openFixture(t, Default, "broken.elf", []byte{0x7f, 'E', 'L', 'F'}); err == nil || errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("truncated ELF should fail in the loader, got %v", err)
	}
}

func TestCatalogDetectAmbiguity(t *testing.T) {
	catalog, err := NewCatalog([]core.Loader{
		fakeLoader{kind: core.FormatELF, match: "ZZ"},
		fakeLoader{kind: core.FormatPE, match: "ZZ"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openFixture(t, catalog, "ambiguous", []byte("ZZZZ")); !errors.Is(err, ErrAmbiguousFormat) {
		t.Fatalf("expected ambiguity, got %v", err)
	}
}

func TestCatalogParserSelectionAndParse(t *testing.T) {
	parser := &fakeParser{kind: core.ArchAMD64, usable: true}
	catalog, err := NewCatalog([]core.Loader{native.ELF{}}, []core.Parser{parser})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := openFixture(t, catalog, "fixture.elf", minimalELF(t))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Parser != parser || len(artifact.Diagnostics) != 0 {
		t.Fatalf("available parser not selected cleanly: %+v", artifact.Diagnostics)
	}
	result, err := artifact.Parse(context.Background(), core.Region{Address: 0x401000, Offset: 0, Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if parser.calls != 1 || result.IR == nil || len(result.IR.Functions) != 1 || len(result.IR.Functions[0].Blocks) != 1 {
		t.Fatalf("parse did not reach the parser: %+v", result)
	}
	other := &fakeParser{kind: core.ArchARM64, usable: true}
	catalog, err = NewCatalog(nil, []core.Parser{other})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := catalog.Parser(core.ArchARM64); !ok {
		t.Fatal("custom parser not registered")
	}
	if _, ok := catalog.Parser(core.ArchAMD64); ok {
		t.Fatal("unregistered parser returned")
	}
}

type countingCloser struct {
	io.ReaderAt
	closed bool
}

func (c *countingCloser) Close() error {
	c.closed = true
	return nil
}

func TestCatalogOpenFileOwnership(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.elf")
	raw := minimalELF(t)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := Default.OpenFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Origin.Name != path || artifact.Origin.Size != int64(len(raw)) {
		t.Fatalf("bad origin: %+v", artifact.Origin)
	}
	if err := artifact.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("input changed: %v", err)
	}

	borrowed := &countingCloser{ReaderAt: bytes.NewReader(raw)}
	artifact, err = Default.Open(context.Background(), "borrowed", borrowed, int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.Close(); err != nil || borrowed.closed {
		t.Fatalf("borrowed source was closed: %v %v", err, borrowed.closed)
	}

	for _, bad := range []string{dir, filepath.Join(dir, "missing")} {
		if _, err := Default.OpenFile(context.Background(), bad); err == nil {
			t.Fatalf("invalid path accepted: %s", bad)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Default.OpenFile(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
