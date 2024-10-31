package ssa

type lazyBuilder struct {
	_build  []func()
	isBuild bool
}

func (l *lazyBuilder) AddLazyBuilder(Builder func(), async ...bool) {
	l._build = append(l._build, Builder)
}

func (n *lazyBuilder) Build() {
	if n._build == nil || n.isBuild {
		return
	}
	n.isBuild = true
	for _, f := range n._build {
		f()
	}
}
