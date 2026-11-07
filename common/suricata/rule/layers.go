package rule

import "github.com/yaklang/yaklang/common/suricata/data/numrange"

type ICMPLayerRule struct {
	IType     *numrange.NumRange // itype
	ICode     *numrange.NumRange // icode
	ICMPId    *int               // icmp_id
	ICMPSeq   *int
	ICMPv6MTU *numrange.NumRange
}

type IPLayerRule struct {
	TTL *numrange.NumRange
	/*
		IP Option	Description
		rr	Record Route
		eol	End of List
		nop	No Op
		ts	Time Stamp
		sec	IP Security
		esec	IP Extended Security
		lsrr	Loose Source Routing
		ssrr	Strict Source Routing
		satid	Stream Identifier
		any	any IP options are set
	*/
	IPOpts     string
	Sameip     bool
	IPProto    string
	Id         *numrange.NumRange
	Geoip      string
	FragBits   string
	FragOffset *numrange.NumRange
	Tos        *numrange.NumRange
}

type DNSRule struct {
	OpcodeNegative bool
	OpcodeSet      bool
	Opcode         int
	RCode          *numrange.NumRange
	RRType         *numrange.NumRange
}

type HTTPConfig struct {
	// deprecated and not implemented
	Uricontent string
	Urilen     *numrange.NumRange
}

type TCPLayerRule struct {
	Seq            *int
	Ack            *int
	NegativeWindow bool
	Window         *int
	TCPMss         *numrange.NumRange
	Flags          string
}
