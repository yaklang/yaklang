package ssaapi

import (
	"context"
)

type SSAParseProcessManager struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewSSAParseProcessManager() *SSAParseProcessManager {
	m := &SSAParseProcessManager{}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	return m
}

func (m *SSAParseProcessManager) Stop() {
	m.cancel()
}

func (m *SSAParseProcessManager) Context() context.Context {
	return m.ctx
}
