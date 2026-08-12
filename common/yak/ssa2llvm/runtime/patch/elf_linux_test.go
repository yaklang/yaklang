//go:build linux

package patch

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// TestPatchRemovesUnusedModtextRelocs runs on a real split libyak and verifies
// that patching for "print" (no modules used) removes the .modtext.poc
// relocations, while patching for "poc" keeps them.
func TestPatchRemovesUnusedModtextRelocs(t *testing.T) {
	src := "/tmp/patch_e2e/libyak_split.a"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("split libyak not available: %v", err)
	}
	// print scenario: no modules used -> remove poc relocs.
	work := t.TempDir() + "/libyak_print.a"
	if err := copyFile(work, src); err != nil {
		t.Fatal(err)
	}
	if err := Patch(Request{ArchivePath: work, UsedModules: nil}); err != nil {
		t.Fatalf("patch print: %v", err)
	}
	removed := countModtextRefs(t, work)
	t.Logf("print: .modtext.poc refs after patch = %d", removed)
	if removed != 0 {
		t.Errorf("print should have 0 .modtext.poc refs, got %d", removed)
	}

	// poc scenario: poc used -> keep relocs.
	work2 := t.TempDir() + "/libyak_poc.a"
	if err := copyFile(work2, src); err != nil {
		t.Fatal(err)
	}
	if err := Patch(Request{ArchivePath: work2, UsedModules: []string{"poc"}}); err != nil {
		t.Fatalf("patch poc: %v", err)
	}
	removed2 := countModtextRefs(t, work2)
	t.Logf("poc: .modtext.poc refs after patch = %d", removed2)
	if removed2 == 0 {
		t.Errorf("poc should keep .modtext.poc refs")
	}
}

func copyFile(dst, src string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

func countModtextRefs(t *testing.T, path string) int {
	t.Helper()
	// Reuse parse via elf package by reading archive members; simplest: extract go.o and count.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Find go.o member bytes by scanning ar format.
	pos := 8
	for pos+60 <= len(raw) {
		hdr := raw[pos : pos+60]
		name := strings.TrimRight(string(hdr[0:16]), " /")
		size := parseArSize(hdr[48:58])
		dataStart := pos + 60
		payload := raw[dataStart : dataStart+size]
		if name == "go.o" {
			sections, _, _, _, err := parseELFSections(payload)
			if err != nil {
				t.Fatalf("parse go.o: %v", err)
			}
			modtextSyms, err := collectModtextSymbols(payload, sections, nil)
			if err != nil {
				t.Fatalf("collect modtext syms: %v", err)
			}
			n := 0
			for _, s := range sections {
				if s.typ != 4 || s.size == 0 {
					continue
				}
				rd := payload[s.offset : s.offset+s.size]
				cnt := int(s.size) / elf64RelaSize
				for i := 0; i < cnt; i++ {
					rInfo := binary.LittleEndian.Uint64(rd[i*elf64RelaSize+8:])
					if _, ok := modtextSyms[uint32(rInfo>>32)]; ok {
						n++
					}
				}
			}
			return n
		}
		advance := 60 + size
		if size%2 == 1 {
			advance++
		}
		pos += advance
	}
	return -1
}

func trimSpace(s string) string {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	j := len(s)
	for j > i && s[j-1] == ' ' {
		j--
	}
	return s[i:j]
}

func parseArSize(b []byte) int {
	s := string(b)
	s = trimSpace(s)
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
