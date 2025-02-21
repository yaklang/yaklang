package netstackvm

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/netx/dns_lookup"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (vm *TunVirtualMachine) HijackDomain(domain string) error {
	if utils.IsIPv4(domain) {
		return vm.HijackIP(domain)
	} else if _, ipnet, err := net.ParseCIDR(domain); err == nil && ipnet != nil {
		return vm.HijackIPNet(ipnet)
	} else {
		for _, ip := range dns_lookup.LookupAll(domain) {
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

	var cmder *exec.Cmd
	if runtime.GOOS == "windows" {
		iface, err := net.InterfaceByName(name)
		if err != nil {
			return utils.Errorf("tun interface not found: %v", err)
		}
		cmder = exec.CommandContext(ctx, "route", "ADD", ipNet.IP.String(), "MASK", MaskToIPString(ipNet.Mask), ipNet.IP.String(), "IF", strconv.Itoa(iface.Index))
	} else {
		ones, _ := ipNet.Mask.Size()
		ipNetStr := fmt.Sprintf("%s/%d", ipNet.IP.String(), ones)
		cmder = exec.CommandContext(ctx, "route", "add", "-net", ipNetStr, "-interface", name)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmder.Stdout = &stdout
	cmder.Stderr = &stderr
	err := cmder.Run()
	if err != nil {
		log.Errorf("route add failed: %v\nmsg: %s", err, string(stderr.Bytes()))
		return utils.Errorf("route add failed: %v", err)
	}
	if raw := strings.TrimSpace(stdout.String()); len(raw) > 0 {
		log.Infof("route add success: %s", raw)
	}
	if raw := strings.TrimSpace(stderr.String()); len(raw) > 0 {
		log.Warnf("route add failed: %s", raw)
		return utils.Errorf("route add failed: %s", raw)
	}
	return nil
}

func MaskToIPString(mask net.IPMask) string {
	return net.IP(mask).String()
}
