package arptable

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/sync/singleflight"
)

type ArpTable map[string]string

var (
	stop     = make(chan struct{})
	arpCache = &cache{
		table: make(ArpTable),
	}
	sfGroup = &singleflight.Group{}
)

func init() {
	arpCache.Refresh()
	AutoRefresh(10 * time.Second)
}

func AutoRefresh(t time.Duration) {
	go func() {
		for {
			select {
			case <-time.After(t):
				arpCache.Refresh()
			case <-stop:
				return
			}
		}
	}()
}

func StopAutoRefresh() {
	stop <- struct{}{}
}

func CacheUpdate() {
	arpCache.Refresh()
}

func CacheLastUpdate() time.Time {
	return arpCache.Updated
}

func CacheUpdateCount() int {
	return arpCache.UpdatedCount
}

// Search looks up the MAC address for an IP address
// in the arpx table
func Search(ip string) string {
	return arpCache.Search(ip)
}

func SearchHardware(ip string) (net.HardwareAddr, error) {
	v, err, _ := sfGroup.Do(ip, func() (interface{}, error) {
		result := Search(ip)
		if result == "" {
			// 异步刷新 ARP 表
			go func() {
				CacheUpdate()
			}()
			return nil, fmt.Errorf("arp search table failed for IP: %s", ip)
		}

		return net.ParseMAC(result)
	})

	if err != nil {
		return nil, err
	}
	return v.(net.HardwareAddr), nil
}
