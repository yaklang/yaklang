package routewrapper

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

var dotZero = []byte{'.', '0'}

func ourParseCIDRv4(s string) (*net.IPNet, error) {
	c := strings.Count(s, ".")
	_s := []byte(s)
	for i := c; i < 3; i++ {
		_s = append(_s, dotZero...)
	}
	if strings.LastIndexByte(s, byte('/')) < 0 {
		_s = append(_s, byte('/'))
		_s = strconv.AppendInt(_s, int64(8*(c+1)), 10)
	}
	s = string(_s)
	_, net, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	return net, err
}

func ourParseCIDRv6(s string) (*net.IPNet, error) {
	_s := []byte(s)
	i := strings.LastIndexByte(s, byte('/'))
	if i < 0 {
		i = len(_s)
		_s = append(_s, '/', '1', '2', '8')
	}
	j := strings.LastIndexByte(s, byte('%'))
	if j >= 0 {
		if j > i {
			return nil, fmt.Errorf("Invalid CIDR block notation: %s", s)
		}
		l := copy(_s[j:], _s[i:])
		_s = _s[0 : j+l]
	}
	s = string(_s)
	_, net, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	return net, err
}
