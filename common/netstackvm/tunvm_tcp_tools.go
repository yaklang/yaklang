package netstackvm

import (
	"github.com/yaklang/yaklang/common/lowtun/netstack"
	"github.com/yaklang/yaklang/common/utils"
)

type TunVmTCPListener struct {
	vm *TunVirtualMachine
	ch chan netstack.TCPConn
}

func (t *TunVmTCPListener) Accept() (netstack.TCPConn, error) {
	select {
	case conn, ok := <-t.ch:
		if ok {
			return conn, nil
		}
		return nil, utils.Error("tun vm tcp listener closed")
	}
}

func (t *TunVmTCPListener) Close() error {
	err := t.vm.Close()
	if err != nil {
		return err
	}
	close(t.ch)
	return nil
}
