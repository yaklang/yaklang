// Package pcapx
// @Author bcy2007  2023/10/31 10:40
package pcapx

import (
	"github.com/google/gopacket/layers"
	"testing"
)

func TestGetPublicLinkLayer(t *testing.T) {
	eth, err := GetPublicLinkLayer(layers.EthernetTypeIPv4, true)
	if err != nil {
		t.Error(eth)
		return
	}
	t.Log(eth)
}
