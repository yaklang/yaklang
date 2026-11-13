package fileformat

import (
	"context"
	"debug/macho"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// MachO loads thin Mach-O images. Fat/universal files are not claimed because
// their magic overlaps with Java class files.
type MachO struct{}

var (
	_ core.Loader = MachO{}
)

func (MachO) Kind() core.Format        { return core.FormatMachO }
func (MachO) Name() string             { return "Mach-O" }
func (MachO) Description() string      { return core.FormatDescription(core.FormatMachO) }
func (MachO) Match(prefix []byte) bool { return core.CheckFormat(prefix) == core.FormatMachO }

func (MachO) Check(r *core.ByteReader) (*core.CheckResult, error) {
	image, target, diagnostics, err := inspectMachO(r)
	if err != nil {
		return nil, err
	}
	return core.NewCheckResult(r, core.FormatMachO, target, image, diagnostics)
}

func (MachO) Load(ctx context.Context, r *core.ByteReader) (*core.Artifact, error) {
	image, target, diagnostics, err := inspectMachO(r)
	if err != nil {
		return nil, err
	}
	return core.NewArtifact(ctx, r, core.FormatMachO, target, image, diagnostics)
}

func inspectMachO(reader *core.ByteReader) (*core.Image, core.Target, []core.Diagnostic, error) {
	file, err := macho.NewFile(reader)
	if err != nil {
		return nil, core.Target{}, nil, err
	}
	defer file.Close()
	bits := 32
	if file.Magic == macho.Magic64 {
		bits = 64
	}
	target := core.Target{
		OS:           core.OSUnknown,
		Architecture: machoArchitecture(file.Cpu),
		Bits:         bits,
		ByteOrder:    core.ByteOrderOf(file.ByteOrder),
		Machine:      fmt.Sprintf("%s/subcpu-0x%x", file.Cpu, file.SubCpu),
	}
	image := &core.Image{ObjectType: file.Type.String(), Sections: make([]core.Section, 0, len(file.Sections))}
	for _, section := range file.Sections {
		fileSize := section.Size
		// Mach-O SECTION_TYPE: S_ZEROFILL, S_GB_ZEROFILL and
		// S_THREAD_LOCAL_ZEROFILL have no file payload.
		switch section.Flags & 0xff {
		case 0x01, 0x0c, 0x12:
			fileSize = 0
		}
		image.Sections = append(image.Sections, core.Section{
			Name:           section.Seg + "," + section.Name,
			FileOffset:     uint64(section.Offset),
			FileSize:       fileSize,
			VirtualAddress: section.Addr,
			MemorySize:     section.Size,
			Flags:          uint64(section.Flags),
		})
	}
	diagnostics := []core.Diagnostic{{
		Code:    "entry-point-unavailable",
		Message: "This loader does not recover an entry-point address from the Mach-O header; omission does not mean no entry point",
	}}
	return image, target, diagnostics, nil
}

func machoArchitecture(cpu macho.Cpu) core.Architecture {
	switch cpu {
	case macho.CpuAmd64:
		return core.ArchAMD64
	case macho.Cpu386:
		return core.Arch386
	case macho.CpuArm64:
		return core.ArchARM64
	case macho.CpuArm:
		return core.ArchARM
	case macho.CpuPpc:
		return core.ArchPPC
	case macho.CpuPpc64:
		return core.ArchPPC64
	default:
		return core.ArchUnknown
	}
}
