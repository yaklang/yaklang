package seed_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/seed"
)

func TestGenerate(t *testing.T) {
	s, err := seed.Generate()
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Len(t, s.OpcodePermutation, seed.NumOpcodes)
	require.Len(t, s.HandlerOrder, seed.NumOpcodes)
}

func TestDeterministicFromBytes(t *testing.T) {
	var raw [32]byte
	for i := range raw {
		raw[i] = byte(i)
	}
	s1 := seed.FromBytes(raw)
	s2 := seed.FromBytes(raw)

	require.Equal(t, s1.OpcodePermutation, s2.OpcodePermutation)
	require.Equal(t, s1.EncodingKey, s2.EncodingKey)
	require.Equal(t, s1.HandlerOrder, s2.HandlerOrder)
}

func TestOpcodePermutationIsPermutation(t *testing.T) {
	s, err := seed.Generate()
	require.NoError(t, err)

	seen := make(map[uint8]bool)
	for _, v := range s.OpcodePermutation {
		require.False(t, seen[v], "duplicate in opcode permutation: %d", v)
		seen[v] = true
	}
	require.Len(t, seen, seed.NumOpcodes)
}

func TestInverseOpcodeMap(t *testing.T) {
	s, err := seed.Generate()
	require.NoError(t, err)

	inv := s.InverseOpcodeMap()
	for canonical := uint8(0); canonical < uint8(seed.NumOpcodes); canonical++ {
		buildSpecific := s.MapOpcode(canonical)
		require.Equal(t, canonical, inv[buildSpecific],
			"inverse mismatch for canonical=%d, build=%d", canonical, buildSpecific)
	}
}

func TestXOREncodeRoundtrip(t *testing.T) {
	s, err := seed.Generate()
	require.NoError(t, err)

	original := []byte("hello protected region")
	data := make([]byte, len(original))
	copy(data, original)

	s.XOREncode(data)
	require.NotEqual(t, original, data, "XOR encoding should change data")

	s.XOREncode(data) // XOR twice = identity
	require.Equal(t, original, data, "double XOR should restore original")
}

func TestDifferentSeedsProduceDifferentPermutations(t *testing.T) {
	s1, err := seed.Generate()
	require.NoError(t, err)
	s2, err := seed.Generate()
	require.NoError(t, err)

	// Extremely unlikely to be identical
	different := false
	for i := range s1.OpcodePermutation {
		if s1.OpcodePermutation[i] != s2.OpcodePermutation[i] {
			different = true
			break
		}
	}
	require.True(t, different, "different seeds should produce different permutations")
}
