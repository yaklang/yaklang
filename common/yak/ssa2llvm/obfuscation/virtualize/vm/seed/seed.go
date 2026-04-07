// Package seed provides build-specific diversification for VM virtualization.
//
// Each build generates a unique Seed that controls:
//   - Opcode table permutation (different opcode assignments per build)
//   - Encoding key (XOR-based scrambling of the blob)
//   - Handler layout order (dispatcher table shuffling)
//
// This ensures that analysis scripts written for one build cannot be directly
// reused against another, even for the same source code.
package seed

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
)

// Seed holds the per-build diversification parameters.
type Seed struct {
	// Raw is the 32-byte random seed material.
	Raw [32]byte

	// OpcodePermutation maps canonical PIR opcode index → build-specific index.
	// Both arrays have NumOpcodes entries.
	OpcodePermutation []uint8

	// EncodingKey is derived from Raw for XOR-scrambling the blob body.
	EncodingKey [16]byte

	// HandlerOrder is a permuted index list for the dispatcher jump table.
	HandlerOrder []uint8
}

// NumOpcodes is the number of PIR opcodes to permute.
// Must match pir.opCount.
const NumOpcodes = 32

// Generate creates a fresh random Seed with all derived fields populated.
func Generate() (*Seed, error) {
	s := &Seed{}
	if _, err := rand.Read(s.Raw[:]); err != nil {
		return nil, fmt.Errorf("seed: random read failed: %w", err)
	}
	s.derive()
	return s, nil
}

// FromBytes recreates a Seed from its 32-byte raw material.
func FromBytes(raw [32]byte) *Seed {
	s := &Seed{Raw: raw}
	s.derive()
	return s
}

// derive populates OpcodePermutation, EncodingKey, and HandlerOrder from Raw.
func (s *Seed) derive() {
	// Encoding key: first 16 bytes of raw
	copy(s.EncodingKey[:], s.Raw[:16])

	// Opcode permutation: Fisher-Yates shuffle seeded from bytes 16-23
	s.OpcodePermutation = identityPerm(NumOpcodes)
	rng := newDeterministicRNG(s.Raw[16:24])
	fisherYatesShuffle(s.OpcodePermutation, rng)

	// Handler order: same shuffle with bytes 24-31
	s.HandlerOrder = identityPerm(NumOpcodes)
	rng2 := newDeterministicRNG(s.Raw[24:32])
	fisherYatesShuffle(s.HandlerOrder, rng2)
}

// MapOpcode converts a canonical opcode to its build-specific encoding.
func (s *Seed) MapOpcode(canonical uint8) uint8 {
	if int(canonical) >= len(s.OpcodePermutation) {
		return canonical
	}
	return s.OpcodePermutation[canonical]
}

// InverseOpcodeMap returns a reverse mapping: build-specific → canonical.
func (s *Seed) InverseOpcodeMap() []uint8 {
	inv := make([]uint8, len(s.OpcodePermutation))
	for canonical, buildSpecific := range s.OpcodePermutation {
		inv[buildSpecific] = uint8(canonical)
	}
	return inv
}

// XOREncode applies the encoding key to data in-place (repeating key XOR).
func (s *Seed) XOREncode(data []byte) {
	for i := range data {
		data[i] ^= s.EncodingKey[i%len(s.EncodingKey)]
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func identityPerm(n int) []uint8 {
	p := make([]uint8, n)
	for i := range p {
		p[i] = uint8(i)
	}
	return p
}

type deterministicRNG struct {
	state uint64
}

func newDeterministicRNG(seed []byte) *deterministicRNG {
	return &deterministicRNG{state: binary.LittleEndian.Uint64(seed)}
}

// next returns a pseudo-random uint64 using xorshift64.
func (r *deterministicRNG) next() uint64 {
	r.state ^= r.state << 13
	r.state ^= r.state >> 7
	r.state ^= r.state << 17
	return r.state
}

func fisherYatesShuffle(perm []uint8, rng *deterministicRNG) {
	n := len(perm)
	for i := n - 1; i > 0; i-- {
		// Use big.Int for unbiased modular reduction
		j := int(new(big.Int).Mod(
			new(big.Int).SetUint64(rng.next()),
			big.NewInt(int64(i+1)),
		).Int64())
		perm[i], perm[j] = perm[j], perm[i]
	}
}
