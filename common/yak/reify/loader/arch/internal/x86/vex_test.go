package x86

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// TestVectorLengths checks the decoded length against the encodings printed by
// llvm-mc for the same assembly. A wrong length desynchronizes the function, so
// every form that appears in a compiled binary has a case here.
func TestVectorLengths(t *testing.T) {
	cases := []struct {
		name   string
		code   []byte
		length int
		memory bool
		store  bool
	}{
		{"vmovdqu", []byte{0xc5, 0xfe, 0x6f, 0x50, 0x20}, 5, true, false},
		{"vptest", []byte{0xc4, 0xe2, 0x7d, 0x17, 0xdb}, 5, false, false},
		{"vmovdqu64", []byte{0x62, 0xf1, 0xfe, 0x48, 0x6f, 0x50, 0x01}, 7, true, false},
		{"vzeroupper", []byte{0xc5, 0xf8, 0x77}, 3, false, false},
		{"vpcmpeqb", []byte{0xc5, 0xd5, 0x74, 0xf4}, 4, false, false},
		{"vpmovmskb", []byte{0xc5, 0xfd, 0xd7, 0xce}, 4, false, false},
		{"vgf2p8affineqb", []byte{0x62, 0xf3, 0xdd, 0x48, 0xce, 0xd3, 0x00}, 7, false, false},
		{"vpermb", []byte{0x62, 0xf2, 0x5d, 0x48, 0x8d, 0xd3}, 6, false, false},
		{"vmovups store", []byte{0xc5, 0xfc, 0x11, 0x08}, 4, true, true},
		{"vpternlogd", []byte{0x62, 0xf3, 0x5d, 0x48, 0x25, 0xd3, 0x96}, 7, false, false},
		{"vpshufd", []byte{0xc5, 0xfd, 0x70, 0xd1, 0x55}, 5, false, false},
		{"vpgatherdd", []byte{0xc4, 0xe2, 0x65, 0x90, 0x0c, 0x90}, 6, true, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			encoding, err := parseVector(64, testCase.code)
			if err != nil {
				t.Fatal(err)
			}
			if encoding.size != testCase.length {
				t.Fatalf("length = %d, want %d", encoding.size, testCase.length)
			}
			if encoding.memory != testCase.memory {
				t.Fatalf("memory = %v, want %v", encoding.memory, testCase.memory)
			}
			if encoding.store != testCase.store {
				t.Fatalf("store = %v, want %v", encoding.store, testCase.store)
			}
		})
	}
}

// TestVectorPrefixScope checks the 32-bit discrimination against LES, LDS and
// BOUND, which share the same lead bytes.
func TestVectorPrefixScope(t *testing.T) {
	if !vectorPrefix(64, []byte{0xc5, 0xf8}) {
		t.Fatal("64-bit C5 must be VEX")
	}
	if vectorPrefix(32, []byte{0xc5, 0x00}) {
		t.Fatal("32-bit C5 with a memory operand is LDS")
	}
	if !vectorPrefix(32, []byte{0xc5, 0xc0}) {
		t.Fatal("32-bit C5 with mod=11 is VEX")
	}
}

// TestVectorLiftStaysOpaque checks one VEX move lifts as an intrinsic with a
// register destination.
func TestVectorLiftStaysOpaque(t *testing.T) {
	reader, err := core.NewByteReader("probe", bytes.NewReader([]byte{0xc5, 0xf8, 0x6f, 0xc1}), 4)
	if err != nil {
		t.Fatal(err)
	}
	profile := Profile64()
	result, err := parse(context.Background(), profile, core.ParseRequest{
		Target:  core.Target{Architecture: core.ArchAMD64, OS: core.OSLinux},
		Address: 0x4000,
		Reader:  reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	instruction := result.IR.Functions[0].Blocks[0].Instructions[0]
	if instruction.Kind != core.OpIntrinsic {
		t.Fatalf("kind = %v", instruction.Kind)
	}
	if instruction.Operands[0].Name != "x86.vmovdqu" {
		t.Fatalf("symbol = %q", instruction.Operands[0].Name)
	}
	if instruction.Effects&core.EffectWriteRegister == 0 {
		t.Fatal("destination register was not written")
	}
}
