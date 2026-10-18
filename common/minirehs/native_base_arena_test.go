//go:build cgo

package minirehs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeBaseArena(t *testing.T) {
	pats := []Pattern{{ID: 0, Expr: `[ab]token`}, {ID: 1, Expr: `[cd]{65}`}, {ID: 2, Expr: `\b[a-z]{3}\b`}, {ID: 3, Expr: `[ab][cd]`}}
	db, err := Compile(pats, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := db.(*database).imageMVS()
	for idx, n := range m.nfas {
		if n == nil {
			t.Fatal("arena case needs actual native unit")
		}
		structSize, base, ok := m.kernel.baseArenaLayout(idx)
		want := uint64((4+n.npos+n.nsym)*n.nword*8 + (n.nsym+129+n.npos)*4)
		if !ok || structSize == 0 || base != want {
			t.Fatalf("rule%d arena ownership/alignment/size: %d/%d/%v want%d", idx, structSize, base, ok, want)
		}
	}
	// The native owner survives overwrite/reuse of every input blob byte.
	blob := bytes.Clone(m.blob)
	k := openMVSKernel(blob, m.n)
	if k == nil {
		t.Fatal("native open")
	}
	defer k.close()
	clear(blob)
	if !k.nfaExists(0, []byte("atoken")) || k.nfaExists(0, []byte("bword")) {
		t.Fatal("native base tables alias caller blob")
	}
	if _, _, ok := k.baseArenaLayout(m.n + 1); ok {
		t.Fatal("out of range layout inspection")
	}
}

func TestNativeBaseArenaInvalidSizes(t *testing.T) {
	n := compileExprToNFA(`[ab]token`)
	blob := buildMVSBlob([]*mvsNFA{n}, nil, nil, nil)
	unitOff := int(binary.LittleEndian.Uint32(blob[24:]))
	for _, change := range []struct {
		off   int
		value uint32
	}{
		{28, 16}, {28, ^uint32(0)}, {24, ^uint32(0)},
		{unitOff, 0x7fffffff}, {unitOff + 4, 0x7fffffff},
		{unitOff + 8, 0}, {unitOff + 8, 0x7fffffff}, {unitOff + 12, 16},
	} {
		bad := bytes.Clone(blob)
		binary.LittleEndian.PutUint32(bad[change.off:], change.value)
		if k := openMVSKernel(bad, 1); k != nil {
			k.close()
			t.Fatalf("invalid size/shape accepted: %+v", change)
		}
	}
}

func TestProgramNativeArenaFixtures(t *testing.T) {
	dir := programEvidenceDir(t)
	for _, set := range []string{"GetRequest", "MITM", "Mixed"} {
		pats := getRequestImagePatterns(t)
		if set == "MITM" {
			pats, _ = compilableMITMPatterns(t)
		} else if set == "Mixed" {
			pats = []Pattern{{ID: 0, Expr: `[ab][cd]`}, {ID: 1, Expr: `[ab]{65}`}, {ID: 2, Expr: `\b[ab]{129}\b`}, {ID: 3, Expr: `\b(?:ab|ac){3}\b`}}
		}
		db, err := Compile(pats, WithBackend(BackendMVS), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		var structSize, bases uint64
		units := 0
		for idx, n := range m.nfas {
			if n == nil {
				continue
			}
			s, size, ok := m.kernel.baseArenaLayout(idx)
			if !ok {
				t.Fatal("invalid native arena", idx)
			}
			structSize = s
			bases += size
			units++
		}
		if m.merged != nil {
			s, size, ok := m.kernel.baseArenaLayout(-1)
			if !ok {
				t.Fatal("invalid merged native arena")
			}
			structSize = s
			bases += size
			units++
		}
		if err := os.WriteFile(filepath.Join(dir, set+".blob"), m.blob, 0600); err != nil {
			t.Fatal(err)
		}
		writeProgramJSON(t, dir, set+"-arena.json", map[string]interface{}{"set": set, "units": units, "base_bytes": bases, "native_struct_bytes": structSize, "base_mallocs": units, "prior_base_mallocs": units * 9})
		if set == "Mixed" {
			for i, val := range []uint32{16, ^uint32(0)} {
				bad := bytes.Clone(m.blob)
				binary.LittleEndian.PutUint32(bad[20+4*m.n+4*units:], val)
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("bad-%d.blob", i)), bad, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		db.Close()
	}
}
