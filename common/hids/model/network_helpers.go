//go:build hids

package model

import "strings"

func HasNetworkEndpoint(network *Network) bool {
	if network == nil {
		return false
	}
	return strings.TrimSpace(network.SourceAddress) != "" ||
		strings.TrimSpace(network.DestAddress) != "" ||
		network.SourcePort > 0 ||
		network.DestPort > 0
}
