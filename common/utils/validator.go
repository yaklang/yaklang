package utils

import (
	"github.com/dlclark/regexp2"
	"net"
	"yaklang/common/log"
	"strconv"
)

var (
	domainRe = regexp2.MustCompile(`(?=^.{3,255}$)[a-zA-Z0-9][-a-zA-Z0-9]{0,62}(\.[a-zA-Z][a-zA-Z]{2,62})+$`, regexp2.Singleline|regexp2.IgnoreCase)
)

func IsValidDomain(raw string) bool {
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
	return err == nil
}
