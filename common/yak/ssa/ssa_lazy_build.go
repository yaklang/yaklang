package ssa

import "sync"

type lazyBuilder struct {
	_build  func()
	isBuild bool
}

func (n *lazyBuilder) SetLazyBuilder(Builder func(), asyns ...bool) {
	asyn := true
	if len(asyns) > 0 {
		asyn = asyns[0]
	}
	once := sync.Once{}
	if asyn {
		n._build = func() { once.Do(Builder) }
	} else {
		n._build = Builder
	}

	n.isBuild = false
}

func (n *lazyBuilder) Build() {
	if n._build == nil || n.isBuild != false {
		return
	}
	n.isBuild = true
	n._build()
}
