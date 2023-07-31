package utils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"math/big"
	"net"
	"strconv"
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

func NetworkByteOrderUint8ToBytes(i any) []byte {
	raw, err := strconv.ParseUint(fmt.Sprint(i), 10, 8)
	if err != nil {
		log.Warnf("cannot convert %v to uint8", i)
		return make([]byte, 1)
	}
	return []byte{byte(raw)}
}

func NetworkByteOrderUint16ToBytes(i any) []byte {
	raw, err := strconv.ParseUint(fmt.Sprint(i), 10, 16)
	if err != nil {
		log.Warnf("cannot convert %v to uint16", i)
		return make([]byte, 2)
	}

	return []byte{
		byte(raw >> 8),
		byte(raw),
	}
}

func NetworkByteOrderBytesToUint16(r []byte) uint16 {
	if len(r) < 2 {
		return 0
	}
	return uint16(r[0])<<8 | uint16(r[1])
}

func NetworkByteOrderUint32ToBytes(i any) []byte {
	raw, err := strconv.ParseUint(fmt.Sprint(i), 10, 32)
	if err != nil {
		log.Warnf("cannot convert %v to uint32", i)
		return make([]byte, 4)
	}

	return []byte{
		byte(raw >> 24),
		byte(raw >> 16),
		byte(raw >> 8),
		byte(raw),
	}
}

func NetworkByteOrderUint64ToBytes(i any) []byte {
	raw, err := strconv.ParseUint(fmt.Sprint(i), 10, 64)
	if err != nil {
		log.Warnf("cannot convert %v to uint64", i)
		return make([]byte, 8)
	}

	return []byte{
		byte(raw >> 56),
		byte(raw >> 48),
		byte(raw >> 40),
		byte(raw >> 32),
		byte(raw >> 24),
		byte(raw >> 16),
		byte(raw >> 8),
		byte(raw),
	}
}
