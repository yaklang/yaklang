package core

import "testing"

func TestSectionAtPrefersFileBackedOverlap(t *testing.T) {
	// A TLS .tbss can share virtual addresses with the data that follows it.
	// Its bytes are not in the file, so a read must not pick it first.
	image := &Image{Sections: []Section{
		{Name: ".tbss", VirtualAddress: 0x2000, MemorySize: 0x40, FileSize: 0},
		{Name: ".data.rel.ro", VirtualAddress: 0x2020, MemorySize: 0x100, FileSize: 0x100, FileOffset: 0x400},
	}}
	section, ok := image.SectionAt(0x2030)
	if !ok || section.Name != ".data.rel.ro" {
		t.Fatalf("SectionAt = %+v, %v", section, ok)
	}
	if section, ok := image.SectionAt(0x2010); !ok || section.Name != ".tbss" {
		t.Fatalf("zero-fill section lost: %+v, %v", section, ok)
	}
	if _, ok := image.SectionAt(0x3000); ok {
		t.Fatal("unmapped address matched")
	}
}
