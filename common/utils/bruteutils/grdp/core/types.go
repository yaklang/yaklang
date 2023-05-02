package core

import "github.com/yaklang/yaklang/common/utils/bruteutils/grdp/emission"

type Transport interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error

	On(event, listener interface{}) *emission.Emitter
	Once(event, listener interface{}) *emission.Emitter
	Emit(event interface{}, arguments ...interface{}) *emission.Emitter
}

type FastPathListener interface {
	RecvFastPath(secFlag byte, s []byte)
}

type FastPathSender interface {
	SendFastPath(secFlag byte, s []byte) (int, error)
}
