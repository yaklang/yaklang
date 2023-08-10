//go:build linux
// +build linux

package arptable

import (
	"bufio"
	"os"
	"strings"
)

const (
	f_IPAddr int = iota
	f_HWType
	f_Flags
	f_HWAddr
	f_Mask
	f_Device
)

func Table() ArpTable {
	f, err := os.Open("/proc/net/arpx")

	if err != nil {
		return nil
	}

	defer f.Close()

	s := bufio.NewScanner(f)
	s.Scan() // skip the field descriptions

	var table = make(ArpTable)

	for s.Scan() {
		line := s.Text()
		fields := strings.Fields(line)
		table[fields[f_IPAddr]] = fields[f_HWAddr]
	}

	return table
}
