package node

import "testing"

func TestNormalizeHostInfoDedupesAddressesAndBackfillsPrimary(t *testing.T) {
	t.Parallel()

	info := normalizeHostInfo(HostInfo{
		Hostname:        " host-a ",
		PrimaryIP:       "",
		IPAddresses:     []string{"10.0.0.2", "10.0.0.2", " 192.168.1.7 "},
		OperatingSystem: " linux ",
		Architecture:    " amd64 ",
	})

	if info.Hostname != "host-a" {
		t.Fatalf("unexpected hostname: %q", info.Hostname)
	}
	if info.PrimaryIP != "10.0.0.2" {
		t.Fatalf("unexpected primary ip: %q", info.PrimaryIP)
	}
	if len(info.IPAddresses) != 2 {
		t.Fatalf("unexpected ip address count: %d", len(info.IPAddresses))
	}
	if info.IPAddresses[1] != "192.168.1.7" {
		t.Fatalf("unexpected secondary ip: %q", info.IPAddresses[1])
	}
	if info.OperatingSystem != "linux" {
		t.Fatalf("unexpected operating system: %q", info.OperatingSystem)
	}
	if info.Architecture != "amd64" {
		t.Fatalf("unexpected architecture: %q", info.Architecture)
	}
}
