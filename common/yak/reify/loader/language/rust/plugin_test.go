package rust

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func artifact(t *testing.T, sections ...core.Section) *core.Artifact {
	t.Helper()
	r, err := core.NewByteReader("rust", bytes.NewReader(nil), 0)
	if err != nil {
		t.Fatal(err)
	}
	return &core.Artifact{Kind: core.KindNative, Format: core.FormatELF, Reader: r,
		Target: core.Target{Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Image:  &core.Image{Sections: sections}}
}

func TestDetectAndRecover(t *testing.T) {
	a := artifact(t)
	a.Image.Symbols = []symbol.Symbol{
		{Name: "_RNvCs15kBYyAo9fc_7mycrate3add", Address: 0x1000, Size: 12, Kind: symbol.SymbolFunction},
		{Name: "puts", Address: 0x2000, Kind: symbol.SymbolFunction},
	}
	report := Rust{}.Detect(a)
	if report == nil || report.Language != core.LanguageRust || report.Confidence != core.ConfidenceCertain {
		t.Fatalf("report = %+v", report)
	}
	model := (Rust{}).Strings()
	if model.Layout != core.StringLayoutPointerLength || model.Encoding != core.StringEncodingUTF8 || !model.ValidateUTF8 {
		t.Fatalf("string model = %+v", model)
	}
	recovered, err := (Rust{}).Recover(a)
	if err != nil || len(recovered) != 1 {
		t.Fatalf("recovered = %+v, %v", recovered, err)
	}
	if got := recovered[0]; got.Name != "mycrate::add" || got.Mangled != "_RNvCs15kBYyAo9fc_7mycrate3add" ||
		got.Source != symbol.SourceRustDemangle || got.Size != 12 {
		t.Fatalf("recovered symbol = %+v", got)
	}
}

func TestCommentOnlyIsLikelyAndRecoversNothing(t *testing.T) {
	comment := []byte("GCC version\x00rustc version 1.97.1 (8bab26f4f 2026-07-14)\x00")
	r, err := core.NewByteReader("comment", bytes.NewReader(comment), int64(len(comment)))
	if err != nil {
		t.Fatal(err)
	}
	a := artifact(t, core.Section{Name: ".comment", FileSize: uint64(len(comment))})
	a.Reader = r
	report := Rust{}.Detect(a)
	if report == nil || report.Confidence != core.ConfidenceLikely || report.Runtime != "rustc version 1.97.1 (8bab26f4f 2026-07-14)" {
		t.Fatalf("report = %+v", report)
	}
	if recovered, err := (Rust{}).Recover(a); err != nil || len(recovered) != 0 {
		t.Fatalf("recovered = %+v, %v", recovered, err)
	}
}

func TestDetectWithoutEvidence(t *testing.T) {
	a := artifact(t)
	a.Image.Symbols = []symbol.Symbol{{Name: "main", Address: 0x1000, Kind: symbol.SymbolFunction}}
	if report := (Rust{}).Detect(a); report != nil {
		t.Fatalf("C-like image reported as Rust: %+v", report)
	}
}

func TestMachONameKeepsEncodedForm(t *testing.T) {
	a := artifact(t)
	a.Image.Symbols = []symbol.Symbol{{Name: "__RNvCs15kBYyAo9fc_7mycrate3add", Address: 0x1000, Kind: symbol.SymbolFunction}}
	recovered, err := (Rust{}).Recover(a)
	if err != nil || len(recovered) != 1 {
		t.Fatalf("recovered = %+v, %v", recovered, err)
	}
	if got := recovered[0]; got.Name != "mycrate::add" || got.Mangled != "__RNvCs15kBYyAo9fc_7mycrate3add" {
		t.Fatalf("symbol = %+v", got)
	}
}
