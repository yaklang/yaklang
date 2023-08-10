package utils

import (
	_ "github.com/yaklang/yaklang/common/utils/arptable"
	"net"
	"strings"
)

var (
	TargetIsLoopback = Errorf("loopback")
)

var (
	ipLoopback = make(map[string]interface{})
)

func init() {
	addrs, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, i := range addrs {
		ret, _ := i.Addrs()
		for _, addr := range ret {
			ipNet, ok := addr.(*net.IPNet)
			if ok {
				ipLoopback[ipNet.IP.String()] = ipNet
			}
		}
	}
}

func IsLoopback(t string) bool {
	ipInstance := net.ParseIP(FixForParseIP(t))
	if ipInstance != nil {
		if ipInstance.IsLoopback() {
			return true
		}
	}

	if strings.HasPrefix(FixForParseIP(t), "127.") {
		return true
	} else {
		_, ok := ipLoopback[FixForParseIP(t)]
		return ok
	}
}
