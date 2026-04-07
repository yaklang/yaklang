// Package encode serializes PIR regions into a VM blob format.
//
// The blob layout (after XOR encoding) is:
//
//	┌──────────────────────────────────────────────────────┐
//	│ Header (fixed 32 bytes)                              │
//	│   magic    [4]byte   "PIRB"                          │
//	│   version  uint32                                    │
//	│   numFuncs uint32                                    │
//	│   numSyms  uint32                                    │
//	│   bodyLen  uint32                                    │
//	│   reserved [12]byte                                  │
//	├──────────────────────────────────────────────────────┤
//	│ Symbol table (variable length)                       │
//	│   for each symbol: uint16 len + UTF-8 bytes          │
//	├──────────────────────────────────────────────────────┤
//	│ Function table (variable length)                     │
//	│   for each function:                                 │
//	│     nameLen uint16, name []byte                      │
//	│     numRegs uint16, numArgs uint16, numSlots uint16  │
//	│     numBlocks uint16                                 │
//	│     entryBlock uint16                                │
//	│     for each block:                                  │
//	│       numInsts uint16                                │
//	│       for each inst:                                 │
//	│         opcode uint8 (seed-permuted)                 │
//	│         dst int16, src0 int16, src1 int16            │
//	│         imm int64                                    │
//	│         block int16, auxBlock int16                   │
//	│         numEdges uint16 (if phi)                     │
//	│           for each edge: blockIdx int16, reg int16   │
//	│         numCallArgs uint16 (if call)                 │
//	│           for each: reg int16                        │
//	└──────────────────────────────────────────────────────┘
package encode

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/pir"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/seed"
)

var blobMagic = [4]byte{'P', 'I', 'R', 'B'}

const blobVersion = 1
const headerSize = 32

// Encode serializes a PIR Region into a protected blob, applying the seed's
// opcode permutation and XOR encoding.
func Encode(region *pir.Region, s *seed.Seed) ([]byte, error) {
	var body bytes.Buffer

	// Symbol table
	for _, sym := range region.HostSymbols {
		if err := writeString(&body, sym); err != nil {
			return nil, fmt.Errorf("encode symbol: %w", err)
		}
	}

	// Function table
	for _, fn := range region.Functions {
		if err := encodeFunction(&body, fn, s); err != nil {
			return nil, fmt.Errorf("encode function %s: %w", fn.Name, err)
		}
	}

	bodyBytes := body.Bytes()

	// Header
	var hdr bytes.Buffer
	hdr.Write(blobMagic[:])
	binary.Write(&hdr, binary.LittleEndian, uint32(blobVersion))
	binary.Write(&hdr, binary.LittleEndian, uint32(len(region.Functions)))
	binary.Write(&hdr, binary.LittleEndian, uint32(len(region.HostSymbols)))
	binary.Write(&hdr, binary.LittleEndian, uint32(len(bodyBytes)))
	// 12 bytes reserved
	hdr.Write(make([]byte, 12))

	// Apply XOR encoding to body
	s.XOREncode(bodyBytes)

	var result bytes.Buffer
	result.Write(hdr.Bytes())
	result.Write(bodyBytes)
	return result.Bytes(), nil
}

// Decode deserializes a protected blob back to a PIR Region, reversing the
// seed's XOR encoding and opcode permutation.
func Decode(data []byte, s *seed.Seed) (*pir.Region, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("blob too short: %d bytes", len(data))
	}

	// Parse header
	if !bytes.Equal(data[:4], blobMagic[:]) {
		return nil, fmt.Errorf("invalid blob magic")
	}
	version := binary.LittleEndian.Uint32(data[4:8])
	if version != blobVersion {
		return nil, fmt.Errorf("unsupported blob version %d", version)
	}
	numFuncs := binary.LittleEndian.Uint32(data[8:12])
	numSyms := binary.LittleEndian.Uint32(data[12:16])
	bodyLen := binary.LittleEndian.Uint32(data[16:20])

	bodyStart := headerSize
	if uint32(len(data)-bodyStart) < bodyLen {
		return nil, fmt.Errorf("blob body truncated")
	}

	bodyBytes := make([]byte, bodyLen)
	copy(bodyBytes, data[bodyStart:bodyStart+int(bodyLen)])

	// Reverse XOR encoding
	s.XOREncode(bodyBytes)

	reader := bytes.NewReader(bodyBytes)

	// Read symbol table
	hostSyms := make([]string, numSyms)
	for i := uint32(0); i < numSyms; i++ {
		sym, err := readString(reader)
		if err != nil {
			return nil, fmt.Errorf("decode symbol %d: %w", i, err)
		}
		hostSyms[i] = sym
	}

	// Read functions
	inverseOp := s.InverseOpcodeMap()
	funcs := make([]*pir.Function, numFuncs)
	for i := uint32(0); i < numFuncs; i++ {
		fn, err := decodeFunction(reader, inverseOp)
		if err != nil {
			return nil, fmt.Errorf("decode function %d: %w", i, err)
		}
		funcs[i] = fn
	}

	return &pir.Region{
		Functions:   funcs,
		HostSymbols: hostSyms,
	}, nil
}

// ---------------------------------------------------------------------------
// encode helpers
// ---------------------------------------------------------------------------

func encodeFunction(w *bytes.Buffer, fn *pir.Function, s *seed.Seed) error {
	writeString(w, fn.Name)
	binary.Write(w, binary.LittleEndian, uint16(fn.NumRegs))
	binary.Write(w, binary.LittleEndian, uint16(fn.NumArgs))
	binary.Write(w, binary.LittleEndian, uint16(fn.NumSlots))
	binary.Write(w, binary.LittleEndian, uint16(len(fn.Blocks)))
	binary.Write(w, binary.LittleEndian, uint16(fn.EntryBlock))

	for i := range fn.Blocks {
		blk := &fn.Blocks[i]
		binary.Write(w, binary.LittleEndian, uint16(len(blk.Insts)))
		for j := range blk.Insts {
			encodeInst(w, &blk.Insts[j], s)
		}
	}
	return nil
}

func encodeInst(w *bytes.Buffer, inst *pir.Inst, s *seed.Seed) {
	w.WriteByte(s.MapOpcode(uint8(inst.Op)))
	binary.Write(w, binary.LittleEndian, int16(inst.Dst))
	binary.Write(w, binary.LittleEndian, int16(inst.Src[0]))
	binary.Write(w, binary.LittleEndian, int16(inst.Src[1]))
	binary.Write(w, binary.LittleEndian, inst.Imm)
	binary.Write(w, binary.LittleEndian, int16(inst.Block))
	binary.Write(w, binary.LittleEndian, int16(inst.AuxBlock))

	// Phi edges
	binary.Write(w, binary.LittleEndian, uint16(len(inst.Edges)))
	for _, e := range inst.Edges {
		binary.Write(w, binary.LittleEndian, int16(e.Block))
		binary.Write(w, binary.LittleEndian, int16(e.Reg))
	}

	// Call args
	binary.Write(w, binary.LittleEndian, uint16(len(inst.CallArgs)))
	for _, a := range inst.CallArgs {
		binary.Write(w, binary.LittleEndian, int16(a))
	}
}

// ---------------------------------------------------------------------------
// decode helpers
// ---------------------------------------------------------------------------

func decodeFunction(r *bytes.Reader, inverseOp []uint8) (*pir.Function, error) {
	name, err := readString(r)
	if err != nil {
		return nil, err
	}
	var numRegs, numArgs, numSlots, numBlocks, entryBlock uint16
	binary.Read(r, binary.LittleEndian, &numRegs)
	binary.Read(r, binary.LittleEndian, &numArgs)
	binary.Read(r, binary.LittleEndian, &numSlots)
	binary.Read(r, binary.LittleEndian, &numBlocks)
	binary.Read(r, binary.LittleEndian, &entryBlock)

	fn := &pir.Function{
		Name:       name,
		NumRegs:    int(numRegs),
		NumArgs:    int(numArgs),
		NumSlots:   int(numSlots),
		EntryBlock: int(entryBlock),
		Blocks:     make([]pir.Block, numBlocks),
	}

	for i := range fn.Blocks {
		fn.Blocks[i].Index = i
		var numInsts uint16
		binary.Read(r, binary.LittleEndian, &numInsts)
		fn.Blocks[i].Insts = make([]pir.Inst, numInsts)
		for j := range fn.Blocks[i].Insts {
			if err := decodeInst(r, &fn.Blocks[i].Insts[j], inverseOp); err != nil {
				return nil, err
			}
		}
	}
	return fn, nil
}

func decodeInst(r *bytes.Reader, inst *pir.Inst, inverseOp []uint8) error {
	opByte, err := r.ReadByte()
	if err != nil {
		return err
	}
	if int(opByte) < len(inverseOp) {
		inst.Op = pir.Opcode(inverseOp[opByte])
	} else {
		inst.Op = pir.Opcode(opByte)
	}

	var dst, src0, src1 int16
	binary.Read(r, binary.LittleEndian, &dst)
	binary.Read(r, binary.LittleEndian, &src0)
	binary.Read(r, binary.LittleEndian, &src1)
	inst.Dst = int(dst)
	inst.Src = [2]int{int(src0), int(src1)}

	binary.Read(r, binary.LittleEndian, &inst.Imm)

	var blk, auxBlk int16
	binary.Read(r, binary.LittleEndian, &blk)
	binary.Read(r, binary.LittleEndian, &auxBlk)
	inst.Block = int(blk)
	inst.AuxBlock = int(auxBlk)

	var numEdges uint16
	binary.Read(r, binary.LittleEndian, &numEdges)
	if numEdges > 0 {
		inst.Edges = make([]pir.PhiEdge, numEdges)
		for i := range inst.Edges {
			var eb, er int16
			binary.Read(r, binary.LittleEndian, &eb)
			binary.Read(r, binary.LittleEndian, &er)
			inst.Edges[i] = pir.PhiEdge{Block: int(eb), Reg: int(er)}
		}
	}

	var numArgs uint16
	binary.Read(r, binary.LittleEndian, &numArgs)
	if numArgs > 0 {
		inst.CallArgs = make([]int, numArgs)
		for i := range inst.CallArgs {
			var a int16
			binary.Read(r, binary.LittleEndian, &a)
			inst.CallArgs[i] = int(a)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// string I/O
// ---------------------------------------------------------------------------

func writeString(w *bytes.Buffer, s string) error {
	binary.Write(w, binary.LittleEndian, uint16(len(s)))
	_, err := w.WriteString(s)
	return err
}

func readString(r *bytes.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	buf := make([]byte, length)
	if _, err := r.Read(buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
