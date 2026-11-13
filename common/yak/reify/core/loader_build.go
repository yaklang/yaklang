package core

import (
	"context"
	"fmt"
)

// NewArtifact assembles and validates the common Artifact envelope. Loaders
// attach the format-specific image and target before calling it.
func NewArtifact(ctx context.Context, r *ByteReader, format Format, target Target, image *Image, diagnostics []Diagnostic) (*Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ordered, err := r.WithOrder(target.ByteOrder)
	if err != nil {
		return nil, err
	}
	artifact := &Artifact{
		Origin:      Origin{Name: r.Name(), Size: r.Size()},
		Kind:        KindNative,
		Format:      format,
		Target:      target,
		Image:       image,
		Diagnostics: diagnostics,
		Reader:      ordered,
	}
	if err := ValidateArtifact(artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

// NewCheckResult validates parsed metadata and returns the loader check report.
func NewCheckResult(r *ByteReader, format Format, target Target, image *Image, diagnostics []Diagnostic) (*CheckResult, error) {
	if err := ValidateTarget(target); err != nil {
		return nil, err
	}
	if err := ValidateImage(image, r.Size()); err != nil {
		return nil, err
	}
	return &CheckResult{
		Kind:        KindNative,
		Format:      format,
		Target:      target,
		ObjectType:  image.ObjectType,
		Sections:    len(image.Sections),
		Diagnostics: diagnostics,
	}, nil
}

// FormatDescription returns the loader description for a container format.
func FormatDescription(format Format) string {
	switch format {
	case FormatELF:
		return "ELF images; the target OS comes from the ELF OS ABI"
	case FormatPE:
		return "PE images; the target OS comes from the PE subsystem"
	case FormatMachO:
		return "thin Mach-O images; entry points are not recovered"
	default:
		return fmt.Sprintf("%s images", format)
	}
}
