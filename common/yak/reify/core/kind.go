// Package core defines the contracts and shared data used by Reify loaders,
// byte readers, architecture parsers and consumers. File formats, target
// platforms and instruction sets are independent dimensions.
package core

import (
	"bytes"
	"encoding/binary"
)

// InputKind classifies the representation of an input, independent of its
// container format or instruction set.
type InputKind string

const (
	KindNative   InputKind = "native"
	KindBytecode InputKind = "bytecode"
	KindSource   InputKind = "source"
)

// Format identifies a container or executable file format. A Format does not
// imply an operating system: ELF also describes BSD images, and PE also
// describes firmware.
type Format string

const (
	FormatUnknown Format = "unknown"
	FormatELF     Format = "elf"
	FormatPE      Format = "pe"
	FormatMachO   Format = "macho"
	FormatCOFF    Format = "coff"
	FormatArchive Format = "archive"
	FormatMZ      Format = "mz"
)

// CheckFormat returns the Format identified by a file prefix. It checks magic
// numbers only and does not validate a complete header; Loader.Check does that.
func CheckFormat(prefix []byte) Format {
	switch {
	case bytes.HasPrefix(prefix, []byte{0x7f, 'E', 'L', 'F'}):
		return FormatELF
	case bytes.HasPrefix(prefix, []byte{'M', 'Z'}):
		// An MZ header either extends into a PE image or is a plain MS-DOS
		// executable. The probe reports the container and the loaders decide:
		// the PE loader checks the extended header, the MZ loader takes the
		// rest.
		return FormatMZ
	case bytes.HasPrefix(prefix, []byte("!<arch>\n")):
		return FormatArchive
	}
	if len(prefix) >= 4 {
		switch binary.BigEndian.Uint32(prefix[:4]) {
		case 0xfeedface, 0xcefaedfe, 0xfeedfacf, 0xcffaedfe:
			return FormatMachO
		}
	}
	// Fat Mach-O (0xcafebabe) is not claimed here: the magic is shared with
	// Java class files. The Mach-O loader validates the fat header itself and
	// decides, instead of guessing from four bytes.
	return FormatUnknown
}

// OperatingSystem names an execution environment. OSUnknown means the
// available metadata does not determine one; it is not a default host value.
type OperatingSystem string

const (
	OSUnknown OperatingSystem = "unknown"
	OSLinux   OperatingSystem = "linux"
	OSWindows OperatingSystem = "windows"
	OSDarwin  OperatingSystem = "darwin"
	OSFreeBSD OperatingSystem = "freebsd"
	OSNetBSD  OperatingSystem = "netbsd"
	OSOpenBSD OperatingSystem = "openbsd"
	OSSolaris OperatingSystem = "solaris"
	OSDOS     OperatingSystem = "dos"
)

// Architecture names an instruction set. Mode carries execution-mode details
// such as Thumb on ARM.
type Architecture string

const (
	ArchUnknown Architecture = "unknown"
	Arch386     Architecture = "386"
	Arch8086    Architecture = "8086"
	ArchAMD64   Architecture = "amd64"
	ArchARM     Architecture = "arm"
	ArchARM64   Architecture = "arm64"
	ArchRISCV32 Architecture = "riscv32"
	ArchRISCV64 Architecture = "riscv64"
	ArchMIPS    Architecture = "mips"
	ArchMIPS64  Architecture = "mips64"
	ArchPPC     Architecture = "ppc"
	ArchPPC64   Architecture = "ppc64"
	ArchS390X   Architecture = "s390x"
	ArchLOONG64 Architecture = "loong64"
)

type ByteOrder string

const (
	ByteOrderUnknown ByteOrder = "unknown"
	LittleEndian     ByteOrder = "little"
	BigEndian        ByteOrder = "big"
)

// ByteOrderOf converts a byte order from encoding/binary.
func ByteOrderOf(order binary.ByteOrder) ByteOrder {
	switch order {
	case binary.LittleEndian:
		return LittleEndian
	case binary.BigEndian:
		return BigEndian
	default:
		return ByteOrderUnknown
	}
}
