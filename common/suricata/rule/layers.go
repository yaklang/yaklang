package rule

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
}

type DNSRule struct {
	OpcodeNegative bool
	Opcode         int
}

type HTTPConfig struct {
	// deprecated and not implemented
	Uricontent string

	// not set 0
	// equal 1
	// bigger than 2
	// smaller than 3
	// between 4
	UrilenOp   int
	UrilenNum1 int
	UrilenNum2 int
}

type TCPLayerRule struct {
	Seq            *int
	Ack            *int
	NegativeWindow bool
	Window         *int
	// not set 0
	// equal 1
	// bigger than 2
	// smaller than 3
	// between 4
	TCPMssOp   int
	TCPMssNum1 int
	TCPMssNum2 int
	Flags      string
}
