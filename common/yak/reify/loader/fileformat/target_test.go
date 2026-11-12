package fileformat

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func TestFormatAndTargetDimensions(t *testing.T) {
	elfInput := func(machine elf.Machine, abi elf.OSABI) []byte {
		raw := elf64Fixture(t)
		raw[elf.EI_OSABI] = byte(abi)
		binary.LittleEndian.PutUint16(raw[18:], uint16(machine))
		return raw
	}
	peInput := func(machine, subsystem uint16) []byte {
		raw := peFixture(t, true)
		binary.LittleEndian.PutUint16(raw[0x84:], machine)
		offset := 0x84 + binary.Size(pe.FileHeader{})
		var header pe.OptionalHeader64
		if err := binary.Read(bytes.NewReader(raw[offset:]), binary.LittleEndian, &header); err != nil {
			t.Fatal(err)
		}
		header.Subsystem = subsystem
		var buffer bytes.Buffer
		writeValue(t, &buffer, binary.LittleEndian, header)
		copy(raw[offset:], buffer.Bytes())
		return raw
	}
	machoInput := func(cpu macho.Cpu) []byte {
		raw := macho64Fixture(t)
		binary.LittleEndian.PutUint32(raw[4:], uint32(cpu))
		return raw
	}
	cases := []struct {
		name   string
		loader core.Loader
		data   []byte
		os     core.OperatingSystem
		arch   core.Architecture
	}{
		{"elf amd64 linux", ELF{}, elfInput(elf.EM_X86_64, elf.ELFOSABI_LINUX), core.OSLinux, core.ArchAMD64},
		{"elf arm64 linux", ELF{}, elfInput(elf.EM_AARCH64, elf.ELFOSABI_LINUX), core.OSLinux, core.ArchARM64},
		{"elf arm64 freebsd", ELF{}, elfInput(elf.EM_AARCH64, elf.ELFOSABI_FREEBSD), core.OSFreeBSD, core.ArchARM64},
		{"elf systemv unknown os", ELF{}, elfInput(elf.EM_X86_64, elf.ELFOSABI_NONE), core.OSUnknown, core.ArchAMD64},
		{"elf unknown machine", ELF{}, elfInput(elf.Machine(0xffff), elf.ELFOSABI_NONE), core.OSUnknown, core.ArchUnknown},
		{"pe amd64 windows", PE{}, peInput(pe.IMAGE_FILE_MACHINE_AMD64, pe.IMAGE_SUBSYSTEM_WINDOWS_CUI), core.OSWindows, core.ArchAMD64},
		{"pe arm64 windows", PE{}, peInput(pe.IMAGE_FILE_MACHINE_ARM64, pe.IMAGE_SUBSYSTEM_WINDOWS_GUI), core.OSWindows, core.ArchARM64},
		{"pe firmware is not windows", PE{}, peInput(pe.IMAGE_FILE_MACHINE_AMD64, pe.IMAGE_SUBSYSTEM_EFI_APPLICATION), core.OSUnknown, core.ArchAMD64},
		{"macho amd64", MachO{}, machoInput(macho.CpuAmd64), core.OSUnknown, core.ArchAMD64},
		{"macho arm64", MachO{}, machoInput(macho.CpuArm64), core.OSUnknown, core.ArchARM64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := loadFixture(t, tc.loader, tc.data)
			if err != nil {
				t.Fatal(err)
			}
			target := artifact.Target
			if target.OS != tc.os || target.Architecture != tc.arch || target.Machine == "" {
				t.Fatalf("unexpected target: %+v", target)
			}
		})
	}
}

func TestELFArchitectureWidth(t *testing.T) {
	for _, tc := range []struct {
		machine elf.Machine
		bits    int
		want    core.Architecture
	}{
		{elf.EM_RISCV, 32, core.ArchRISCV32},
		{elf.EM_RISCV, 64, core.ArchRISCV64},
		{elf.EM_MIPS, 32, core.ArchMIPS},
		{elf.EM_MIPS, 64, core.ArchMIPS64},
	} {
		if got := elfArchitecture(tc.machine, tc.bits); got != tc.want {
			t.Fatalf("%s/%d: got %s, want %s", tc.machine, tc.bits, got, tc.want)
		}
	}
}
