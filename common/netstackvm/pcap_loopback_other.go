//go:build !linux

package netstackvm

import "github.com/gopacket/gopacket/layers"

func newPCAPLoopbackWriter(_ string, _ layers.LinkType, fallback func([]byte) error) (func([]byte) error, func()) {
	return fallback, func() {}
}
