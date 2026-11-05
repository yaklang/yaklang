package match

import (
	"encoding/binary"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"strings"
)

func tcpParser(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	// buffer provider
	provider := newTCPProvider(c.PK, c.overridePayload)
	if !c.Must(provider != nil) {
		return nil
	}

	// register buffer provider
	c.SetBufferProvider(provider)

	return nil
}

func tcpCfgMatch(c *matchContext) error {
	if c.Rule.ContentRuleConfig.TcpConfig == nil {
		return nil
	}
	tcpConfig := c.Rule.ContentRuleConfig.TcpConfig

	tcp, ok := c.PK.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if !ok {
		return fmt.Errorf("tcp layer not found")
	}

	if tcpConfig.Seq != nil && !c.Must(*tcpConfig.Seq == int(tcp.Seq)) {
		return nil
	}

	if tcpConfig.Ack != nil && !c.Must(*tcpConfig.Ack == int(tcp.Ack)) {
		return nil
	}

	if tcpConfig.Window != nil &&
		!c.Must(negIf(tcpConfig.NegativeWindow, *tcpConfig.Window == int(tcp.Window))) {
		return nil
	}

	if tcpConfig.Flags != "" {
		matched, err := matchTCPFlags(tcpConfig.Flags, tcp)
		if err != nil {
			return err
		}
		if !c.Must(matched) {
			return nil
		}
	}

	if tcpConfig.TCPMss != nil {
		var mss int
		var set bool
		for _, opt := range tcp.Options {
			if opt.OptionType == layers.TCPOptionKindMSS {
				mss = int(binary.BigEndian.Uint16(opt.OptionData))
				set = true
				break
			}
		}
		// if option mss not found, skip
		if !c.Must(set) {
			return nil
		}

		if !c.Must(tcpConfig.TCPMss.Match(mss)) {
			return nil
		}
	}

	return nil
}

func matchTCPFlags(raw string, tcp *layers.TCP) (bool, error) {
	parts := strings.Split(raw, ",")
	flags := strings.ToUpper(strings.TrimSpace(parts[0]))
	var mode byte
	if len(flags) > 0 {
		switch flags[0] {
		case '!', '*', '+':
			mode, flags = flags[0], flags[1:]
		}
	}
	ignored := ""
	if len(parts) > 1 {
		ignored = strings.ToUpper(strings.TrimSpace(parts[1]))
	}
	if len(parts) > 2 {
		return false, fmt.Errorf("invalid tcp flags expression %q", raw)
	}

	wanted, err := tcpFlagMask(flags)
	if err != nil {
		return false, err
	}
	ignoreMask, err := tcpFlagMask(ignored)
	if err != nil {
		return false, err
	}
	current := tcpCurrentFlagMask(tcp) &^ ignoreMask
	wanted &^= ignoreMask
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

func tcpFlagMask(raw string) (uint16, error) {
	var mask uint16
	for _, flag := range raw {
		switch flag {
		case 'F':
			mask |= 1 << 0
		case 'S':
			mask |= 1 << 1
		case 'R':
			mask |= 1 << 2
		case 'P':
			mask |= 1 << 3
		case 'A':
			mask |= 1 << 4
		case 'U':
			mask |= 1 << 5
		case 'E':
			mask |= 1 << 6
		case 'C':
			mask |= 1 << 7
		case '0':
		case 'N', '1':
			mask |= 1 << 8
		case '2':
			mask |= 1 << 9
		default:
			return 0, fmt.Errorf("unsupported tcp flag %q", flag)
		}
	}
	return mask, nil
}

func tcpCurrentFlagMask(tcp *layers.TCP) uint16 {
	var mask uint16
	values := []bool{tcp.FIN, tcp.SYN, tcp.RST, tcp.PSH, tcp.ACK, tcp.URG, tcp.ECE, tcp.CWR, tcp.NS}
	for index, set := range values {
		if set {
			mask |= 1 << index
		}
	}
	return mask
}

func newTCPProvider(pk gopacket.Packet, override ...[]byte) func(modifier modifier.Modifier) []byte {
	tcp, ok := pk.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if !ok {
		return nil
	}
	return func(mdf modifier.Modifier) []byte {
		switch mdf {
		case modifier.TCPHDR:
			return tcp.Contents
		case modifier.Default:
			if len(override) > 0 && override[0] != nil {
				return override[0]
			}
			return tcp.Payload
		}
		return nil
	}
}
