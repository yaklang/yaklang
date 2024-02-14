package arpx

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestArpWithPcap(t *testing.T) {
	a, err := ArpWithPcap(context.Background(), "en0", "192.168.31.1/24")
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}
