package arptable

import (
	"errors"
	"fmt"
	"net"
	"time"
)

type ArpTable map[string]string

var (
	stop     = make(chan struct{})
	arpCache = &cache{
		table: make(ArpTable),
	}
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
// in the arp table
func Search(ip string) string {
	return arpCache.Search(ip)
}

func SearchHardware(ip string) (net.HardwareAddr, error) {
	result := Search(ip)
	if result != "" {
		return net.ParseMAC(result)
	}
	return nil, errors.New(fmt.Sprintf("arp search table failed: %s", ip))
}
