package scannode

import "context"

type legionJobExecutionContextKey struct{}

func withLegionJobExecutionRef(
	ctx context.Context,
	ref jobExecutionRef,
) context.Context {
	copyRef := ref
	return context.WithValue(ctx, legionJobExecutionContextKey{}, &copyRef)
}

func legionJobExecutionRefFromContext(ctx context.Context) *jobExecutionRef {
	if ctx == nil {
		return nil
	}
	ref, _ := ctx.Value(legionJobExecutionContextKey{}).(*jobExecutionRef)
	return ref
}
