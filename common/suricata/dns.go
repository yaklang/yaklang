package suricata

type DNSRule struct {
	OpcodeNegative bool
	Opcode         int
	DNSQuery       bool
}
