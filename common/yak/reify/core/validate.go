package core

import (
	"fmt"
	"math"
)

// ValidateTarget checks representation fields, not an ISA/ABI compatibility
// matrix. A registered parser validates its own target support.
func ValidateTarget(target Target) error {
	if target.Bits != 16 && target.Bits != 32 && target.Bits != 64 {
		return fmt.Errorf("unsupported address width: %d", target.Bits)
	}
	if target.ByteOrder != LittleEndian && target.ByteOrder != BigEndian {
		return fmt.Errorf("invalid byte order: %q", target.ByteOrder)
	}
	if target.OS == "" || target.Architecture == "" {
		return fmt.Errorf("target requires OS and architecture identifiers")
	}
	return nil
}

// ValidateImage checks metadata and section ranges against the input size. It
// does not inspect machine instructions or an operating system load policy.
func ValidateImage(image *Image, fileSize int64) error {
	if image == nil {
		return nil
	}
	if fileSize < 0 {
		return fmt.Errorf("invalid input size: %d", fileSize)
	}
	size := uint64(fileSize)
	for _, section := range image.Sections {
		if section.FileSize != 0 && (section.FileOffset > size || section.FileSize > size-section.FileOffset) {
			return fmt.Errorf("section %q extends beyond the input file", section.Name)
		}
		if section.MemorySize > math.MaxUint64-section.VirtualAddress {
			return fmt.Errorf("section %q has an overflowing virtual address range", section.Name)
		}
	}
	return nil
}

// ValidateArtifact checks the common envelope. Native artifacts require an
// image and a complete target; bytecode and source artifacts may omit both.
func ValidateArtifact(a *Artifact) error {
	if a == nil {
		return fmt.Errorf("nil artifact")
	}
	if a.Origin.Name == "" || a.Origin.Size < 0 {
		return fmt.Errorf("invalid artifact origin")
	}
	if a.Kind == "" || a.Format == "" || a.Format == FormatUnknown {
		return fmt.Errorf("artifact is missing a kind or format")
	}
	if a.Kind == KindNative && a.Image == nil {
		return fmt.Errorf("native artifact is missing image metadata")
	}
	if a.Image == nil {
		return nil
	}
	if err := ValidateTarget(a.Target); err != nil {
		return fmt.Errorf("artifact target: %w", err)
	}
	if err := ValidateImage(a.Image, a.Origin.Size); err != nil {
		return fmt.Errorf("artifact image: %w", err)
	}
	return nil
}
