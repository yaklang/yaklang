package synscanx

type ProtocolType int

const (
	TCP ProtocolType = iota
	UDP
	ICMP
	ARP
)

type SynxTarget struct {
	Host string
	Port int
	Mode ProtocolType
}
