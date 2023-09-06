package utils

import (
	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/log"
	"net"
	"strconv"
	"strings"
)

var (
	domainRe = regexp2.MustCompile(`^(((?!-))(xn--|_)?[a-z0-9-]{0,61}[a-z0-9]{1,1}\.)*(xn--)?([a-z0-9][a-z0-9\-]{0,60}|[a-z0-9-]{1,30}\.[a-z]{2,})$`, regexp2.Singleline|regexp2.IgnoreCase)
)

func IsValidDomain(raw string) bool {
	var isIDN bool
	for _, b := range raw {
		if b > 127 {
			isIDN = true
			continue
		}
		if !(b >= 'a' && b <= 'z') && !(b >= 'A' && b <= 'Z') && !(b >= '0' && b <= '9') && b != '-' && b != '.' && b != '_' {
			return false
		}
	}

	if isIDN {
		return strings.Trim(raw, ".-") == raw
	}

	result, err := domainRe.MatchString(raw)
	if err != nil {
		log.Errorf("domain match failed; %s", err)
		return false
	}
	return result
}

func IsValidCIDR(raw string) bool {
	_, _, err := net.ParseCIDR(raw)
	if err != nil {
		return false
	}
	return true
}

func IsValidHostsRange(raw string) bool {
	r := ParseStringToHosts(raw)
	result := len(r) > 1
	if result {
		return true
	}

	if len(r) == 1 {
		return raw != r[0]
	}
	return false
}

func IsValidPortsRange(ports string) bool {
	return len(ParseStringToPorts(ports)) > 1
}

func IsValidInteger(raw string) bool {
	_, err := strconv.ParseInt(raw, 10, 64)
	return err == nil
}

func IsValidFloat(raw string) bool {
	_, err := strconv.ParseFloat(raw, 64)
	return err == nil && strings.Contains(strings.Trim(raw, "."), ".")
}
