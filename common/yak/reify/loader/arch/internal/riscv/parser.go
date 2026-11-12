package riscv

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

const maxDiagnostics = 64

type diagnostics struct {
	profile Profile
	items   []core.Diagnostic
	dropped int
}

func (d *diagnostics) add(code, message string) {
	if len(d.items) < maxDiagnostics {
		d.items = append(d.items, core.Diagnostic{Code: code, Message: message})
		return
	}
	d.dropped++
}

func (d *diagnostics) finish() []core.Diagnostic {
	if d.dropped == 0 {
		return d.items
	}
	return append(d.items, core.Diagnostic{
		Code:    "diagnostics-truncated",
		Message: "further " + d.profile.Name + " diagnostics were omitted",
	})
}

// Parse decodes one code region with the profile that matches the target.
func Parse(ctx context.Context, profile Profile, request core.ParseRequest) (*core.ParseResult, error) {
	if err := profile.Check(request.Target); err != nil {
		return nil, err
	}
	return parse(ctx, profile, request)
}

// Disassemble renders one code region as assembly text.
func Disassemble(ctx context.Context, profile Profile, request core.ParseRequest) (*core.Disassembly, error) {
	if err := profile.Check(request.Target); err != nil {
		return nil, err
	}
	return disassemble(ctx, profile, request)
}

// DiscoverBoundaries finds function starts with the shared decoder.
func DiscoverBoundaries(profile Profile, reader *core.ByteReader, hints core.BoundaryHints, image *core.Image) (core.BoundaryResult, error) {
	return discoverBoundaries(profile, reader, hints, image)
}
