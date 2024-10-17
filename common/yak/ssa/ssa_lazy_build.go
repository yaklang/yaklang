package ssa

import "sync"

type lazyBuilder struct {
	_build  func()
	isBuild bool
}

func (n *lazyBuilder) SetLazyBuilder(Builder func()) {
	once := sync.Once{}
	n._build = func() { once.Do(Builder) }
	n.isBuild = false
}

func (n *lazyBuilder) Build() {
	if n._build == nil || n.isBuild != false {
		return
	}
	n.isBuild = true
	n._build()
}
