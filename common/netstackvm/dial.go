package netstackvm

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/netx/dns_lookup"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"net/netip"
	"time"
)

func (vm *NetStackVirtualMachine) DialTCP(timeout time.Duration, hostport string) (net.Conn, error) {
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
		host = dns_lookup.LookupFirst(host)
	}

	target := tcpip.FullAddress{
		NIC:  vm.MainNICID(),
		Addr: tcpip.AddrFrom4(netip.MustParseAddr(host).As4()),
		Port: uint16(port),
	}
	local := tcpip.FullAddress{
		NIC:  vm.MainNICID(),
		Addr: tcpip.AddrFrom4(netip.MustParseAddr(vm.GetMainNICIPv4Address().String()).As4()),
	}
	conn, err := gonet.DialTCPWithBind(
		utils.TimeoutContext(timeout), vm.stack,
		local,
		target, header.IPv4ProtocolNumber)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
