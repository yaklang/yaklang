package ssa

import "sync"

type lazyBuilder struct {
	_build func()
}

func (n *lazyBuilder) SetLazyBuilder(Builder func()) {
	once := sync.Once{}
	n._build = func() { once.Do(Builder) }
}

func (n *lazyBuilder) Build() {
	if n._build == nil {
		return
	}
	n._build()
}
