package pcaputil

import (
	"bytes"
	"fmt"
)

func telnetNegotiationStart(w []byte) bool {
	return len(w) >= 3 && w[0] == 255 && w[1] >= 251 && w[1] <= 254
}

// Inspect only a bounded consecutive IAC prefix. Arbitrary data containing
// 0xff is not evidence for Telnet. Subnegotiation commonly separates the
// initial DO/WILL commands (e.g. TERMINAL-TYPE SEND), even on ephemeral ports.
func telnetNegotiationPrefix(w []byte) (options [][2]byte, sub bool, more bool) {
	w = w[:min(len(w), 1024)]
	for at := 0; at < len(w); {
		if w[at] != 255 {
			return
		}
		if len(w)-at < 3 {
			more = len(w) < 1024
			return
		}
		if telnetNegotiationStart(w[at:]) {
			options = append(options, [2]byte{w[at+1], w[at+2]})
			at += 3
			continue
		}
		if w[at+1] != 250 {
			return
		}
		n, _, err := (*binFlow)(nil).frameTelnet(w[at:])
		if err != nil {
			return nil, false, false
		}
		if n == 0 {
			more = len(w) < 1024
			return
		}
		for _, op := range options {
			if op[1] == w[at+2] {
				sub = true
			}
		}
		at += n
	}
	return
}

func telnetInitialEvidence(w []byte) bool {
	opts, sub, _ := telnetNegotiationPrefix(w)
	return len(opts) >= 2 || len(opts) == 1 && sub
}

func needsMoreTelnetPrefix(w []byte) bool {
	opts, _, more := telnetNegotiationPrefix(w)
	return len(w) < 1024 && len(opts) > 0 && more
}

func telnetMatchingNegotiation(a, b []byte) bool {
	as, _, _ := telnetNegotiationPrefix(a)
	bs, _, _ := telnetNegotiationPrefix(b)
	for _, x := range as {
		for _, y := range bs {
			if x[1] == y[1] && (x[0] == 251 && y[0] == 253 || x[0] == 253 && y[0] == 251 || x[0] == 252 && y[0] == 254 || x[0] == 254 && y[0] == 252) {
				return true
			}
		}
	}
	return false
}
func (f *binFlow) frameTelnet(w []byte) (int, *binSpec, error) {
	if len(w) == 0 {
		return 0, nil, nil
	}
	n := 0
	if w[0] != 255 {
		if at := bytes.IndexByte(w, 255); at >= 0 {
			n = at
		} else {
			n = len(w)
		}
	} else {
		if len(w) < 2 {
			return 0, nil, nil
		}
		cmd := w[1]
		if cmd < 236 {
			return 0, nil, fmt.Errorf("telnet: invalid IAC command")
		}
		n = 2
		if cmd >= 251 && cmd <= 254 {
			n = 3
		}
		if cmd == 250 {
			if len(w) < 3 {
				return 0, nil, nil
			}
			n = 0
			for i := 3; i < len(w); i++ {
				if w[i] != 255 {
					continue
				}
				if i+1 >= len(w) {
					break
				}
				if w[i+1] == 255 {
					i++
					continue
				}
				if w[i+1] == 240 {
					n = i + 2
					break
				}
				return 0, nil, fmt.Errorf("telnet: unescaped IAC in subnegotiation")
			}
			if n == 0 {
				return 0, nil, nil
			}
		}
	}
	return n, &binSpec{rule: "network_control_fields", entry: "TelnetMessage"}, nil
}
