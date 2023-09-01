package rule

import (
	"net/netip"
	"testing"
)

func TestAddressRule_Generate(t *testing.T) {
	rule := &AddressRule{
		Any:      false,
		Negative: false,
		positiveRules: []*AddressRule{
			{
				IPv4CIDR: "172.16.0.0/12",
			},
		},
		negativeRules: []*AddressRule{
			{
				IPv4CIDR: "172.16.0.0/13",
			},
		},
	}
	prefix := netip.MustParsePrefix("172.24.0.0/13")
	var errcount int
	for i := 0; i < 10000; i++ {
		ip := netip.MustParseAddr(rule.Generate())
		if !prefix.Contains(ip) {
			errcount++
		}
	}
	t.Logf("err percent: %f%%", float64(errcount)/100)
}
