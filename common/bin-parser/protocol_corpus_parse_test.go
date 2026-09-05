package bin_parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type protocolCorpusParseContract struct {
	RuleFile      string
	EntryNode     string
	Layer         string
	OuterOnly     bool
	FrameOffset   int
	InputLength   int
	TrimPrefix    int
	TrimSuffix    []byte
	StartMagic    []byte
	StartAfter    []byte
	ReassembleTCP bool
	UseFullFrame  bool
}

type protocolCorpusCaptureParseSpec struct {
	Name            string
	Contract        protocolCorpusParseContract
	RequiredNodes   []string
	AlternateReason string
}

type protocolCorpusLayerParseSpec struct {
	CaptureID     string
	Name          string
	Contract      protocolCorpusParseContract
	RequiredNodes []string
}

type protocolCorpusClassifierSpec struct {
	Layer        string
	Prefix       []byte
	Contains     []byte
	MinimumBytes int
	MaximumBytes int
	ExactBytes   int
	AtOffset     map[int][]byte
}

type protocolCorpusRejectionSpec struct {
	Name                  string
	Contract              protocolCorpusParseContract
	ControlCaptureID      string
	ControlContract       protocolCorpusParseContract
	ExpectedFailureClass  protocolCorpusFailureClass
	ExpectedErrorContains string
}

type protocolCorpusFailureClass string

const (
	protocolCorpusFailureInvalidValue protocolCorpusFailureClass = "invalid-value"
	protocolCorpusFailureLengthBounds protocolCorpusFailureClass = "length-bounds"
	protocolCorpusFailureTruncated    protocolCorpusFailureClass = "truncated"
)

type protocolCorpusBoundedReader struct {
	reader *bytes.Reader
}

func newProtocolCorpusBoundedReader(input []byte) *protocolCorpusBoundedReader {
	return &protocolCorpusBoundedReader{reader: bytes.NewReader(input)}
}

func (r *protocolCorpusBoundedReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *protocolCorpusBoundedReader) Len() int {
	return r.reader.Len()
}

func (r *protocolCorpusBoundedReader) InputBitLength() uint64 {
	return uint64(r.reader.Len()) * 8
}

var protocolCorpusParseContracts = map[string]protocolCorpusParseContract{
	"ARP":                {FrameOffset: 10},
	"AFP":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "AFP", Layer: "L7"},
	"ActiveMQ OpenWire":  {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "ActiveMQOpenWire", Layer: "L7"},
	"BGP":                {FrameOffset: 48},
	"BFD":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "BFD", Layer: "L7"},
	"BACnet":             {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "BACnetIP", Layer: "L7"},
	"Beckhoff ADS":       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "BeckhoffADS", Layer: "L7"},
	"CAN/ISO-TP":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CANEthernet", Layer: "L7"},
	"Cassandra CQL":      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CassandraCQL", Layer: "L7"},
	"Ceph":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CephConnect", Layer: "L7", ReassembleTCP: true},
	"Citrix ICA":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CitrixICA", Layer: "L7"},
	"Collectd":           {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "Collectd", Layer: "L7"},
	"DB2 DRDA":           {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "DRDA", Layer: "L7"},
	"DLMS/COSEM":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "DLMSHDLC", Layer: "L7"},
	"DNP3":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "DNP3", Layer: "L7"},
	"Diameter":           {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "Diameter", Layer: "L7"},
	"EtherNet/IP CIP":    {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "EtherNetIPCIPIO", Layer: "L7"},
	"FIX":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "FIX", Layer: "L7"},
	"GTP Prime":          {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GTPPrime", Layer: "L7"},
	"GTP-C":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GTPv2", Layer: "L7"},
	"GTP-U":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GTPv1", Layer: "L7"},
	"GLBP":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GLBP", Layer: "L7"},
	"Git daemon":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GitDaemon", Layer: "L7"},
	"Gnutella":           {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "Gnutella", Layer: "L7"},
	"H.323":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "H323", Layer: "L7"},
	"Hart-IP":            {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "HartIP", Layer: "L7"},
	"IEC 60870-5-104":    {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IEC104", Layer: "L7"},
	"IEC 61850 MMS":      {RuleFile: "application-layer/mms.yaml", EntryNode: "MMSPDU", Layer: "L7", StartMagic: []byte{0xa8, 0x26}, InputLength: 40},
	"IPP":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IPP", Layer: "L7"},
	"IRC":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IRC", Layer: "L7"},
	"KNX/IP":             {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "KNXIP", Layer: "L7"},
	"LDP":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "LDP", Layer: "L7"},
	"MELSEC":             {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "MELSEC", Layer: "L7"},
	"MGCP":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "MGCP", Layer: "L7"},
	"MPEG-TS":            {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "MPEGTransportStream", Layer: "L7"},
	"Modbus TCP":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "ModbusTCP", Layer: "L7"},
	"NATS":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "NATS", Layer: "L7"},
	"NetFlow v9":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "NetFlowV9", Layer: "L7"},
	"NNTP":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "NNTP", Layer: "L7"},
	"OPC UA":             {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "OPCUAHello", Layer: "L7"},
	"Omron FINS":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "OmronFINS", Layer: "L7"},
	"PFCP":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "PFCP", Layer: "L7"},
	"PIM":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "PIM", Layer: "L3"},
	"Profinet IO":        {RuleFile: "application-layer/profinet_io.yaml", EntryNode: "ProfinetIOReadImplicit", Layer: "L7"},
	"RSH":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "RSH", Layer: "L7"},
	"SCCP/Skinny":        {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "Skinny", Layer: "L7"},
	"S7comm":             {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "S7comm", Layer: "L7"},
	"S7comm-plus":        {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "S7commPlus", Layer: "L7"},
	"SMPP":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SMPPBind", Layer: "L7"},
	"SRVLOC/SLP":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SLPv2", Layer: "L7"},
	"STOMP":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "STOMP", Layer: "L7"},
	"Teredo":             {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "TeredoAuthentication", Layer: "L7"},
	"T.38":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SDPT38Advertisement", Layer: "L7", OuterOnly: true, StartMagic: []byte("v=0\r\n")},
	"UPnP":               {RuleFile: "application-layer/upnp.yaml", EntryNode: "UPnPSSDPNotify", Layer: "L7"},
	"XDMCP":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "XDMCP", Layer: "L7"},
	"sFlow":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SFlowV5", Layer: "L7"},
	"FTP":                {EntryNode: "FTP"},
	"HTTP/2":             {EntryNode: "HTTP2Connection"},
	"JSON-RPC":           {StartMagic: []byte("{")},
	"Kerberos":           {EntryNode: "Kerberos"},
	"NetBIOS":            {RuleFile: "application-layer/nbns.yaml", EntryNode: "NBNS"},
	"PostgreSQL":         {EntryNode: "Startup"},
	"Protobuf":           {TrimSuffix: []byte{'\n'}},
	"RTP":                {EntryNode: "RTP"},
	"RTMP":               {ReassembleTCP: true},
	"SMB":                {EntryNode: "SMB", TrimPrefix: 4},
	"SMTP":               {EntryNode: "SMTP"},
	"SNMP":               {EntryNode: "SNMP"},
	"SOCKS5":             {EntryNode: "Request"},
	"SSH":                {EntryNode: "SSH"},
	"STUN":               {TrimPrefix: 2},
	"TeamViewer":         {RuleFile: "application-layer/teamviewer.yaml", EntryNode: "TeamViewerRecord", Layer: "L7"},
	"VNC/RFB":            {EntryNode: "VNC"},
	"VXLAN":              {TrimPrefix: 8},
	"6in4":               {RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3", FrameOffset: 14},
	"AnyDesk":            {RuleFile: "application-layer/tls.yaml", Layer: "L7", OuterOnly: true},
	"DingTalk":           {RuleFile: "application-layer/tls.yaml", Layer: "L7", OuterOnly: true, ReassembleTCP: true},
	"DoH":                {RuleFile: "application-layer/tls.yaml", Layer: "L7", OuterOnly: true},
	"DoQ":                {RuleFile: "application-layer/quic.yaml", EntryNode: "QUIC", Layer: "L7", OuterOnly: true},
	"DoT":                {RuleFile: "application-layer/tls.yaml", Layer: "L7", OuterOnly: true},
	"Elasticsearch":      {RuleFile: "application-layer/elasticsearch.yaml", EntryNode: "Elasticsearch", Layer: "L7"},
	"FTPS":               {RuleFile: "application-layer/ftp.yaml", EntryNode: "FTPAuthTLS", Layer: "L7"},
	"HTTP Proxy CONNECT": {RuleFile: "application-layer/http.yaml", EntryNode: "HTTP", Layer: "L7"},
	"HTTP/3":             {RuleFile: "application-layer/quic.yaml", EntryNode: "QUIC", Layer: "L7", OuterOnly: true},
	"IKEv1":              {RuleFile: "ike.yaml", EntryNode: "IKE", Layer: "L4"},
	"IMAPS":              {RuleFile: "application-layer/tls.yaml", Layer: "L7", OuterOnly: true},
	"IIOP/GIOP":          {RuleFile: "application-layer/iiop.yaml", EntryNode: "GIOP", Layer: "L7"},
	"IPsec ESP":          {RuleFile: "ipsec.yaml", EntryNode: "ESP", Layer: "L3"},
	"mDNS":               {RuleFile: "application-layer/dns.yaml", EntryNode: "DNS", Layer: "L7"},
	"NFS":                {RuleFile: "onc_rpc.yaml", EntryNode: "ONCRPC", Layer: "L7"},
	"OCSP":               {RuleFile: "application-layer/ocsp.yaml", EntryNode: "OCSPRequest", Layer: "L7", ReassembleTCP: true, StartAfter: []byte("\r\n\r\n"), InputLength: 84},
	"PTP":                {RuleFile: "ptp.yaml", EntryNode: "PTP", Layer: "L7"},
	"RTCP":               {RuleFile: "rtp.yaml", EntryNode: "RTCP", Layer: "L7"},
	"RDP":                {RuleFile: "application-layer/msrdp.yaml", EntryNode: "RDPConnectionRequest", Layer: "L7"},
	"SMTPS":              {RuleFile: "application-layer/tls.yaml", Layer: "L7", OuterOnly: true},
	"SOAP":               {RuleFile: "application-layer/http.yaml", EntryNode: "HTTP", Layer: "L7"},
	"SSDP":               {RuleFile: "application-layer/upnp.yaml", EntryNode: "UPnPSSDPNotify", Layer: "L7"},
	"WebDAV":             {RuleFile: "application-layer/http.yaml", EntryNode: "HTTP", Layer: "L7"},
	"WeChat/MicroMsg":    {RuleFile: "application-layer/tls.yaml", Layer: "L7", OuterOnly: true},
}

// Capture-specific entries cover source material that intentionally sits
// outside ProtocolRoadmap, plus capture-specific samples that need an explicit
// extraction contract. Keeping these keyed by capture ID makes additions fail
// closed instead of silently inheriting a similarly named protocol contract.
var protocolCorpusCaptureParseSpecs = map[string]protocolCorpusCaptureParseSpec{
	"tcpdump-radiotap": {
		Name:          "IEEE 802.11 Radiotap",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "RadioTapHeader", Layer: "L2", UseFullFrame: true},
		RequiredNodes: []string{"Version", "Header Length", "Present Flags", "Fields", "Frame Data"},
	},
	"ndpi-dlep": {
		Name:            "DLEP",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "DLEPSignal", Layer: "L7"},
		RequiredNodes:   []string{"Magic", "Signal Type", "Signal Length", "Data Items", "Type", "Length", "Value"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and directly exercises the DLEP record",
	},
	"ndpi-someip-sd": {
		Name:            "SOME/IP SD",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SOMEIP", Layer: "L7", FrameOffset: 58},
		RequiredNodes:   []string{"Message ID", "Length", "Request ID", "Protocol Version", "Interface Version", "Message Type", "Return Code", "Payload"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and targets its service-discovery record",
	},
	"ndpi-nat-pmp": {
		Name:            "NAT-PMP",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "NATPMPPublicAddressRequest", Layer: "L7"},
		RequiredNodes:   []string{"Version", "Opcode"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and directly exercises the NAT-PMP request",
	},
	"ndpi-someip-method": {
		Name:            "SOME/IP Method",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SOMEIP", Layer: "L7"},
		RequiredNodes:   []string{"Message ID", "Length", "Request ID", "Protocol Version", "Interface Version", "Message Type", "Return Code", "Payload"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and targets its method record",
	},
	"ndpi-c1222": {
		Name:            "ANSI C12.22",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "C1222Message", Layer: "L7"},
		RequiredNodes:   []string{"Type", "Length", "Children", "OBJECT IDENTIFIER"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and directly exercises the C12.22 record",
	},
	"ndpi-coap": {
		Name:            "CoAP",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CoAP", Layer: "L7"},
		RequiredNodes:   []string{"Version", "Type", "Token Length", "Code", "Message ID", "Token", "Options and Payload"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and directly exercises the CoAP record",
	},
	"ndpi-tcp-measurement": {
		Name:            "TCP",
		Contract:        protocolCorpusParseContract{RuleFile: "transmission_control_protocol.yaml", EntryNode: "TCP", Layer: "L4"},
		RequiredNodes:   []string{"Source Port", "Destination Port", "Sequence Number", "Header Length", "Flags", "Window", "Checksum"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and explicitly validates its TCP envelope",
	},
	"wireshark-http": {
		Name:          "HTTP",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/http.yaml", EntryNode: "HTTP", Layer: "L7"},
		RequiredNodes: []string{"HTTP Request", "FirstLine", "Method", "Path", "Version", "Headers", "Item"},
	},
	"wireshark-usb-hid": {
		Name:            "USB HID report sample",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "USBHIDReport", Layer: "L7", OuterOnly: true, FrameOffset: 64, InputLength: 4},
		RequiredNodes:   []string{"Report ID", "Report Data"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and validates a complete descriptor-dependent input report as an opaque envelope",
	},
	"wireshark-ipx-rip": {
		Name:          "IPX",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IPX", Layer: "L3", FrameOffset: 14, InputLength: 40},
		RequiredNodes: []string{"Checksum", "Length", "Transport Control", "Packet Type", "Destination Network", "Destination Node", "Destination Socket", "Source Network", "Source Node", "Source Socket", "Payload"},
	},
}

// A representative capture can prove more than the application name attached
// to it. These contracts independently feed the exact bytes of a nested or
// enclosing layer to that layer's rule, so IPv4/IPv6/TCP/UDP/QUIC and BOOTP
// coverage is executable instead of being inferred from frame metadata.
var protocolCorpusLayerParseSpecs = []protocolCorpusLayerParseSpec{
	{
		CaptureID:     "wireshark-dhcp",
		Name:          "BOOTP",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/bootp.yaml", EntryNode: "BOOTP", Layer: "L7"},
		RequiredNodes: []string{"Operation", "Hardware Type", "Hardware Length", "Xid", "Client MAC", "Server Host Name", "Boot File", "Vendor Area"},
	},
	{
		CaptureID:     "wireshark-icmp-ascii",
		Name:          "IPv4",
		Contract:      protocolCorpusParseContract{RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3"},
		RequiredNodes: []string{"Version", "Header Length", "Total Length", "Protocol", "Source", "Destination", "Payload"},
	},
	{
		CaptureID:     "ndpi-6in4",
		Name:          "IPv6",
		Contract:      protocolCorpusParseContract{RuleFile: "internet_protocol_version_6.yaml", EntryNode: "Internet Protocol Version 6", Layer: "L3", FrameOffset: 34},
		RequiredNodes: []string{"Version", "Payload Length", "Next Header", "Source", "Destination", "Payload"},
	},
	{
		CaptureID:     "wireshark-http3-qpack",
		Name:          "QUIC",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/quic.yaml", EntryNode: "QUIC", Layer: "L7"},
		RequiredNodes: []string{"First Byte", "Version", "DCID Length", "DCID", "SCID Length", "Protected Payload"},
	},
	{
		CaptureID:     "ndpi-dns",
		Name:          "UDP",
		Contract:      protocolCorpusParseContract{RuleFile: "user_datagram_protocol.yaml", EntryNode: "UDP", Layer: "L4"},
		RequiredNodes: []string{"Source Port", "Destination Port", "Length", "Checksum", "Payload"},
	},
}

// These three captures are useful classification fixtures, but their product
// payload formats are not public protocol specifications. They are therefore
// checked as opaque transport payloads with stable sample fingerprints and are
// never promoted to semantic parser coverage.
var protocolCorpusClassifierSpecs = map[string]protocolCorpusClassifierSpec{
	// These three proprietary formats do not publish field grammars. The pinned
	// nDPI detector also identifies them only through the invariants below, so
	// the corpus deliberately records classification evidence instead of
	// presenting opaque bytes as semantic parsing.
	"ndpi-iqiyi": {
		Layer: "L7", Contains: []byte("PPStream"), MinimumBytes: 121, MaximumBytes: 299,
	},
	"ndpi-tencent-games": {
		Layer: "L7", Prefix: []byte{0x33, 0x66, 0x00, 0x0b}, MinimumBytes: 51,
		AtOffset: map[int][]byte{4: {0x00, 0x0b}},
	},
	"ndpi-netease-games": {
		Layer: "L7", Prefix: []byte{0x01}, ExactBytes: 12,
		AtOffset: map[int][]byte{2: {0xd0, 0x01}, 8: {0x00, 0x01, 0x01, 0x01}},
	},
}

// Upstream-negative captures must be rejected by the rule at the malformed
// layer. A new negative fixture has no default: it needs an explicit rule and
// extraction contract here before the corpus matrix accepts it.
var protocolCorpusRejectionSpecs = map[string]protocolCorpusRejectionSpec{
	"ndpi-dhcp-boundary": {
		Name:                  "DHCP",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/dhcp.yaml", EntryNode: "DHCP", Layer: "L7"},
		ControlCaptureID:      "wireshark-dhcp",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/dhcp.yaml", EntryNode: "DHCP", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "node type string,length 392 over max size 296",
	},
	"ndpi-radius-classification-boundary": {
		Name:                  "RADIUS",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/radius.yaml", EntryNode: "RADIUS", Layer: "L7"},
		ControlCaptureID:      "tcpdump-radius",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/radius.yaml", EntryNode: "RADIUS", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureInvalidValue,
		ExpectedErrorContains: "radius: unknown code",
	},
	"ndpi-quic-length-boundary": {
		Name:                  "QUIC",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/quic.yaml", EntryNode: "QUIC", Layer: "L4", FrameOffset: 20},
		ControlCaptureID:      "wireshark-http3-qpack",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/quic.yaml", EntryNode: "QUIC", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureInvalidValue,
		ExpectedErrorContains: "quic: short header missing fixed bit",
	},
	"tcpdump-ipv4-invalid-length": {
		Name:                  "IPv4",
		Contract:              protocolCorpusParseContract{RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3", FrameOffset: 14},
		ControlCaptureID:      "wireshark-icmp-ascii",
		ControlContract:       protocolCorpusParseContract{RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3"},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "length 672 over max size 152",
	},
	"tcpdump-ipv6-short-header": {
		Name:                  "IPv6",
		Contract:              protocolCorpusParseContract{RuleFile: "internet_protocol_version_6.yaml", EntryNode: "Internet Protocol Version 6", Layer: "L3", FrameOffset: 14},
		ControlCaptureID:      "ndpi-6in4",
		ControlContract:       protocolCorpusParseContract{RuleFile: "internet_protocol_version_6.yaml", EntryNode: "Internet Protocol Version 6", Layer: "L3", FrameOffset: 34},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "node type raw,length 128 over max size 8",
	},
	"tcpdump-esp-truncated": {
		Name:                  "ESP",
		Contract:              protocolCorpusParseContract{RuleFile: "ipsec.yaml", EntryNode: "ESP", Layer: "L3", FrameOffset: 42},
		ControlCaptureID:      "ndpi-ipsec-esp",
		ControlContract:       protocolCorpusParseContract{RuleFile: "ipsec.yaml", EntryNode: "ESP", Layer: "L3"},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "node type uint32,length 32 over max size 0",
	},
	"tcpdump-lldp-tlv-boundary": {
		Name:                  "LLDP",
		Contract:              protocolCorpusParseContract{RuleFile: "lldp.yaml", EntryNode: "LLDP", Layer: "L2", FrameOffset: 14},
		ControlCaptureID:      "tcpdump-lldp",
		ControlContract:       protocolCorpusParseContract{RuleFile: "lldp.yaml", EntryNode: "LLDP", Layer: "L2"},
		ExpectedFailureClass:  protocolCorpusFailureInvalidValue,
		ExpectedErrorContains: "lldp: second TLV must be port ID",
	},
	"tcpdump-snmp-length-boundary": {
		Name:                  "SNMP",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/snmp.yaml", EntryNode: "SNMP", Layer: "L7", FrameOffset: 42},
		ControlCaptureID:      "ndpi-snmp",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/snmp.yaml", EntryNode: "SNMP", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureInvalidValue,
		ExpectedErrorContains: "snmp: expected INTEGER version",
	},
	"tcpdump-bootp-packet-boundary": {
		Name:                  "BOOTP",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/bootp.yaml", EntryNode: "BOOTP", Layer: "L7", FrameOffset: 42},
		ControlCaptureID:      "wireshark-dhcp",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/bootp.yaml", EntryNode: "BOOTP", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureInvalidValue,
		ExpectedErrorContains: "bootp: invalid operation",
	},
	"tcpdump-pim-header-boundary": {
		Name:                  "PIM",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "PIM", Layer: "L3", FrameOffset: 34},
		ControlCaptureID:      "ndpi-pim",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "PIM", Layer: "L3"},
		ExpectedFailureClass:  protocolCorpusFailureTruncated,
		ExpectedErrorContains: "pim: truncated register message",
	},
	"tcpdump-tftp-packet-boundary": {
		Name:                  "TFTP",
		Contract:              protocolCorpusParseContract{RuleFile: "tftp.yaml", EntryNode: "TFTP", Layer: "L7", FrameOffset: 44},
		ControlCaptureID:      "ndpi-tftp",
		ControlContract:       protocolCorpusParseContract{RuleFile: "tftp.yaml", EntryNode: "TFTP", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureTruncated,
		ExpectedErrorContains: "parse node Filename error:",
	},
	"tcpdump-radiotap-header-boundary": {
		Name:                  "RadioTap",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "RadioTapHeader", Layer: "L2", UseFullFrame: true},
		ControlCaptureID:      "tcpdump-radiotap",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "RadioTapHeader", Layer: "L2", UseFullFrame: true},
		ExpectedFailureClass:  protocolCorpusFailureInvalidValue,
		ExpectedErrorContains: "radiotap: version must be zero",
	},
	"tcpdump-tcp-header-boundary": {
		Name:                  "TCP",
		Contract:              protocolCorpusParseContract{RuleFile: "transmission_control_protocol.yaml", EntryNode: "TCP", Layer: "L4", FrameOffset: 34},
		ControlCaptureID:      "ndpi-tcp-measurement",
		ControlContract:       protocolCorpusParseContract{RuleFile: "transmission_control_protocol.yaml", EntryNode: "TCP", Layer: "L4"},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "node type uint8,length 4 over max size 0",
	},
	"tcpdump-udp-length-boundary": {
		Name:                  "UDP",
		Contract:              protocolCorpusParseContract{RuleFile: "user_datagram_protocol.yaml", EntryNode: "UDP", Layer: "L4", FrameOffset: 34},
		ControlCaptureID:      "ndpi-dns",
		ControlContract:       protocolCorpusParseContract{RuleFile: "user_datagram_protocol.yaml", EntryNode: "UDP", Layer: "L4"},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "node type uint16,length 16 over max size 0",
	},
}

var protocolCorpusStructuralSpecs = map[string]string{
	"tcpdump-empty-pcapng":         "empty capture",
	"tcpdump-unsupported-linktype": "unsupported link type",
}

// These nodes are protocol-specific semantic checkpoints, not merely proof
// that the parser returned some tree. Every directly parsed protocol name must
// declare at least one, so a newly added name cannot silently inherit the
// generic non-empty-result assertion.
var protocolCorpusRequiredNodes = map[string][]string{
	"6in4":               {"Total Length", "Protocol", "IPv6", "Payload Length", "Next Header", "ICMPv6", "Type", "Identifier", "Sequence Number", "Echo Data"},
	"AFP":                {"DSI Command", "Request ID", "Function"},
	"AJP":                {"Magic", "Code"},
	"AMQP":               {"Type", "Frame End"},
	"ARP":                {"Hardware type", "Opcode"},
	"ActiveMQ OpenWire":  {"Frame Length", "Data Type", "Magic", "Version"},
	"AnyDesk":            {"ContentType", "Version", "Length"},
	"BGP":                {"Marker", "Type"},
	"BFD":                {"Version Diagnostic", "State Flags", "My Discriminator"},
	"BACnet":             {"BVLC Type", "BVLC Function", "NPDU Version", "NPDU Control"},
	"Beckhoff ADS":       {"Target Net ID", "Command ID", "Invoke ID"},
	"BitTorrent":         {"Pstr", "Info Hash"},
	"CAN/ISO-TP":         {"Magic", "Version", "Frame Count", "Options"},
	"Cassandra CQL":      {"Version", "Stream", "Opcode", "Body Length"},
	"Ceph":               {"Protocol Banner", "Server Identity", "Client Identity"},
	"Citrix ICA":         {"Magic", "Terminator"},
	"Collectd":           {"Parts", "Part", "Type", "Length", "Value"},
	"DCE/RPC":            {"RPC Vers", "Packet Type", "Object ID", "Operation Number", "Stub"},
	"DB2 DRDA":           {"Messages", "Magic", "Correlation ID", "Code Point"},
	"DHCP":               {"Operation", "Hardware Type", "Hardware Length", "Xid", "Client MAC", "Magic Cookie", "Options", "Code", "Message Type"},
	"DNS":                {"ID", "Questions"},
	"DLMS/COSEM":         {"Opening Flag", "Frame Format and Length", "Control", "Closing Flag"},
	"DNP3":               {"Start", "Control", "Destination", "Source"},
	"DTLS":               {"Content Type", "Fragment"},
	"DingTalk":           {"ContentType", "TLSClientHello"},
	"Diameter":           {"Version", "Command Code", "Application ID", "Hop-by-Hop ID"},
	"DoH":                {"ContentType", "Length"},
	"DoQ":                {"First Byte", "Version"},
	"DoT":                {"ContentType", "Length"},
	"Elasticsearch":      {"Magic", "Request ID", "Action"},
	"EtherNet/IP CIP":    {"Item Count", "Connection ID", "Encapsulation Sequence", "Sequence Count"},
	"FTP":                {"Code", "Message"},
	"FTPS":               {"Command", "Mechanism"},
	"FastCGI":            {"Version", "Type", "Request ID"},
	"FIX":                {"Begin String", "Message Type", "Checksum"},
	"GRE":                {"Flags And Version", "Protocol Type"},
	"GTP Prime":          {"Flags", "Message Type", "Sequence Number"},
	"GTP-C":              {"Flags", "Message Type", "TEID", "Sequence Number"},
	"GTP-U":              {"Flags", "Message Type", "TEID"},
	"GLBP":               {"Version", "Group", "Owner ID", "TLV Type", "TLV Length"},
	"Git daemon":         {"Packet Length", "Service Path", "Host"},
	"Gnutella":           {"Start Line", "Headers"},
	"H.323":              {"TPKT Version", "Packet Length", "Q931 Protocol Discriminator"},
	"HSRP":               {"TLVs", "Virtual IP"},
	"Hart-IP":            {"Version", "Message Type", "Sequence Number", "Byte Count"},
	"HTTP":               {"HTTP Request", "Headers"},
	"HTTP Proxy CONNECT": {"HTTP Request", "Method", "Path"},
	"HTTP/2":             {"Frames", "Type"},
	"HTTP/3":             {"First Byte", "Version"},
	"ICMP":               {"Type", "Checksum"},
	"IIOP/GIOP":          {"Magic", "Message Type", "Message Size", "Request ID", "Operation", "Service Context Count", "Context ID", "Context Data", "Stub Data"},
	"IKEv1":              {"Initiator SPI", "Exchange Type", "Payloads"},
	"IKEv2":              {"Initiator SPI", "Exchange Type", "Payloads"},
	"IMAP":               {"Tag", "Command"},
	"IMAPS":              {"ContentType", "Length"},
	"IEC 60870-5-104":    {"Start", "APDU Length", "Control"},
	"IEC 61850 MMS":      {"PDU Tag", "PDU Length", "Local Detail Calling", "Proposed Calling Limit", "Proposed Called Limit", "Proposed Nesting Level", "Proposed Version", "Parameter CBB Bits", "Services Supported Bits"},
	"IPMI":               {"Version", "Class", "Session"},
	"IPP":                {"Version Major", "Operation ID", "Request ID", "Attribute Groups"},
	"IPsec AH":           {"SPI", "Sequence", "ICV"},
	"IPsec ESP":          {"SPI", "Sequence", "Ciphertext"},
	"JSON-RPC":           {"Brace", "Pairs"},
	"Kafka":              {"API Key", "Correlation ID", "Body"},
	"Kerberos":           {"Application Tag", "Seq Tag", "Seq Length"},
	"IRC":                {"Command", "Parameters"},
	"KNX/IP":             {"Header Length", "Service Type", "Total Length"},
	"LDP":                {"Version", "PDU Length", "LSR ID", "Messages"},
	"LLDP":               {"TLVs", "TypeLen", "Chassis ID", "Port ID", "TTL"},
	"MELSEC":             {"Subheader", "Network Number", "PC Number", "IO Number"},
	"MGCP":               {"Request Line", "Headers"},
	"MPEG-TS":            {"Packets", "Sync Byte", "PID High", "Flags and Continuity"},
	"MQTT":               {"Packet Type", "RL0"},
	"MSSQL TDS":          {"Type", "Length", "PacketID"},
	"Memcached":          {"Text Line"},
	"MongoDB":            {"Message Length", "Op Code"},
	"Modbus TCP":         {"Transaction ID", "Protocol ID", "Function Code"},
	"MySQL":              {"Payload Length", "Sequence ID"},
	"NATS":               {"Operation", "Arguments"},
	"NFS":                {"XID", "Message Type", "Program"},
	"NNTP":               {"Status Code", "Message"},
	"NTP":                {"Version", "Mode"},
	"NetBIOS":            {"ID", "Questions"},
	"NetFlow v9":         {"Version", "Count", "Sequence Number", "FlowSets", "FlowSet ID"},
	"OCSP":               {"Request Tag", "TBS Request", "Request List", "Certificate ID", "Algorithm OID", "Issuer Name Hash", "Issuer Key Hash", "Serial Number"},
	"OPC UA":             {"Message Type", "Message Size", "Endpoint URL"},
	"OSPF":               {"Version", "Type", "LSAs"},
	"OpenVPN":            {"Opcode", "Key ID"},
	"Omron FINS":         {"ICF", "Service ID", "Main Request Code", "Sub Request Code"},
	"Oracle TNS":         {"Packet Length", "Packet Type"},
	"POP3":               {"Status", "Arg", "LF"},
	"PPP":                {"Address", "Protocol"},
	"PPPoE Discovery":    {"VersionType", "Code", "Payload"},
	"PPTP":               {"Length", "ControlMessageType"},
	"PFCP":               {"Flags", "Message Type", "Sequence Number", "Information Elements"},
	"PIM":                {"Version Type", "Checksum", "Message Data"},
	"PTP":                {"Message Type", "Clock Identity", "Sequence ID"},
	"PostgreSQL":         {"Length", "Protocol"},
	"Protobuf":           {"Fields", "Tag"},
	"Profinet IO":        {"RPC Version", "Packet Type", "Interface ID", "Operation Number", "Fragment Length", "Block Type", "Block Length", "Sequence Number", "API", "Slot Number", "Subslot Number", "Index", "Record Data Length"},
	"RADIUS":             {"Code", "Identifier", "Length", "Authenticator", "Attributes", "Type"},
	"RMI/JRMP":           {"Magic", "Version"},
	"RSH":                {"Secondary Port"},
	"RTCP":               {"Packet Type", "SSRC"},
	"RDP":                {"PacketLength", "TPDUCode", "RequestedProtocols"},
	"RTMP":               {"Version", "Time", "Zero", "Random"},
	"RTP":                {"VersionPXPCC", "Sequence", "SSRC"},
	"RTSP":               {"RTSP Request", "Headers"},
	"Rsync daemon":       {"Magic", "Major", "Minor"},
	"SCTP":               {"Source Port", "Destination Port", "Chunks"},
	"SCCP/Skinny":        {"Data Length", "Header Version", "Message ID"},
	"S7comm":             {"TPKT Version", "Protocol ID", "ROSCTR", "Parameter Length"},
	"S7comm-plus":        {"TPKT Version", "Protocol ID", "Protocol Version", "Message Data"},
	"SIP":                {"SIP Request", "Headers"},
	"SMB":                {"ProtocolId", "Command"},
	"SMTP":               {"Code", "Message"},
	"SMPP":               {"Command Length", "Command ID", "System ID", "Interface Version"},
	"SMTPS":              {"ContentType", "Length"},
	"SNMP":               {"Version", "Community", "PDU Tag"},
	"SOAP":               {"HTTP Response", "Status", "Headers"},
	"SOCKS5":             {"Version", "Command", "AddressType"},
	"SSDP":               {"Method", "Target", "Version", "Headers", "Line", "Message End"},
	"SRVLOC/SLP":         {"Version", "Function ID", "XID", "Language Tag"},
	"STOMP":              {"Command", "Headers", "Terminator"},
	"SSH":                {"Identification"},
	"STUN":               {"Message Type", "Magic Cookie", "Transaction ID"},
	"Syslog":             {"PRI", "Message"},
	"TFTP":               {"Opcode", "Filename", "Mode"},
	"Teredo":             {"Indicator Type", "Nonce", "Confirmation", "Next Header", "Source", "Destination"},
	"T.38":               {"Session Fragment"},
	"TLS":                {"ContentType", "Version", "Length"},
	"Telnet":             {"Items", "IACByte", "Command", "Option"},
	"TeamViewer":         {"Magic", "Command", "Body Length 16", "Opaque Body"},
	"Thrift":             {"Version", "Name", "Seq ID"},
	"VNC/RFB":            {"Magic", "Major", "Minor"},
	"VRRP":               {"VersionType", "VRID", "Priority"},
	"VXLAN":              {"I", "VNI", "Inner"},
	"UPnP":               {"Method", "Target", "Version", "Headers", "Line", "Message End"},
	"WeChat/MicroMsg":    {"ContentType", "TLSClientHello"},
	"WebDAV":             {"HTTP Request", "Method", "Headers"},
	"WebSocket":          {"Opcode", "Payload Len"},
	"WireGuard":          {"Type", "Receiver", "Counter", "Ciphertext"},
	"XDMCP":              {"Version", "Opcode", "Message Length"},
	"Zabbix agent":       {"Magic", "Flags", "Length"},
	"mDNS":               {"ID", "Questions"},
	"sFlow":              {"Version", "Agent Address Type", "Sequence Number", "Sample Count", "Samples"},
}

// Exact values for newly added authoritative samples. RequiredNodes above
// protects the shape of every direct result; these checks additionally anchor
// the values of stable header fields against the independent decoder output
// recorded when each sample was selected.
var protocolCorpusExactValues = map[string]map[string]any{
	"ndpi-6in4/6in4": {
		"Version":         uint64(4),
		"Header Length":   uint64(5),
		"Total Length":    uint64(124),
		"Protocol":        uint64(41),
		"Payload Length":  uint64(64),
		"Next Header":     uint64(58),
		"Hop Limit":       uint64(63),
		"Type":            uint64(128),
		"Code":            uint64(0),
		"Identifier":      uint64(0x5d8f),
		"Sequence Number": uint64(346),
	},
	"ndpi-ftps/FTPS": {
		"Command":   "AUTH",
		"Mechanism": "TLS",
	},
	"ndpi-ssdp/SSDP": {
		"Method":  "NOTIFY",
		"Target":  "*",
		"Version": "HTTP/1.1",
	},
	"wireshark-http/HTTP": {
		"Method":  "HEAD",
		"Path":    "/v4/iuident.cab?0307011208",
		"Version": "HTTP/1.1",
	},
	"wireshark-usb-hid/USB HID report sample": {
		"Report ID":   uint64(1),
		"Report Data": []byte{0xff, 0xff, 0xff},
	},
	"wireshark-ipx-rip/IPX": {
		"Checksum":            uint64(0xffff),
		"Length":              uint64(40),
		"Transport Control":   uint64(0),
		"Packet Type":         uint64(1),
		"Destination Network": uint64(40),
		"Destination Node":    []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		"Destination Socket":  uint64(0x0453),
		"Source Network":      uint64(40),
		"Source Node":         []byte{0x00, 0xaa, 0x00, 0xa3, 0xe3, 0xa4},
		"Source Socket":       uint64(0x0453),
		"Payload":             []byte{0x00, 0x02, 0x39, 0x17, 0x29, 0xe2, 0x00, 0x01, 0x00, 0x02},
	},
	"ndpi-iec61850-mms/IEC 61850 MMS": {
		"PDU Tag":                   uint64(0xa8),
		"PDU Length":                uint64(38),
		"Local Detail Calling":      []byte{0x00, 0xfa, 0x00},
		"Proposed Calling Limit":    uint64(10),
		"Proposed Called Limit":     uint64(10),
		"Proposed Nesting Level":    uint64(5),
		"Detail Length":             uint64(22),
		"Proposed Version":          uint64(1),
		"Parameter CBB Unused Bits": uint64(5),
		"Parameter CBB Bits":        []byte{0xe1, 0x00},
		"Services Unused Bits":      uint64(3),
		"Services Supported Bits":   []byte{0xa0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xe1, 0x10},
	},
	"ndpi-dcerpc/DCE/RPC": {
		"RPC Vers":         uint64(4),
		"Packet Type":      uint64(0),
		"Operation Number": []byte{0x00, 0x00},
	},
	"ndpi-kerberos-login/Kerberos": {
		"Application Tag": uint64(0x6c),
		"Length":          uint64(0x82),
		"Length 16":       uint64(0x04b7),
		"Seq Tag":         uint64(0x30),
		"Seq Length":      uint64(0x82),
		"Seq Length 16":   uint64(0x04b3),
	},
	"ndpi-pop3/POP3": {
		"Status": "+OK",
		"Arg":    "POP server ready H migmxus005",
		"LF":     uint64(0x0a),
	},
	"ndpi-rtmp/RTMP": {
		"Version": uint64(3),
		"Time":    uint64(0x0007aa57),
		"Zero":    uint64(0),
	},
	"ndpi-soap/SOAP": {
		"Version": "HTTP/1.1",
		"Status":  "302",
		"Message": "Moved Temporarily",
	},
	"ndpi-tcp-measurement/TCP": {
		"Source Port":      uint64(43895),
		"Destination Port": uint64(80),
		"Header Length":    uint64(5),
	},
	"ndpi-wireguard/WireGuard": {
		"Type":     uint64(4),
		"Receiver": uint64(0x79497615),
		"Counter":  uint64(19),
	},
	"wireshark-http3-qpack/QUIC": {
		"First Byte":  uint64(0xd4),
		"Version":     uint64(1),
		"DCID Length": uint64(8),
		"DCID":        []byte{0x88, 0x59, 0x71, 0x1e, 0x46, 0xd2, 0x44, 0x9f},
		"SCID Length": uint64(0),
	},
	"ndpi-corba/IIOP/GIOP": {
		"Magic":                 []byte("GIOP"),
		"Major":                 uint64(1),
		"Minor":                 uint64(2),
		"Flags":                 uint64(0),
		"Message Type":          uint64(0),
		"Message Size":          uint64(216),
		"Request ID":            uint64(0),
		"Response Flags":        uint64(3),
		"Key Len":               uint64(60),
		"Op Len":                uint64(5),
		"Operation":             "echo\x00",
		"Service Context Count": uint64(1),
		"Context ID":            uint64(7),
		"Context Data Length":   uint64(40),
	},
	"ndpi-sctp/SCTP": {
		"Source Port":      uint64(16384),
		"Destination Port": uint64(2944),
		"Verification Tag": uint64(0x00016f0a),
		"Checksum":         uint64(0x6db01882),
		"Type":             uint64(0),
		"Flags":            uint64(3),
		"Length":           uint64(91),
		"TSN":              uint64(671236933),
		"Stream ID":        uint64(0),
		"Stream Seq":       uint64(41149),
		"PPI":              uint64(7),
	},
	"ndpi-telnet/Telnet": {
		"IACByte": uint64(0xff),
		"Command": uint64(0xfd),
		"Option":  uint64(3),
	},
	"ndpi-teamviewer/TeamViewer": {
		"Magic":          uint64(0x1724),
		"Command":        uint64(0x0a),
		"Body Length 16": uint64(32),
	},
	"wireshark-dhcp/DHCP": {
		"Operation":       uint64(1),
		"Hardware Type":   uint64(1),
		"Hardware Length": uint64(6),
		"Xid":             uint64(0x00003d1d),
		"Client MAC":      []byte{0x00, 0x0b, 0x82, 0x01, 0xfc, 0x42},
		"Magic Cookie":    uint64(0x63825363),
		"Message Type":    uint64(1),
	},
	"wireshark-dhcp/BOOTP": {
		"Operation":       uint64(1),
		"Hardware Type":   uint64(1),
		"Hardware Length": uint64(6),
		"Xid":             uint64(0x00003d1d),
		"Client MAC":      []byte{0x00, 0x0b, 0x82, 0x01, 0xfc, 0x42},
	},
	"tcpdump-radius/RADIUS": {
		"Code":       uint64(1),
		"Identifier": uint64(5),
		"Length":     uint64(139),
	},
	"tcpdump-lldp/LLDP": {
		"Chassis ID Subtype": uint64(4),
		"Port ID Subtype":    uint64(1),
		"TTL":                uint64(120),
	},
	"tcpdump-radiotap/IEEE 802.11 Radiotap": {
		"Version":       uint64(0),
		"Header Length": uint64(89),
		"Present Flags": uint64(0x8000486f),
	},
	"ndpi-ocsp/OCSP": {
		"Algorithm OID":    []byte{0x2b, 0x0e, 0x03, 0x02, 0x1a},
		"Issuer Name Hash": []byte{0x42, 0x46, 0x30, 0xc2, 0x27, 0x19, 0xdb, 0xde, 0x70, 0xf0, 0x8f, 0xfc, 0x73, 0xe5, 0xa6, 0x5f, 0x66, 0x38, 0x17, 0xbc},
		"Issuer Key Hash":  []byte{0x98, 0xd1, 0xf8, 0x6e, 0x10, 0xeb, 0xcf, 0x9b, 0xec, 0x60, 0x9f, 0x18, 0x90, 0x1b, 0xa0, 0xeb, 0x7d, 0x09, 0xfd, 0x2b},
		"Serial Number":    []byte{0x00, 0xf8, 0x2a, 0xe3, 0x06, 0x79, 0x62, 0x79, 0x5c, 0x03, 0x00, 0x00, 0x00, 0x00, 0xcc, 0x24, 0x26},
	},
	"ndpi-profinet-io/Profinet IO": {
		"RPC Version":        uint64(4),
		"Packet Type":        uint64(0),
		"Operation Number":   uint64(5),
		"Fragment Length":    uint64(84),
		"Block Type":         uint64(0x0009),
		"Block Length":       uint64(60),
		"Sequence Number":    uint64(10),
		"API":                uint64(0),
		"Slot Number":        uint64(0),
		"Subslot Number":     uint64(1),
		"Index":              uint64(0xf840),
		"Record Data Length": uint64(32768),
	},
	"ndpi-upnp/UPnP": {
		"Method":  "NOTIFY",
		"Target":  "*",
		"Version": "HTTP/1.1",
	},
}

func protocolCorpusRequireCaptureParseSpec(t *testing.T, capture protocolCorpusCapture, spec protocolCorpusCaptureParseSpec) {
	t.Helper()
	require.NotNil(t, capture.RepresentativeFrame, "%s has a capture-specific parse contract but no representative frame", capture.ID)
	require.Equal(t, "upstream-positive", capture.EvidenceKind, "%s capture-specific parse contract cannot validate %s material", capture.ID, capture.EvidenceKind)
	require.NotEmpty(t, spec.Name, "%s capture-specific parse contract has no target name", capture.ID)
	require.NotEmpty(t, spec.Contract.RuleFile, "%s capture-specific parse contract has no rule", capture.ID)
	require.NotEmpty(t, spec.Contract.Layer, "%s capture-specific parse contract has no layer", capture.ID)
	require.NotEmpty(t, spec.RequiredNodes, "%s capture-specific parse contract has no semantic field contract", capture.ID)
	if capture.RoadmapName != nil {
		require.Equal(t, *capture.RoadmapName, spec.Name, "%s is mapped to roadmap name %q and cannot use alternate parse target %q", capture.ID, *capture.RoadmapName, spec.Name)
		require.Empty(t, spec.AlternateReason, "%s is roadmap-mapped and must not declare an alternate-target reason", capture.ID)
		return
	}
	require.NotEmpty(t, strings.TrimSpace(spec.AlternateReason), "%s uses non-roadmap parse target %q without an explicit reason", capture.ID, spec.Name)
}

// TestProtocolCorpusDirectRuleParsing is the first executable gate between the
// packet corpus and bin-parser. A capture being present, hashable, or
// recognizable by another decoder is deliberately not enough here: every case
// below supplies bytes from the protocol's declared layer to its bin-parser
// rule and requires a non-empty structured result backed by consumed input.
//
// Protocol aliases, encrypted outer transports, and roadmap entries without a
// dedicated rule are handled by the explicit coverage matrix built on top of
// this gate. They must not be silently counted as direct semantic parses.
func TestProtocolCorpusDirectRuleParsing(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)

	catalog := make(map[string]ProtocolInfo, len(ProtocolCatalog))
	for _, info := range ProtocolCatalog {
		catalog[info.Name] = info
	}

	type parseCase struct {
		capture       protocolCorpusCapture
		name          string
		contract      protocolCorpusParseContract
		info          ProtocolInfo
		requiredNodes []string
	}
	var cases []parseCase
	protocolNames := make(map[string]struct{})
	capturesByID := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		capturesByID[capture.ID] = capture
		if spec, ok := protocolCorpusCaptureParseSpecs[capture.ID]; ok {
			protocolCorpusRequireCaptureParseSpec(t, capture, spec)
			cases = append(cases, parseCase{
				capture:       capture,
				name:          spec.Name,
				contract:      spec.Contract,
				info:          ProtocolInfo{Name: spec.Name, Layer: spec.Contract.Layer, RuleFile: spec.Contract.RuleFile},
				requiredNodes: spec.RequiredNodes,
			})
			protocolNames[spec.Name] = struct{}{}
			continue
		}
		if capture.RepresentativeFrame == nil {
			continue
		}
		if capture.RoadmapName == nil || capture.EvidenceKind != "upstream-positive" {
			continue
		}
		name := *capture.RoadmapName
		contract, hasContract := protocolCorpusParseContracts[name]
		info, inCatalog := catalog[name]
		if !inCatalog && !(hasContract && contract.RuleFile != "" && contract.Layer != "") {
			continue
		}
		if !inCatalog {
			info = ProtocolInfo{Name: name, Layer: contract.Layer, RuleFile: contract.RuleFile}
		}
		cases = append(cases, parseCase{
			capture:       capture,
			name:          name,
			contract:      contract,
			info:          info,
			requiredNodes: protocolCorpusRequiredNodes[name],
		})
		protocolNames[name] = struct{}{}
	}
	for _, spec := range protocolCorpusLayerParseSpecs {
		capture, ok := capturesByID[spec.CaptureID]
		require.True(t, ok, "layer contract points to missing capture %s", spec.CaptureID)
		cases = append(cases, parseCase{
			capture:       capture,
			name:          spec.Name,
			contract:      spec.Contract,
			info:          ProtocolInfo{Name: spec.Name, Layer: spec.Contract.Layer, RuleFile: spec.Contract.RuleFile},
			requiredNodes: spec.RequiredNodes,
		})
		protocolNames[spec.Name] = struct{}{}
	}
	sort.Slice(cases, func(i, j int) bool {
		if cases[i].name == cases[j].name {
			return cases[i].capture.ID < cases[j].capture.ID
		}
		return cases[i].name < cases[j].name
	})
	require.NotEmpty(t, cases)
	t.Logf("validating %d direct captures across %d protocol names", len(cases), len(protocolNames))

	for _, item := range cases {
		item := item
		capture := item.capture
		testName := capture.ID
		if capture.RoadmapName == nil || item.name != *capture.RoadmapName {
			testName += "/" + strings.NewReplacer("/", "-", " ", "-").Replace(item.name)
		}
		t.Run(testName, func(t *testing.T) {
			contract := item.contract
			info := item.info
			input := protocolCorpusParseInput(t, corpusDir, capture, info, contract)
			require.NotEmpty(t, input, "%s has no %s bytes in %s", capture.ID, info.Layer, capture.LinkType)

			ruleFile := info.RuleFile
			if contract.RuleFile != "" {
				ruleFile = contract.RuleFile
			}
			rule := strings.TrimSuffix(strings.ReplaceAll(ruleFile, "/", "."), ".yaml")
			entryNode := contract.EntryNode
			if entryNode == "" {
				entryNode = info.EntryNode
			}
			reader := newProtocolCorpusBoundedReader(input)
			var node *base.Node
			var err error
			if entryNode == "" {
				node, err = parser.ParseBinary(reader, rule)
			} else {
				node, err = parser.ParseBinary(reader, rule, entryNode)
			}
			require.NoError(t, err, "%s failed to parse %s bytes with %s", capture.ID, info.Layer, ruleFile)
			require.NotNil(t, node)
			require.NotEmpty(t, item.requiredNodes, "%s has no semantic field contract", item.name)
			for _, requiredNode := range item.requiredNodes {
				require.True(t, protocolCorpusHasNode(node, requiredNode), "%s did not produce required node %q; processed terminal fields: %s", capture.ID, requiredNode, strings.Join(protocolCorpusProcessedTerminalNames(node), ", "))
			}
			for fieldName, expected := range protocolCorpusExactValues[capture.ID+"/"+item.name] {
				protocolCorpusRequireValue(t, node, fieldName, expected)
			}

			value, err := node.Result()
			require.NoError(t, err, "%s produced no structured result", capture.ID)
			require.NotNil(t, value, "%s produced a nil structured result", capture.ID)
			require.NotEmpty(t, value.Children(), "%s produced an empty structured result", capture.ID)
			terminals, firstBit, lastBit := protocolCorpusConsumedRange(node, uint64(len(input))*8)
			require.Positive(t, terminals, "%s produced no terminal field results", capture.ID)
			require.Equal(t, uint64(0), firstBit, "%s left an unparsed prefix", capture.ID)
			require.Positive(t, lastBit, "%s consumed no input", capture.ID)
			require.Equal(t, uint64(len(input))*8, lastBit, "%s left an unparsed suffix", capture.ID)
			coveredTerminals, coverageErr := protocolCorpusTerminalCoverage(node, input)
			require.NoError(t, coverageErr, "%s terminal fields do not cover the complete bounded input", capture.ID)
			require.Positive(t, coveredTerminals, "%s produced no covered terminal field results", capture.ID)
			require.Zero(t, reader.Len(), "%s left %d of %d protocol bytes unread", capture.ID, reader.Len(), len(input))
			t.Logf("capture=%s layer=%s input=%d reader_consumed=%d result_end=%d terminals=%d covered_terminals=%d", capture.ID, info.Layer, len(input), len(input)-reader.Len(), (lastBit+7)/8, terminals, coveredTerminals)
		})
	}
}

func TestProtocolCorpusMalformedInputsAreRejected(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	captures := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		captures[capture.ID] = capture
	}

	ids := make([]string, 0, len(protocolCorpusRejectionSpecs))
	for id := range protocolCorpusRejectionSpecs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		id := id
		t.Run(id, func(t *testing.T) {
			spec := protocolCorpusRejectionSpecs[id]
			capture, ok := captures[id]
			require.True(t, ok, "rejection contract points to a missing capture")
			require.Equal(t, "upstream-negative", capture.EvidenceKind)
			protocolCorpusRequireRuleEntry(t, id, spec.Contract)
			require.NotEmpty(t, spec.ControlCaptureID, "%s rejection contract has no positive control capture", id)
			controlCapture, ok := captures[spec.ControlCaptureID]
			require.True(t, ok, "%s positive control points to missing capture %s", id, spec.ControlCaptureID)
			require.Equal(t, "upstream-positive", controlCapture.EvidenceKind, "%s positive control must use positive material", id)
			require.Equal(t, spec.Contract.RuleFile, spec.ControlContract.RuleFile, "%s positive control must use the rejected rule", id)
			require.Equal(t, spec.Contract.EntryNode, spec.ControlContract.EntryNode, "%s positive control must use the rejected entry", id)
			controlInfo := ProtocolInfo{Name: spec.Name, Layer: spec.ControlContract.Layer, RuleFile: spec.ControlContract.RuleFile}
			controlInput := protocolCorpusParseInput(t, corpusDir, controlCapture, controlInfo, spec.ControlContract)
			require.NotEmpty(t, controlInput, "%s positive control has no parser input", id)
			controlReader := newProtocolCorpusBoundedReader(controlInput)
			controlNode, err := protocolCorpusParseRule(controlReader, spec.ControlContract)
			require.NoError(t, err, "%s positive control %s did not parse with %s/%s", id, spec.ControlCaptureID, spec.Contract.RuleFile, spec.Contract.EntryNode)
			require.NotNil(t, controlNode, "%s positive control returned no parse tree", id)
			controlValue, err := controlNode.Result()
			require.NoError(t, err, "%s positive control returned no structured result", id)
			require.NotNil(t, controlValue, "%s positive control returned a nil structured result", id)
			require.NotEmpty(t, controlValue.Children(), "%s positive control returned an empty structured result", id)
			require.Zero(t, controlReader.Len(), "%s positive control left %d of %d protocol bytes unread", id, controlReader.Len(), len(controlInput))

			info := ProtocolInfo{Name: spec.Name, Layer: spec.Contract.Layer, RuleFile: spec.Contract.RuleFile}
			input := protocolCorpusParseInput(t, corpusDir, capture, info, spec.Contract)
			require.NotEmpty(t, input)
			reader := newProtocolCorpusBoundedReader(input)
			_, err = protocolCorpusParseRule(reader, spec.Contract)
			protocolCorpusRequireExpectedFailure(t, id, spec, err)
			t.Logf("capture=%s rule=%s input=%d rejected=%v", id, spec.Contract.RuleFile, len(input), err)
		})
	}
}

func protocolCorpusRequireRuleEntry(t *testing.T, id string, contract protocolCorpusParseContract) {
	t.Helper()
	require.NotEmpty(t, contract.RuleFile, "%s rejection contract has no rule", id)
	require.NotEmpty(t, contract.EntryNode, "%s rejection contract has no entry", id)
	root, err := base.ParseRule(contract.RuleFile)
	require.NoError(t, err, "%s rejection rule %s is unavailable or invalid", id, contract.RuleFile)
	require.NotNil(t, root)
	require.NotNil(t, base.GetNodeByPath(root, "@"+contract.EntryNode), "%s rejection entry %s is missing from %s", id, contract.EntryNode, contract.RuleFile)
}

func protocolCorpusParseRule(reader *protocolCorpusBoundedReader, contract protocolCorpusParseContract) (*base.Node, error) {
	rule := strings.TrimSuffix(strings.ReplaceAll(contract.RuleFile, "/", "."), ".yaml")
	if contract.EntryNode == "" {
		return parser.ParseBinary(reader, rule)
	}
	return parser.ParseBinary(reader, rule, contract.EntryNode)
}

func protocolCorpusRequireExpectedFailure(t *testing.T, id string, spec protocolCorpusRejectionSpec, err error) {
	t.Helper()
	require.Error(t, err, "%s malformed input was accepted by %s", id, spec.Contract.RuleFile)
	require.ErrorContains(t, err, "parse node "+spec.Contract.EntryNode+" error", "%s failed outside its declared parser entry", id)
	require.NotEmpty(t, strings.TrimSpace(spec.ExpectedErrorContains), "%s rejection contract has no expected error text", id)
	require.ErrorContains(t, err, spec.ExpectedErrorContains, "%s failed for an unexpected reason", id)
	switch spec.ExpectedFailureClass {
	case protocolCorpusFailureInvalidValue:
		require.ErrorContains(t, err, "YakVM Panic:", "%s was not rejected by a field-value invariant", id)
	case protocolCorpusFailureLengthBounds:
		require.ErrorContains(t, err, "over max size", "%s was not rejected by an input-length bound", id)
	case protocolCorpusFailureTruncated:
		message := err.Error()
		require.True(t, strings.Contains(message, "truncated") || strings.Contains(message, "EOF"), "%s was not rejected as truncated: %v", id, err)
	default:
		t.Fatalf("%s rejection contract has unknown failure class %q", id, spec.ExpectedFailureClass)
	}
}

func TestProtocolCorpusValidationMatrix(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	catalog := make(map[string]ProtocolInfo, len(ProtocolCatalog))
	for _, info := range ProtocolCatalog {
		catalog[info.Name] = info
	}

	counts := map[string]int{}
	for _, capture := range manifest.Captures {
		captureSpec, captureParse := protocolCorpusCaptureParseSpecs[capture.ID]
		if captureParse {
			protocolCorpusRequireCaptureParseSpec(t, capture, captureSpec)
		}
		direct := captureParse
		if !direct && capture.EvidenceKind == "upstream-positive" && capture.RoadmapName != nil && capture.RepresentativeFrame != nil {
			contract, hasContract := protocolCorpusParseContracts[*capture.RoadmapName]
			_, inCatalog := catalog[*capture.RoadmapName]
			direct = inCatalog || (hasContract && contract.RuleFile != "" && contract.Layer != "")
		}
		classifierSpec, classifier := protocolCorpusClassifierSpecs[capture.ID]
		_, rejection := protocolCorpusRejectionSpecs[capture.ID]
		structuralKind, structural := protocolCorpusStructuralSpecs[capture.ID]
		categoryCount := 0
		for _, member := range []bool{direct, classifier, rejection, structural} {
			if member {
				categoryCount++
			}
		}
		require.Equal(t, 1, categoryCount, "%s must have exactly one validation category", capture.ID)

		switch {
		case direct:
			counts["parsed"]++
		case classifier:
			counts["classifier"]++
			require.Equal(t, "upstream-positive", capture.EvidenceKind)
			info := ProtocolInfo{Name: capture.ID, Layer: classifierSpec.Layer}
			input := protocolCorpusParseInput(t, corpusDir, capture, info, protocolCorpusParseContract{Layer: classifierSpec.Layer})
			require.GreaterOrEqual(t, len(input), classifierSpec.MinimumBytes, "%s payload is shorter than its classification fixture", capture.ID)
			if classifierSpec.MaximumBytes > 0 {
				require.LessOrEqual(t, len(input), classifierSpec.MaximumBytes, "%s payload is longer than its classification fixture", capture.ID)
			}
			if classifierSpec.ExactBytes > 0 {
				require.Len(t, input, classifierSpec.ExactBytes, "%s payload length changed", capture.ID)
			}
			require.True(t, bytes.HasPrefix(input, classifierSpec.Prefix), "%s payload prefix changed", capture.ID)
			if len(classifierSpec.Contains) > 0 {
				require.True(t, bytes.Contains(input, classifierSpec.Contains), "%s payload marker is missing", capture.ID)
			}
			for offset, expected := range classifierSpec.AtOffset {
				require.LessOrEqual(t, offset+len(expected), len(input), "%s offset fixture exceeds payload", capture.ID)
				require.Equal(t, expected, input[offset:offset+len(expected)], "%s payload bytes at offset %d changed", capture.ID, offset)
			}
		case rejection:
			counts["rejected"]++
			require.Equal(t, "upstream-negative", capture.EvidenceKind)
		case structural:
			counts["structural"]++
			require.Equal(t, "upstream-negative", capture.EvidenceKind)
			captureData := readProtocolCorpusFile(t, corpusDir, capture.CaptureFile)
			switch structuralKind {
			case "empty capture":
				ngReader, err := pcapgo.NewNgReader(bytes.NewReader(captureData), pcapgo.NgReaderOptions{SkipUnknownVersion: true})
				require.NoError(t, err)
				_, _, err = ngReader.ReadPacketData()
				require.ErrorIs(t, err, io.EOF)
			case "unsupported link type":
				_, _, frame := inspectProtocolCorpusCapture(t, captureData, capture.RepresentativeFrame)
				_, err := protocolCorpusFirstLayer(frame, capture.LinkType)
				require.ErrorContains(t, err, "unsupported corpus link type")
			default:
				t.Fatalf("unknown structural validation %q", structuralKind)
			}
		}
	}

	require.Equal(t, len(manifest.Captures), counts["parsed"]+counts["classifier"]+counts["rejected"]+counts["structural"])
	require.Equal(t, 159, counts["parsed"], "parsed-capture validation category changed; review every added or reclassified capture")
	require.Equal(t, 3, counts["classifier"], "classifier validation category changed; review every added or reclassified capture")
	require.Equal(t, 14, counts["rejected"], "rejected-capture validation category changed; review every added or reclassified capture")
	require.Equal(t, 2, counts["structural"], "structural validation category changed; review every added or reclassified capture")
	t.Logf("validated matrix: total=%d parsed=%d classifier=%d rejected=%d structural=%d", len(manifest.Captures), counts["parsed"], counts["classifier"], counts["rejected"], counts["structural"])
}

func TestProtocolCorpusRoadmapNameValidation(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	catalog := make(map[string]ProtocolInfo, len(ProtocolCatalog))
	for _, info := range ProtocolCatalog {
		catalog[info.Name] = info
	}

	roadmapNames := make(map[string]struct{})
	semantic := make(map[string]struct{})
	outer := make(map[string]struct{})
	classifier := make(map[string]struct{})
	for _, capture := range manifest.Captures {
		if capture.RoadmapName != nil {
			roadmapNames[*capture.RoadmapName] = struct{}{}
		}
		if spec, ok := protocolCorpusCaptureParseSpecs[capture.ID]; ok {
			protocolCorpusRequireCaptureParseSpec(t, capture, spec)
			if capture.RoadmapName != nil && spec.Name == *capture.RoadmapName {
				if spec.Contract.OuterOnly {
					outer[spec.Name] = struct{}{}
				} else {
					semantic[spec.Name] = struct{}{}
				}
			}
			continue
		}
		if _, ok := protocolCorpusClassifierSpecs[capture.ID]; ok {
			if capture.RoadmapName != nil {
				classifier[*capture.RoadmapName] = struct{}{}
			}
			continue
		}
		if capture.EvidenceKind != "upstream-positive" || capture.RoadmapName == nil || capture.RepresentativeFrame == nil {
			continue
		}
		name := *capture.RoadmapName
		contract, hasContract := protocolCorpusParseContracts[name]
		_, inCatalog := catalog[name]
		if !inCatalog && !(hasContract && contract.RuleFile != "" && contract.Layer != "") {
			continue
		}
		if contract.OuterOnly {
			outer[name] = struct{}{}
		} else {
			semantic[name] = struct{}{}
		}
	}
	for _, spec := range protocolCorpusLayerParseSpecs {
		if spec.Contract.OuterOnly {
			outer[spec.Name] = struct{}{}
		} else {
			semantic[spec.Name] = struct{}{}
		}
	}
	for _, spec := range protocolCorpusCaptureParseSpecs {
		if _, coveredName := roadmapNames[spec.Name]; !coveredName {
			continue
		}
		if spec.Contract.OuterOnly {
			outer[spec.Name] = struct{}{}
		} else {
			semantic[spec.Name] = struct{}{}
		}
	}

	// Prefer the stronger category when multiple captures exercise the same
	// name; the weaker categories remain useful per-capture checks above.
	for name := range semantic {
		delete(outer, name)
		delete(classifier, name)
	}
	for name := range outer {
		delete(classifier, name)
	}
	for name := range semantic {
		_, ok := catalog[name]
		require.True(t, ok, "semantic protocol %q is not discoverable in ProtocolCatalog", name)
	}
	for name := range outer {
		_, ok := catalog[name]
		require.True(t, ok, "outer-format protocol %q is not discoverable in ProtocolCatalog", name)
	}

	var missing []string
	for name := range roadmapNames {
		_, semanticOK := semantic[name]
		_, outerOK := outer[name]
		_, classifierOK := classifier[name]
		if !semanticOK && !outerOK && !classifierOK {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	require.Len(t, roadmapNames, 156, "the pinned corpus roadmap-name set changed")
	require.Empty(t, missing, "roadmap names without executable positive validation")
	require.Equal(t, len(roadmapNames), len(semantic)+len(outer)+len(classifier))
	require.Len(t, semantic, 143, "field-level roadmap validation set changed; review every added or reclassified name")
	require.Len(t, outer, 10, "outer-format roadmap validation set changed; review every added or reclassified name")
	require.Len(t, classifier, 3, "classifier-only roadmap validation set changed; review every added or reclassified name")
	t.Logf("validated roadmap names: total=%d semantic=%d outer=%d classifier=%d", len(roadmapNames), len(semantic), len(outer), len(classifier))
	t.Logf("field-level names: %s", strings.Join(protocolCorpusSortedNames(semantic), ", "))
	t.Logf("outer-format names: %s", strings.Join(protocolCorpusSortedNames(outer), ", "))
	t.Logf("classification-only names: %s", strings.Join(protocolCorpusSortedNames(classifier), ", "))
}

func protocolCorpusSortedNames(names map[string]struct{}) []string {
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func protocolCorpusParseInput(t *testing.T, corpusDir string, capture protocolCorpusCapture, info ProtocolInfo, contract protocolCorpusParseContract) []byte {
	t.Helper()
	require.NotNil(t, capture.RepresentativeFrame, "%s has no representative frame", capture.ID)
	captureData := readProtocolCorpusFile(t, corpusDir, capture.CaptureFile)
	_, _, frame := inspectProtocolCorpusCapture(t, captureData, capture.RepresentativeFrame)
	var input []byte
	if contract.ReassembleTCP {
		input = protocolCorpusTCPStreamThroughFrame(t, captureData, capture.LinkType, capture.RepresentativeFrame.Number)
	} else if contract.UseFullFrame {
		input = append([]byte(nil), frame...)
	} else if contract.FrameOffset > 0 {
		require.Less(t, contract.FrameOffset, len(frame), "%s protocol offset exceeds frame", capture.ID)
		input = append([]byte(nil), frame[contract.FrameOffset:]...)
	} else {
		input = protocolCorpusRuleBytes(t, frame, capture.LinkType, info)
	}
	if contract.TrimPrefix > 0 {
		require.Greater(t, len(input), contract.TrimPrefix, "%s input is shorter than its framing prefix", capture.ID)
		input = input[contract.TrimPrefix:]
	}
	if len(contract.TrimSuffix) > 0 {
		require.True(t, bytes.HasSuffix(input, contract.TrimSuffix), "%s does not end with its declared framing suffix", capture.ID)
		input = input[:len(input)-len(contract.TrimSuffix)]
	}
	if len(contract.StartMagic) > 0 {
		offset := bytes.Index(input, contract.StartMagic)
		require.NotEqual(t, -1, offset, "%s does not contain the expected message start", capture.ID)
		input = input[offset:]
	}
	if len(contract.StartAfter) > 0 {
		offset := bytes.Index(input, contract.StartAfter)
		require.NotEqual(t, -1, offset, "%s does not contain the expected message delimiter", capture.ID)
		input = input[offset+len(contract.StartAfter):]
	}
	if contract.InputLength > 0 {
		require.GreaterOrEqual(t, len(input), contract.InputLength, "%s input is shorter than its declared protocol length", capture.ID)
		input = input[:contract.InputLength]
	}
	return input
}

func protocolCorpusHasNode(node *base.Node, name string) bool {
	if node.Name == name && protocolCorpusNodeHasResult(node) {
		return true
	}
	for _, child := range node.Children {
		if protocolCorpusHasNode(child, name) {
			return true
		}
	}
	return false
}

// Schema nodes are materialized before their operators necessarily process
// them. A name alone therefore proves only that a field is declared by the
// rule. Require an actual result on the node or in its subtree so RequiredNodes
// cannot pass on a dormant branch.
func protocolCorpusNodeHasResult(node *base.Node) bool {
	if stream_parser.NodeHasResult(node) {
		return true
	}
	for _, child := range node.Children {
		if protocolCorpusNodeHasResult(child) {
			return true
		}
	}
	return false
}

func protocolCorpusProcessedTerminalNames(node *base.Node) []string {
	names := make(map[string]struct{})
	var walk func(*base.Node)
	walk = func(current *base.Node) {
		if stream_parser.NodeIsTerminal(current) && stream_parser.NodeHasResult(current) {
			names[current.Name] = struct{}{}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(node)
	return protocolCorpusSortedNames(names)
}

func protocolCorpusFindNode(node *base.Node, name string) *base.Node {
	if node.Name == name && stream_parser.NodeHasResult(node) {
		return node
	}
	for _, child := range node.Children {
		if found := protocolCorpusFindNode(child, name); found != nil {
			return found
		}
	}
	return nil
}

func protocolCorpusRequireValue(t *testing.T, node *base.Node, name string, expected any) {
	t.Helper()
	field := protocolCorpusFindNode(node, name)
	require.NotNil(t, field, "missing exact-value field %q", name)
	value, err := field.Result()
	require.NoError(t, err, "read exact-value field %q", name)
	require.NotNil(t, value, "exact-value field %q has no result", name)
	switch expected := expected.(type) {
	case uint64:
		require.Equal(t, expected, uintVal(t, value), "field %q", name)
	case []byte:
		require.Equal(t, expected, bytesVal(t, value), "field %q", name)
	case string:
		require.Equal(t, expected, strVal(t, value), "field %q", name)
	default:
		t.Fatalf("unsupported exact-value type %T for %q", expected, name)
	}
}

func protocolCorpusRuleBytes(t *testing.T, frame []byte, linkType string, info ProtocolInfo) []byte {
	t.Helper()
	firstLayer, err := protocolCorpusFirstLayer(frame, linkType)
	require.NoError(t, err)
	packet := gopacket.NewPacket(frame, firstLayer, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
	switch info.Layer {
	case "L2":
		if info.Name == "Ethernet" || info.Name == "PPP" || info.Name == "Linux SLL" || info.Name == "IEEE 802.11" {
			return append([]byte(nil), frame...)
		}
		link := packet.LinkLayer()
		require.NotNil(t, link, "packet has no link layer")
		return append([]byte(nil), link.LayerPayload()...)
	case "L3":
		network := packet.NetworkLayer()
		require.NotNil(t, network, "packet has no network layer")
		if info.Name == "IPv4" || info.Name == "IPv6" {
			return protocolCorpusLayerAndPayload(t, network, "network")
		}
		return append([]byte(nil), network.LayerPayload()...)
	case "L4":
		transport := packet.TransportLayer()
		require.NotNil(t, transport, "packet has no transport layer")
		switch info.Name {
		case "TCP", "UDP", "SCTP":
			return protocolCorpusLayerAndPayload(t, transport, "transport")
		default:
			return append([]byte(nil), transport.LayerPayload()...)
		}
	case "L7":
		transport := packet.TransportLayer()
		require.NotNil(t, transport, "packet has no transport layer")
		return append([]byte(nil), transport.LayerPayload()...)
	default:
		t.Fatalf("unsupported protocol layer %q", info.Layer)
		return nil
	}
}

func protocolCorpusFirstLayer(frame []byte, linkType string) (gopacket.Decoder, error) {
	switch linkType {
	case "Ethernet":
		return layers.LayerTypeEthernet, nil
	case "PPP":
		return layers.LayerTypePPP, nil
	case "Linux SLL":
		return layers.LayerTypeLinuxSLL, nil
	case "Null":
		return layers.LayerTypeLoopback, nil
	case "RadioTap":
		return layers.LayerTypeRadioTap, nil
	case "Raw":
		if len(frame) == 0 {
			return nil, fmt.Errorf("empty raw frame")
		}
		switch frame[0] >> 4 {
		case 4:
			return layers.LayerTypeIPv4, nil
		case 6:
			return layers.LayerTypeIPv6, nil
		default:
			return nil, fmt.Errorf("raw frame starts with IP version %d", frame[0]>>4)
		}
	default:
		return nil, fmt.Errorf("unsupported corpus link type %q", linkType)
	}
}

func protocolCorpusLayerAndPayload(t *testing.T, layer gopacket.Layer, kind string) []byte {
	t.Helper()
	require.NotNil(t, layer, "packet has no %s layer", kind)
	data := make([]byte, 0, len(layer.LayerContents())+len(layer.LayerPayload()))
	data = append(data, layer.LayerContents()...)
	data = append(data, layer.LayerPayload()...)
	return data
}

func protocolCorpusConsumedRange(node *base.Node, inputBits uint64) (terminals int, firstBit, lastBit uint64) {
	first := true
	var walk func(*base.Node)
	walk = func(current *base.Node) {
		if stream_parser.NodeHasResult(current) {
			position := stream_parser.GetNodeResultPos(current)
			end := position[1]
			// Delimited terminals store the result range without the delimiter,
			// even though the parser consumes and writes those bytes separately.
			// Account for that framing here so exact-consumption validation does
			// not mistake a final CRLF/SOH/NUL for an unparsed suffix. Optional
			// delimiters missing at EOF are not counted because they would extend
			// beyond the bounded input.
			if stream_parser.NodeIsDelimiter(current) {
				delimiter := current.Cfg.GetString(stream_parser.CfgDelimiter)
				if delimiter == "" {
					delimiter = current.Cfg.GetString(stream_parser.CfgDel)
				}
				delimitedEnd := end + uint64(len(delimiter))*8
				if delimitedEnd <= inputBits {
					end = delimitedEnd
				}
			}
			terminals++
			if first || position[0] < firstBit {
				firstBit = position[0]
			}
			if first || end > lastBit {
				lastBit = end
			}
			first = false
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(node)
	return terminals, firstBit, lastBit
}

type protocolCorpusTerminalInterval struct {
	start uint64
	end   uint64
	path  string
}

// protocolCorpusTerminalCoverage proves that actual terminal results, rather
// than a parent/container result range, account for every bit in the bounded
// parser input. Delimiters are not part of a terminal's stored result range;
// extend such a range only when the exact delimiter bytes are present in the
// input immediately after the result.
func protocolCorpusTerminalCoverage(node *base.Node, input []byte) (int, error) {
	inputBits := uint64(len(input)) * 8
	intervals := make([]protocolCorpusTerminalInterval, 0)
	var problems []string
	var walk func(*base.Node, string)
	walk = func(current *base.Node, parentPath string) {
		path := current.Name
		if parentPath != "" {
			path = parentPath + "/" + current.Name
		}
		if stream_parser.NodeIsTerminal(current) && stream_parser.NodeHasResult(current) {
			position := stream_parser.GetNodeResultPos(current)
			start, end := position[0], position[1]
			if start > end {
				problems = append(problems, fmt.Sprintf("terminal %q has reversed interval [%d,%d)", path, start, end))
			} else if start > inputBits || end > inputBits {
				problems = append(problems, fmt.Sprintf("terminal %q interval [%d,%d) is outside bounded input [0,%d)", path, start, end, inputBits))
			} else {
				if stream_parser.NodeIsDelimiter(current) {
					delimiter := current.Cfg.GetString(stream_parser.CfgDelimiter)
					if delimiter == "" {
						delimiter = current.Cfg.GetString(stream_parser.CfgDel)
					}
					delimiterBits := uint64(len(delimiter)) * 8
					delimiterEnd := end + delimiterBits
					if delimiter != "" && end%8 == 0 && delimiterEnd <= inputBits {
						startByte := end / 8
						endByte := delimiterEnd / 8
						if bytes.Equal(input[startByte:endByte], []byte(delimiter)) {
							end = delimiterEnd
						}
					}
				}
				intervals = append(intervals, protocolCorpusTerminalInterval{start: start, end: end, path: path})
			}
		}
		for _, child := range current.Children {
			walk(child, path)
		}
	}
	walk(node, "")

	sort.Slice(intervals, func(i, j int) bool {
		if intervals[i].start == intervals[j].start {
			if intervals[i].end == intervals[j].end {
				return intervals[i].path < intervals[j].path
			}
			return intervals[i].end < intervals[j].end
		}
		return intervals[i].start < intervals[j].start
	})
	if len(intervals) == 0 {
		problems = append(problems, "no terminal result intervals")
	}

	cursor := uint64(0)
	previousPath := "<start>"
	for _, interval := range intervals {
		if interval.start > cursor {
			problems = append(problems, fmt.Sprintf("uncovered bits [%d,%d) (bytes %.3f..%.3f) between %q and %q", cursor, interval.start, float64(cursor)/8, float64(interval.start)/8, previousPath, interval.path))
		}
		if interval.end > cursor {
			cursor = interval.end
			previousPath = interval.path
		}
	}
	if cursor < inputBits {
		problems = append(problems, fmt.Sprintf("uncovered bits [%d,%d) (bytes %.3f..%.3f) after %q", cursor, inputBits, float64(cursor)/8, float64(inputBits)/8, previousPath))
	}
	if len(problems) > 0 {
		return len(intervals), fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return len(intervals), nil
}

func TestProtocolCorpusTerminalCoverageCannotBeMaskedByContainer(t *testing.T) {
	terminal := func(name string, start, end uint64) *base.Node {
		cfg := base.NewEmptyConfig()
		cfg.SetItem(stream_parser.CfgIsTerminal, true)
		cfg.SetItem(stream_parser.CfgNodeResult, [2]uint64{start, end})
		return &base.Node{Name: name, Cfg: cfg}
	}

	t.Run("container result cannot hide a terminal gap", func(t *testing.T) {
		rootCfg := base.NewEmptyConfig()
		rootCfg.SetItem(stream_parser.CfgNodeResult, [2]uint64{0, 24})
		root := &base.Node{
			Name:     "Container",
			Cfg:      rootCfg,
			Children: []*base.Node{terminal("First", 0, 8), terminal("Third", 16, 24)},
		}
		count, err := protocolCorpusTerminalCoverage(root, []byte{1, 2, 3})
		require.Equal(t, 2, count)
		require.ErrorContains(t, err, "uncovered bits [8,16)")
	})

	t.Run("delimiter extends coverage only on an exact match", func(t *testing.T) {
		field := terminal("Delimited", 0, 8)
		field.Cfg.SetItem(stream_parser.CfgDelimiter, ":")
		count, err := protocolCorpusTerminalCoverage(field, []byte("a:"))
		require.NoError(t, err)
		require.Equal(t, 1, count)

		_, err = protocolCorpusTerminalCoverage(field, []byte("ab"))
		require.ErrorContains(t, err, "uncovered bits [8,16)")
	})

	t.Run("out of bounds terminal is rejected", func(t *testing.T) {
		_, err := protocolCorpusTerminalCoverage(terminal("Outside", 0, 9), []byte{1})
		require.ErrorContains(t, err, "outside bounded input [0,8)")
	})
}

func protocolCorpusTCPStreamThroughFrame(t *testing.T, captureData []byte, linkType string, targetFrame int) []byte {
	t.Helper()
	reader := bytes.NewReader(captureData)
	var packetReader protocolCorpusPacketReader
	if len(captureData) >= 4 && bytes.Equal(captureData[:4], []byte{'\x0a', '\x0d', '\x0d', '\x0a'}) {
		ngReader, err := pcapgo.NewNgReader(reader, pcapgo.NgReaderOptions{SkipUnknownVersion: true})
		require.NoError(t, err)
		packetReader = ngReader
	} else {
		pcapReader, err := pcapgo.NewReader(reader)
		if err != nil {
			normalized, ok := normalizeProtocolCorpusLegacyPcapHeader(captureData)
			require.True(t, ok, "capture has an unsupported legacy header: %v", err)
			pcapReader, err = pcapgo.NewReader(bytes.NewReader(normalized))
			require.NoError(t, err)
		}
		packetReader = pcapReader
	}

	type decodedFrame struct {
		number int
		flow   string
		seq    uint32
		data   []byte
	}
	var decoded []decodedFrame
	var targetFlow string
	for number := 1; number <= targetFrame; number++ {
		frame, _, err := packetReader.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		first, err := protocolCorpusFirstLayer(frame, linkType)
		require.NoError(t, err)
		packet := gopacket.NewPacket(frame, first, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		if tcpLayer == nil || packet.NetworkLayer() == nil {
			continue
		}
		tcp := tcpLayer.(*layers.TCP)
		flow := packet.NetworkLayer().NetworkFlow().String() + "|" + tcp.TransportFlow().String()
		if number == targetFrame {
			targetFlow = flow
		}
		if len(tcp.Payload) > 0 {
			decoded = append(decoded, decodedFrame{number: number, flow: flow, seq: tcp.Seq, data: append([]byte(nil), tcp.Payload...)})
		}
	}
	require.NotEmpty(t, targetFlow, "representative frame %d is not TCP", targetFrame)

	var segments []decodedFrame
	for _, frame := range decoded {
		if frame.flow == targetFlow && frame.number <= targetFrame {
			segments = append(segments, frame)
		}
	}
	require.NotEmpty(t, segments, "TCP direction has no payload through frame %d", targetFrame)
	sort.Slice(segments, func(i, j int) bool { return segments[i].seq < segments[j].seq })

	stream := append([]byte(nil), segments[0].data...)
	next := uint64(segments[0].seq) + uint64(len(segments[0].data))
	for _, segment := range segments[1:] {
		start := uint64(segment.seq)
		end := start + uint64(len(segment.data))
		if end <= next {
			continue
		}
		require.LessOrEqual(t, start, next, "TCP stream has a gap before frame %d", segment.number)
		overlap := next - start
		stream = append(stream, segment.data[overlap:]...)
		next = end
	}
	return stream
}
