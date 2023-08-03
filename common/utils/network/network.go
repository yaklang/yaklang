package network

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakdns"
	"strings"
	"sync"
	"time"
)

func ParseStringToCClassHosts(targets string) string {
	var target []string
	var cclassMap = new(sync.Map)
	for _, r := range utils.ParseStringToHosts(targets) {
		if utils.IsIPv4(r) {
			netStr, err := utils.IPv4ToCClassNetwork(r)
			if err != nil {
				target = append(target, r)
				continue
			}
			cclassMap.Store(netStr, nil)
			continue
		}

		if utils.IsIPv6(r) {
			target = append(target, r)
			continue
		}
		ip := yakdns.LookupFirst(r, yakdns.WithTimeout(5*time.Second))
		if ip != "" && utils.IsIPv4(ip) {
			netStr, err := utils.IPv4ToCClassNetwork(ip)
			if err != nil {
				target = append(target, r)
				continue
			}
			cclassMap.Store(netStr, nil)
			continue
		} else {
			target = append(target, r)
		}
	}
	cclassMap.Range(func(key, value interface{}) bool {
		s := key.(string)
		target = append(target, s)
		return true
	})
	return strings.Join(target, ",")
}
