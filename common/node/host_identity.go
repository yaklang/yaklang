package node

import (
	"os"
	"strings"
)

type systemHostIdentityProvider struct{}

func (systemHostIdentityProvider) Snapshot() HostIdentity {
	return normalizeHostIdentity(HostIdentity{
		MachineID: readFirstTrimmedFile(
			"/etc/machine-id",
			"/var/lib/dbus/machine-id",
		),
		SystemUUID: readFirstTrimmedFile(
			"/sys/class/dmi/id/product_uuid",
			"/sys/devices/virtual/dmi/id/product_uuid",
		),
		InstanceID: readFirstTrimmedFile(
			"/var/lib/cloud/data/instance-id",
			"/var/lib/cloud/instance/instance-id",
		),
	})
}

func readFirstTrimmedFile(paths ...string) string {
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(raw))
		if value != "" {
			return value
		}
	}
	return ""
}
