//go:build hids && linux

package ebpf

import (
	"bytes"
	"encoding/binary"
	"os"
	"strconv"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

func TestNetworkCloseProgramExcludesCollectorThreads(t *testing.T) {
	t.Parallel()

	const collectorTGID uint32 = 4242
	spec := buildNetworkCloseProgramSpec("hids_close_test", 99, collectorTGID)
	for _, tid := range []uint32{collectorTGID, collectorTGID + 1, collectorTGID + 137} {
		t.Run("thread_"+decimalPID(tid), func(t *testing.T) {
			result := executeCloseProgram(t, spec, collectorTGID, tid)
			if len(result.records) != 0 || result.memoryWrites != 0 || result.commCalls != 0 {
				t.Fatalf("collector thread reached record construction: %+v", result)
			}
		})
	}
}

func TestNetworkCloseProgramPreservesExternalProcessRecords(t *testing.T) {
	t.Parallel()

	const collectorTGID uint32 = 4242
	spec := buildNetworkCloseProgramSpec("hids_close_test", 99, collectorTGID)
	for _, identity := range []struct{ tgid, tid uint32 }{
		{9001, 9001},
		{9001, 9002},
		// Matching the low thread ID must not be mistaken for our TGID.
		{9001, collectorTGID},
	} {
		t.Run("thread_"+decimalPID(identity.tid), func(t *testing.T) {
			result := executeCloseProgram(t, spec, identity.tgid, identity.tid)
			if len(result.records) != 1 || result.commCalls != 1 {
				t.Fatalf("external close not emitted exactly once: %+v", result)
			}
			raw := result.records[0]
			if decodeRecordKind(raw) != recordKindNetworkClose {
				t.Fatalf("unexpected record kind: %d", decodeRecordKind(raw))
			}
			record, err := parseNetworkRecord(raw)
			if err != nil {
				t.Fatal(err)
			}
			if record.pid != identity.tid || record.tgid != identity.tgid || record.fd != 57 || record.comm != "close-test" {
				t.Fatalf("external close identity/payload changed: %+v", record)
			}
		})
	}
}

func TestNetworkCollectorBindsOwnTGIDToCloseProgram(t *testing.T) {
	t.Parallel()

	collector := NewNetwork().(*Collector)
	var closeSpec *ebpf.ProgramSpec
	for _, point := range collector.tracepointSpecs(99) {
		if point.group == "syscalls" && point.name == "sys_enter_close" {
			closeSpec = point.spec
		}
	}
	if closeSpec == nil {
		t.Fatal("network close tracepoint was removed")
	}
	self := uint32(os.Getpid())
	if result := executeCloseProgram(t, closeSpec, self, self+31); len(result.records) != 0 {
		t.Fatal("actual network collector did not exclude its own thread")
	}
	if result := executeCloseProgram(t, closeSpec, self+100, self+131); len(result.records) != 1 {
		t.Fatal("actual network collector suppressed another process")
	}
}

func decimalPID(pid uint32) string {
	return strconv.FormatUint(uint64(pid), 10)
}

type closeProgramResult struct {
	records      [][]byte
	memoryWrites int
	commCalls    int
}

// Execute the shipping program after BPF encoding/decoding, with deterministic
// kernel helper results. This tests the actual branch and record bytes without
// requiring BPF privileges; it is not a substitute for kernel attachment tests.
// The interpreter deliberately rejects every instruction/helper outside this
// small close program's instruction set rather than silently treating it as a
// no-op. PCs use raw instruction slots, including the two-slot map-FD load.
func executeCloseProgram(t *testing.T, spec *ebpf.ProgramSpec, tgid, tid uint32) closeProgramResult {
	t.Helper()
	var encoded bytes.Buffer
	if err := append(asm.Instructions(nil), spec.Instructions...).Marshal(&encoded, binary.LittleEndian); err != nil {
		t.Fatalf("encode close program: %v", err)
	}
	var instructions asm.Instructions
	if err := instructions.Unmarshal(&encoded, binary.LittleEndian); err != nil {
		t.Fatalf("decode close program: %v", err)
	}
	byPC := make(map[int]asm.Instruction, len(instructions))
	pc := 0
	for _, instruction := range instructions {
		byPC[pc] = instruction
		pc += int(instruction.Size() / asm.InstructionSize)
	}
	var registers [11]uint64
	registers[asm.R1], registers[asm.RFP] = 1024, 2048
	memory := map[uint64]byte{}
	var fd [8]byte
	binary.LittleEndian.PutUint64(fd[:], 57)
	for index, value := range fd {
		memory[1024+uint64(closeFDArgOffset)+uint64(index)] = value
	}
	result := closeProgramResult{}
	read := func(address uint64, size int) []byte {
		data := make([]byte, size)
		for index := range data {
			value, ok := memory[address+uint64(index)]
			if !ok {
				t.Fatalf("program read uninitialized memory at %d", address+uint64(index))
			}
			data[index] = value
		}
		return data
	}
	pc = 0
	for step := 0; step < len(instructions)*2; step++ {
		instruction, ok := byPC[pc]
		if !ok {
			t.Fatalf("program jumped outside instructions: pc=%d", pc)
		}
		next := pc + int(instruction.Size()/asm.InstructionSize)
		switch instruction.OpCode.Class() {
		case asm.ALU64Class:
			value := uint64(instruction.Constant)
			if instruction.OpCode.Source() == asm.RegSource {
				value = registers[instruction.Src]
			}
			switch instruction.OpCode.ALUOp() {
			case asm.Mov:
				registers[instruction.Dst] = value
			case asm.Add:
				registers[instruction.Dst] += value
			case asm.RSh:
				registers[instruction.Dst] >>= value
			default:
				t.Fatalf("unsupported ALU instruction: %v", instruction)
			}
		case asm.LdClass:
			if !instruction.IsConstantLoad(asm.DWord) && !(instruction.IsLoadFromMap() && instruction.Src == asm.PseudoMapFD) {
				t.Fatalf("unsupported immediate load: %v", instruction)
			}
			registers[instruction.Dst] = uint64(instruction.Constant)
		case asm.LdXClass:
			var raw [8]byte
			address := registers[instruction.Src] + uint64(int64(instruction.Offset))
			copy(raw[:], read(address, instruction.OpCode.Size().Sizeof()))
			registers[instruction.Dst] = binary.LittleEndian.Uint64(raw[:])
		case asm.StXClass:
			var raw [8]byte
			binary.LittleEndian.PutUint64(raw[:], registers[instruction.Src])
			address := registers[instruction.Dst] + uint64(int64(instruction.Offset))
			for index, value := range raw[:instruction.OpCode.Size().Sizeof()] {
				memory[address+uint64(index)] = value
				result.memoryWrites++
			}
		case asm.JumpClass:
			switch instruction.OpCode.JumpOp() {
			case asm.JEq:
				if registers[instruction.Dst] == uint64(instruction.Constant) {
					next += int(instruction.Offset)
				}
			case asm.Call:
				switch asm.BuiltinFunc(instruction.Constant) {
				case asm.FnGetCurrentPidTgid:
					registers[asm.R0] = uint64(tgid)<<32 | uint64(tid)
				case asm.FnGetCurrentComm:
					comm := make([]byte, int(registers[asm.R2]))
					copy(comm, "close-test")
					for index, value := range comm {
						memory[registers[asm.R1]+uint64(index)] = value
					}
					result.commCalls++
					registers[asm.R0] = 0
				case asm.FnRingbufOutput:
					result.records = append(result.records, read(registers[asm.R2], int(registers[asm.R3])))
					registers[asm.R0] = 0
				default:
					t.Fatalf("unsupported helper: %v", instruction)
				}
				// Helper calls invalidate the caller-saved argument registers.
				for register := asm.R1; register <= asm.R5; register++ {
					registers[register] = 0xdeadbeef
				}
			case asm.Exit:
				if registers[asm.R0] != 0 {
					t.Fatalf("unexpected program return: %d", registers[asm.R0])
				}
				return result
			default:
				t.Fatalf("unsupported jump: %v", instruction)
			}
		default:
			t.Fatalf("unsupported instruction: %v", instruction)
		}
		pc = next
	}
	t.Fatal("close program did not terminate")
	return result
}
