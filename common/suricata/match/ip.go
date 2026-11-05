package match

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gopacket/gopacket/layers"
)

func ipCfgMatch(c *matchContext) error {
	if c.Rule.ContentRuleConfig == nil {
		return nil
	}
	config := c.Rule.ContentRuleConfig.IPConfig
	if config == nil {
		return nil
	}

	if ipv4, ok := c.PK.Layer(layers.LayerTypeIPv4).(*layers.IPv4); ok {
		if config.TTL != nil && !c.Must(config.TTL.Match(int(ipv4.TTL))) {
			return nil
		}
		if config.Sameip && !c.Must(ipv4.SrcIP.Equal(ipv4.DstIP)) {
			return nil
		}
		if config.IPProto != "" {
			protocol, err := parseIPProtocol(config.IPProto)
			if err != nil {
				return err
			}
			if !c.Must(ipv4.Protocol == protocol) {
				return nil
			}
		}
		if config.Id != nil && !c.Must(config.Id.Match(int(ipv4.Id))) {
			return nil
		}
		if config.FragOffset != nil && !c.Must(config.FragOffset.Match(int(ipv4.FragOffset))) {
			return nil
		}
		if config.Tos != nil && !c.Must(config.Tos.Match(int(ipv4.TOS))) {
			return nil
		}
		if config.FragBits != "" {
			matched, err := matchIPv4FragBits(config.FragBits, ipv4.Flags)
			if err != nil {
				return err
			}
			if !c.Must(matched) {
				return nil
			}
		}
		if config.IPOpts != "" {
			matched, err := matchIPv4Option(config.IPOpts, ipv4.Options)
			if err != nil {
				return err
			}
			if !c.Must(matched) {
				return nil
			}
		}
		return nil
	}

	if ipv6, ok := c.PK.Layer(layers.LayerTypeIPv6).(*layers.IPv6); ok {
		if config.TTL != nil && !c.Must(config.TTL.Match(int(ipv6.HopLimit))) {
			return nil
		}
		if config.Sameip && !c.Must(ipv6.SrcIP.Equal(ipv6.DstIP)) {
			return nil
		}
		if config.IPProto != "" {
			protocol, err := parseIPProtocol(config.IPProto)
			if err != nil {
				return err
			}
			if !c.Must(ipv6.NextHeader == protocol) {
				return nil
			}
		}
		if config.Id != nil || config.FragBits != "" || config.FragOffset != nil || config.IPOpts != "" || config.Tos != nil {
			c.Reject()
		}
		return nil
	}

	c.Reject()
	return nil
}

func parseIPProtocol(raw string) (layers.IPProtocol, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if value, err := strconv.ParseUint(raw, 0, 8); err == nil {
		return layers.IPProtocol(value), nil
	}
	switch raw {
	case "icmp":
		return layers.IPProtocolICMPv4, nil
	case "icmpv6", "icmp6":
		return layers.IPProtocolICMPv6, nil
	case "tcp":
		return layers.IPProtocolTCP, nil
	case "udp":
		return layers.IPProtocolUDP, nil
	case "gre":
		return layers.IPProtocolGRE, nil
	case "esp":
		return layers.IPProtocolESP, nil
	case "ah":
		return layers.IPProtocolAH, nil
	default:
		return 0, fmt.Errorf("unsupported ip_proto value %q", raw)
	}
}

func matchIPv4FragBits(raw string, flags layers.IPv4Flag) (bool, error) {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	mode := byte(0)
	if len(raw) > 0 {
		switch raw[0] {
		case '!', '*', '+':
			mode, raw = raw[0], raw[1:]
		}
	}
	var wanted layers.IPv4Flag
	for _, flag := range raw {
		switch flag {
		case 'M':
			wanted |= layers.IPv4MoreFragments
		case 'D':
			wanted |= layers.IPv4DontFragment
		case 'R':
			wanted |= layers.IPv4EvilBit
		default:
			return false, fmt.Errorf("unsupported fragbits flag %q", flag)
		}
	}
	current := flags & (layers.IPv4MoreFragments | layers.IPv4DontFragment | layers.IPv4EvilBit)
	switch mode {
	case '!':
		return current&wanted == 0, nil
	case '*':
		return current&wanted != 0, nil
	case '+':
		return current&wanted == wanted, nil
	default:
		return current == wanted, nil
	}
}

func matchIPv4Option(raw string, options []layers.IPv4Option) (bool, error) {
	optionTypes := map[string]uint8{
		"eol":   0,
		"nop":   1,
		"sec":   2,
		"lsrr":  3,
		"ts":    4,
		"esec":  5,
		"rr":    7,
		"satid": 8,
		"ssrr":  9,
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "any" {
		return len(options) > 0, nil
	}
	optionType, ok := optionTypes[raw]
	if !ok {
		return false, fmt.Errorf("unsupported ipopts value %q", raw)
	}
	for _, option := range options {
		if option.OptionType == optionType {
			return true, nil
		}
	}
	return false, nil
}
