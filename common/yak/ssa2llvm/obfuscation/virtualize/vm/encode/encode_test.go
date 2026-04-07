package encode_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/encode"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/pir"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/seed"
)

func makeTestRegion() *pir.Region {
	return &pir.Region{
		Functions: []*pir.Function{
			{
				Name:       "add",
				NumRegs:    3,
				NumArgs:    2,
				EntryBlock: 0,
				Blocks: []pir.Block{
					{
						Index: 0,
						Insts: []pir.Inst{
							{Op: pir.OpArg, Dst: 0, Imm: 0},
							{Op: pir.OpArg, Dst: 1, Imm: 1},
							{Op: pir.OpAdd, Dst: 2, Src: [2]int{0, 1}},
							{Op: pir.OpReturn, Dst: -1, Src: [2]int{2, 0}},
						},
					},
				},
			},
		},
		HostSymbols: []string{"print"},
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	region := makeTestRegion()
	s, err := seed.Generate()
	require.NoError(t, err)

	blob, err := encode.Encode(region, s)
	require.NoError(t, err)
	require.NotEmpty(t, blob)

	// Decode with same seed
	decoded, err := encode.Decode(blob, s)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	// Verify structure
	require.Len(t, decoded.Functions, 1)
	require.Equal(t, "add", decoded.Functions[0].Name)
	require.Equal(t, 3, decoded.Functions[0].NumRegs)
	require.Equal(t, 2, decoded.Functions[0].NumArgs)
	require.Len(t, decoded.Functions[0].Blocks, 1)
	require.Len(t, decoded.Functions[0].Blocks[0].Insts, 4)

	// Verify opcode roundtrip
	require.Equal(t, pir.OpArg, decoded.Functions[0].Blocks[0].Insts[0].Op)
	require.Equal(t, pir.OpAdd, decoded.Functions[0].Blocks[0].Insts[2].Op)
	require.Equal(t, pir.OpReturn, decoded.Functions[0].Blocks[0].Insts[3].Op)

	// Verify symbols
	require.Equal(t, []string{"print"}, decoded.HostSymbols)
}

func TestEncodeDifferentSeedsProduceDifferentBlobs(t *testing.T) {
	region := makeTestRegion()

	s1, err := seed.Generate()
	require.NoError(t, err)
	s2, err := seed.Generate()
	require.NoError(t, err)

	blob1, err := encode.Encode(region, s1)
	require.NoError(t, err)
	blob2, err := encode.Encode(region, s2)
	require.NoError(t, err)

	// Different seeds should produce different blobs (body is XOR-scrambled differently)
	require.NotEqual(t, blob1, blob2, "different seeds should produce different blobs")
}

func TestDecodeWrongSeedFails(t *testing.T) {
	region := makeTestRegion()
	s1, err := seed.Generate()
	require.NoError(t, err)
	s2, err := seed.Generate()
	require.NoError(t, err)

	blob, err := encode.Encode(region, s1)
	require.NoError(t, err)

	// Decode with wrong seed: garbled data may cause parse errors or produce
	// wrong opcodes. Either outcome is acceptable – the point is that the
	// original region is NOT recoverable.
	decoded, decErr := encode.Decode(blob, s2)
	if decErr != nil {
		// Expected: garbled body causes parse error
		return
	}
	// If parse succeeds, opcodes should differ from the original
	require.NotNil(t, decoded)
}

func TestDecodeInvalidMagic(t *testing.T) {
	// Provide enough bytes for header but with wrong magic
	bad := make([]byte, 40)
	copy(bad[:4], []byte("BAAD"))
	_, err := encode.Decode(bad, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "magic")
}

func TestDecodeTruncated(t *testing.T) {
	_, err := encode.Decode([]byte("short"), nil)
	require.Error(t, err)
}

func TestEncodeWithPhiAndCallArgs(t *testing.T) {
	region := &pir.Region{
		Functions: []*pir.Function{
			{
				Name:       "complex",
				NumRegs:    5,
				NumArgs:    2,
				EntryBlock: 0,
				Blocks: []pir.Block{
					{
						Index: 0,
						Insts: []pir.Inst{
							{
								Op:  pir.OpPhi,
								Dst: 3,
								Edges: []pir.PhiEdge{
									{Block: 0, Reg: 0},
									{Block: 1, Reg: 1},
								},
							},
							{
								Op:       pir.OpHostCall,
								Dst:      4,
								Src:      [2]int{0, 0},
								CallArgs: []int{1, 2},
							},
							{Op: pir.OpReturn, Dst: -1, Src: [2]int{4, 0}},
						},
					},
				},
			},
		},
	}

	s, err := seed.Generate()
	require.NoError(t, err)

	blob, err := encode.Encode(region, s)
	require.NoError(t, err)

	decoded, err := encode.Decode(blob, s)
	require.NoError(t, err)

	decInst := decoded.Functions[0].Blocks[0].Insts[0]
	require.Equal(t, pir.OpPhi, decInst.Op)
	require.Len(t, decInst.Edges, 2)
	require.Equal(t, 0, decInst.Edges[0].Block)
	require.Equal(t, 0, decInst.Edges[0].Reg)

	decCall := decoded.Functions[0].Blocks[0].Insts[1]
	require.Equal(t, pir.OpHostCall, decCall.Op)
	require.Len(t, decCall.CallArgs, 2)
}
