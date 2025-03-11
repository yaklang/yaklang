package netstackvm

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"net/netip"
)

func (vm *NetStackVirtualMachineEntry) ListenTCP(hostport string) (net.Listener, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic: %v", err)
		}
	}()
	host, port, err := utils.ParseStringToHostPort(hostport)
	if err != nil {
		return nil, err
	}
	if !utils.IsIPv4(host) {
		host = netx.LookupFirst(host)
	}

	address := tcpip.FullAddress{
		NIC:  vm.MainNICID(),
		Addr: tcpip.AddrFrom4(netip.MustParseAddr(host).As4()),
		Port: uint16(port),
	}
	return gonet.ListenTCP(vm.stack, address, header.IPv4ProtocolNumber)
}
