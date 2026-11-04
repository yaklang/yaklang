package native

import (
	"context"
	"debug/pe"
	"fmt"
	"math"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// PE loads Portable Executable images. The subsystem is not always Windows:
// firmware and other execution environments also use the PE container.
type PE struct{}

var (
	_ core.Loader = PE{}
)

func (PE) Kind() core.Format        { return core.FormatPE }
func (PE) Name() string             { return "PE" }
func (PE) Description() string      { return describe(core.FormatPE) }
func (PE) Match(prefix []byte) bool { return matchFormat(prefix, core.FormatPE) }

func (PE) Check(r *core.ByteReader) (*core.CheckResult, error) {
	image, target, diagnostics, err := inspectPE(r)
	if err != nil {
		return nil, err
	}
	return checkResult(r, core.FormatPE, target, image, diagnostics)
}

func (PE) Load(ctx context.Context, r *core.ByteReader) (*core.Artifact, error) {
	image, target, diagnostics, err := inspectPE(r)
	if err != nil {
		return nil, err
	}
	return newArtifact(ctx, r, core.FormatPE, target, image, diagnostics)
}

func inspectPE(reader *core.ByteReader) (*core.Image, core.Target, []core.Diagnostic, error) {
	file, err := pe.NewFile(reader)
	if err != nil {
		return nil, core.Target{}, nil, err
	}
	defer file.Close()
	target := core.Target{
		Architecture: peArchitecture(file.Machine),
		ByteOrder:    core.LittleEndian,
		Machine:      fmt.Sprintf("0x%04x", file.Machine),
	}
	image := &core.Image{Sections: make([]core.Section, 0, len(file.Sections))}
	var entryRVA uint32
	var subsystem uint16
	switch header := file.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		target.Bits, image.Base, entryRVA = 32, uint64(header.ImageBase), header.AddressOfEntryPoint
		image.ObjectType, subsystem = "PE32", header.Subsystem
	case *pe.OptionalHeader64:
		target.Bits, image.Base, entryRVA = 64, header.ImageBase, header.AddressOfEntryPoint
		image.ObjectType, subsystem = "PE32+", header.Subsystem
	default:
		return nil, core.Target{}, nil, fmt.Errorf("PE image has no supported optional header")
	}
	target.OS = peOS(subsystem)
	target.ABI = fmt.Sprintf("pe-subsystem-%d", subsystem)
	if entryRVA != 0 {
		entry, err := peAddress(image.Base, entryRVA)
		if err != nil {
			return nil, core.Target{}, nil, err
		}
		image.Entry = &entry
	}
	for _, section := range file.Sections {
		address, err := peAddress(image.Base, section.VirtualAddress)
		if err != nil {
			return nil, core.Target{}, nil, fmt.Errorf("section %q: %w", section.Name, err)
		}
		memorySize := uint64(section.VirtualSize)
		if memorySize < uint64(section.Size) {
			memorySize = uint64(section.Size)
		}
		image.Sections = append(image.Sections, core.Section{
			Name:           section.Name,
			FileOffset:     uint64(section.Offset),
			FileSize:       uint64(section.Size),
			VirtualAddress: address,
			MemorySize:     memorySize,
			Flags:          uint64(section.Characteristics),
		})
	}
	return image, target, nil, nil
}

func peAddress(base uint64, rva uint32) (uint64, error) {
	if uint64(rva) > math.MaxUint64-base {
		return 0, fmt.Errorf("PE image base plus RVA overflows")
	}
	return base + uint64(rva), nil
}

func peArchitecture(machine uint16) core.Architecture {
	switch machine {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return core.ArchAMD64
	case pe.IMAGE_FILE_MACHINE_I386:
		return core.Arch386
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return core.ArchARM64
	case pe.IMAGE_FILE_MACHINE_ARM, pe.IMAGE_FILE_MACHINE_ARMNT:
		return core.ArchARM
	default:
		return core.ArchUnknown
	}
}

func peOS(subsystem uint16) core.OperatingSystem {
	switch subsystem {
	case pe.IMAGE_SUBSYSTEM_NATIVE, pe.IMAGE_SUBSYSTEM_WINDOWS_GUI,
		pe.IMAGE_SUBSYSTEM_WINDOWS_CUI, pe.IMAGE_SUBSYSTEM_NATIVE_WINDOWS,
		pe.IMAGE_SUBSYSTEM_WINDOWS_CE_GUI, pe.IMAGE_SUBSYSTEM_WINDOWS_BOOT_APPLICATION:
		return core.OSWindows
	default:
		// PE also describes firmware and other execution environments.
		return core.OSUnknown
	}
}
