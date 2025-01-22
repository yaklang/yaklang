// Copyright 2019 The Fuchsia Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dhcp implements a DHCP client and server as described in RFC 2131.
package dhcp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
)

// Seconds represents a time duration in seconds.
//
// RFC 2131 Section 3.3
// https://tools.ietf.org/html/rfc2131#section-3.3
//
//	Throughout the protocol, times are to be represented in units of seconds.
//
//	Representing relative times in units of seconds in an unsigned 32 bit
//	word gives a range of relative times from 0 to approximately 100 years,
//	which is sufficient for the relative times to be measured using DHCP.
type Seconds uint32

func (s Seconds) String() string {
	return fmt.Sprintf("%ds", s)
}

func (s Seconds) Duration() time.Duration {
	return time.Duration(s) * time.Second
}

// Config is standard DHCP configuration.
type Config struct {
	ServerAddress tcpip.Address     // address of the server
	SubnetMask    tcpip.AddressMask // client address subnet mask
	Router        []tcpip.Address   // client router addresses
	DNS           []tcpip.Address   // client DNS server addresses
	UpdatedAt     time.Time         // monotonic time at which lease was last updated
	LeaseLength   Seconds           // time until lease expires, relative to the value of UpdatedAt
	RenewTime     Seconds           // time until client enters RENEWING state, relative to the value of UpdatedAt
	RebindTime    Seconds           // time until client enters REBINDING state, relative to the value of UpdatedAt
	Declined      bool              // server sent a NAK
}

func (cfg *Config) decode(opts []option) error {
	*cfg = Config{}
	for _, opt := range opts {
		b := opt.body
		if !opt.code.lenValid(len(b)) {
			return fmt.Errorf("%s: bad length: %d", opt.code, len(b))
		}
		switch opt.code {
		case optLeaseTime:
			cfg.LeaseLength = Seconds(binary.BigEndian.Uint32(b))
		case optRenewalTime:
			cfg.RenewTime = Seconds(binary.BigEndian.Uint32(b))
		case optRebindingTime:
			cfg.RebindTime = Seconds(binary.BigEndian.Uint32(b))
		case optSubnetMask:
			cfg.SubnetMask = tcpip.MaskFromBytes(b)
		case optDHCPServer:
			cfg.ServerAddress = tcpip.AddrFromSlice(b)
		case optRouter:
			for len(b) != 0 {
				cfg.Router = append(cfg.Router, tcpip.AddrFromSlice(b[:4]))
				b = b[4:]
			}
		case optDomainNameServer:
			for len(b) != 0 {
				cfg.DNS = append(cfg.DNS, tcpip.AddrFromSlice(b[:4]))
				b = b[4:]
			}
		}
	}
	return nil
}

func (cfg Config) encode() (opts []option) {
	if !cfg.ServerAddress.Unspecified() {
		opts = append(opts, option{optDHCPServer, []byte(cfg.ServerAddress.AsSlice())})
	}
	if cfg.SubnetMask.Len() > 0 {
		opts = append(opts, option{optSubnetMask, []byte(cfg.SubnetMask.AsSlice())})
	}
	if len(cfg.Router) > 0 {
		router := make([]byte, 0, 4*len(cfg.Router))
		for _, addr := range cfg.Router {
			router = append(router, addr.AsSlice()...)
		}
		opts = append(opts, option{optRouter, router})
	}
	if len(cfg.DNS) > 0 {
		dns := make([]byte, 0, 4*len(cfg.DNS))
		for _, addr := range cfg.DNS {
			dns = append(dns, addr.AsSlice()...)
		}
		opts = append(opts, option{optDomainNameServer, dns})
	}
	if l := cfg.LeaseLength; l != 0 {
		opts = append(opts, serializeLeaseOption(l, optLeaseTime))
	}
	if r := cfg.RebindTime; r != 0 {
		opts = append(opts, serializeLeaseOption(r, optRebindingTime))
	}
	if r := cfg.RenewTime; r != 0 {
		opts = append(opts, serializeLeaseOption(r, optRenewalTime))
	}
	return opts
}

func serializeLeaseOption(s Seconds, o optionCode) option {
	v := make([]byte, 4)
	binary.BigEndian.PutUint32(v, uint32(s))
	return option{o, v}
}

const (
	// ServerPort is the well-known UDP port number for a DHCP server.
	ServerPort = 67
	// ClientPort is the well-known UDP port number for a DHCP client.
	ClientPort = 68

	dhcpMinimumSize = 241
)

var magicCookie = []byte{99, 130, 83, 99} // RFC 1497

type hdr []byte

// Per https://tools.ietf.org/html/rfc2131#section-2:
//
//	FIELD      OCTETS       DESCRIPTION
//	-----      ------       -----------
//
//	...
//
//	xid           4  Transaction ID, a random number chosen by the
//	.                client, used by the client and server to associate
//	.                messages and responses between a client and a
//	.                server.
type xid [4]byte

func (h hdr) init() {
	h[1] = 0x01       // htype
	h[2] = 0x06       // hlen
	h[3] = 0x00       // hops
	h[8], h[9] = 0, 0 // secs
	copy(h[236:240], magicCookie)
}

func (h hdr) isValid() bool {
	if len(h) < dhcpMinimumSize {
		return false
	}
	if o := h.op(); o != opRequest && o != opReply {
		return false
	}
	if !bytes.Equal(h[1:3], []byte{0x1, 0x6}) {
		return false
	}
	return bytes.Equal(h[236:240], magicCookie)
}

func (h hdr) op() op           { return op(h[0]) }
func (h hdr) setOp(o op)       { h[0] = byte(o) }
func (h hdr) xidbytes() []byte { return h[4:][:len(xid{})] }
func (h hdr) xid() uint32      { return binary.BigEndian.Uint32(h.xidbytes()) }
func (h hdr) setBroadcast()    { h[10] |= 1 << 7 }
func (h hdr) broadcast() bool  { return h[10]&1<<7 != 0 }
func (h hdr) ciaddr() []byte   { return h[12:16] }
func (h hdr) yiaddr() []byte   { return h[16:20] }
func (h hdr) siaddr() []byte   { return h[20:24] }
func (h hdr) giaddr() []byte   { return h[24:28] }
func (h hdr) chaddr() []byte   { return h[28:44] }
func (h hdr) sname() []byte    { return h[44:108] }
func (h hdr) file() []byte     { return h[108:236] }

func (h hdr) options() (options, error) {
	{
		// Silence a warning: assignment to method receiver doesn't propagate to
		// other calls.
		h := h
		if !h.isValid() {
			return nil, fmt.Errorf("invalid header: %s", h)
		}
		hStart := h
		var nextH hdr
		h = h[headerBaseSize:]
		var opts options
		var overload optionOverloadValue
		// To make sure overload is read only once, so another overload option
		// inside overloaded field gets ignored.
		overloadRead := false
		for {
			if len(h) < 1 {
				return nil, fmt.Errorf("missing end option; options=%s", opts)
			}
			code := optionCode(h[0])
			h = h[1:]
			switch code {
			case 0:
				// Padding.
				continue
			case optEnd:
				// End.
				if nextH != nil {
					h = nextH
					nextH = nil
					continue
				}
				switch overload {
				case optOverloadValueFile:
					h = hStart.file()
					overload = 0
					continue
				case optOverloadValueSname:
					h = hStart.sname()
					overload = 0
					continue
				case optOverloadValueBoth:
					h = hStart.file()
					nextH = hStart.sname()
					overload = 0
					continue
				}
				return opts, nil
			}

			if len(h) < 1 {
				return nil, fmt.Errorf("%s missing length; options=%s", code, opts)
			}
			length := h[0]
			h = h[1:]
			if len(h) < int(length) {
				return nil, fmt.Errorf("%s length=%d remaining=%d; options=%s", code, length, len(h), opts)
			}
			if code == optOverload && !overloadRead && length == 1 {
				overload = optionOverloadValue(h[0])
				overloadRead = true
			}
			opts = append(opts, option{
				code: code,
				body: h[:length],
			})
			h = h[length:]
		}
	}
}

func (h hdr) setOptions(opts []option) {
	i := headerBaseSize
	for _, opt := range opts {
		h[i] = byte(opt.code)
		h[i+1] = byte(len(opt.body))
		copy(h[i+2:i+2+len(opt.body)], opt.body)
		i += 2 + len(opt.body)
	}
	h[i] = 255 // End option
	i++
	for ; i < len(h); i++ {
		h[i] = 0
	}
}

func (h hdr) String() string {
	var buf strings.Builder
	if _, err := fmt.Fprintf(&buf, "len=%d", len(h)); err != nil {
		panic(err)
	}
	if !h.isValid() {
		return fmt.Sprintf("DHCP invalid; %s; %x", buf.String(), []byte(h))
	}
	opts, err := h.options()
	if err != nil {
		return fmt.Sprintf("DHCP options=%s; %s; %x", err, buf.String(), []byte(h))
	}
	buf.WriteString(";options=")
	for i, opt := range opts {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(opt.String())
	}
	msgType, err := opts.dhcpMsgType()
	if err != nil {
		return fmt.Sprintf("DHCP type=%s; %s; %x", err, buf.String(), []byte(h))
	}
	if _, err := fmt.Fprintf(
		&buf,
		"type=%s;ciaddr=%s;yiaddr=%s;siaddr=%s;giaddr=%s;chaddr=%x",
		msgType,
		tcpip.AddrFromSlice(h.ciaddr()),
		tcpip.AddrFromSlice(h.yiaddr()),
		tcpip.AddrFromSlice(h.siaddr()),
		tcpip.AddrFromSlice(h.giaddr()),
		h.chaddr(),
	); err != nil {
		panic(err)
	}
	return buf.String()
}

// headerBaseSize is the size of a DHCP packet, including the magic cookie.
//
// Note that a DHCP packet is required to have an 'end' option that takes
// up an extra byte, so the minimum DHCP packet size is headerBaseSize + 1.
const headerBaseSize = 240

type option struct {
	code optionCode
	body []byte
}

func (opt option) String() string {
	return fmt.Sprintf("%s: %x", opt.code, opt.body)
}

type optionCode byte

const (
	optSubnetMask optionCode = 1
	optTimeOffset optionCode = 2
	// RFC 2132 section 3.5:
	//   3.5. Router Option
	//
	//   The router option specifies a list of IP addresses for routers on the
	//   client's subnet.  Routers SHOULD be listed in order of preference.
	//
	//   The code for the router option is 3.  The minimum length for the
	//   router option is 4 octets, and the length MUST always be a multiple
	//   of 4.
	optRouter           optionCode = 3
	optTimeServer       optionCode = 4
	optDomainNameServer optionCode = 6
	optDomainName       optionCode = 15
	optReqIPAddr        optionCode = 50
	optLeaseTime        optionCode = 51
	optOverload         optionCode = 52
	optDHCPMsgType      optionCode = 53 // dhcpMsgType
	optDHCPServer       optionCode = 54
	optParamReq         optionCode = 55
	optMessage          optionCode = 56
	optRenewalTime      optionCode = 58
	optRebindingTime    optionCode = 59
	optClientID         optionCode = 61
	optEnd              optionCode = 255
)

type optionOverloadValue uint8

const (
	optOverloadValueFile  optionOverloadValue = 1
	optOverloadValueSname optionOverloadValue = 2
	optOverloadValueBoth  optionOverloadValue = 3
)

func (i optionCode) lenValid(l int) bool {
	switch i {
	case optSubnetMask,
		optReqIPAddr,
		optLeaseTime,
		optDHCPServer,
		optRenewalTime,
		optRebindingTime:
		return l == 4
	case optDHCPMsgType:
		return l == 1
	case optRouter, optDomainNameServer:
		return l%4 == 0
	case optMessage, optDomainName, optClientID:
		return l >= 1
	case optParamReq:
		return true // no fixed length
	default:
		return true // unknown option, assume ok
	}
}

type options []option

func (opts options) dhcpMsgType() (dhcpMsgType, error) {
	for _, opt := range opts {
		if opt.code == optDHCPMsgType {
			if len(opt.body) != 1 {
				return 0, fmt.Errorf("%s: bad length: %d", opt.code, len(opt.body))
			}
			v := opt.body[0]
			if v <= 0 || v >= 8 {
				return 0, fmt.Errorf("DHCP bad length: %d", len(opt.body))
			}
			return dhcpMsgType(v), nil
		}
	}
	return 0, nil
}

func (opts options) message() string {
	for _, opt := range opts {
		if opt.code == optMessage {
			return string(opt.body)
		}
	}
	return ""
}

func (opts options) len() int {
	l := 0
	for _, opt := range opts {
		l += 1 + 1 + len(opt.body) // code + len + body
	}
	return l + 1 // extra byte for 'pad' option
}

type op byte

const (
	opRequest op = 0x01
	opReply   op = 0x02
)

// dhcpMsgType is the DHCP Message Type from RFC 1533, section 9.4.
type dhcpMsgType byte

const (
	dhcpDISCOVER dhcpMsgType = 1
	dhcpOFFER    dhcpMsgType = 2
	dhcpREQUEST  dhcpMsgType = 3
	dhcpDECLINE  dhcpMsgType = 4
	dhcpACK      dhcpMsgType = 5
	dhcpNAK      dhcpMsgType = 6
	dhcpRELEASE  dhcpMsgType = 7
)

type dhcpClientState uint8

const (
	initSelecting dhcpClientState = iota
	bound
	renewing
	rebinding
)
