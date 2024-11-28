package ssa

import (
	"fmt"
	"runtime"
)

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
	defer func() {
		if msg := recover(); msg != nil {
			buf := make([]byte, 1024*4)
			n := runtime.Stack(buf, false)
			fmt.Printf("Recovered from panic: %s\nStack Trace:\n%s\n", msg, buf[:n])
		}
	}()
	n.isBuild = true
	for _, f := range n._build {
		f()
	}
}
