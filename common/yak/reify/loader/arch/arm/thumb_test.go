package arm

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// thumbBytes builds a little-endian halfword sequence.
func thumbBytes(halfwords ...uint16) []byte {
	code := make([]byte, 0, len(halfwords)*2)
	for _, value := range halfwords {
		code = binary.LittleEndian.AppendUint16(code, value)
	}
	return code
}

// thumbTarget is the target of a Thumb code region.
func thumbTarget() core.Target {
	return core.Target{
		OS:           core.OSLinux,
		Architecture: core.ArchARM,
		Bits:         32,
		ByteOrder:    core.LittleEndian,
		Mode:         "thumb",
	}
}

// parseThumbAt decodes and lifts one Thumb region.
func parseThumbAt(t *testing.T, address uint64, code []byte) *core.ParseResult {
	t.Helper()
	reader, err := core.NewByteReader("test", bytesReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	request := core.ParseRequest{Target: thumbTarget(), Address: address, Reader: reader, WithAssembly: true}
	result, err := (Parser{}).Parse(context.Background(), request)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return result
}

// TestThumbSixteenBitForms checks the 16-bit decode table against the encodings
// llvm-mc prints for the same assembly.
func TestThumbSixteenBitForms(t *testing.T) {
	cases := []struct {
		name   string
		code   []uint16
		want   armasm.Op
		args   int
		memory bool
	}{
		{"movs r0, #1", []uint16{0x2001}, armasm.MOV, 2, false},
		{"cmp r0, #5", []uint16{0x2805}, armasm.CMP, 2, false},
		{"adds r0, #7", []uint16{0x3007}, armasm.ADD, 2, false},
		{"subs r0, #7", []uint16{0x3807}, armasm.SUB, 2, false},
		{"ands r0, r1", []uint16{0x4008}, armasm.AND, 2, false},
		{"eors r0, r1", []uint16{0x4048}, armasm.EOR, 2, false},
		{"lsls r0, r1", []uint16{0x4088}, armasm.LSL, 2, false},
		{"lsrs r0, r1", []uint16{0x40c8}, armasm.LSR, 2, false},
		{"asrs r0, r1", []uint16{0x4108}, armasm.ASR, 2, false},
		{"adcs r0, r1", []uint16{0x4148}, armasm.ADC, 2, false},
		{"sbcs r0, r1", []uint16{0x4188}, armasm.SBC, 2, false},
		{"rors r0, r1", []uint16{0x41c8}, armasm.ROR, 2, false},
		{"tst r0, r1", []uint16{0x4208}, armasm.TST, 2, false},
		{"cmp r0, r1", []uint16{0x4288}, armasm.CMP, 2, false},
		{"cmn r0, r1", []uint16{0x42c8}, armasm.CMN, 2, false},
		{"orrs r0, r1", []uint16{0x4308}, armasm.ORR, 2, false},
		{"muls r0, r1, r0", []uint16{0x4348}, armasm.MUL, 3, false},
		{"bics r0, r1", []uint16{0x4388}, armasm.BIC, 2, false},
		{"mvns r0, r1", []uint16{0x43c8}, armasm.MVN, 2, false},
		{"lsls r0, r1, #1", []uint16{0x0048}, armasm.LSL, 3, false},
		{"lsrs r0, r1, #1", []uint16{0x0848}, armasm.LSR, 3, false},
		{"adds r0, r1, r2", []uint16{0x1888}, armasm.ADD, 3, false},
		{"subs r0, r1, #3", []uint16{0x1ec8}, armasm.SUB, 3, false},
		{"adds r0, r1, #3", []uint16{0x1cc8}, armasm.ADD, 3, false},
		{"ldr r0, [r1, #4]", []uint16{0x6848}, armasm.LDR, 2, true},
		{"ldrb r0, [r1, #3]", []uint16{0x78c8}, armasm.LDRB, 2, true},
		{"strh r2, [r3, #6]", []uint16{0x80da}, armasm.STRH, 2, true},
		{"ldr r0, [sp, #8]", []uint16{0x9802}, armasm.LDR, 2, true},
		{"str r1, [sp, #12]", []uint16{0x9103}, armasm.STR, 2, true},
		{"add r0, sp, #16", []uint16{0xa804}, armasm.ADD, 3, false},
		{"push {r0, lr}", []uint16{0xb501}, armasm.PUSH, 1, false},
		{"pop {r0, pc}", []uint16{0xbd01}, armasm.POP, 1, false},
		{"svc #1", []uint16{0xdf01}, armasm.SVC, 1, false},
		{"bkpt #0", []uint16{0xbe00}, armasm.BKPT, 1, false},
		{"sxth r0, r1", []uint16{0xb208}, armasm.SXTH, 2, false},
		{"uxtb r2, r3", []uint16{0xb2da}, armasm.UXTB, 2, false},
		{"rev r0, r1", []uint16{0xba08}, armasm.REV, 2, false},
		{"rev16 r0, r1", []uint16{0xba48}, armasm.REV16, 2, false},
		{"revsh r0, r1", []uint16{0xbac8}, armasm.REVSH, 2, false},
		{"ldm r0!, {r1, r2}", []uint16{0xc806}, armasm.LDM, 2, false},
		{"stm r0!, {r1, r2}", []uint16{0xc006}, armasm.STM, 2, false},
		{"mov r8, r1", []uint16{0x4688}, armasm.MOV, 2, false},
		{"cmp r8, r9", []uint16{0x45c8}, armasm.CMP, 2, false},
		{"bx lr", []uint16{0x4770}, armasm.BX, 1, false},
		{"blx r3", []uint16{0x4798}, armasm.BLX, 1, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decoded, size := decodeThumb(thumbBytes(testCase.code...), &thumbState{})
			if decoded.undefined {
				t.Fatalf("not decoded")
			}
			if size != 2 {
				t.Fatalf("size = %d, want 2", size)
			}
			if baseOp(decoded.inst.Op) != testCase.want {
				t.Fatalf("op = %v, want %v", decoded.inst.Op, testCase.want)
			}
			args := args(instruction{inst: decoded.inst})
			if testCase.memory {
				// The memory operand counts as one argument.
				if len(args) != testCase.args {
					t.Fatalf("args = %d, want %d", len(args), testCase.args)
				}
				return
			}
			if len(args) != testCase.args {
				t.Fatalf("args = %d, want %d (%v)", len(args), testCase.args, args)
			}
		})
	}
}

// TestThumbBranchTargets checks the branch offsets. The Thumb program counter is
// the instruction address plus four.
func TestThumbBranchTargets(t *testing.T) {
	cases := []struct {
		name   string
		code   []uint16
		target uint64
	}{
		{"b .+8", []uint16{0xe002}, 0x4008},
		{"beq .+8", []uint16{0xd002}, 0x4008},
		{"b .-4", []uint16{0xe7fc}, 0x3ffc},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decoded, _ := decodeThumb(thumbBytes(testCase.code...), &thumbState{})
			target, ok := thumbStaticTarget(decoded.inst, 0x4000, 2)
			if !ok {
				t.Fatal("no target")
			}
			if target != testCase.target {
				t.Fatalf("target = %#x, want %#x", target, testCase.target)
			}
		})
	}
}

// TestThumbITBlockConditions checks the IT state machine against every block
// mask. A clear mask bit selects the inverse condition.
func TestThumbITBlockConditions(t *testing.T) {
	// llvm-objdump prints these sequences as it/itt/ite/ittt/itet/...
	cases := []struct {
		mask   uint16
		length int
		conds  []int
	}{
		{0x8, 1, []int{0}},
		{0x4, 2, []int{0, 0}},
		{0xc, 2, []int{0, 1}},
		{0x2, 3, []int{0, 0, 0}},
		{0xa, 3, []int{0, 1, 0}},
		{0x6, 3, []int{0, 0, 1}},
		{0xe, 3, []int{0, 1, 1}},
		{0x1, 4, []int{0, 0, 0, 0}},
		{0x9, 4, []int{0, 1, 0, 0}},
		{0x5, 4, []int{0, 0, 1, 0}},
		{0xd, 4, []int{0, 1, 1, 0}},
		{0x3, 4, []int{0, 0, 0, 1}},
		{0xb, 4, []int{0, 1, 0, 1}},
		{0x7, 4, []int{0, 0, 1, 1}},
		{0xf, 4, []int{0, 1, 1, 1}},
	}
	for _, testCase := range cases {
		state := thumbState{}
		state.start(0xbf00 | testCase.mask)
		if state.length != testCase.length {
			t.Fatalf("mask %#x: length = %d, want %d", testCase.mask, state.length, testCase.length)
		}
		for index, want := range testCase.conds {
			if got := state.next(); got != want {
				t.Fatalf("mask %#x position %d: cond = %d, want %d", testCase.mask, index, got, want)
			}
		}
		if got := state.next(); got != 14 {
			t.Fatalf("mask %#x: condition after the block = %d, want AL", testCase.mask, got)
		}
	}
}

// TestThumbITBlockAppliesConditions checks that the decoded instructions carry
// the IT condition in their opcode.
func TestThumbITBlockAppliesConditions(t *testing.T) {
	// itt eq; moveq r0, #1; moveq r1, #2
	code := thumbBytes(0xbf04, 0x2001, 0x2102)
	reader, err := core.NewByteReader("test", bytesReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	request := core.ParseRequest{Target: thumbTarget(), Address: 0x4000, Reader: reader, WithAssembly: true}
	result, err := (Parser{}).Parse(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Assembly
	if !strings.Contains(text, "itt eq") {
		t.Fatalf("IT block not rendered: %q", text)
	}
	if !strings.Contains(text, "moveq") {
		t.Fatalf("condition not applied: %q", text)
	}
}

// TestThumbThirtyTwoBitForms checks the wide encodings against llvm-mc.
func TestThumbThirtyTwoBitForms(t *testing.T) {
	cases := []struct {
		name string
		code []uint16
		want armasm.Op
	}{
		{"ldr.w r0, [r1, #16]", []uint16{0xf8d1, 0x0010}, armasm.LDR},
		{"str.w r2, [r3, #8]", []uint16{0xf8c3, 0x2008}, armasm.STR},
		{"ldrb.w r0, [r1, #3]", []uint16{0xf891, 0x0003}, armasm.LDRB},
		{"ldrh.w r2, [r3, #6]", []uint16{0xf8b3, 0x2006}, armasm.LDRH},
		{"add.w r0, r1, #16", []uint16{0xf101, 0x0010}, armasm.ADD},
		{"sub.w r0, r1, #16", []uint16{0xf1a1, 0x0010}, armasm.SUB},
		{"mov.w r0, #16", []uint16{0xf04f, 0x0010}, armasm.MOV},
		{"movw r0, #4660", []uint16{0xf241, 0x2034}, armasm.MOVW},
		{"movt r0, #4660", []uint16{0xf2c1, 0x2034}, armasm.MOVT},
		{"ldrd r2, r3, [r0, #8]", []uint16{0xe9d0, 0x2302}, armasm.LDRD},
		{"strd r2, r3, [r0, #8]", []uint16{0xe9c0, 0x2302}, armasm.STRD},
		// A negative word offset. The immediate is a word count, so the
		// encoded 2 means eight bytes below the base.
		{"ldrd r0, r1, [r2, #-8]", []uint16{0xe952, 0x0102}, armasm.LDRD},
		{"strd r0, r1, [r2, #-8]", []uint16{0xe942, 0x0102}, armasm.STRD},
		{"ldrd r1, r3, [r12, #-208]", []uint16{0xe95c, 0x1334}, armasm.LDRD},
		{"pkhbt r0, r4, r1", []uint16{0xeac4, 0x0001}, armasm.PKHBT},
		{"push.w {r4-r8, lr}", []uint16{0xe92d, 0x41f0}, armasm.STMDB},
		{"pop.w {r4-r8, pc}", []uint16{0xe8bd, 0x81f0}, armasm.LDM},
		{"ldm.w r0, {r1-r3}", []uint16{0xe890, 0x000e}, armasm.LDM},
		{"stm.w r0!, {r1-r3}", []uint16{0xe8a0, 0x000e}, armasm.STM},
		{"and.w r0, r1, #255", []uint16{0xf001, 0x00ff}, armasm.AND},
		{"orr.w r0, r1, r2", []uint16{0xea41, 0x0002}, armasm.ORR},
		{"add.w r8, r8, r8", []uint16{0xeb08, 0x0808}, armasm.ADD},
		{"cmp.w r0, #5", []uint16{0xf1b0, 0x0f05}, armasm.CMP},
		{"mov.w r8, r9", []uint16{0xea4f, 0x0809}, armasm.MOV},
		{"mla r0, r1, r2, r3", []uint16{0xfb01, 0x3002}, armasm.MLA},
		{"umull r0, r1, r2, r3", []uint16{0xfba2, 0x0103}, armasm.UMULL},
		{"smull r0, r1, r2, r3", []uint16{0xfb82, 0x0103}, armasm.SMULL},
		{"clz r0, r1", []uint16{0xfab1, 0xf081}, armasm.CLZ},
		{"rev.w r0, r1", []uint16{0xfa91, 0xf081}, armasm.REV},
		{"rbit r0, r1", []uint16{0xfa91, 0xf0a1}, armasm.RBIT},
		{"uxtb.w r0, r1", []uint16{0xfa5f, 0xf081}, armasm.UXTB},
		{"sxtb.w r0, r1", []uint16{0xfa4f, 0xf081}, armasm.SXTB},
		{"sbfx r0, r1, #2, #4", []uint16{0xf341, 0x0083}, armasm.SBFX},
		{"ubfx r0, r1, #2, #4", []uint16{0xf3c1, 0x0083}, armasm.UBFX},
		{"bfi r0, r1, #2, #4", []uint16{0xf361, 0x0085}, armasm.BFI},
		{"subw r0, r1, #16", []uint16{0xf2a1, 0x0010}, armasm.SUB},
		{"addw r0, r1, #16", []uint16{0xf201, 0x0010}, armasm.ADD},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decoded, size := decodeThumb(thumbBytes(testCase.code...), &thumbState{})
			if decoded.undefined {
				t.Fatalf("not decoded")
			}
			if size != 4 {
				t.Fatalf("size = %d, want 4", size)
			}
			if baseOp(decoded.inst.Op) != testCase.want {
				t.Fatalf("op = %v, want %v", decoded.inst.Op, testCase.want)
			}
		})
	}
}

// TestThumbVectorFormsStayOpaque checks the coprocessor and Advanced SIMD space
// decodes as a named intrinsic instead of reporting a decode error. vpush is the
// prologue of most floating-point functions, so treating it as undecodable
// breaks the basic block that the whole function hangs from.
func TestThumbVectorFormsStayOpaque(t *testing.T) {
	cases := []struct {
		name string
		code []uint16
	}{
		{"vpush {d8}", []uint16{0xed2d, 0x8b02}},
		{"vld4.8 {d16, d17, d18, d19}, [r4], r1", []uint16{0xf964, 0x0001}},
		{"vldr d0, [pc, #8]", []uint16{0xed9f, 0x0b02}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decoded, size := decodeThumb(thumbBytes(testCase.code...), &thumbState{})
			if decoded.undefined {
				t.Fatalf("not decoded")
			}
			if size != 4 {
				t.Fatalf("size = %d, want 4", size)
			}
			if decoded.opaque != "vfp" {
				t.Fatalf("opaque = %q, want vfp", decoded.opaque)
			}
		})
	}
}

// TestThumbLiftsIntoLowIR checks one whole Thumb region reaches LowIR with a
// branch and a call.
func TestThumbLiftsIntoLowIR(t *testing.T) {
	// movs r0, #1; push {lr}; bl .+8; pop {pc}
	code := thumbBytes(0x2001, 0xb500, 0xf000, 0xf802, 0xbd00)
	result := parseThumbAt(t, 0x4000, code)
	if len(result.IR.Functions) != 1 {
		t.Fatalf("functions = %d", len(result.IR.Functions))
	}
	var calls, blocks int
	for _, block := range result.IR.Functions[0].Blocks {
		blocks++
		for _, instruction := range block.Instructions {
			if instruction.Kind == core.OpCall {
				calls++
			}
		}
	}
	if blocks == 0 {
		t.Fatal("no blocks")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

// TestThumbCompareAndBranchLifts checks CBZ and CBNZ, which have no armasm
// opcode and are lifted as a compare plus a branch.
func TestThumbCompareAndBranchLifts(t *testing.T) {
	// cbz r0, .+4; movs r1, #1; movs r2, #2
	code := thumbBytes(0xb100, 0x2101, 0x2202)
	result := parseThumbAt(t, 0x4000, code)
	var branches int
	for _, block := range result.IR.Functions[0].Blocks {
		for _, instruction := range block.Instructions {
			if instruction.Kind == core.OpBranch {
				branches++
			}
		}
	}
	if branches != 1 {
		t.Fatalf("branches = %d, want 1", branches)
	}
}

// TestThumbModeIsRejectedWithoutTheDecoder checks the mode contract: the parser
// accepts the Thumb modes and nothing else.
func TestThumbModeIsRejectedWithoutTheDecoder(t *testing.T) {
	parser := Parser{}
	for _, mode := range []string{"thumb", "thumb2"} {
		if err := parser.Check(core.Target{Architecture: core.ArchARM, Bits: 32, Mode: mode}); err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
	}
	if err := parser.Check(core.Target{Architecture: core.ArchARM, Bits: 32, Mode: "armv8"}); err == nil {
		t.Fatal("unknown mode accepted")
	}
}
