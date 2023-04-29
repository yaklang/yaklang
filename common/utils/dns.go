package utils

import (
	"context"
	"net"
	"net/url"
	"time"
)

func GetIPByDomain(target string, timeout time.Duration) ([]net.IPAddr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, target)
	cancel()
	return addrs, err
}

func GetIPByURL(url url.URL, timeout time.Duration) ([]net.IPAddr, error) {
	hostname := url.Hostname()
	ip := net.ParseIP(hostname)
	if ip != nil {
		return []net.IPAddr{{ip, ""}}, nil
	} else {
		return GetIPByDomain(hostname, timeout)
	}
}
