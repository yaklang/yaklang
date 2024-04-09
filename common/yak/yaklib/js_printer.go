package yaklib

import (
	"github.com/dop251/goja_nodejs/console"
	"github.com/yaklang/yaklang/common/log"
)

type StdPrinter struct{}

var (
	_                 console.Printer = (*StdPrinter)(nil)
	defaultStdPrinter                 = &StdPrinter{}
)

func (s *StdPrinter) Log(msg string) {
	log.Info(msg)
}

func (s *StdPrinter) Warn(msg string) {
	log.Warn(msg)
}

func (s *StdPrinter) Error(msg string) {
	log.Error(msg)
}
