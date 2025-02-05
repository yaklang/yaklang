package netstackvm

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"os/exec"
	"time"
)

func (vm *TunVirtualMachine) HijackDomain(domain string) error {
	if utils.IsIPv4(domain) {
		return vm.HijackIP(domain)
	} else if _, ipnet, err := net.ParseCIDR(domain); err == nil && ipnet != nil {
		return vm.HijackIPNet(ipnet)
	} else {
		for _, ip := range netx.LookupAll(domain) {
			if err := vm.HijackIP(ip); err != nil {
				log.Errorf("hijack ip %s failed: %v", ip, err)
			}
		}
		return nil
	}
}

func (vm *TunVirtualMachine) HijackIP(ip string) error {
	var ipNet *net.IPNet
	var err error
	if utils.IsIPv4(ip) {
		cidr := ip
		cidr += "/32"
		_, ipNet, err = net.ParseCIDR(cidr)
		if err != nil {
			return utils.Errorf("invalid ip: %s", ip)
		}
	} else if _, ipnet, err := net.ParseCIDR(ip); err == nil && ipnet != nil {
		ipNet = ipnet
	} else {
		return utils.Errorf("invalid ip: %s", ip)
	}
	if ipNet == nil {
		return utils.Errorf("invalid ip: %s", ip)
	}
	return vm.HijackIPNet(ipNet)
}

func (vm *TunVirtualMachine) HijackIPNet(ipNet *net.IPNet) error {
	ctx, cancel := context.WithTimeout(vm.ctx, 5*time.Second)
	defer cancel()

	name := vm.GetTunnelName()
	if name == "" {
		return utils.Errorf("tunnel name not set")
	}

	err := exec.CommandContext(ctx, "route", "add", "-net", ipNet.String(), "-interface", name).Run()
	if err != nil {
		return utils.Errorf("route add failed: %v", err)
	}
	return nil
}
