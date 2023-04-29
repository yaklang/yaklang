package suricata

type TCPLayerRule struct {
	Seq            int
	Ack            int
	NegativeWindow bool
	Window         int
	TCPMss         string
	TCPHeader      bool
	Flags          string
}

type ICMPLayerRule struct {
	IType        string // itype
	ICode        string // icode
	ICMPId       int    //  icmp_id
	ICMPSeq      int
	ICMPv4Header bool
	ICMPv6Header bool
	ICMPv6MTU    string
}

type UDPLayerRule struct {
	UDPHeader bool
}

type IPLayerRule struct {
	TTL int
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
	Id         int
	Geoip      string
	FragBits   string
	FragOffset string
	Tos        string

	// contains
	IPv4Header bool
	IPv6Header bool
}
