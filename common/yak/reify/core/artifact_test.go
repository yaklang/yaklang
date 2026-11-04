package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

type stubParser struct {
	available bool
	request   ParseRequest
	result    *ParseResult
	checkErr  error
}

func (stubParser) Kind() Architecture    { return ArchAMD64 }
func (stubParser) Name() string          { return "stub" }
func (p *stubParser) Available() bool    { return p.available }
func (p *stubParser) Check(Target) error { return p.checkErr }
func (p *stubParser) Parse(_ context.Context, request ParseRequest) (*ParseResult, error) {
	p.request = request
	if p.result != nil {
		return p.result, nil
	}
	return &ParseResult{IR: &LowIR{Target: request.Target}}, nil
}

func testArtifact(t *testing.T) *Artifact {
	t.Helper()
	entry := uint64(0x1000)
	return &Artifact{
		Origin: Origin{Name: "fixture", Size: 100},
		Kind:   KindNative,
		Format: FormatELF,
		Target: Target{OS: OSLinux, Architecture: ArchAMD64, Bits: 64, ByteOrder: LittleEndian},
		Image: &Image{
			ObjectType: "ET_EXEC",
			Entry:      &entry,
			Sections: []Section{
				{Name: ".text", FileOffset: 0x10, FileSize: 0x20, VirtualAddress: 0x1000, MemorySize: 0x1000},
				{Name: ".bss", FileOffset: 0x30, FileSize: 0, VirtualAddress: 0x2000, MemorySize: 0x1000},
			},
		},
		Reader: newReader(t, make([]byte, 100)),
	}
}

func TestValidateTarget(t *testing.T) {
	valid := Target{OS: OSUnknown, Architecture: ArchUnknown, Bits: 64, ByteOrder: LittleEndian}
	if err := ValidateTarget(valid); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Target){
		func(target *Target) { target.Bits = 0 },
		func(target *Target) { target.ByteOrder = ByteOrderUnknown },
		func(target *Target) { target.OS = "" },
		func(target *Target) { target.Architecture = "" },
	} {
		target := valid
		change(&target)
		if ValidateTarget(target) == nil {
			t.Fatalf("invalid target accepted: %+v", target)
		}
	}
}

func TestValidateArtifact(t *testing.T) {
	if err := ValidateArtifact(testArtifact(t)); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Artifact){
		func(a *Artifact) { a.Image.Sections[0].FileOffset = 99 },
		func(a *Artifact) { a.Image.Sections[0].FileSize = 0x100 },
		func(a *Artifact) { a.Image.Sections[1].VirtualAddress = math.MaxUint64 },
		func(a *Artifact) { a.Image = nil },
		func(a *Artifact) { a.Target.Bits = 0 },
		func(a *Artifact) { a.Format = FormatUnknown },
		func(a *Artifact) { a.Origin.Name = "" },
		func(a *Artifact) { a.Origin.Size = -1 },
	} {
		artifact := testArtifact(t)
		change(artifact)
		if ValidateArtifact(artifact) == nil {
			t.Fatalf("invalid artifact accepted")
		}
	}
	if ValidateArtifact(nil) == nil {
		t.Fatal("nil artifact accepted")
	}
	source := &Artifact{Origin: Origin{Name: "x", Size: 1}, Kind: KindSource, Format: "python"}
	if err := ValidateArtifact(source); err != nil {
		t.Fatalf("source artifact rejected: %v", err)
	}
}

func TestArtifactRegionAt(t *testing.T) {
	artifact := testArtifact(t)
	region, err := artifact.RegionAt(0x1010, 4)
	if err != nil {
		t.Fatal(err)
	}
	if region.Address != 0x1010 || region.Offset != 0x20 || region.Size != 4 {
		t.Fatalf("bad region: %+v", region)
	}
	if region, err := artifact.RegionAt(0x1004, 1); err != nil || region.Offset != 0x14 {
		t.Fatalf("backed range rejected: %+v %v", region, err)
	}
	for _, tc := range []struct{ address, size uint64 }{
		{0x101f, 2}, {0x1020, 1}, {0x2000, 1}, {0x3000, 1}, {0x1000, 0}, {math.MaxUint64, 2},
	} {
		if _, err := artifact.RegionAt(tc.address, tc.size); !errors.Is(err, ErrRegion) {
			t.Fatalf("range %#x/%d accepted: %v", tc.address, tc.size, err)
		}
	}
	if _, err := (&Artifact{}).RegionAt(0x1000, 1); !errors.Is(err, ErrRegion) {
		t.Fatal("artifact without image accepted")
	}
	if region, err := artifact.EntryRegion(4); err != nil || region.Address != 0x1000 || region.Offset != 0x10 {
		t.Fatalf("entry region: %+v %v", region, err)
	}
	artifact.Image.Entry = nil
	if _, err := artifact.EntryRegion(4); !errors.Is(err, ErrRegion) {
		t.Fatal("artifact without entry accepted")
	}
}

func TestArtifactParse(t *testing.T) {
	artifact := testArtifact(t)
	parser := &stubParser{available: true}
	artifact.Parser = parser
	result, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 0x10, Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if parser.request.Address != 0x1000 || !bytes.Equal(parser.request.Code, make([]byte, 4)) || result.IR.Target != artifact.Target {
		t.Fatalf("bad parse request: %+v", parser.request)
	}
	artifact.Parser = nil
	if _, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 0x10, Size: 4}); !errors.Is(err, ErrNoParser) {
		t.Fatalf("expected no-parser error, got %v", err)
	}
	artifact.Parser = parser
	if _, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 99, Size: 4}); !errors.Is(err, ErrRegion) {
		t.Fatalf("expected region error, got %v", err)
	}
	if _, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 0x10, Size: 0}); !errors.Is(err, ErrRegion) {
		t.Fatalf("expected empty-region error, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := artifact.Parse(ctx, Region{Address: 0x1000, Offset: 0x10, Size: 4}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	failure := errors.New("unsupported mode")
	artifact.Parser = &stubParser{available: true, checkErr: failure}
	if _, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 0x10, Size: 4}); !errors.Is(err, failure) {
		t.Fatalf("parser check was skipped: %v", err)
	}
	artifact.Parser = &stubParser{available: true, result: &ParseResult{}}
	if _, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 0x10, Size: 4}); err == nil || !strings.Contains(err.Error(), "no LowIR") {
		t.Fatalf("missing LowIR accepted: %v", err)
	}
	otherTarget := artifact.Target
	otherTarget.OS = OSWindows
	artifact.Parser = &stubParser{available: true, result: &ParseResult{IR: &LowIR{Target: otherTarget}}}
	if _, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 0x10, Size: 4}); err == nil || !strings.Contains(err.Error(), "inconsistent target") {
		t.Fatalf("inconsistent target accepted: %v", err)
	}
}

func TestArtifactClose(t *testing.T) {
	ownedCloser := &countingCloser{ReaderAt: bytes.NewReader([]byte{1, 2, 3})}
	owned, err := NewByteReader("owned", ownedCloser, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Artifact{Reader: owned.WithOwner()}).Close(); err != nil || !ownedCloser.closed {
		t.Fatalf("owned close failed: %v %v", err, ownedCloser.closed)
	}
	borrowedCloser := &countingCloser{ReaderAt: bytes.NewReader([]byte{1, 2, 3})}
	borrowed, err := NewByteReader("borrowed", borrowedCloser, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Artifact{Reader: borrowed}).Close(); err != nil || borrowedCloser.closed {
		t.Fatalf("borrowed source was closed: %v %v", err, borrowedCloser.closed)
	}
	var nilArtifact *Artifact
	if err := nilArtifact.Close(); err != nil {
		t.Fatal(err)
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
