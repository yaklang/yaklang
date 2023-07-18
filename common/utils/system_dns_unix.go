//go:build linux || darwin
// +build linux darwin

package utils

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

func GetSystemDnsServers() ([]string, error) {
	var servers []string
	file, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil, Errorf("open /etc/resolv.conf failed: %v", err)
	}
	defer file.Close()
	readLine := bufio.NewReader(file)
	for {
		line, _, err := readLine.ReadLine()
		if err != nil {
			break
		}
		if len(line) > 0 && (line[0] == ';' || line[0] == '#') {
			// comment.
			continue
		}
		regex := regexp.MustCompile(" |\r|\t|\n")
		f := regex.Split(strings.TrimSpace(string(line)), -1)
		if len(f) < 1 {
			continue
		}
		switch f[0] {
		case "nameserver": // add one name server
			if len(f) > 1 { // small, but the standard limit
				servers = append(servers, f[1])
			}
		default:
			continue
		}
	}
	return servers, nil
}
