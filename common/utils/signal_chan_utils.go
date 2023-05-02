package utils

import (
	"os"
	"os/signal"
	"syscall"
	"yaklang.io/yaklang/common/log"
)

func NewSignalChannel(targetSignal ...os.Signal) chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, targetSignal...)
	return c
}

var WaitReleaseBySignal = func(fn func()) {
	sigC := NewSignalChannel(os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	defer signal.Stop(sigC)

	for {
		select {
		case <-sigC:
			log.Warn("recv signal abort")
			fn()
			return
		}
	}
}

var WaitBySignal = func(fn func(), sigs ...os.Signal) {
	sigC := NewSignalChannel(sigs...)
	defer signal.Stop(sigC)

	for {
		select {
		case <-sigC:
			log.Warn("recv signal abort")
			fn()
			return
		}
	}
}
