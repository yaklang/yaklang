package hids

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestSystemHealthStats(t *testing.T) {
	info, err := SystemHealthStats()
	if err != nil {
		panic(err)
	}
	spew.Dump(info)
}
