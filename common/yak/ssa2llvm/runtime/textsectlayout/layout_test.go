package textsectlayout

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const go126Module = `package runtime
type moduledata struct {
	sys.NotInHeap
	pcHeader     *pcHeader
	funcnametab  []byte
	cutab        []uint32
	filetab      []byte
	pctab        []byte
	pclntable    []byte
	ftab         []functab
	findfunctab  uintptr
	minpc, maxpc uintptr
	text, etext           uintptr
	noptrdata, enoptrdata uintptr
	data, edata           uintptr
	bss, ebss             uintptr
	noptrbss, enoptrbss   uintptr
	covctrs, ecovctrs     uintptr
	end, gcdata, gcbss    uintptr
	types, etypes         uintptr
	rodata                uintptr
	gofunc                uintptr
	epclntab              uintptr
	textsectmap []textsect
}
`

const go127Module = `package runtime
type moduledata struct {
	sys.NotInHeap
	pcHeader     *pcHeader
	funcnametab  []byte
	cutab        []uint32
	filetab      []byte
	pctab        []byte
	pclntable    []byte
	ftab         []functab
	findfunctab  uintptr
	minpc, maxpc uintptr
	text, etext                uintptr
	noptrdata, enoptrdata      uintptr
	data, edata                uintptr
	bss, ebss                  uintptr
	noptrbss, enoptrbss        uintptr
	covctrs, ecovctrs          uintptr
	end, gcdata, gcbss         uintptr
	types, typedesclen, etypes uintptr
	itaboffset, itabsize       uintptr
	rodata                     uintptr
	gofunc                     uintptr
	epclntab                   uintptr
	textsectmap []textsect
}
`

func TestProbeGo126AndGo127Layouts(t *testing.T) {
	off126, err := ProbeSource(go126Module)
	if err != nil {
		t.Fatal(err)
	}
	if off126 != 0x150 {
		t.Fatalf("go1.26 textsectmap=%#x, want 0x150", off126)
	}
	off127, err := ProbeSource(go127Module)
	if err != nil {
		t.Fatal(err)
	}
	if off127 != 0x168 {
		t.Fatalf("go1.27 textsectmap=%#x, want 0x168", off127)
	}
}

func TestProbeRejectsUnknownTypeAndMissingField(t *testing.T) {
	unknown := strings.Replace(go126Module, "epclntab              uintptr", "epclntab              uintptr\n\topaque chan int", 1)
	if _, err := ProbeSource(unknown); !errors.Is(err, ErrProbe) {
		t.Fatalf("unknown type err=%v, want ErrProbe", err)
	}
	missing := strings.Replace(go126Module, "textsectmap []textsect", "typelinks []int32", 1)
	if _, err := ProbeSource(missing); !errors.Is(err, ErrProbe) {
		t.Fatalf("missing field err=%v, want ErrProbe", err)
	}
	if _, err := ProbeSource("package runtime\n"); !errors.Is(err, ErrProbe) {
		t.Fatalf("no struct err=%v, want ErrProbe", err)
	}
}

func TestValidateFailClosed(t *testing.T) {
	if err := Validate("go1.26.5", 1, 26, 0x150); err != nil {
		t.Fatal(err)
	}
	if err := Validate("go1.27.1", 1, 27, 0x168); err != nil {
		t.Fatal(err)
	}
	err := Validate("go1.30.0", 1, 30, 0x150)
	if !errors.Is(err, ErrUnknownVersion) || !strings.Contains(err.Error(), "需人工验证 moduledata 布局") {
		t.Fatalf("unknown version err=%v", err)
	}
	err = Validate("go1.26.5", 1, 26, 0x168)
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("mismatch err=%v", err)
	}
}

func TestCheckRejectsUnknownVersionEvenWhenProbeSucceeds(t *testing.T) {
	dir := t.TempDir()
	writeToolchain(t, dir, "go1.30.0\n", go126Module)
	_, err := Check(dir)
	if !errors.Is(err, ErrUnknownVersion) {
		t.Fatalf("Check err=%v, want ErrUnknownVersion", err)
	}
}

func TestCheckRejectsBrokenProbe(t *testing.T) {
	dir := t.TempDir()
	writeToolchain(t, dir, "go1.26.5\n", "package runtime\n")
	_, err := Check(dir)
	if !errors.Is(err, ErrProbe) {
		t.Fatalf("Check err=%v, want ErrProbe", err)
	}
}

func TestCurrentToolchainMatchesTable(t *testing.T) {
	res, err := Check(runtime.GOROOT())
	if err != nil {
		t.Fatal(err)
	}
	want, ok := ExpectedOffset(res.Major, res.Minor)
	if !ok || res.Offset != want {
		t.Fatalf("version %s offset %#x, table %#x known=%v", res.Version, res.Offset, want, ok)
	}
}

func writeToolchain(t *testing.T, dir, version, symtab string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(version), 0o644); err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(dir, "src", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "symtab.go"), []byte(symtab), 0o644); err != nil {
		t.Fatal(err)
	}
}
