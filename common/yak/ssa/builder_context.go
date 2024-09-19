package ssa

import "context"

func (b *FunctionBuilder) IsStop() bool {
	if b.ctx == nil {
		return false
	}
	select {
	case <-b.ctx.Done():
		return true
	default:
		return false
	}
}

func (b *FunctionBuilder) SetContext(ctx context.Context) {
	b.ctx = ctx
}
