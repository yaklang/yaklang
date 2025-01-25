package netstackvm

import (
	"context"
	"net"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/dhcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
)

func (vm *NetStackVirtualMachine) StartDHCP(callback func(ip net.IP)) error {
	if vm.dhcpStarted.IsSet() {
		log.Warn("dhcp client already started, do not start again")
		return nil
	}
	vm.dhcpStarted.Set()

	log.Info("start to create dhcp client")
	vm.dhcpClient = dhcp.NewClient(
		vm.stack, vm.MainNICID(),
		vm.config.DHCPAcquireTimeout,
		vm.config.DHCPAcquireInterval,
		vm.config.DHCPAcquireRetryInterval,
		func(ctx context.Context, lost, acq tcpip.AddressWithPrefix, cfg dhcp.Config) {
			preferIp, perferNet, err := net.ParseCIDR(acq.String())
			if err != nil {
				log.Errorf("failed to parse cidr: %v", err)
				return
			}
			log.Infof("dhcp client acquired ip: %v, net: %v", preferIp.String(), perferNet.String())
		},
	)

	log.Info("start to run dhcp client")
	go func() {
		vm.dhcpClient.Run(vm.config.ctx)
	}()
	return nil
}
