package netstackvm

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/dhcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/utils/arptable"
	"net"
	"time"
)

func (vm *NetStackVirtualMachine) StartDHCP() error {
	if vm.dhcpStarted.IsSet() {
		log.Warn("dhcp client already started, do not start again")
		return nil
	}
	vm.dhcpStarted.Set()

	log.Infof("start to create dhcp client nic: %v, acq timeout: %v, acq interval: %v, retry interval: %v",
		vm.MainNICID(),
		vm.config.DHCPAcquireTimeout, vm.config.DHCPAcquireInterval, vm.config.DHCPAcquireRetryInterval,
	)
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
			var getawey net.IP
			if !cfg.ServerAddress.Unspecified() {
				getawey = net.ParseIP(cfg.ServerAddress.String())
			}
			log.Infof("dhcp client acquired ip: %v, net: %v getaway: %v", preferIp.String(), perferNet.String(), getawey)

			if macAddr, err := arptable.SearchHardware(getawey.String()); err == nil {
				tcpErr := vm.stack.AddStaticNeighbor(
					vm.MainNICID(),
					header.IPv4ProtocolNumber,
					tcpip.AddrFrom4([4]byte(getawey.To4())),
					tcpip.LinkAddress(string(macAddr)),
				)
				if tcpErr != nil {
					log.Errorf("add static neighbor failed: %v", tcpErr)
				}
			}

			vm.driver.SetGatewayIP(getawey)
			err = vm.SetMainNICv4(preferIp, perferNet, getawey)
			if err != nil {
				log.Errorf("set nic ip failed: %v", err)
				return
			}
			log.Infof("finish to set nic ip: %v", preferIp.String())

			log.Infof("start to set default route")
			err = vm.SetDefaultRoute(getawey)
			if err != nil {
				log.Errorf("set default route failed: %v", err)
				return
			}

			vm.arpPersistentMap.Store(preferIp.String(), struct{}{})
			// start to announcement arp localIP macAddr
			if err := vm.StartAnnounceARP(); err != nil {
				log.Errorf("start to announce arp failed: %v", err)
				return
			}
		},
	)

	vm.dhcpClient.SetOverrideLinkAddr(tcpip.LinkAddress(vm.GetMainNICLinkAddress()))
	log.Info("start to run dhcp client")
	go func() {
		results := vm.dhcpClient.Run(vm.config.ctx)
		_ = results
		spew.Dump(results)
	}()
	return nil
}

func (t *NetStackVirtualMachine) WaitDHCPFinished(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if t.GetMainNICIPv4Address() != nil && t.GetMainNICIPv4Gateway() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.config.ctx.Done():
			return t.config.ctx.Err()
		case <-ticker.C:
		}
	}
}
