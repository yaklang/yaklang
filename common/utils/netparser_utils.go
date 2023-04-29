package utils

import (
	"fmt"
	"math/big"
	"net"
)

func InetNtoA(ip int64) net.IP {
	return net.ParseIP(fmt.Sprintf("%d.%d.%d.%d",
		byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)))
}

func InetAtoN(ip net.IP) int64 {
	if ip != nil {
		ret := big.NewInt(0)
		ret.SetBytes(ip.To4())
		return ret.Int64()
	} else {
		return -1
	}
}

func IPv4ToCClassNetwork(s string) (string, error) {
	ip := net.ParseIP(FixForParseIP(s))
	if ip != nil && ip.To4() != nil {
		_, network, err := net.ParseCIDR(fmt.Sprintf("%v/24", s))
		if err != nil {
			return "", err
		}
		return network.String(), nil
	}
	return "", Errorf("invalid ipv4: %v", s)
}
