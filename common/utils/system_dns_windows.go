//go:build windows
// +build windows

package utils

import "golang.org/x/sys/windows/registry"

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
	servers := []string{}
	for _, keyName := range interfaceKeys {
		interfaceKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`+keyName, registry.QUERY_VALUE)
		if err != nil {
			return nil, err
		}
		defer interfaceKey.Close()
		dns, _, err := interfaceKey.GetStringValue("NameServer")
		if err != nil {
			continue
		}
		if dns != "" {
			servers = append(servers, dns)
		}
	}
	return servers, nil
}
