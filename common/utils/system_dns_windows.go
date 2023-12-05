//go:build windows
// +build windows

package utils

import (
	"golang.org/x/sys/windows/registry"
	"strings"
)

func GetSystemDnsServers() ([]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()

	interfaceKeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	var servers []string
	for _, keyName := range interfaceKeys {
		interfaceKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`+keyName, registry.QUERY_VALUE)
		if err != nil {
			return nil, err
		}
		func() {
			defer interfaceKey.Close()
			dns, _, err := interfaceKey.GetStringValue("NameServer")
			if err != nil || dns == "" {
				dhcpDns, _, err := interfaceKey.GetStringValue("DhcpNameServer")
				if err == nil {
					dns = dhcpDns
				}
			}

			if dns != "" {
				if strings.Contains(dns, ",") {
					servers = append(servers, strings.Split(dns, ",")...)
				} else if strings.Contains(dns, " ") {
					servers = append(servers, strings.Split(dns, " ")...)
				} else {
					servers = append(servers, dns)
				}
			}
		}()
	}
	// 去重 servers
	servers = RemoveRepeatStringSlice(servers)
	return servers, nil
}
