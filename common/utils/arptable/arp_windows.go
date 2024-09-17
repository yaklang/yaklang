//go:build windows
// +build windows

package arptable

// Windows arpx table reader added by Claudio Matsuoka.
// Tested only in Windows 8.1, hopefully the arpx command output format
// is the same in other Windows versions.

import (
	"github.com/yaklang/yaklang/common/utils/execx"
	"strings"
)

func Table() ArpTable {
	data, err := execx.Command("arp", "-a").Output()
	if err != nil {
		return nil
	}

	var table = make(ArpTable)
	skipNext := false
	for _, line := range strings.Split(string(data), "\n") {
		// skip empty lines
		if len(line) <= 0 {
			continue
		}
		// skip Interface: lines
		if line[0] != ' ' {
			skipNext = true
			continue
		}
		// skip column headers
		if skipNext {
			skipNext = false
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ip := fields[0]
		// Normalize MAC address to colon-separated format
		table[ip] = strings.Replace(fields[1], "-", ":", -1)
	}

	return table
}
