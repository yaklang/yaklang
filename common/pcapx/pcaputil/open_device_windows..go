package pcaputil

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
)

var RegistryNetworkPath = "SYSTEM\\CurrentControlSet\\Control\\Network\\{4D36E972-E325-11CE-BFC1-08002BE10318}"
var RegistryConnectPath = "SYSTEM\\CurrentControlSet\\Control\\Network\\{4D36E972-E325-11CE-BFC1-08002BE10318}\\%s\\Connection"

func deviceNameToPcapGuidWindows(wantName string) (string, error) {
	networkKey, _ := registry.OpenKey(registry.LOCAL_MACHINE, RegistryNetworkPath, registry.ENUMERATE_SUB_KEYS)
	defer networkKey.Close()
	guids, _ := networkKey.ReadSubKeyNames(0)
	for _, guid := range guids {
		connectionPath := fmt.Sprintf(RegistryConnectPath, guid)
		connKey, err := registry.OpenKey(registry.LOCAL_MACHINE, connectionPath, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		defer connKey.Close()

		name, _, _ := connKey.GetStringValue("Name")
		if name != wantName {
			continue
		}
		return fmt.Sprintf(`\Device\NPF_%s`, guid), nil
	}
	return "", NewConvertIfaceNameError(wantName)
}
