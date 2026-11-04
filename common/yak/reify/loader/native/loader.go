// Package native provides one loader per executable file format. Container
// interpretation is independent of the architecture parsers selected later.
package native

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// newArtifact assembles the common envelope. Loader and Parser are attached by
// the catalog, which owns those instances.
func newArtifact(ctx context.Context, r *core.ByteReader, format core.Format, target core.Target, image *core.Image, diagnostics []core.Diagnostic) (*core.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ordered, err := r.WithOrder(target.ByteOrder)
	if err != nil {
		return nil, err
	}
	artifact := &core.Artifact{
		Origin:      core.Origin{Name: r.Name(), Size: r.Size()},
		Kind:        core.KindNative,
		Format:      format,
		Target:      target,
		Image:       image,
		Diagnostics: diagnostics,
		Reader:      ordered,
	}
	if err := core.ValidateArtifact(artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func checkResult(r *core.ByteReader, format core.Format, target core.Target, image *core.Image, diagnostics []core.Diagnostic) (*core.CheckResult, error) {
	if err := core.ValidateTarget(target); err != nil {
		return nil, err
	}
	if err := core.ValidateImage(image, r.Size()); err != nil {
		return nil, err
	}
	return &core.CheckResult{
		Kind:        core.KindNative,
		Format:      format,
		Target:      target,
		ObjectType:  image.ObjectType,
		Sections:    len(image.Sections),
		Diagnostics: diagnostics,
	}, nil
}

func matchFormat(prefix []byte, format core.Format) bool {
	return core.CheckFormat(prefix) == format
}

func describe(format core.Format) string {
	switch format {
	case core.FormatELF:
		return "ELF images; the target OS comes from the ELF OS ABI"
	case core.FormatPE:
		return "PE images; the target OS comes from the PE subsystem"
	case core.FormatMachO:
		return "thin Mach-O images; entry points are not recovered"
	default:
		return fmt.Sprintf("%s images", format)
	}
}
