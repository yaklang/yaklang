package hids

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestSystemHealthStats(t *testing.T) {
	info, err := SystemHealthStats()
	if err != nil {
		panic(err)
	}
	spew.Dump(info)
}
