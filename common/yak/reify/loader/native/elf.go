package native

import (
	"context"
	"debug/elf"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// ELF loads Executable and Linkable Format images. OS, architecture and byte
// order are independent fields derived from the header.
type ELF struct{}

var (
	_ core.Loader = ELF{}
)

func (ELF) Kind() core.Format        { return core.FormatELF }
func (ELF) Name() string             { return "ELF" }
func (ELF) Description() string      { return describe(core.FormatELF) }
func (ELF) Match(prefix []byte) bool { return matchFormat(prefix, core.FormatELF) }

func (ELF) Check(r *core.ByteReader) (*core.CheckResult, error) {
	image, target, diagnostics, err := inspectELF(r)
	if err != nil {
		return nil, err
	}
	return checkResult(r, core.FormatELF, target, image, diagnostics)
}

func (ELF) Load(ctx context.Context, r *core.ByteReader) (*core.Artifact, error) {
	image, target, diagnostics, err := inspectELF(r)
	if err != nil {
		return nil, err
	}
	return newArtifact(ctx, r, core.FormatELF, target, image, diagnostics)
}

func inspectELF(reader *core.ByteReader) (*core.Image, core.Target, []core.Diagnostic, error) {
	file, err := elf.NewFile(reader)
	if err != nil {
		return nil, core.Target{}, nil, err
	}
	defer file.Close()
	bits := 32
	if file.Class == elf.ELFCLASS64 {
		bits = 64
	}
	target := core.Target{
		OS:           elfOS(file.OSABI),
		Architecture: elfArchitecture(file.Machine, bits),
		Bits:         bits,
		ByteOrder:    core.ByteOrderOf(file.ByteOrder),
		ABI:          file.OSABI.String(),
		Machine:      file.Machine.String(),
	}
	image := &core.Image{ObjectType: file.Type.String(), Sections: make([]core.Section, 0, len(file.Sections))}
	if file.Entry != 0 {
		entry := file.Entry
		image.Entry = &entry
	}
	for _, section := range file.Sections {
		if section.Type == elf.SHT_NULL {
			continue
		}
		fileSize := section.FileSize
		if section.Type == elf.SHT_NOBITS {
			fileSize = 0
		}
		image.Sections = append(image.Sections, core.Section{
			Name:           section.Name,
			FileOffset:     section.Offset,
			FileSize:       fileSize,
			VirtualAddress: section.Addr,
			MemorySize:     section.Size,
			Flags:          uint64(section.Flags),
		})
	}
	return image, target, nil, nil
}

func elfArchitecture(machine elf.Machine, bits int) core.Architecture {
	switch machine {
	case elf.EM_X86_64:
		return core.ArchAMD64
	case elf.EM_386:
		return core.Arch386
	case elf.EM_AARCH64:
		return core.ArchARM64
	case elf.EM_ARM:
		return core.ArchARM
	case elf.EM_RISCV:
		if bits == 64 {
			return core.ArchRISCV64
		}
		return core.ArchRISCV32
	case elf.EM_MIPS:
		if bits == 64 {
			return core.ArchMIPS64
		}
		return core.ArchMIPS
	case elf.EM_PPC:
		return core.ArchPPC
	case elf.EM_PPC64:
		return core.ArchPPC64
	default:
		return core.ArchUnknown
	}
}

func elfOS(abi elf.OSABI) core.OperatingSystem {
	switch abi {
	case elf.ELFOSABI_LINUX:
		return core.OSLinux
	case elf.ELFOSABI_FREEBSD:
		return core.OSFreeBSD
	case elf.ELFOSABI_NETBSD:
		return core.OSNetBSD
	case elf.ELFOSABI_OPENBSD:
		return core.OSOpenBSD
	case elf.ELFOSABI_SOLARIS:
		return core.OSSolaris
	default:
		// System V, HP-UX, Standalone and unknown ABIs do not identify an OS.
		return core.OSUnknown
	}
}
