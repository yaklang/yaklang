package analyzer

import "context"

type reportOnlyKey struct{}

// ReportOnly omits legacy relationship maps when a typed Requirement already
// carries the same edge. It does not affect parsing, evidence, or resolution.
// The pipeline uses it only when the caller requested a Report, not Packages.
func ReportOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, reportOnlyKey{}, true)
}

func reportOnly(ctx context.Context) bool {
	v, _ := ctx.Value(reportOnlyKey{}).(bool)
	return v
}
