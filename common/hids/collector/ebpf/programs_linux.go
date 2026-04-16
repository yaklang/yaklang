//go:build hids && linux

package ebpf

import (
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"golang.org/x/sys/unix"
)

const (
	processExecveFilenameArgOffset   = 16
	processExecveAtFilenameArgOffset = 24

	connectFDArgOffset       = 16
	connectSockaddrArgOffset = 24
	closeFDArgOffset         = 16
	acceptRetOffset          = 16
	inetStateOldStateOffset  = 16
	inetStateNewStateOffset  = 20
	inetStateSportOffset     = 24
	inetStateDportOffset     = 26
	inetStateFamilyOffset    = 28
	inetStateProtocolOffset  = 30
	inetStateSaddrOffset     = 31
	inetStateDaddrOffset     = 35
	inetStateSaddrV6Offset   = 39
	inetStateDaddrV6Offset   = 55

	recordKindProcessExec    = 1
	recordKindNetworkConnect = 2
	recordKindProcessExit    = 3
	recordKindNetworkClose   = 4
	recordKindNetworkAccept  = 5
	recordKindNetworkState   = 6

	processFieldKindOffset  = 0
	processFieldPIDOffset   = 4
	processFieldTGIDOffset  = 8
	processFieldCommOffset  = 16
	processFieldImageOffset = 32
	processCommSize         = 16
	processImageSize        = 128
	processRecordSize       = 160

	networkFieldKindOffset    = 0
	networkFieldPIDOffset     = 4
	networkFieldTGIDOffset    = 8
	networkFieldFDOffset      = 12
	networkFieldFamilyOffset  = 16
	networkFieldPortOffset    = 18
	networkFieldAddrLenOffset = 20
	networkFieldCommOffset    = 24
	networkFieldAddrOffset    = 40
	networkCommSize           = 16
	networkAddrSize           = 16
	networkRecordSize         = 64
	sockaddrScratchSize       = 32

	networkStateFieldKindOffset       = 0
	networkStateFieldFamilyOffset     = 4
	networkStateFieldProtocolOffset   = 6
	networkStateFieldOldStateOffset   = 8
	networkStateFieldNewStateOffset   = 12
	networkStateFieldSourcePortOffset = 16
	networkStateFieldDestPortOffset   = 18
	networkStateFieldSourceAddrLen    = 20
	networkStateFieldDestAddrLen      = 24
	networkStateFieldSourceAddrOffset = 28
	networkStateFieldDestAddrOffset   = 44
	networkStateRecordSize            = 64

	bpfObjectNameMaxLen      = 15
	ringbufMapSuffix         = "_events"
	ringbufMapBaseNameMaxLen = bpfObjectNameMaxLen - len(ringbufMapSuffix)
)

func newRingbufMapSpec(name string) *ebpf.MapSpec {
	return &ebpf.MapSpec{
		Name:       sanitizeMapName(name) + ringbufMapSuffix,
		Type:       ebpf.RingBuf,
		MaxEntries: 1 << 20,
	}
}

func buildProcessExecProgramSpec(
	name string,
	filenameArgOffset int16,
	eventsFD int,
) *ebpf.ProgramSpec {
	insns := asm.Instructions{
		asm.Mov.Reg(asm.R9, asm.R1),
		asm.LoadMem(asm.R3, asm.R9, filenameArgOffset, asm.DWord),
		asm.JEq.Imm(asm.R3, 0, "exit"),

		asm.Mov.Reg(asm.R6, asm.RFP),
		asm.Add.Imm(asm.R6, -processRecordSize),
		asm.Mov.Imm(asm.R0, 0),
	}
	insns = append(insns, zeroBufferInstructions(asm.R6, processRecordSize)...)
	insns = append(insns,
		asm.Mov.Imm(asm.R1, recordKindProcessExec),
		asm.StoreMem(asm.R6, processFieldKindOffset, asm.R1, asm.Word),

		asm.FnGetCurrentPidTgid.Call(),
		asm.StoreMem(asm.R6, processFieldPIDOffset, asm.R0, asm.Word),
		asm.Mov.Reg(asm.R1, asm.R0),
		asm.RSh.Imm(asm.R1, 32),
		asm.StoreMem(asm.R6, processFieldTGIDOffset, asm.R1, asm.Word),

		asm.Mov.Reg(asm.R1, asm.R6),
		asm.Add.Imm(asm.R1, processFieldCommOffset),
		asm.Mov.Imm(asm.R2, processCommSize),
		asm.FnGetCurrentComm.Call(),

		asm.Mov.Reg(asm.R1, asm.R6),
		asm.Add.Imm(asm.R1, processFieldImageOffset),
		asm.Mov.Imm(asm.R2, processImageSize),
		asm.LoadMem(asm.R3, asm.R9, filenameArgOffset, asm.DWord),
		asm.FnProbeReadUserStr.Call(),

		asm.LoadMapPtr(asm.R1, eventsFD),
		asm.Mov.Reg(asm.R2, asm.R6),
		asm.Mov.Imm(asm.R3, processRecordSize),
		asm.Mov.Imm(asm.R4, 0),
		asm.FnRingbufOutput.Call(),

		asm.Mov.Imm(asm.R0, 0).WithSymbol("exit"),
		asm.Return(),
	)

	return &ebpf.ProgramSpec{
		Name:         name,
		Type:         ebpf.TracePoint,
		License:      "GPL",
		Instructions: insns,
	}
}

func buildProcessExitProgramSpec(name string, eventsFD int) *ebpf.ProgramSpec {
	insns := asm.Instructions{
		asm.Mov.Reg(asm.R6, asm.RFP),
		asm.Add.Imm(asm.R6, -processRecordSize),
		asm.Mov.Imm(asm.R0, 0),
	}
	insns = append(insns, zeroBufferInstructions(asm.R6, processRecordSize)...)
	insns = append(insns,
		asm.Mov.Imm(asm.R1, recordKindProcessExit),
		asm.StoreMem(asm.R6, processFieldKindOffset, asm.R1, asm.Word),

		asm.FnGetCurrentPidTgid.Call(),
		asm.StoreMem(asm.R6, processFieldPIDOffset, asm.R0, asm.Word),
		asm.Mov.Reg(asm.R1, asm.R0),
		asm.RSh.Imm(asm.R1, 32),
		asm.StoreMem(asm.R6, processFieldTGIDOffset, asm.R1, asm.Word),

		asm.Mov.Reg(asm.R1, asm.R6),
		asm.Add.Imm(asm.R1, processFieldCommOffset),
		asm.Mov.Imm(asm.R2, processCommSize),
		asm.FnGetCurrentComm.Call(),

		asm.LoadMapPtr(asm.R1, eventsFD),
		asm.Mov.Reg(asm.R2, asm.R6),
		asm.Mov.Imm(asm.R3, processRecordSize),
		asm.Mov.Imm(asm.R4, 0),
		asm.FnRingbufOutput.Call(),

		asm.Mov.Imm(asm.R0, 0),
		asm.Return(),
	)

	return &ebpf.ProgramSpec{
		Name:         name,
		Type:         ebpf.TracePoint,
		License:      "GPL",
		Instructions: insns,
	}
}

func buildNetworkConnectProgramSpec(name string, eventsFD int) *ebpf.ProgramSpec {
	insns := asm.Instructions{
		asm.Mov.Reg(asm.R9, asm.R1),
		asm.LoadMem(asm.R3, asm.R9, connectSockaddrArgOffset, asm.DWord),
		asm.JEq.Imm(asm.R3, 0, "exit"),

		asm.Mov.Reg(asm.R6, asm.RFP),
		asm.Add.Imm(asm.R6, -networkRecordSize),
		asm.Mov.Reg(asm.R7, asm.RFP),
		asm.Add.Imm(asm.R7, -(networkRecordSize + sockaddrScratchSize)),
		asm.Mov.Imm(asm.R0, 0),
	}
	insns = append(insns, zeroBufferInstructions(asm.R6, networkRecordSize)...)
	insns = append(insns,
		asm.Mov.Imm(asm.R1, recordKindNetworkConnect),
		asm.StoreMem(asm.R6, networkFieldKindOffset, asm.R1, asm.Word),

		asm.FnGetCurrentPidTgid.Call(),
		asm.StoreMem(asm.R6, networkFieldPIDOffset, asm.R0, asm.Word),
		asm.Mov.Reg(asm.R1, asm.R0),
		asm.RSh.Imm(asm.R1, 32),
		asm.StoreMem(asm.R6, networkFieldTGIDOffset, asm.R1, asm.Word),

		asm.LoadMem(asm.R1, asm.R9, connectFDArgOffset, asm.DWord),
		asm.StoreMem(asm.R6, networkFieldFDOffset, asm.R1, asm.Word),

		asm.Mov.Reg(asm.R1, asm.R6),
		asm.Add.Imm(asm.R1, networkFieldCommOffset),
		asm.Mov.Imm(asm.R2, networkCommSize),
		asm.FnGetCurrentComm.Call(),

		asm.Mov.Reg(asm.R1, asm.R7),
		asm.Mov.Imm(asm.R2, sockaddrScratchSize),
		asm.LoadMem(asm.R3, asm.R9, connectSockaddrArgOffset, asm.DWord),
		asm.FnProbeReadUser.Call(),
		asm.JNE.Imm(asm.R0, 0, "exit"),

		asm.LoadMem(asm.R2, asm.R7, 0, asm.Half),
		asm.StoreMem(asm.R6, networkFieldFamilyOffset, asm.R2, asm.Half),
		asm.LoadMem(asm.R1, asm.R7, 2, asm.Half),
		asm.StoreMem(asm.R6, networkFieldPortOffset, asm.R1, asm.Half),

		asm.JEq.Imm(asm.R2, unix.AF_INET, "ipv4"),
		asm.JEq.Imm(asm.R2, unix.AF_INET6, "ipv6"),
		asm.Ja.Label("exit"),

		asm.LoadMem(asm.R1, asm.R7, 4, asm.Word).WithSymbol("ipv4"),
		asm.StoreMem(asm.R6, networkFieldAddrOffset, asm.R1, asm.Word),
		asm.StoreImm(asm.R6, networkFieldAddrLenOffset, 4, asm.Word),
		asm.Ja.Label("output"),

		asm.LoadMem(asm.R1, asm.R7, 8, asm.DWord).WithSymbol("ipv6"),
		asm.StoreMem(asm.R6, networkFieldAddrOffset, asm.R1, asm.DWord),
		asm.LoadMem(asm.R1, asm.R7, 16, asm.DWord),
		asm.StoreMem(asm.R6, networkFieldAddrOffset+8, asm.R1, asm.DWord),
		asm.StoreImm(asm.R6, networkFieldAddrLenOffset, 16, asm.Word),

		asm.LoadMapPtr(asm.R1, eventsFD).WithSymbol("output"),
		asm.Mov.Reg(asm.R2, asm.R6),
		asm.Mov.Imm(asm.R3, networkRecordSize),
		asm.Mov.Imm(asm.R4, 0),
		asm.FnRingbufOutput.Call(),

		asm.Mov.Imm(asm.R0, 0).WithSymbol("exit"),
		asm.Return(),
	)

	return &ebpf.ProgramSpec{
		Name:         name,
		Type:         ebpf.TracePoint,
		License:      "GPL",
		Instructions: insns,
	}
}

func buildNetworkCloseProgramSpec(name string, eventsFD int) *ebpf.ProgramSpec {
	insns := asm.Instructions{
		asm.Mov.Reg(asm.R9, asm.R1),

		asm.Mov.Reg(asm.R6, asm.RFP),
		asm.Add.Imm(asm.R6, -networkRecordSize),
		asm.Mov.Imm(asm.R0, 0),
	}
	insns = append(insns, zeroBufferInstructions(asm.R6, networkRecordSize)...)
	insns = append(insns,
		asm.Mov.Imm(asm.R1, recordKindNetworkClose),
		asm.StoreMem(asm.R6, networkFieldKindOffset, asm.R1, asm.Word),

		asm.FnGetCurrentPidTgid.Call(),
		asm.StoreMem(asm.R6, networkFieldPIDOffset, asm.R0, asm.Word),
		asm.Mov.Reg(asm.R1, asm.R0),
		asm.RSh.Imm(asm.R1, 32),
		asm.StoreMem(asm.R6, networkFieldTGIDOffset, asm.R1, asm.Word),

		asm.LoadMem(asm.R1, asm.R9, closeFDArgOffset, asm.DWord),
		asm.StoreMem(asm.R6, networkFieldFDOffset, asm.R1, asm.Word),

		asm.Mov.Reg(asm.R1, asm.R6),
		asm.Add.Imm(asm.R1, networkFieldCommOffset),
		asm.Mov.Imm(asm.R2, networkCommSize),
		asm.FnGetCurrentComm.Call(),

		asm.LoadMapPtr(asm.R1, eventsFD),
		asm.Mov.Reg(asm.R2, asm.R6),
		asm.Mov.Imm(asm.R3, networkRecordSize),
		asm.Mov.Imm(asm.R4, 0),
		asm.FnRingbufOutput.Call(),

		asm.Mov.Imm(asm.R0, 0),
		asm.Return(),
	)

	return &ebpf.ProgramSpec{
		Name:         name,
		Type:         ebpf.TracePoint,
		License:      "GPL",
		Instructions: insns,
	}
}

func buildNetworkAcceptProgramSpec(name string, eventsFD int) *ebpf.ProgramSpec {
	insns := asm.Instructions{
		asm.Mov.Reg(asm.R9, asm.R1),
		asm.LoadMem(asm.R1, asm.R9, acceptRetOffset, asm.DWord),
		asm.JSLT.Imm(asm.R1, 0, "exit"),

		asm.Mov.Reg(asm.R6, asm.RFP),
		asm.Add.Imm(asm.R6, -networkRecordSize),
		asm.Mov.Imm(asm.R0, 0),
	}
	insns = append(insns, zeroBufferInstructions(asm.R6, networkRecordSize)...)
	insns = append(insns,
		asm.Mov.Imm(asm.R2, recordKindNetworkAccept),
		asm.StoreMem(asm.R6, networkFieldKindOffset, asm.R2, asm.Word),

		asm.FnGetCurrentPidTgid.Call(),
		asm.StoreMem(asm.R6, networkFieldPIDOffset, asm.R0, asm.Word),
		asm.Mov.Reg(asm.R2, asm.R0),
		asm.RSh.Imm(asm.R2, 32),
		asm.StoreMem(asm.R6, networkFieldTGIDOffset, asm.R2, asm.Word),

		asm.LoadMem(asm.R1, asm.R9, acceptRetOffset, asm.DWord),
		asm.StoreMem(asm.R6, networkFieldFDOffset, asm.R1, asm.Word),

		asm.Mov.Reg(asm.R1, asm.R6),
		asm.Add.Imm(asm.R1, networkFieldCommOffset),
		asm.Mov.Imm(asm.R2, networkCommSize),
		asm.FnGetCurrentComm.Call(),

		asm.LoadMapPtr(asm.R1, eventsFD),
		asm.Mov.Reg(asm.R2, asm.R6),
		asm.Mov.Imm(asm.R3, networkRecordSize),
		asm.Mov.Imm(asm.R4, 0),
		asm.FnRingbufOutput.Call(),

		asm.Mov.Imm(asm.R0, 0).WithSymbol("exit"),
		asm.Return(),
	)

	return &ebpf.ProgramSpec{
		Name:         name,
		Type:         ebpf.TracePoint,
		License:      "GPL",
		Instructions: insns,
	}
}

func buildNetworkStateProgramSpec(name string, eventsFD int) *ebpf.ProgramSpec {
	insns := asm.Instructions{
		asm.Mov.Reg(asm.R9, asm.R1),

		asm.LoadMem(asm.R1, asm.R9, inetStateProtocolOffset, asm.Byte),
		asm.JNE.Imm(asm.R1, unix.IPPROTO_TCP, "exit"),

		asm.Mov.Reg(asm.R6, asm.RFP),
		asm.Add.Imm(asm.R6, -networkStateRecordSize),
		asm.Mov.Imm(asm.R0, 0),
	}
	insns = append(insns, zeroBufferInstructions(asm.R6, networkStateRecordSize)...)
	insns = append(insns,
		asm.Mov.Imm(asm.R2, recordKindNetworkState),
		asm.StoreMem(asm.R6, networkStateFieldKindOffset, asm.R2, asm.Word),

		asm.LoadMem(asm.R2, asm.R9, inetStateFamilyOffset, asm.Half),
		asm.StoreMem(asm.R6, networkStateFieldFamilyOffset, asm.R2, asm.Half),

		asm.LoadMem(asm.R2, asm.R9, inetStateProtocolOffset, asm.Byte),
		asm.StoreMem(asm.R6, networkStateFieldProtocolOffset, asm.R2, asm.Byte),

		asm.LoadMem(asm.R2, asm.R9, inetStateOldStateOffset, asm.Word),
		asm.StoreMem(asm.R6, networkStateFieldOldStateOffset, asm.R2, asm.Word),

		asm.LoadMem(asm.R2, asm.R9, inetStateNewStateOffset, asm.Word),
		asm.StoreMem(asm.R6, networkStateFieldNewStateOffset, asm.R2, asm.Word),

		asm.LoadMem(asm.R2, asm.R9, inetStateSportOffset, asm.Half),
		asm.StoreMem(asm.R6, networkStateFieldSourcePortOffset, asm.R2, asm.Half),

		asm.LoadMem(asm.R2, asm.R9, inetStateDportOffset, asm.Half),
		asm.StoreMem(asm.R6, networkStateFieldDestPortOffset, asm.R2, asm.Half),

		asm.LoadMem(asm.R1, asm.R9, inetStateFamilyOffset, asm.Half),
		asm.JEq.Imm(asm.R1, unix.AF_INET, "ipv4"),
		asm.JEq.Imm(asm.R1, unix.AF_INET6, "ipv6"),
		asm.Ja.Label("exit"),
	)
	insns = append(insns, copyTraceContextBytesInstructionsWithLabel(
		asm.R6,
		networkStateFieldSourceAddrOffset,
		asm.R9,
		inetStateSaddrOffset,
		4,
		"ipv4",
	)...)
	insns = append(insns, copyTraceContextBytesInstructions(
		asm.R6,
		networkStateFieldDestAddrOffset,
		asm.R9,
		inetStateDaddrOffset,
		4,
	)...)
	insns = append(insns,
		asm.StoreImm(asm.R6, networkStateFieldSourceAddrLen, 4, asm.Word),
		asm.StoreImm(asm.R6, networkStateFieldDestAddrLen, 4, asm.Word),
		asm.Ja.Label("output"),
	)
	insns = append(insns, copyTraceContextBytesInstructionsWithLabel(
		asm.R6,
		networkStateFieldSourceAddrOffset,
		asm.R9,
		inetStateSaddrV6Offset,
		16,
		"ipv6",
	)...)
	insns = append(insns, copyTraceContextBytesInstructions(
		asm.R6,
		networkStateFieldDestAddrOffset,
		asm.R9,
		inetStateDaddrV6Offset,
		16,
	)...)
	insns = append(insns,
		asm.StoreImm(asm.R6, networkStateFieldSourceAddrLen, 16, asm.Word),
		asm.StoreImm(asm.R6, networkStateFieldDestAddrLen, 16, asm.Word),

		asm.LoadMapPtr(asm.R1, eventsFD).WithSymbol("output"),
		asm.Mov.Reg(asm.R2, asm.R6),
		asm.Mov.Imm(asm.R3, networkStateRecordSize),
		asm.Mov.Imm(asm.R4, 0),
		asm.FnRingbufOutput.Call(),

		asm.Mov.Imm(asm.R0, 0).WithSymbol("exit"),
		asm.Return(),
	)

	return &ebpf.ProgramSpec{
		Name:         name,
		Type:         ebpf.TracePoint,
		License:      "GPL",
		Instructions: insns,
	}
}

func zeroBufferInstructions(base asm.Register, size int16) asm.Instructions {
	insns := asm.Instructions{}
	if size <= 0 {
		return insns
	}
	for offset := int16(0); offset < size; offset += 8 {
		insns = append(insns, asm.StoreMem(base, offset, asm.R0, asm.DWord))
	}
	return insns
}

func copyTraceContextBytesInstructions(
	dstBase asm.Register,
	dstOffset int16,
	srcBase asm.Register,
	srcOffset int16,
	size int,
) asm.Instructions {
	insns := make(asm.Instructions, 0, size*2)
	for index := 0; index < size; index++ {
		insns = append(insns,
			asm.LoadMem(asm.R1, srcBase, srcOffset+int16(index), asm.Byte),
			asm.StoreMem(dstBase, dstOffset+int16(index), asm.R1, asm.Byte),
		)
	}
	return insns
}

func copyTraceContextBytesInstructionsWithLabel(
	dstBase asm.Register,
	dstOffset int16,
	srcBase asm.Register,
	srcOffset int16,
	size int,
	label string,
) asm.Instructions {
	insns := copyTraceContextBytesInstructions(dstBase, dstOffset, srcBase, srcOffset, size)
	if len(insns) > 0 && label != "" {
		insns[0] = insns[0].WithSymbol(label)
	}
	return insns
}

func sanitizeMapName(value string) string {
	if value == "" {
		return "hids"
	}

	result := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		switch ch := value[i]; {
		case ch >= 'a' && ch <= 'z':
			result = append(result, ch)
		case ch >= 'A' && ch <= 'Z':
			result = append(result, ch+('a'-'A'))
		case ch >= '0' && ch <= '9':
			result = append(result, ch)
		default:
			result = append(result, '_')
		}
	}
	if len(result) > ringbufMapBaseNameMaxLen {
		result = result[:ringbufMapBaseNameMaxLen]
	}
	return string(result)
}

func validateRecordLayout() {
	if processRecordSize%8 != 0 || networkRecordSize%8 != 0 || networkStateRecordSize%8 != 0 {
		panic("ebpf record size must be aligned to 8 bytes")
	}
	if processFieldCommOffset+processCommSize != processFieldImageOffset {
		panic("ebpf process record field offsets mismatch")
	}
	if processFieldImageOffset+processImageSize != processRecordSize {
		panic("ebpf process record layout mismatch")
	}
	if networkFieldCommOffset+networkCommSize != networkFieldAddrOffset {
		panic("ebpf network record field offsets mismatch")
	}
	if networkFieldAddrOffset+networkAddrSize > networkRecordSize {
		panic("ebpf network record layout mismatch")
	}
	if networkStateFieldSourceAddrOffset+networkAddrSize > networkStateRecordSize {
		panic("ebpf network state record source layout mismatch")
	}
	if networkStateFieldDestAddrOffset+networkAddrSize > networkStateRecordSize {
		panic("ebpf network state record destination layout mismatch")
	}
}

func init() {
	validateRecordLayout()
}
