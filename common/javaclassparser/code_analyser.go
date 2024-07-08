package javaclassparser

import (
	"bytes"
	"fmt"
)

type OpCode struct {
	instr *Instruction
	data  []byte
	jmp   int
}

func convert2bytesToInt(data []byte) uint16 {
	b1 := uint16(data[0])
	b2 := uint16(data[1])
	return ((b1 & 0xFF) << 8) | (b2 & 0xFF)
}

func showOpcodes(codes []*OpCode) {
	for i, opCode := range codes {
		if opCode.instr.Name == "if_icmpge" || opCode.instr.Name == "goto" {
			fmt.Printf("%d %s jmpto:%d\n", i, opCode.instr.Name, opCode.jmp)
		} else {
			fmt.Printf("%d %s %v\n", i, opCode.instr.Name, opCode.data)
		}
	}
}
func ParseBytesCode(dumper *ClassObjectDumper, codeAttr *CodeAttribute) (string, error) {
	code := ""
	code += "\n"
	opcodes := []*OpCode{}
	offsetToIndex := map[uint16]int{}
	indexToOffset := map[int]uint16{}
	reader := bytes.NewReader(codeAttr.Code)
	i := 0
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		instr, ok := InstrInfos[int(b)]
		if !ok {
			return "", fmt.Errorf("unknow op: %x", b)
		}
		opcode := &OpCode{instr: instr, data: make([]byte, instr.Length)}
		reader.Read(opcode.data)
		opcodes = append(opcodes, opcode)
		offsetToIndex[uint16(i)] = len(opcodes) - 1
		indexToOffset[len(opcodes)-1] = uint16(i)
		i += instr.Length + 1
	}
	for i, opcode := range opcodes {
		if opcode.instr.OpCode == 0xa7 {
			offset := convert2bytesToInt(opcode.data)
			opcode.jmp = offsetToIndex[indexToOffset[i]+offset]
		}
		if opcode.instr.OpCode == 0xa2 {
			offset := convert2bytesToInt(opcode.data)
			opcode.jmp = offsetToIndex[indexToOffset[i]+offset]
		}
	}
	showOpcodes(opcodes)
	//code += strings.Join(instrNameList, "\n")
	code += "\n"
	return code, nil
}
