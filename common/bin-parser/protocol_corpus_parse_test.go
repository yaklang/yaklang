package bin_parser

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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
	Base64After   []byte
	ReassembleTCP bool
	// ReassembleThroughFrame pins a later transport boundary when the material
	// representative is only the first fragment. Zero retains the historical
	// through-representative behavior.
	ReassembleThroughFrame int
	UseFullFrame           bool
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
	UseFullFrame bool
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
	"SMB3": {RuleFile: "application-layer/smb3.yaml", EntryNode: "SMB3Negotiate", Layer: "L7", TrimPrefix: 4},
	// Every sampled XTP representative is Ethernet + a checked 20-byte IPv4 header.
	// An explicit offset avoids inventing a gopacket XTP transport decoder.
	"XTP":                       {RuleFile: "transport-layer/xtp.yaml", EntryNode: "XTP", Layer: "L4", FrameOffset: 34},
	"H.225":                     {RuleFile: "application-layer/h225.yaml", EntryNode: "H225RAS", Layer: "L7"},
	"XYPLEX":                    {RuleFile: "xyplex.yaml", EntryNode: "XYPLEXRequest", Layer: "L7"},
	"SNA":                       {RuleFile: "sna.yaml", EntryNode: "SNAEthernet", Layer: "L2", FrameOffset: 14},
	"swIPe":                     {RuleFile: "swipe.yaml", EntryNode: "SwIPe", Layer: "L3", FrameOffset: 34},
	"Megaco/H.248":              {RuleFile: "application-layer/megaco.yaml", EntryNode: "Megaco", Layer: "L7"},
	"Zigbee":                    {RuleFile: "zigbee.yaml", EntryNode: "Zigbee", Layer: "L2", UseFullFrame: true},
	"VINES":                     {RuleFile: "vines.yaml", EntryNode: "VINESVIP", Layer: "L2", FrameOffset: 14},
	"SIGCOMP":                   {RuleFile: "application-layer/sigcomp.yaml", EntryNode: "SIGCOMP", Layer: "L7"},
	"WEP":                       {RuleFile: "wep.yaml", EntryNode: "WEP", Layer: "L2", FrameOffset: 32},
	"NCP":                       {RuleFile: "application-layer/ncp.yaml", EntryNode: "NCP", Layer: "L7"},
	"LAT":                       {RuleFile: "lat.yaml", EntryNode: "LAT", Layer: "L2", FrameOffset: 14},
	"SEBEK":                     {RuleFile: "application-layer/sebek.yaml", EntryNode: "SEBEK", Layer: "L7"},
	"AllJoyn":                   {RuleFile: "application-layer/alljoyn.yaml", EntryNode: "AllJoynNS", Layer: "L7"},
	"FCoE":                      {RuleFile: "fcoe.yaml", EntryNode: "FCoE", Layer: "L2", FrameOffset: 14},
	"Fibre Channel":             {RuleFile: "fibre_channel.yaml", EntryNode: "FibreChannel", Layer: "L2", FrameOffset: 28, InputLength: 28},
	"NAT-T":                     {RuleFile: "nat_t.yaml", EntryNode: "NATT", Layer: "L4"},
	"MACSec":                    {RuleFile: "macsec.yaml", EntryNode: "MACSec", Layer: "L2", FrameOffset: 14},
	"WSMP":                      {RuleFile: "wsmp.yaml", EntryNode: "WSMP", Layer: "L2", FrameOffset: 14},
	"Steam":                     {RuleFile: "application-layer/steam_discovery.yaml", EntryNode: "SteamDiscovery", Layer: "L7"},
	"SliMP3":                    {RuleFile: "application-layer/slimp3.yaml", EntryNode: "SliMP3", Layer: "L7"},
	"TDMoE":                     {RuleFile: "tdmoe.yaml", EntryNode: "TDMoE", Layer: "L2", FrameOffset: 14},
	"DECnet":                    {RuleFile: "decnet.yaml", EntryNode: "DECnet", Layer: "L2", FrameOffset: 14},
	"TIPC":                      {RuleFile: "tipc.yaml", EntryNode: "TIPC", Layer: "L2", FrameOffset: 14},
	"AARP":                      {RuleFile: "aarp.yaml", EntryNode: "AARP", Layer: "L2", FrameOffset: 22, InputLength: 28},
	"AppleTalk":                 {RuleFile: "appletalk.yaml", EntryNode: "DDP", Layer: "L2", FrameOffset: 22, InputLength: 23},
	"CFM":                       {RuleFile: "cfm.yaml", EntryNode: "CFM", Layer: "L2", FrameOffset: 14},
	"TRILL":                     {RuleFile: "trill.yaml", EntryNode: "TRILL", Layer: "L2", FrameOffset: 14},
	"XNS":                       {RuleFile: "xns.yaml", EntryNode: "XNS", Layer: "L2", FrameOffset: 14},
	"IGRP":                      {RuleFile: "igrp.yaml", EntryNode: "IGRP", Layer: "L3", FrameOffset: 34},
	"DVMRP":                     {RuleFile: "dvmrp.yaml", EntryNode: "DVMRP", Layer: "L3", FrameOffset: 34},
	"NHRP":                      {RuleFile: "nhrp.yaml", EntryNode: "NHRP", Layer: "L3", FrameOffset: 34},
	"IS-IS":                     {RuleFile: "isis.yaml", EntryNode: "ISIS", Layer: "L3", FrameOffset: 17},
	"Powerlink":                 {RuleFile: "powerlink.yaml", EntryNode: "Powerlink", Layer: "L2", FrameOffset: 14},
	"Mobile IP":                 {RuleFile: "mobility_samples.yaml", EntryNode: "MobileIP", Layer: "L4", FrameOffset: 42},
	"MIPv6":                     {RuleFile: "mobility_samples.yaml", EntryNode: "MIPv6Packet", Layer: "L3", FrameOffset: 14},
	"E-LMI":                     {RuleFile: "management_samples.yaml", EntryNode: "ELMI", Layer: "L2", FrameOffset: 14},
	"EOAM":                      {RuleFile: "management_samples.yaml", EntryNode: "EOAM", Layer: "L2", FrameOffset: 14},
	"MRP-MMRP":                  {RuleFile: "management_samples.yaml", EntryNode: "MMRP", Layer: "L2", FrameOffset: 14},
	"MRP-MVRP":                  {RuleFile: "management_samples.yaml", EntryNode: "MVRP", Layer: "L2", FrameOffset: 14},
	"MRP-MSRP":                  {RuleFile: "management_samples.yaml", EntryNode: "MSRP", Layer: "L2", FrameOffset: 14},
	"IPcomp":                    {RuleFile: "network_samples.yaml", EntryNode: "IPComp", Layer: "L3", FrameOffset: 34},
	"NVGRE":                     {RuleFile: "network_samples.yaml", EntryNode: "NVGRE", Layer: "L3", FrameOffset: 34},
	"MPLS PW":                   {RuleFile: "network_samples.yaml", EntryNode: "MPLSEthernetPW", Layer: "L2", FrameOffset: 14},
	"LACP-marker":               {RuleFile: "link_samples.yaml", EntryNode: "LACPMarker", Layer: "L2", FrameOffset: 14},
	"IEEE 802.3 Slow Protocols": {RuleFile: "link_samples.yaml", EntryNode: "SlowProtocols", Layer: "L2", FrameOffset: 14},
	"HomePlug AV":               {RuleFile: "link_samples.yaml", EntryNode: "HomePlugAV", Layer: "L2", FrameOffset: 14},
	"VNTAG":                     {RuleFile: "link_samples.yaml", EntryNode: "VNTag", Layer: "L2", FrameOffset: 14},
	"IEEE 802.1ah PBB":          {RuleFile: "link_samples.yaml", EntryNode: "ProviderBackboneBridge", Layer: "L2", FrameOffset: 14},
	"NBT SS":                    {RuleFile: "application-layer/nbss.yaml", EntryNode: "NBSS", Layer: "L4"},
	"Java serialization":        {RuleFile: "application-layer/java_ser.yaml", EntryNode: "JavaSer", Layer: "L7"},
	"NTLMSSP":                   {RuleFile: "application-layer/ntlm.yaml", EntryNode: "NTLMSSP", Layer: "L7", Base64After: []byte("Authorization: NTLM ")},
	"NTLM":                      {RuleFile: "application-layer/ntlm.yaml", EntryNode: "NTLMSSP", Layer: "L7", Base64After: []byte("WWW-Authenticate: NTLM ")},
	"NetNTLMv2":                 {RuleFile: "application-layer/ntlm.yaml", EntryNode: "NTLMSSP", Layer: "L7", Base64After: []byte("Authorization: NTLM ")},
	"NTLM v1/v2":                {RuleFile: "application-layer/ntlm.yaml", EntryNode: "NTLMSSP", Layer: "L7", Base64After: []byte("Authorization: NTLM ")},
	"SPNEGO":                    {RuleFile: "application-layer/spnego.yaml", EntryNode: "SPNEGO", Layer: "L7", Base64After: []byte("Authorization: Negotiate ")},
	"RSTP":                      {RuleFile: "stp.yaml", EntryNode: "STP", Layer: "L2", FrameOffset: 17},
	"Ethernet SNAP":             {RuleFile: "ethernet.yaml", EntryNode: "Ethernet", Layer: "L2", UseFullFrame: true},
	"SNAP":                      {RuleFile: "llc.yaml", EntryNode: "LLC", Layer: "L2", FrameOffset: 14},
	"Ethernet 802.2":            {RuleFile: "llc.yaml", EntryNode: "LLC", Layer: "L2", FrameOffset: 14},
	"Portmap/Rpcbind":           {RuleFile: "onc_rpc.yaml", EntryNode: "ONCRPC", Layer: "L7"},
	"Mount":                     {RuleFile: "onc_rpc.yaml", EntryNode: "ONCRPC", Layer: "L7"},
	"Bonjour":                   {RuleFile: "application-layer/dns.yaml", EntryNode: "DNS", Layer: "L7"},
	"ACMEv2":                    {RuleFile: "application-layer/acme.yaml", EntryNode: "ACMEJWSRequest", Layer: "L7"},
	"Kubernetes API":            {RuleFile: "application-layer/kubernetes_api.yaml", EntryNode: "KubernetesAPIRequest", Layer: "L7"},
	"WinRM HTTP":                {RuleFile: "application-layer/winrm.yaml", EntryNode: "WinRMHTTP", Layer: "L7"},
	"Redfish SSDP":              {RuleFile: "application-layer/redfish_ssdp.yaml", EntryNode: "RedfishSSDP", Layer: "L7"},
	"Submission":                {RuleFile: "application-layer/smtp.yaml", EntryNode: "SMTPCommand", Layer: "L7"},
	"CIFS":                      {RuleFile: "application-layer/cifs.yaml", EntryNode: "CIFSDirectTCP", Layer: "L7"},
	"GTPv2":                     {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GTPv2", Layer: "L7"},
	"Zabbix":                    {RuleFile: "zabbix.yaml", EntryNode: "Zabbix", Layer: "L7"},
	"Rsync":                     {RuleFile: "rsync.yaml", EntryNode: "Rsync", Layer: "L7"},
	"RMI":                       {RuleFile: "rmi.yaml", EntryNode: "RMIHeader", Layer: "L7"},
	"IIOP Locate":               {RuleFile: "application-layer/iiop.yaml", EntryNode: "GIOP", Layer: "L7"},
	"GSS-API":                   {RuleFile: "application-layer/gssapi.yaml", EntryNode: "GSSAPIHTTP", Layer: "L7"},
	"9P":                        {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "NinePVersion", Layer: "L7"},
	"CMPP":                      {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "CMPPConnect", Layer: "L7"},
	"Quake":                     {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "QuakeControl", Layer: "L7"},
	"Quake2":                    {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "QuakeConnectionless", Layer: "L7"},
	"Quake3":                    {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "QuakeConnectionless", Layer: "L7"},
	"Rlogin":                    {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "RloginRequest", Layer: "L7"},
	"TZSP":                      {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "TZSP", Layer: "L7"},
	"OLSR":                      {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "OLSRPacket", Layer: "L7"},
	"MSDP":                      {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "MSDPMessage", Layer: "L7"},
	"X11":                       {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "X11SetupLittle", Layer: "L7"},
	"iSCSI":                     {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "ISCSIBasicHeader", Layer: "L7"},
	"UCP/EMI":                   {RuleFile: "application-layer/sample_protocols.yaml", EntryNode: "UCPMessage", Layer: "L7"},
	"IPMI RMCP+":                {RuleFile: "ipmi.yaml", EntryNode: "IPMI", Layer: "L7"},
	"LLMNR response":            {EntryNode: "LLMNR"},
	"PAP":                       {EntryNode: "PAP", FrameOffset: 22},
	"STP":                       {EntryNode: "STP", FrameOffset: 17},
	"ETCD":                      {RuleFile: "application-layer/etcd.yaml", EntryNode: "EtcdVersionHTTP", Layer: "L7"},
	"MinIO/S3":                  {RuleFile: "application-layer/minio_s3.yaml", EntryNode: "S3SignatureV4Request", Layer: "L7"},
	"Docker API":                {RuleFile: "application-layer/docker_api.yaml", EntryNode: "DockerContainerListRequest", Layer: "L7"},
	"Redfish":                   {RuleFile: "application-layer/redfish.yaml", EntryNode: "RedfishServiceRootRequest", Layer: "L7"},
	"WPAD":                      {RuleFile: "application-layer/wpad.yaml", EntryNode: "WPADRequest", Layer: "L7"},
	"WPAD proxy":                {RuleFile: "application-layer/wpad.yaml", EntryNode: "WPADRequest", Layer: "L7"},
	"NVMe-oF":                   {EntryNode: "NVMeTCP"},
	"J1939":                     {UseFullFrame: true},
	"RARP":                      {UseFullFrame: true},
	"6to4":                      {FrameOffset: 14},
	"UDP-Lite":                  {FrameOffset: 14},
	"DCCP":                      {RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3", FrameOffset: 14},
	"eMule/ED2K":                {RuleFile: "application-layer/edonkey.yaml", EntryNode: "EDonkey", Layer: "L7", FrameOffset: 54},
	"XMPP":                      {RuleFile: "application-layer/xmpp.yaml", EntryNode: "XMPP", Layer: "L7", ReassembleTCP: true, ReassembleThroughFrame: 8},
	"IPv6 Hop-by-Hop":           {FrameOffset: 14},
	"IPv6 Destination Options":  {FrameOffset: 14},
	"IPv6 Routing Header":       {FrameOffset: 14},
	"IPv6 Fragment":             {FrameOffset: 14},
	"IPIP":                      {FrameOffset: 14},
	"COTP":                      {TrimPrefix: 4},
	"IEC 61850 SV":              {FrameOffset: 18},
	"SDP":                       {StartAfter: []byte("\r\n\r\n")},
	"Ethernet II":               {RuleFile: "ethernet.yaml", EntryNode: "Ethernet", Layer: "L2", UseFullFrame: true},
	"Ethernet 802.3":            {RuleFile: "ethernet.yaml", EntryNode: "Ethernet", Layer: "L2", UseFullFrame: true},
	"EAP":                       {RuleFile: "eapol.yaml", EntryNode: "EAPOL", Layer: "L2", FrameOffset: 14},
	"SNMPv3":                    {RuleFile: "application-layer/snmpv3.yaml", EntryNode: "SNMPv3", Layer: "L7"},
	"RSVP":                      {RuleFile: "application-layer/observed_protocols.yaml", EntryNode: "RSVP", Layer: "L3"},
	"SSL":                       {RuleFile: "application-layer/tls.yaml", Layer: "L7"},
	"IEEE 802.1X":               {RuleFile: "eapol.yaml", EntryNode: "EAPOL", Layer: "L2"},
	"LDAP":                      {EntryNode: "LDAPMessage"},
	"NBNS":                      {EntryNode: "NBNS"},
	"NBT NS":                    {EntryNode: "NBNS"},
	"IEEE 802.11":               {EntryNode: "Dot11", FrameOffset: 8},
	"CHAP":                      {FrameOffset: 22},
	"LCP":                       {FrameOffset: 22},
	"CDP":                       {FrameOffset: 22},
	"MPLS":                      {FrameOffset: 14},
	"SMB2":                      {EntryNode: "SMB2", TrimPrefix: 4},
	"ARP":                       {FrameOffset: 10},
	"AFP":                       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "AFP", Layer: "L7"},
	"ActiveMQ OpenWire":         {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "ActiveMQOpenWire", Layer: "L7"},
	"BGP":                       {FrameOffset: 48},
	"BFD":                       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "BFD", Layer: "L7"},
	"BACnet":                    {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "BACnetIP", Layer: "L7"},
	"Beckhoff ADS":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "BeckhoffADS", Layer: "L7"},
	"CAN/ISO-TP":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CANEthernet", Layer: "L7"},
	"Cassandra CQL":             {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CassandraCQL", Layer: "L7"},
	"Ceph":                      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CephConnect", Layer: "L7", ReassembleTCP: true},
	"Citrix ICA":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "CitrixICA", Layer: "L7"},
	"Collectd":                  {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "Collectd", Layer: "L7"},
	"DB2 DRDA":                  {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "DRDA", Layer: "L7"},
	"DLMS/COSEM":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "DLMSHDLC", Layer: "L7"},
	"DNP3":                      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "DNP3", Layer: "L7"},
	"Diameter":                  {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "Diameter", Layer: "L7"},
	"EtherNet/IP CIP":           {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "EtherNetIPCIPIO", Layer: "L7"},
	"FIX":                       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "FIX", Layer: "L7"},
	"GTP Prime":                 {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GTPPrime", Layer: "L7"},
	"GTP-C":                     {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GTPv2", Layer: "L7"},
	"GTP-U":                     {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GTPv1", Layer: "L7"},
	"GLBP":                      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GLBP", Layer: "L7"},
	"Git daemon":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "GitDaemon", Layer: "L7"},
	"Gnutella":                  {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "Gnutella", Layer: "L7"},
	"H.323":                     {RuleFile: "application-layer/h225.yaml", EntryNode: "H225Call", Layer: "L7"},
	"Hart-IP":                   {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "HartIP", Layer: "L7"},
	"IEC 60870-5-104":           {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IEC104", Layer: "L7"},
	"IEC 61850 MMS":             {RuleFile: "application-layer/mms.yaml", EntryNode: "MMSPDU", Layer: "L7", StartMagic: []byte{0xa8, 0x26}, InputLength: 40},
	"IPP":                       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IPP", Layer: "L7"},
	"IRC":                       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IRC", Layer: "L7"},
	"KNX/IP":                    {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "KNXIP", Layer: "L7"},
	"LDP":                       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "LDP", Layer: "L7"},
	"MELSEC":                    {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "MELSEC", Layer: "L7"},
	"MGCP":                      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "MGCP", Layer: "L7"},
	"MPEG-TS":                   {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "MPEGTransportStream", Layer: "L7"},
	"Modbus TCP":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "ModbusTCP", Layer: "L7"},
	"NATS":                      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "NATS", Layer: "L7"},
	"NetFlow v9":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "NetFlowV9", Layer: "L7"},
	"NNTP":                      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "NNTP", Layer: "L7"},
	"OPC UA":                    {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "OPCUAHello", Layer: "L7"},
	"Omron FINS":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "OmronFINS", Layer: "L7"},
	"PFCP":                      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "PFCP", Layer: "L7"},
	"PIM":                       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "PIM", Layer: "L3"},
	"Profinet IO":               {RuleFile: "application-layer/profinet_io.yaml", EntryNode: "ProfinetIOReadImplicit", Layer: "L7"},
	"RSH":                       {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "RSH", Layer: "L7"},
	"SCCP/Skinny":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "Skinny", Layer: "L7"},
	"S7comm":                    {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "S7comm", Layer: "L7"},
	"S7comm-plus":               {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "S7commPlus", Layer: "L7"},
	"SMPP":                      {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SMPPBind", Layer: "L7"},
	"SRVLOC/SLP":                {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SLPv2", Layer: "L7"},
	"STOMP":                     {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "STOMP", Layer: "L7"},
	"Teredo":                    {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "TeredoAuthentication", Layer: "L7"},
	// Frame 21's complete Megaco SDP Text node, including both alternatives.
	// This is advertisement-field evidence, not a T.38 media payload.
	"T.38":               {RuleFile: "application-layer/t38_sdp.yaml", EntryNode: "T38H248SDPAdvertisement", Layer: "L7", OuterOnly: true, FrameOffset: 210, InputLength: 161},
	"UPnP":               {RuleFile: "application-layer/upnp.yaml", EntryNode: "UPnPSSDPNotify", Layer: "L7"},
	"XDMCP":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "XDMCP", Layer: "L7"},
	"sFlow":              {RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "SFlowV5", Layer: "L7"},
	"FTP":                {EntryNode: "FTP"},
	"HTTP/2":             {EntryNode: "HTTP2Connection"},
	"JSON-RPC":           {StartMagic: []byte("{")},
	"JSON-RPC 2.0":       {RuleFile: "application-layer/jsonrpc_v2.yaml", EntryNode: "JSONRPC2", Layer: "L7", StartAfter: []byte("\r\n\r\n")},
	"XML-RPC":            {StartAfter: []byte("\r\n\r\n")},
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
	"scapy-zigbee-skke": {
		Name: "Zigbee", Contract: protocolCorpusParseContract{RuleFile: "zigbee.yaml", EntryNode: "ZigbeeFCS", Layer: "L2", UseFullFrame: true},
		RequiredNodes: []string{"MAC Frame Control", "Zigbee NWK", "Zigbee APS", "APS Frame Control", "APS Counter", "APS Command ID", "Transport Key Type", "Transport Key Material", "Transport Key Sequence Number", "Transport Destination Extended", "Transport Source Extended", "Frame Check Sequence"},
	},
	"ndpi-c37118": {
		Name:            "IEEE C37.118 synchrophasor",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/c37118.yaml", EntryNode: "C37118", Layer: "L7", FrameOffset: 66},
		RequiredNodes:   []string{"Sync", "Frame Type", "Version", "Frame Size", "ID Code", "Second Of Century", "Time Quality", "Fraction Of Second", "Command", "Checksum"},
		AlternateReason: "outside the roadmap; representative command fields and CRC are checked here. Separate every-record tests decode both CFG-2 frames and all data frames using explicit same-association configuration",
	},
	"ndpi-wsd-original": {
		Name:            "WS-Discovery",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/ws_discovery.yaml", EntryNode: "WSDiscovery", Layer: "L7", FrameOffset: 62},
		RequiredNodes:   []string{"XML Text"},
		AlternateReason: "the original unmodified SOAP Resolve uses discovery 2005/04 and addressing 2004/08, unlike the later-version schema. Whole-capture tests assert decoded header and endpoint fields independently",
	},
	"ndpi-dicom": {
		Name:            "DICOM Upper Layer",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/dicom.yaml", EntryNode: "DICOM", Layer: "L7", FrameOffset: 56},
		RequiredNodes:   []string{"PDU Type", "PDU Length", "Protocol Version", "Called AE Title", "Calling AE Title", "Application Context", "Presentation Context", "User Information", "Item Type", "Item Length", "Context ID", "Abstract Syntax", "Transfer Syntax", "UID", "Maximum PDU Length", "Implementation Class", "Implementation Version"},
		AlternateReason: "outside the roadmap; complete UL association negotiation, not an image dataset. A separate all-record test verifies both segmented PDUs as well as the representative message",
	},
	"gen-rmi-valid": {
		Name: "RMI", Contract: protocolCorpusParseContracts["RMI"],
		RequiredNodes: []string{"Magic", "Version", "Protocol", "Message", "Type", "Ser Magic", "Ser Version", "Block Length", "Object Number", "Operation", "Method Hash", "String"},
	},
	"google-ipx-session": {
		Name: "IPX",
		// Frame 1: Ethernet (14), LLC UI (3), declared IPX length (50),
		// then a separate nonzero link trailer. The whole-capture IPX test
		// derives and verifies both boundaries independently on every record.
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IPX", Layer: "L3", FrameOffset: 17, InputLength: 50},
		RequiredNodes: []string{"Checksum", "Length", "Transport Control", "Packet Type", "Destination Network", "Destination Node", "Destination Socket", "Source Network", "Source Node", "Source Socket", "Payload"},
	},
	"pr5023-gen-linux-sll2": {
		Name:          "Linux SLL2",
		Contract:      protocolCorpusParseContract{RuleFile: "linux_sll2.yaml", EntryNode: "LinuxSLL2", Layer: "L2", UseFullFrame: true},
		RequiredNodes: []string{"Protocol", "Reserved", "Interface Index", "ARPHRD Type", "Packet Type", "Address Length", "Address", "IP", "ICMP", "Identifier", "Sequence Number"},
	},
	"google-usb-engraver": {
		Name:            "USBPcap",
		Contract:        protocolCorpusParseContract{RuleFile: "usb_pcap.yaml", EntryNode: "USBPcap", Layer: "L2", UseFullFrame: true},
		RequiredNodes:   []string{"Header Length", "IRP ID", "Status", "Function", "Information", "Bus", "Device", "Endpoint", "Transfer Type", "Data Length", "Data"},
		AlternateReason: "the capture has no roadmap mapping; its actual format is USBPcap and the device-specific HID report data is retained without inferring a schema",
	},
	"ndpi-netbeui": {
		Name:          "NetBEUI",
		Contract:      protocolCorpusParseContract{RuleFile: "netbios_frame.yaml", EntryNode: "NetBIOSFrame", Layer: "L2", FrameOffset: 17},
		RequiredNodes: []string{"Header Length", "Delimiter", "Command", "Data 1", "Data 2", "Transmit Correlator", "Response Correlator", "Receiver Name Bytes", "Receiver Name Suffix", "Sender Name Bytes", "Sender Name Suffix"},
	},
	"ndpi-ethersio": {
		Name:            "Ether-S-I/O",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/ether_sio.yaml", EntryNode: "EtherSIO", Layer: "L7"},
		RequiredNodes:   []string{"Magic", "Telegram Type", "Version", "Length", "Transaction ID", "Telegram ID", "Source Station ID", "Transfer Count", "Transfer Flags", "Transfer ID", "Destination Station ID", "Data Length", "Data"},
		AlternateReason: "the capture has no ProtocolRoadmap mapping and exercises Ether-S-I/O data and RIO status datagrams",
	},
	"ndpi-ethersbus": {
		Name:            "Ether-S-Bus",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/ether_sbus.yaml", EntryNode: "EtherSBus", Layer: "L7"},
		RequiredNodes:   []string{"Length", "Version", "Protocol", "Sequence", "Attribute", "Destination", "Command", "Checksum"},
		AlternateReason: "the capture has no ProtocolRoadmap mapping and exercises Ether-S-Bus request/response records",
	},
	"gen-prometheus": {
		Name: "Prometheus exposition",
		// Frame 4 is only GET /metrics. Preserve that material representative,
		// but validate the exposition in the complete frame-5 response body.
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/prometheus.yaml", EntryNode: "PrometheusExposition", Layer: "L7", ReassembleTCP: true, ReassembleThroughFrame: 5, StartAfter: []byte("\r\n\r\n")},
		RequiredNodes: []string{"Exposition Text"},
	},
	"ndpi-jsonrpc": {
		Name:          "JSON-RPC",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/jsonrpc_v2.yaml", EntryNode: "JSONRPC2", Layer: "L7", StartAfter: []byte("\r\n\r\n")},
		RequiredNodes: []string{"JSON Text"},
	},
	"ndpi-checkmk": {
		Name:          "Checkmk",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/checkmk.yaml", EntryNode: "CheckmkReport", Layer: "L7", ReassembleTCP: true, ReassembleThroughFrame: 96},
		RequiredNodes: []string{"Lines", "Text"},
	},
	"ndpi-hl7": {
		Name:            "HL7 v2 MLLP",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/hl7.yaml", EntryNode: "HL7Message", Layer: "L7"},
		RequiredNodes:   []string{"Start Block", "Segments", "Segment ID", "Field Separator", "Fields", "End Block"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and directly exercises its MLLP-framed acknowledgment",
	},
	"ndpi-trdp": {
		Name:            "TRDP",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/trdp.yaml", EntryNode: "TRDPMessage", Layer: "L7"},
		RequiredNodes:   []string{"Sequence Counter", "Message Type", "Communication ID", "Dataset Length", "Session ID", "Source URI", "Header CRC", "Dataset", "Padding"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and directly exercises its MD request/reply messages",
	},
	"ndpi-egd": {
		Name:            "GE Ethernet Global Data",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/egd.yaml", EntryNode: "EGD", Layer: "L7"},
		RequiredNodes:   []string{"PDU Type", "Version", "Request ID", "Producer ID", "Exchange ID", "Timestamp Seconds", "Timestamp Nanoseconds", "Status", "Configuration Signature", "Exchange Data"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping; EGD is distinct from GE SRTP",
	},
	"ndpi-hislip": {
		Name:            "HiSLIP",
		Contract:        protocolCorpusParseContract{RuleFile: "application-layer/hislip.yaml", EntryNode: "HiSLIP", Layer: "L7"},
		RequiredNodes:   []string{"Prologue", "Message Type", "Control Code", "Parameter", "Payload Length", "Payload"},
		AlternateReason: "the source capture has no ProtocolRoadmap mapping and directly exercises its instrument-protocol initialization message",
	},
	"gen-6to4": {
		Name:          "6in4",
		Contract:      protocolCorpusParseContract{RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3", FrameOffset: 14},
		RequiredNodes: []string{"Total Length", "Protocol", "IPv6", "Source", "Destination", "Payload Length", "Next Header", "ICMPv6", "Type", "Identifier", "Sequence Number"},
	},
	"ws-git": {
		Name:          "Git daemon",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/observed_protocols.yaml", EntryNode: "GitPacketLines", Layer: "L7"},
		RequiredNodes: []string{"Packet Length", "Line"},
	},
	"ndpi-enip-cip": {
		Name:          "EtherNet/IP CIP",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/observed_protocols.yaml", EntryNode: "EtherNetIPEncapsulation", Layer: "L7"},
		RequiredNodes: []string{"Command", "Length", "Session Handle", "Status", "Sender Context", "Options", "Interface Handle", "Timeout", "Item Count", "Type ID", "Item Length", "Item Data"},
	},
	"ws-ipx-rip": {
		Name:          "IPX",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "IPX", Layer: "L3", FrameOffset: 14, InputLength: 40},
		RequiredNodes: []string{"Checksum", "Length", "Transport Control", "Packet Type", "Destination Network", "Destination Node", "Destination Socket", "Source Network", "Source Node", "Source Socket", "Payload"},
	},
	"gen-loopback": {
		Name:          "Loopback",
		Contract:      protocolCorpusParseContract{RuleFile: "packet_envelope.yaml", EntryNode: "Null IPv4 Envelope", Layer: "L2", UseFullFrame: true},
		RequiredNodes: []string{"Family Byte 1", "Family Byte 2", "Family Byte 3", "Family Byte 4", "Version", "Total Length", "Protocol", "Source", "Destination", "Payload"},
	},
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
		CaptureID:     "gen-fcoe-valid",
		Name:          "Fibre Channel",
		Contract:      protocolCorpusParseContracts["Fibre Channel"],
		RequiredNodes: []string{"R_CTL", "D_ID", "CS_CTL", "S_ID", "FC Type", "F_CTL", "SEQ_ID", "DF_CTL", "SEQ_CNT", "OX_ID", "RX_ID", "Parameter", "FC Data"},
	},
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

// Classification fixtures preserve representative bytes whose available
// material is sufficient for stable protocol identification but not for a
// field-level parser contract. Exact source and representative-frame hashes
// remain pinned by the corpus manifest; the invariants below additionally pin
// the protocol marker and boundary used by the coverage ledger.
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

	// These nDPI materials are retained as exact protocol-identification
	// fixtures. Their representative frames are checked in full so transport
	// extraction cannot accidentally turn an unrelated payload into a match.
	"ndpi-alicloud":   {Layer: "L2", UseFullFrame: true, ExactBytes: 74, Contains: []byte{0xce, 0xfa, 0xbe, 0xba}},
	"ndpi-atg":        {Layer: "L2", UseFullFrame: true, ExactBytes: 75},
	"ndpi-genshin":    {Layer: "L2", UseFullFrame: true, ExactBytes: 62},
	"ndpi-matter":     {Layer: "L2", UseFullFrame: true, ExactBytes: 134},
	"ndpi-meshtastic": {Layer: "L2", UseFullFrame: true, ExactBytes: 60},
	"ndpi-tristation": {Layer: "L2", UseFullFrame: true, ExactBytes: 48},
	"ndpi-tuya":       {Layer: "L2", UseFullFrame: true, ExactBytes: 230},
	"ndpi-umas":       {Layer: "L2", UseFullFrame: true, ExactBytes: 64},
	"ndpi-weibo":      {Layer: "L2", UseFullFrame: true, ExactBytes: 179},
	"ndpi-xiaomi":     {Layer: "L2", UseFullFrame: true, ExactBytes: 136},

	// Long-tail generated materials are identification fixtures until a public
	// field grammar and a bounded parser contract are available. Every case pins
	// the full representative frame plus a link-, network-, or payload marker.
	"pr5023-gen-isis": {Layer: "L2", UseFullFrame: true, ExactBytes: 44, AtOffset: map[int][]byte{14: {0xfe, 0xfe, 0x03}}},
}

// Upstream-negative captures must be rejected by the rule at the malformed
// layer. A new negative fixture has no default: it needs an explicit rule and
// extraction contract here before the corpus matrix accepts it.
var protocolCorpusRejectionSpecs = map[string]protocolCorpusRejectionSpec{
	"pr5023-gen-rmi": {
		Name: "RMI", Contract: protocolCorpusParseContracts["RMI"],
		ControlCaptureID: "gen-rmi-valid", ControlContract: protocolCorpusParseContracts["RMI"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "transport header has trailing bytes",
	},
	"pr5023-gen-smb3": {
		Name: "SMB3", Contract: protocolCorpusParseContracts["SMB3"],
		ControlCaptureID: "gen-smb3-valid", ControlContract: protocolCorpusParseContracts["SMB3"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "requires exactly one PREAUTH",
	},
	"pr5023-gen-cifs": {
		Name: "CIFS", Contract: protocolCorpusParseContracts["CIFS"],
		ControlCaptureID: "gen-cifs-valid", ControlContract: protocolCorpusParseContracts["CIFS"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "cifs: dialect ByteCount does not equal bounded data",
	},
	"pr5023-gen-xtp": {
		Name: "XTP", Contract: protocolCorpusParseContracts["XTP"],
		ControlCaptureID: "gen-xtp-valid", ControlContract: protocolCorpusParseContracts["XTP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "xtp: unsupported version",
	},
	"pr5023-gen-h225": {
		Name: "H.225", Contract: protocolCorpusParseContracts["H.225"],
		ControlCaptureID: "gen-h225-valid", ControlContract: protocolCorpusParseContracts["H.225"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "PER bit 16: nonzero alignment padding",
	},
	"pr5023-gen-sna": {
		Name: "SNA", Contract: protocolCorpusParseContracts["SNA"],
		ControlCaptureID: "gen-sna-valid", ControlContract: protocolCorpusParseContracts["SNA"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "sna: encapsulated length does not equal bounded LLC bytes",
	},
	"pr5023-gen-swipe": {
		Name: "swIPe", Contract: protocolCorpusParseContracts["swIPe"],
		ControlCaptureID: "gen-swipe-valid", ControlContract: protocolCorpusParseContracts["swIPe"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "swipe: invalid header word length",
	},
	"pr5023-gen-megaco": {
		Name: "Megaco/H.248", Contract: protocolCorpusParseContracts["Megaco/H.248"],
		ControlCaptureID: "gen-megaco-valid", ControlContract: protocolCorpusParseContracts["Megaco/H.248"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "empty Context structure",
	},
	"pr5023-gen-vines": {
		Name: "VINES", Contract: protocolCorpusParseContracts["VINES"],
		ControlCaptureID: "gen-vines-valid", ControlContract: protocolCorpusParseContracts["VINES"],
		ExpectedFailureClass: protocolCorpusFailureTruncated, ExpectedErrorContains: "vines: truncated 18-byte VIP header",
	},
	"pr5023-gen-sigcomp": {
		Name: "SIGCOMP", Contract: protocolCorpusParseContracts["SIGCOMP"],
		ControlCaptureID: "gen-sigcomp-valid", ControlContract: protocolCorpusParseContracts["SIGCOMP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "sigcomp: feedback exceeds message boundary",
	},
	"pr5023-gen-ncp": {
		Name: "NCP", Contract: protocolCorpusParseContracts["NCP"],
		ControlCaptureID: "gen-ncp-valid", ControlContract: protocolCorpusParseContracts["NCP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "ncp: connection control must be exactly 7 bytes",
	},
	"pr5023-gen-lat": {
		Name: "LAT", Contract: protocolCorpusParseContracts["LAT"],
		ControlCaptureID: "gen-lat-valid", ControlContract: protocolCorpusParseContracts["LAT"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "lat: Run circuit identifiers must be nonzero",
	},
	"pr5023-gen-giop": {
		Name: "IIOP Locate", Contract: protocolCorpusParseContracts["IIOP Locate"],
		ControlCaptureID: "gen-giop-valid", ControlContract: protocolCorpusParseContracts["IIOP Locate"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "giop: declared size does not fit message boundary",
	},
	"pr5023-gen-redfish-ssdp": {
		Name: "Redfish SSDP", Contract: protocolCorpusParseContracts["Redfish SSDP"],
		ControlCaptureID: "gen-redfish-ssdp-valid", ControlContract: protocolCorpusParseContracts["Redfish SSDP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "redfish-ssdp: unsupported Redfish service target",
	},
	"pr5023-gen-alljoyn": {
		Name: "AllJoyn", Contract: protocolCorpusParseContracts["AllJoyn"],
		ControlCaptureID: "gen-alljoyn-valid", ControlContract: protocolCorpusParseContracts["AllJoyn"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "alljoyn-ns: trailing bytes after declared records",
	},
	"pr5023-gen-fcoe": {
		Name: "FCoE", Contract: protocolCorpusParseContracts["FCoE"],
		ControlCaptureID: "gen-fcoe-valid", ControlContract: protocolCorpusParseContracts["FCoE"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "fcoe: incomplete minimum frame",
	},
	"pr5023-gen-fibre-channel": {
		Name: "Fibre Channel",
		// The retained frame contains only eighteen FC bytes. Its CRC and EOF
		// are outside this explicit boundary and cannot complete the header.
		Contract:         protocolCorpusParseContract{RuleFile: "fibre_channel.yaml", EntryNode: "FibreChannel", Layer: "L2", FrameOffset: 28, InputLength: 18},
		ControlCaptureID: "gen-fcoe-valid", ControlContract: protocolCorpusParseContracts["Fibre Channel"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "fc: incomplete fixed header",
	},
	"pr5023-gen-nat-t": {
		Name: "NAT-T", Contract: protocolCorpusParseContracts["NAT-T"],
		ControlCaptureID: "gen-nat-t-valid", ControlContract: protocolCorpusParseContracts["NAT-T"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "nat-t: IKE length must match the complete datagram after the marker",
	},
	"pr5023-gen-macsec": {
		Name: "MACSec", Contract: protocolCorpusParseContracts["MACSec"],
		ControlCaptureID: "gen-macsec-valid", ControlContract: protocolCorpusParseContracts["MACSec"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "macsec: zero short length requires at least 48 data bytes",
	},
	"pr5023-gen-wsmp": {
		Name: "WSMP", Contract: protocolCorpusParseContracts["WSMP"],
		ControlCaptureID: "gen-wsmp-valid", ControlContract: protocolCorpusParseContracts["WSMP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "wsmp: unsupported legacy extension identifier",
	},
	"pr5023-gen-slimp3": {
		Name: "SliMP3", Contract: protocolCorpusParseContracts["SliMP3"],
		ControlCaptureID: "gen-slimp3-valid", ControlContract: protocolCorpusParseContracts["SliMP3"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "slimp3: message is shorter than the 18-byte header",
	},
	"pr5023-gen-tdmoe": {
		Name: "TDMoE", Contract: protocolCorpusParseContracts["TDMoE"],
		ControlCaptureID: "gen-tdmoe-valid", ControlContract: protocolCorpusParseContracts["TDMoE"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "tdmoe: samples per channel must be eight",
	},
	"pr5023-gen-decnet": {
		Name: "DECnet", Contract: protocolCorpusParseContracts["DECnet"],
		ControlCaptureID: "gen-decnet-valid", ControlContract: protocolCorpusParseContracts["DECnet"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "decnet: unsupported data-packet format",
	},
	"pr5023-gen-tipc": {
		Name: "TIPC", Contract: protocolCorpusParseContracts["TIPC"],
		ControlCaptureID: "gen-tipc-valid", ControlContract: protocolCorpusParseContracts["TIPC"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "tipc: unsupported version",
	},
	"pr5023-gen-trill": {
		Name: "TRILL", Contract: protocolCorpusParseContracts["TRILL"],
		ControlCaptureID: "gen-trill-valid", ControlContract: protocolCorpusParseContracts["TRILL"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "trill: unsupported inner tag; C-tag required",
	},
	"pr5023-gen-xns": {
		Name: "XNS", Contract: protocolCorpusParseContracts["XNS"],
		ControlCaptureID: "gen-xns-valid", ControlContract: protocolCorpusParseContracts["XNS"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "xns: datagram wire size must be 30 through 65536 bytes",
	},
	"pr5023-gen-igrp": {
		Name: "IGRP", Contract: protocolCorpusParseContracts["IGRP"],
		ControlCaptureID: "gen-igrp-valid", ControlContract: protocolCorpusParseContracts["IGRP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "igrp: unsupported version",
	},
	"pr5023-gen-dvmrp": {
		Name: "DVMRP", Contract: protocolCorpusParseContracts["DVMRP"],
		ControlCaptureID: "gen-dvmrp-valid", ControlContract: protocolCorpusParseContracts["DVMRP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "dvmrp: checksum mismatch",
	},
	"pr5023-gen-appletalk": {
		Name: "AppleTalk", Contract: protocolCorpusParseContract{RuleFile: "appletalk.yaml", EntryNode: "DDP", Layer: "L2", FrameOffset: 14},
		ControlCaptureID: "gen-appletalk-valid", ControlContract: protocolCorpusParseContracts["AppleTalk"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "ddp: extended datagram size must be 13 through 599 bytes",
	},
	"pr5023-gen-cfm": {
		Name: "CFM", Contract: protocolCorpusParseContracts["CFM"],
		ControlCaptureID: "gen-cfm-valid", ControlContract: protocolCorpusParseContracts["CFM"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "cfm: CCM is shorter than its fixed body",
	},
	"pr5023-gen-nhrp": {
		Name: "NHRP", Contract: protocolCorpusParseContracts["NHRP"],
		ControlCaptureID: "gen-nhrp-valid", ControlContract: protocolCorpusParseContracts["NHRP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "nhrp: packet size outside 28..65535 bytes",
	},
	"pr5023-gen-aarp": {
		Name: "AARP", Contract: protocolCorpusParseContract{RuleFile: "aarp.yaml", EntryNode: "AARP", Layer: "L2", FrameOffset: 14},
		ControlCaptureID: "gen-aarp-valid", ControlContract: protocolCorpusParseContracts["AARP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "aarp: Ethernet AppleTalk message length must be 28 bytes",
	},
	"pr5023-gen-powerlink": {
		Name: "Powerlink", Contract: protocolCorpusParseContracts["Powerlink"],
		ControlCaptureID: "gen-powerlink-valid", ControlContract: protocolCorpusParseContracts["Powerlink"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "powerlink: invalid node address",
	},
	"pr5023-gen-mip": {
		Name: "Mobile IP", Contract: protocolCorpusParseContracts["Mobile IP"],
		ControlCaptureID: "gen-mip-valid", ControlContract: protocolCorpusParseContracts["Mobile IP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "missing required registration extension",
	},
	"pr5023-gen-mipv6": {
		Name: "MIPv6", Contract: protocolCorpusParseContracts["MIPv6"],
		ControlCaptureID: "gen-mipv6-valid", ControlContract: protocolCorpusParseContracts["MIPv6"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "Mobility Header length differs",
	},
	"pr5023-gen-elmi": {
		Name: "E-LMI", Contract: protocolCorpusParseContracts["E-LMI"],
		ControlCaptureID: "gen-elmi-valid", ControlContract: protocolCorpusParseContracts["E-LMI"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "version must be one",
	},
	"pr5023-gen-eoam": {
		Name: "EOAM", Contract: protocolCorpusParseContracts["EOAM"],
		ControlCaptureID: "gen-eoam-valid", ControlContract: protocolCorpusParseContracts["EOAM"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "missing local information TLV",
	},
	"pr5023-gen-mmrp": {
		Name: "MRP-MMRP", Contract: protocolCorpusParseContracts["MRP-MMRP"],
		ControlCaptureID: "gen-mmrp-valid", ControlContract: protocolCorpusParseContracts["MRP-MMRP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "invalid fixed attribute length",
	},
	"pr5023-gen-mvrp": {
		Name: "MRP-MVRP", Contract: protocolCorpusParseContracts["MRP-MVRP"],
		ControlCaptureID: "gen-mvrp-valid", ControlContract: protocolCorpusParseContracts["MRP-MVRP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "invalid fixed attribute length",
	},
	"pr5023-gen-msrp": {
		Name: "MRP-MSRP", Contract: protocolCorpusParseContracts["MRP-MSRP"],
		ControlCaptureID: "gen-msrp-valid", ControlContract: protocolCorpusParseContracts["MRP-MSRP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "invalid fixed attribute length",
	},
	"pr5023-gen-ipcomp": {
		Name: "IPcomp", Contract: protocolCorpusParseContracts["IPcomp"],
		ControlCaptureID: "gen-ipcomp-valid", ControlContract: protocolCorpusParseContracts["IPcomp"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "invalid DEFLATE payload",
	},
	"pr5023-gen-nvgre": {
		Name: "NVGRE", Contract: protocolCorpusParseContracts["NVGRE"],
		ControlCaptureID: "gen-nvgre-valid", ControlContract: protocolCorpusParseContracts["NVGRE"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "mandatory key flag",
	},
	"pr5023-gen-quake": {
		Name: "Quake", Contract: protocolCorpusParseContracts["Quake"],
		ControlCaptureID: "gen-quake-valid", ControlContract: protocolCorpusParseContracts["Quake"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "declared length does not match datagram",
	},
	"pr5023-gen-olsr": {
		Name: "OLSR", Contract: protocolCorpusParseContracts["OLSR"],
		ControlCaptureID: "gen-olsr-valid", ControlContract: protocolCorpusParseContracts["OLSR"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "packet length does not match input",
	},
	"pr5023-gen-msdp": {
		Name: "MSDP", Contract: protocolCorpusParseContracts["MSDP"],
		ControlCaptureID: "gen-msdp-valid", ControlContract: protocolCorpusParseContracts["MSDP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "message length does not match input",
	},
	"pr5023-gen-x11": {
		Name: "X11", Contract: protocolCorpusParseContracts["X11"],
		ControlCaptureID: "gen-x11-valid", ControlContract: protocolCorpusParseContracts["X11"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "authorization lengths do not match input",
	},
	"pr5023-gen-iscsi": {
		Name: "iSCSI", Contract: protocolCorpusParseContracts["iSCSI"],
		ControlCaptureID: "gen-iscsi-valid", ControlContract: protocolCorpusParseContracts["iSCSI"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "declared AHS/data lengths do not match input",
	},
	"pr5023-gen-ucp": {
		Name: "UCP/EMI", Contract: protocolCorpusParseContracts["UCP/EMI"],
		ControlCaptureID: "gen-ucp-valid", ControlContract: protocolCorpusParseContracts["UCP/EMI"],
		ExpectedFailureClass: protocolCorpusFailureTruncated, ExpectedErrorContains: "delimiter not found within field boundary",
	},
	"pr5023-gen-ipmi-rmcpplus": {
		Name: "IPMI RMCP+", Contract: protocolCorpusParseContracts["IPMI RMCP+"],
		ControlCaptureID: "gen-ipmi-rmcpplus-valid", ControlContract: protocolCorpusParseContracts["IPMI RMCP+"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "invalid message length",
	},
	"pr5023-gen-nbt-ss": {
		Name: "NBT SS", Contract: protocolCorpusParseContracts["NBT SS"],
		ControlCaptureID: "gen-nbt-ss-valid", ControlContract: protocolCorpusParseContracts["NBT SS"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "session request must contain two encoded names",
	},
	"pr5023-gen-java-ser": {
		Name: "Java serialization", Contract: protocolCorpusParseContracts["Java serialization"],
		ControlCaptureID: "gen-java-ser-valid", ControlContract: protocolCorpusParseContracts["Java serialization"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "content type requires an unsupported complete descriptor",
	},
	"pr5023-gen-ntlmssp": {
		Name: "NTLMSSP", Contract: protocolCorpusParseContracts["NTLMSSP"],
		ControlCaptureID: "gen-ntlmssp-valid", ControlContract: protocolCorpusParseContracts["NTLMSSP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "NEGOTIATE_VERSION requires Version",
	},
	"pr5023-gen-ntlm": {
		Name: "NTLM", Contract: protocolCorpusParseContracts["NTLM"],
		ControlCaptureID: "gen-ntlm-valid", ControlContract: protocolCorpusParseContracts["NTLM"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "NEGOTIATE_VERSION requires Version",
	},
	"pr5023-gen-netntlmv2": {
		Name: "NetNTLMv2", Contract: protocolCorpusParseContracts["NetNTLMv2"],
		ControlCaptureID: "gen-netntlmv2-valid", ControlContract: protocolCorpusParseContracts["NetNTLMv2"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "authentication message is missing NegotiateFlags",
	},
	"pr5023-gen-ntlm-v2": {
		Name: "NTLM v1/v2", Contract: protocolCorpusParseContracts["NTLM v1/v2"],
		ControlCaptureID: "gen-ntlm-v2-valid", ControlContract: protocolCorpusParseContracts["NTLM v1/v2"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "authentication message is missing NegotiateFlags",
	},
	"pr5023-gen-gtpv2": {
		Name: "GTPv2", Contract: protocolCorpusParseContracts["GTPv2"],
		ControlCaptureID: "gen-gtpv2-valid", ControlContract: protocolCorpusParseContracts["GTPv2"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "TEID flag requires the extended header",
	},
	"pr5023-gen-winrm-http": {
		Name: "WinRM HTTP", Contract: protocolCorpusParseContracts["WinRM HTTP"],
		ControlCaptureID: "gen-winrm-identify-valid", ControlContract: protocolCorpusParseContracts["WinRM HTTP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "Content-Length does not match exact record body",
	},
	"pr5023-gen-acme": {
		Name: "ACMEv2", Contract: protocolCorpusParseContracts["ACMEv2"],
		ControlCaptureID: "gen-acme-jws-valid", ControlContract: protocolCorpusParseContracts["ACMEv2"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "exactly protected, payload and signature members",
	},
	"gen-etcd": {
		Name: "ETCD", Contract: protocolCorpusParseContracts["ETCD"],
		ControlCaptureID: "gen-etcd-version-valid", ControlContract: protocolCorpusParseContracts["ETCD"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "version endpoint path must be /version, not /v3/version",
	},
	"gen-minio-s3": {
		Name: "MinIO/S3", Contract: protocolCorpusParseContracts["MinIO/S3"],
		ControlCaptureID: "gen-minio-s3-valid", ControlContract: protocolCorpusParseContracts["MinIO/S3"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "missing authorization parameter SignedHeaders",
	},
	"gen-winrm-http-valid": {
		Name: "WinRM HTTP", Contract: protocolCorpusParseContracts["WinRM HTTP"],
		ControlCaptureID: "gen-winrm-identify-valid", ControlContract: protocolCorpusParseContracts["WinRM HTTP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "unbound namespace prefix",
	},
	"pr5023-gen-gssapi-http": {
		Name: "GSS-API", Contract: protocolCorpusParseContracts["GSS-API"],
		ControlCaptureID: "gen-gssapi-valid", ControlContract: protocolCorpusParseContracts["GSS-API"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "missing SPNEGO negotiation token after mechanism OID",
	},
	"pr5023-gen-spnego": {
		Name: "SPNEGO", Contract: protocolCorpusParseContracts["SPNEGO"],
		ControlCaptureID: "gen-spnego-valid", ControlContract: protocolCorpusParseContracts["SPNEGO"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "SPNEGO mechanism is missing NegotiationToken",
	},
	"gen-rarp": {
		Name: "RARP", Contract: protocolCorpusParseContract{RuleFile: "rarp.yaml", EntryNode: "RARPFrame", Layer: "L2", UseFullFrame: true},
		ControlCaptureID: "gen-rarp-valid", ControlContract: protocolCorpusParseContract{RuleFile: "rarp.yaml", EntryNode: "RARPFrame", Layer: "L2", UseFullFrame: true},
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "rarp: wrong EtherType",
	},
	"gen-udplite": {
		Name: "UDP-Lite", Contract: protocolCorpusParseContract{RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3", FrameOffset: 14},
		ControlCaptureID: "gen-udplite-valid", ControlContract: protocolCorpusParseContract{RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3", FrameOffset: 14},
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "udp-lite: checksum mismatch",
	},
	"pr5023-gen-dccp": {
		Name: "DCCP", Contract: protocolCorpusParseContracts["DCCP"],
		ControlCaptureID: "gen-dccp-valid", ControlContract: protocolCorpusParseContracts["DCCP"],
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "dccp: packet type requires extended sequence numbers",
	},
	"gen-cldap": {
		Name: "CLDAP", Contract: protocolCorpusParseContract{RuleFile: "application-layer/cldap.yaml", EntryNode: "CLDAP", Layer: "L7"},
		ControlCaptureID: "gen-cldap-valid", ControlContract: protocolCorpusParseContract{RuleFile: "application-layer/cldap.yaml", EntryNode: "CLDAP", Layer: "L7"},
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "cldap: operation is not a search request",
	},
	"gen-aoe": {
		Name: "AoE", Contract: protocolCorpusParseContract{RuleFile: "aoe.yaml", EntryNode: "AoE", Layer: "L2"},
		ControlCaptureID: "gen-aoe-valid", ControlContract: protocolCorpusParseContract{RuleFile: "aoe.yaml", EntryNode: "AoE", Layer: "L2"},
		ExpectedFailureClass: protocolCorpusFailureInvalidValue, ExpectedErrorContains: "aoe: only query-config command is decoded",
	},
	"gen-llc": {
		Name:                  "LLC",
		Contract:              protocolCorpusParseContract{RuleFile: "llc.yaml", EntryNode: "LLC", Layer: "L2", FrameOffset: 14},
		ControlCaptureID:      "gen-llc-valid",
		ControlContract:       protocolCorpusParseContract{RuleFile: "llc.yaml", EntryNode: "LLC", Layer: "L2", FrameOffset: 14},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "over max size",
	},
	"gen-ieee8021x": {
		Name:                  "IEEE 802.1X",
		Contract:              protocolCorpusParseContract{RuleFile: "eapol.yaml", EntryNode: "EAPOL", Layer: "L2", FrameOffset: 14},
		ControlCaptureID:      "gen-ieee8021x-valid",
		ControlContract:       protocolCorpusParseContract{RuleFile: "eapol.yaml", EntryNode: "EAPOL", Layer: "L2", FrameOffset: 14},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "over max size",
	},
	"gen-cdp": {
		Name:                  "CDP",
		Contract:              protocolCorpusParseContract{RuleFile: "cdp.yaml", EntryNode: "CDP", Layer: "L2", FrameOffset: 22},
		ControlCaptureID:      "gen-cdp-valid",
		ControlContract:       protocolCorpusParseContract{RuleFile: "cdp.yaml", EntryNode: "CDP", Layer: "L2", FrameOffset: 22},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "over max size",
	},
	"gen-rsvp": {
		Name:                  "RSVP",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/observed_protocols.yaml", EntryNode: "RSVP", Layer: "L3"},
		ControlCaptureID:      "gen-rsvp-valid",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/observed_protocols.yaml", EntryNode: "RSVP", Layer: "L3"},
		ExpectedFailureClass:  protocolCorpusFailureInvalidValue,
		ExpectedErrorContains: "rsvp: invalid object length",
	},
	"gen-snmpv3": {
		Name:                  "SNMPv3",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/snmpv3.yaml", EntryNode: "SNMPv3", Layer: "L7"},
		ControlCaptureID:      "gen-snmpv3-valid",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/snmpv3.yaml", EntryNode: "SNMPv3", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureInvalidValue,
		ExpectedErrorContains: "snmpv3: header length mismatch",
	},
	"gen-ssl": {
		Name:                  "SSL",
		Contract:              protocolCorpusParseContract{RuleFile: "application-layer/tls.yaml", EntryNode: "Transport Layer Security", Layer: "L7"},
		ControlCaptureID:      "gen-ssl-valid",
		ControlContract:       protocolCorpusParseContract{RuleFile: "application-layer/tls.yaml", EntryNode: "Transport Layer Security", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "over max size",
	},
	"gen-l2tp": {
		Name:                  "L2TP",
		Contract:              protocolCorpusParseContract{RuleFile: "l2tp.yaml", EntryNode: "L2TP", Layer: "L7"},
		ControlCaptureID:      "gen-l2tp-valid",
		ControlContract:       protocolCorpusParseContract{RuleFile: "l2tp.yaml", EntryNode: "L2TP", Layer: "L7"},
		ExpectedFailureClass:  protocolCorpusFailureLengthBounds,
		ExpectedErrorContains: "over max size",
	},
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
	"SMB3":                      {"ProtocolId", "Header StructureSize", "Command", "Negotiate Request", "DialectCount", "ClientGuid", "ClientStartTime Reserved", "Dialects"},
	"XTP":                       {"Key", "Command", "NOCHECK", "BTAG", "Version", "Packet Format", "Data Length", "Checksum", "Sort", "Synchronization", "Sequence"},
	"H.225":                     {"RasMessage Choice", "requestSeqNum", "protocolIdentifier Bytes", "ip Bytes", "port"},
	"XYPLEX":                    {"Protocol Type", "Padding", "Server Port", "Return Port", "Reserved"},
	"SNA":                       {"SNA Ethernet Length", "SNA Ethernet Padding", "DSAP", "SSAP", "LLC Control", "LLC Extended Control", "Format Identification", "Begin BIU", "End BIU", "Destination Local Address", "Origin Local Address", "Sequence Number", "Response Indicator", "RU Category", "Uninterpreted RU Bytes"},
	"swIPe":                     {"Packet Type", "Header Length Words", "Policy Identifier", "Inner IP Version", "Inner Header Length Words", "Inner Total Length", "Inner Source", "Inner Destination", "Inner Payload"},
	"Megaco/H.248":              {"Protocol Marker", "Version", "Address", "Port", "Transaction ID", "Context ID", "Termination ID", "Service Change Method", "Service Change Reason"},
	"Zigbee":                    {"MAC Frame Control", "MAC Sequence Number", "MAC Destination PAN", "MAC Destination Short", "MAC Source Short", "Zigbee NWK", "NWK Frame Control", "NWK Destination", "NWK Source", "NWK Radius", "NWK Sequence Number", "NWK Source Extended", "NWK Auxiliary Header", "Auxiliary Control", "Auxiliary Frame Counter", "Auxiliary Source Extended", "Auxiliary Key Sequence Number", "Protected Payload and MIC"},
	"VINES":                     {"Checksum", "Packet Length", "Control High Bit", "Control Flags", "Hop Count", "Protocol", "Destination Network", "Destination Subnetwork", "Source Network", "Source Subnetwork", "Source Port", "Destination Port", "IPC Packet Type", "Datagram Padding", "IPC Data"},
	"SIGCOMP":                   {"Signature", "Feedback Present", "State ID Length Selector", "Partial State Identifier", "Compressed Data"},
	"WEP":                       {"Initialization Vector", "Key Index", "Extended IV", "Reserved", "Encrypted Data", "Encrypted ICV"},
	"NCP":                       {"Packet Type", "Sequence Number", "Connection Number Low", "Task Number", "Reserved", "Request Code"},
	"LAT":                       {"Message Type", "Master", "Response Required", "Slot Count", "Destination Circuit ID", "Source Circuit ID", "Message Sequence Number", "Message Acknowledgment Number"},
	"SEBEK":                     {"Magic", "Version", "Record Type", "Counter", "Time Seconds", "Time Microseconds", "Process ID", "User ID", "File Descriptor", "Command Name", "Data Length", "Record Data"},
	"AllJoyn":                   {"Sender Version", "Message Version", "Question Count", "Answer Count", "Timer", "Questions", "Question Type", "Question Reserved", "T Flag", "U Flag", "S Flag", "F Flag", "Name Count", "Names", "Byte Length", "Text"},
	"FCoE":                      {"FCoE Version", "FCoE Version Reserved", "FCoE Header Reserved", "SOF", "FC Frame", "R_CTL", "D_ID", "S_ID", "F_CTL", "FC Data", "FC CRC32", "EOF", "FCoE Trailer Reserved"},
	"Fibre Channel":             {"R_CTL", "D_ID", "CS_CTL", "S_ID", "FC Type", "F_CTL", "SEQ_ID", "DF_CTL", "SEQ_CNT", "OX_ID", "RX_ID", "Parameter", "FC Data"},
	"NAT-T":                     {"Marker or SPI", "Initiator SPI", "Responder SPI", "Next Payload", "Version", "Exchange Type", "Flags", "Message ID", "IKE Length", "Payloads", "Notify Type", "Nonce Data"},
	"MACSec":                    {"Version", "End Station", "SCI Present", "Single Copy Broadcast", "Encrypted", "Changed Text", "Association Number", "Reserved", "Short Length", "Packet Number", "Inner EtherType", "Clear Data", "ICV"},
	"WSMP":                      {"Version", "Encoded PSID", "WAVE Element ID", "WSM Length", "WSM Data"},
	"Steam":                     {"Signature", "Header Length", "Header", "Body Length", "Body", "client_id", "msg_type", "instance_id", "hostname", "ostype", "users", "steamid", "auth_key_id", "mac_addresses", "ip_addresses"},
	"SliMP3":                    {"Opcode", "Discovery Reserved", "Device ID", "Firmware Revision", "Discovery Request Reserved", "Client MAC"},
	"TDMoE":                     {"Subaddress", "Samples Per Channel", "Signaling Bits Present", "Packet Counter", "Channel Count", "Channel Samples"},
	"DECnet":                    {"Octet Count", "Format", "Destination Node", "Source Node", "Visit Count", "NSP Payload"},
	"TIPC":                      {"Version", "User", "Header Size Words", "Message Size", "Message Type", "Error Code", "Broadcast ACK", "Link ACK", "Link Sequence", "Previous Node", "Origin Port", "Destination Port", "Application Data"},
	"AppleTalk":                 {"Reserved", "Hop Count", "Datagram Length", "Checksum", "Destination Network", "Source Network", "Destination Node", "Source Node", "Destination Socket", "Source Socket", "DDP Type", "DDP Data"},
	"TRILL":                     {"TRILL Version", "TRILL Reserved", "Multi Destination", "Option Length", "TRILL Hop Count", "Egress Nickname", "Ingress Nickname", "Inner Destination", "Inner Source", "Inner Tag Type", "Inner Priority", "Inner C", "Inner VLAN ID", "Inner EtherType"},
	"DVMRP":                     {"Version", "Type", "Code", "Checksum", "Command Type", "Address Family", "Metric", "Infinity", "Subnet Mask", "Destination Unreachable", "Split Horizon Concealed", "Flags Reserved", "Count", "Destination Address"},
	"XNS":                       {"Checksum", "Datagram Length", "Reserved", "Hop Count", "Packet Type", "Destination Network", "Destination Host", "Destination Socket", "Source Network", "Source Host", "Source Socket", "Echo Operation", "Echo Data", "Garbage Byte"},
	"IGRP":                      {"Version", "Opcode", "Edition", "Autonomous System", "Interior Count", "System Count", "Exterior Count", "Checksum", "Interior Routes", "System Routes", "Exterior Routes", "Number", "Delay", "Bandwidth", "MTU", "Reliability", "Load", "Hop Count"},
	"CFM":                       {"MD Level", "Version", "Opcode", "First TLV Offset", "Sequence Number", "MEP ID", "Maintenance Association Identifier", "TLVs"},
	"NHRP":                      {"Address Family", "Protocol Type", "Protocol SNAP", "Hop Count", "Packet Length", "Checksum", "Extension Offset", "Version", "Message Type", "Source NBMA Type Length", "Source Subaddress Type Length", "Source Protocol Length", "Destination Protocol Length", "Source NBMA Address", "Source Protocol Address", "Destination Protocol Address", "Flags", "Request ID", "Clients"},
	"IS-IS":                     {"Intradomain Routing Protocol Discriminator", "Length Indicator", "Version Protocol ID Extension", "ID Length", "PDU Type", "Version", "Maximum Area Addresses", "Circuit Type", "Source System ID", "Holding Timer", "PDU Length", "Priority", "LAN ID", "Area Addresses", "Protocols Supported", "IPv4 Interface Addresses"},
	"Powerlink":                 {"Message Type", "Destination Node", "Source Node", "Cycle Flags", "Net Time Seconds", "Net Time Nanoseconds", "Relative Time"},
	"AARP":                      {"Hardware Type", "Protocol Type", "Hardware Length", "Protocol Length", "Function", "Source Hardware Address", "Source Protocol Reserved", "Source Network", "Source Node", "Destination Hardware Address", "Destination Protocol Reserved", "Destination Network", "Destination Node"},
	"Mobile IP":                 {"Message Type", "Lifetime", "Home Address", "Home Agent", "Identification", "Extensions", "SPI"},
	"MIPv6":                     {"Version", "Payload Length", "Next Header", "Source Address", "Destination Address", "Mobility Header", "MH Type", "Checksum"},
	"E-LMI":                     {"Version", "Message Type", "Report Type", "Send Sequence", "Receive Sequence", "Data Instance"},
	"EOAM":                      {"Subtype", "Code", "OAM Version", "Revision", "Maximum PDU Size", "Organization ID", "Vendor Information"},
	"MRP-MMRP":                  {"Protocol Version", "Attribute Type", "Attribute Length", "Number of Values", "Service Requirement", "First MAC Address", "Packed Events"},
	"MRP-MVRP":                  {"Protocol Version", "Attribute Type", "Attribute Length", "Number of Values", "First VLAN ID", "Packed Events"},
	"MRP-MSRP":                  {"Protocol Version", "Attribute List Length", "Stream ID", "Destination MAC", "Maximum Frame Size", "Bridge ID", "Failure Code", "Packed Declarations", "Class ID"},
	"IPcomp":                    {"Next Header", "Flags", "Compression Parameter Index", "Compressed Payload"},
	"NVGRE":                     {"Flags And Version", "Protocol Type", "Virtual Subnet ID", "Flow ID", "Inner Frame"},
	"MPLS PW":                   {"Label Stack", "Control Word", "Inner Frame"},
	"LACP-marker":               {"Subtype", "Version", "TLV Type", "TLV Length", "Requester Port", "Requester System", "Transaction ID", "Requester Pad", "Terminator Type", "Terminator Length", "Reserved Padding"},
	"IEEE 802.3 Slow Protocols": {"Subtype", "Version", "Actor", "Partner", "Collector Maximum Delay", "Terminator Type", "Reserved Padding"},
	"HomePlug AV":               {"Version", "Message Type", "Fragment Count", "Fragment Index", "Fragment Sequence", "Message Data"},
	"VNTAG":                     {"Direction", "Pointer", "Destination Interface", "Looped", "Reserved", "Version", "Source Interface", "Protocol Type", "Protocol Data"},
	"IEEE 802.1ah PBB":          {"Priority", "Drop Eligible", "No Customer Addresses", "Reserved 1", "Reserved 2", "Service ID", "Customer Ethernet", "Total Length", "Identifier"},
	"NBT SS":                    {"Type", "Flags", "Length", "Session Request", "Called Name", "Calling Name", "Encoded Length", "Encoded Name", "Terminator"},
	"Java serialization":        {"Magic", "Version", "JavaContent", "Content Type"},
	"NTLMSSP":                   {"Signature", "MessageType", "NegotiateFlags", "DomainNameFields", "WorkstationFields"},
	"NTLM":                      {"Signature", "MessageType", "TargetNameFields", "NegotiateFlags", "ServerChallenge", "TargetInfoFields"},
	"NetNTLMv2":                 {"Signature", "MessageType", "NtChallengeResponseFields", "NegotiateFlags", "NT Challenge Response", "NTProofStr", "RespType", "HiRespType", "TimeStamp", "ClientChallenge"},
	"NTLM v1/v2":                {"Signature", "MessageType", "NtChallengeResponseFields", "NegotiateFlags", "NT Challenge Response", "NTProofStr", "RespType", "HiRespType", "TimeStamp", "ClientChallenge"},
	"SPNEGO":                    {"Tag", "Length", "OID Tag", "OID Length", "OID", "Token", "SPNEGOInit", "Sequence Tag", "MechOID"},
	"RSTP":                      {"Protocol ID", "Version", "BPDU Type", "Flags", "Root MAC", "Root Path Cost", "Bridge MAC", "Port ID"},
	"Ethernet SNAP":             {"Destination", "Source", "Type", "DSAP", "SSAP", "Control", "OUI", "IP", "ICMP"},
	"SNAP":                      {"DSAP", "SSAP", "Control", "OUI", "Type", "SNAP Payload"},
	"Ethernet 802.2":            {"DSAP", "SSAP", "Control", "LLC Payload"},
	"Portmap/Rpcbind":           {"XID", "Message Type", "RPC Version", "Program", "Program Version", "Procedure", "Cred Flavor", "Cred Length", "Verf Flavor", "Verf Length"},
	"Mount":                     {"XID", "Message Type", "RPC Version", "Program", "Program Version", "Procedure", "Cred Flavor", "Cred Length", "Verf Flavor", "Verf Length"},
	"Bonjour":                   {"ID", "Flags", "Questions", "Text", "Type", "Class"},
	"ACMEv2":                    {"HTTP Request", "Method", "Path", "Version", "Headers", "Body", "Octets"},
	"Kubernetes API":            {"Method", "Path", "API Prefix", "API Version", "Resource", "Version", "HTTP Headers"},
	"WinRM HTTP":                {"Method", "Path", "Version", "HTTP Headers", "Content Length", "SOAP", "Envelope", "Header", "Body", "Identify"},
	"Redfish SSDP":              {"Method", "Target", "Version", "Headers", "Name", "Value", "Message End"},
	"Submission":                {"Line"},
	"CIFS":                      {"Direct TCP Length", "ProtocolId", "Command", "Status", "Flags", "Flags2", "SecurityFeatures", "WordCount", "ByteCount", "Dialects", "BufferFormat", "Dialect Name"},
	"GTPv2":                     {"Flags", "Message Type", "Payload Length", "Sequence Number", "Spare"},
	"Zabbix":                    {"Magic", "Flags", "Length", "Reserved", "JSON"},
	"Rsync":                     {"Magic", "Space", "Major", "Minor"},
	"RMI":                       {"Magic", "Version", "Protocol"},
	"IIOP Locate":               {"Magic", "Major", "Minor", "Flags", "Message Type", "Message Size", "Request ID", "Key Len", "Object Key"},
	"GSS-API":                   {"HTTP Request", "Method", "Path", "Version", "Headers"},
	"9P":                        {"Size", "Type", "Tag", "Msize", "Version Length", "Version"},
	"CMPP":                      {"Total Length", "Command ID", "Sequence ID", "Source Address", "Authenticator Source", "Version", "Timestamp"},
	"Quake":                     {"Length and Flags", "Control Data"},
	"Quake2":                    {"Marker", "Command"},
	"Quake3":                    {"Marker", "Command"},
	"Rlogin":                    {"Leading Zero", "Client User", "Server User", "Terminal"},
	"TZSP":                      {"Version", "Type", "Encapsulation", "Ethernet Frame"},
	"OLSR":                      {"Packet Length", "Packet Sequence Number"},
	"MSDP":                      {"Type", "Length"},
	"X11":                       {"Byte Order", "Protocol Major", "Protocol Minor", "Authorization Name Length", "Authorization Data Length"},
	"iSCSI":                     {"Opcode", "Flags", "Total AHS Length", "Data Length High", "Data Length Middle", "Data Length Low", "Header Fields", "Data"},
	"UCP/EMI":                   {"STX", "Transaction Reference", "Length Text", "Operation", "Operation Type", "Data", "Checksum", "ETX"},
	"IPMI RMCP+":                {"Version", "Class", "Authentication Type", "Payload Type", "RMCP+ Session ID", "RMCP+ Session Sequence", "Payload Length", "Open Session Request", "Console Session ID", "Authentication Algorithm", "Integrity Algorithm", "Confidentiality Algorithm"},
	"JDWP":                      {"Handshake"},
	"LLMNR response":            {"ID", "Flags", "Questions", "Answer RRs", "Text", "Type", "Class", "TTL", "RDLength", "Address"},
	"Linux SLL":                 {"Packet Type", "ARPHRD", "Address Length", "Source MAC", "Unused", "Protocol", "IP", "ICMP", "Identifier"},
	"PAP":                       {"Code", "Identifier", "Length", "Peer ID Length", "Peer ID", "Password Length", "Password"},
	"STP":                       {"Protocol ID", "Version", "BPDU Type", "Flags", "Root MAC", "Root Path Cost", "Bridge MAC", "Port ID", "Message Age", "Max Age", "Hello Time", "Forward Delay"},
	"TACACS+":                   {"Version", "Type", "Sequence", "Flags", "Session ID", "Length", "Obfuscated Body"},
	"Redis":                     {"Prefix", "Count", "Len", "Command", "CRLF"},
	"ETCD":                      {"Version", "HTTP Headers"},
	"MinIO/S3":                  {"Method", "Request Target", "HTTP Version", "Algorithm", "Access Key ID", "Scope Date", "Region", "Service", "Scope Terminator", "Signed Header 0", "Signed Header 1", "Signature Hex", "Payload"},
	"Docker API":                {"HTTP Request", "Method", "Path", "Version", "Headers", "Item"},
	"Redfish":                   {"HTTP Request", "Method", "Path", "Version", "Headers", "Item"},
	"WPAD":                      {"HTTP Request", "Method", "Path", "Version", "Headers", "Item"},
	"LPD":                       {"Command", "Queue"},
	"NVMe-oF":                   {"PDU Type", "Flags", "Header Length", "PDU Data Offset", "PDU Length", "Opcode", "Command Flags", "Command ID", "Namespace ID", "CDW2", "CDW3", "Metadata Pointer", "Data Pointer", "CDW10", "CDW11", "CDW12", "CDW13", "CDW14", "CDW15"},
	"IPFIX":                     {"Version", "Length", "Export Time", "Sequence Number", "Observation Domain ID", "Set ID", "Set Length", "Template ID", "Field Count", "Enterprise Bit", "Information Element ID", "Field Length"},
	"LLMNR":                     {"ID", "Flags", "Questions", "Text", "Type", "Class"},
	"LLMNR-MDNS collision":      {"ID", "Flags", "Questions", "Text", "Type", "Class"},
	"OSPFv3":                    {"Version", "Type", "Packet Length", "Router ID", "Area ID", "Checksum", "Instance ID", "Interface ID", "Router Priority", "Options", "Hello Interval", "Router Dead Interval", "Designated Router", "Backup Designated Router"},
	"J1939":                     {"Extended Flag", "Priority", "Data Page", "PDU Format", "PDU Specific", "Source Address", "Data Length", "Data"},
	"IPv6 Hop-by-Hop":           {"Hop By Hop Options", "Header Extension Length", "Options", "Type", "ICMPv6", "Identifier"},
	"IPv6 Destination Options":  {"Destination Options", "Header Extension Length", "Options", "Type", "ICMPv6", "Identifier"},
	"IPv6 Routing Header":       {"Routing Header", "Header Extension Length", "Routing Type", "Segments Left", "Address", "ICMPv6", "Identifier"},
	"IPv6 Fragment":             {"Fragment Header", "Fragment Offset", "More Fragments", "Identification", "Fragment Data"},
	"IPIP":                      {"IPv4", "Total Length", "Source", "Destination", "ICMP", "Identifier"},
	"Geneve":                    {"Version", "Option Length", "OAM", "Critical Options", "Protocol Type", "VNI", "IPv4", "ICMP"},
	"RIPng":                     {"Command", "Version", "Reserved"},
	"RARP":                      {"EtherType", "Hardware type", "Protocol type", "Hardware size", "Protocol size", "Opcode", "Sender MAC address", "Target IP address"},
	"6to4":                      {"Total Length", "Protocol", "IPv6", "Payload Length", "Next Header", "ICMPv6", "Type", "Identifier"},
	"UDP-Lite":                  {"UDP-Lite", "Source Port", "Destination Port", "Checksum Coverage", "Checksum", "Covered Payload"},
	"CLDAP":                     {"Sequence Tag", "Message ID", "Search", "Base Object", "Scope", "Alias Mode", "Size Limit", "Time Limit", "Types Only", "Present Attribute", "Attribute"},
	"AoE":                       {"Version", "Flags", "Error", "Major", "Minor", "Command", "Tag", "Buffer Count", "Firmware", "Sector Count", "Config Version", "Config Command", "Config Length"},
	"PPPoE Session":             {"VersionType", "Code", "Session ID", "Length", "Protocol", "Source", "Destination", "ICMP"},
	"NetFlow v5":                {"Version", "Count", "System Uptime", "Flow Sequence", "Engine Type", "Sampling Mode", "Sampling Interval", "Records", "Source Address", "Destination Address", "Packet Count", "Octet Count", "Source Port", "Destination Port"},
	"LACP":                      {"Subtype", "Version", "Actor", "Partner", "System Priority", "System ID", "Key", "Port Priority", "Port", "State", "Collector Maximum Delay", "Terminator Type", "Reserved Padding"},
	"TPKT":                      {"Version", "Reserved", "Packet Length", "Length Indicator", "PDU Type", "End Of TSDU", "TPDU Number", "User Data"},
	"COTP":                      {"Length Indicator", "PDU Type", "End Of TSDU", "TPDU Number", "User Data"},
	"Profinet DCP":              {"Frame ID", "Service ID", "Service Type", "Transaction ID", "Response Delay", "Data Length", "Option", "Suboption", "Block Length"},
	"IEC 61850 SV":              {"APPID", "Length", "ASDU Count", "ASDUs", "SV ID", "Sample Counter", "Configuration Revision", "Sample Synchronization", "Sample Data"},
	"IEC 61850 GOOSE":           {"APPID", "Length", "Control Block Reference", "Time Allowed To Live", "Dataset", "GOOSE ID", "Timestamp", "State Number", "Sequence Number", "Simulation", "Configuration Revision", "Needs Commissioning", "Dataset Entry Count", "Dataset Values", "Boolean", "Unused Bits", "Bits"},
	"DoIP":                      {"Version", "Inverse Version", "Payload Type", "Payload Length", "Source Address", "Target Address", "Acknowledgement Code", "Previous Message"},
	"EtherCAT":                  {"Frame Header", "Command", "Index", "Station Address", "Register Offset", "Length Flags", "Interrupt", "Data", "Working Counter"},
	"Ethernet II":               {"Destination", "Source", "Type", "Version", "Total Length", "Protocol", "ICMP", "Checksum"},
	"Ethernet 802.3":            {"Destination", "Source", "Type", "DSAP", "SSAP", "Control", "OUI", "SNAP Payload"},
	"EAP":                       {"Protocol Version", "Packet Type", "Body Length", "Code", "Identifier", "Length", "Type", "Identity"},
	"ICMPv6 NDP":                {"Type", "Code", "Checksum", "Target Address", "Options", "Link Layer"},
	"ICMPv6 MLD":                {"Type", "Code", "Checksum", "Maximum Response Delay", "Multicast Address"},
	"IPv6 RA":                   {"Type", "Code", "Checksum", "Hop Limit", "Router Lifetime", "Reachable Time", "Retrans Timer"},
	"ICMP Timestamp":            {"Type", "Code", "Checksum", "Identifier", "Sequence Number", "Originate", "Receive", "Transmit"},
	"SNTP":                      {"Version", "Mode", "Transmit Timestamp"},
	"DHCPv6 server exchange":    {"Message Type", "Transaction ID", "Options"},
	"Memcache binary":           {"Magic", "Opcode", "Key Length", "Extras Length", "Total Body", "Opaque", "CAS", "Key"},
	"RPC":                       {"XID", "Message Type", "Program", "Procedure"},
	"SDP":                       {"Type", "Value", "Username", "Sess ID", "Sess Version", "Net Type", "Addr Type", "Address", "Session Name"},
	"MariaDB":                   {"Payload Length", "Sequence ID", "Server Version", "Connection ID", "Character Set", "Status Flags"},
	"JDWP handshake":            {"Handshake"},
	"SNMPv3":                    {"Version", "Header Data", "Message ID", "Maximum Size", "Message Flags", "Message Model", "Engine ID", "Engine Boots", "Engine Time", "Scoped PDU", "PDU Type", "Request ID", "Error Status", "Variable Bindings"},
	"LLC":                       {"DSAP", "SSAP", "Control", "Protocol ID", "Version", "BPDU Type"},
	"L2TP":                      {"Flags", "Length", "Tunnel ID", "Session ID", "Ns", "Nr"},
	"RSVP":                      {"Version", "Message Type", "Length", "Object Length", "Class", "C Type", "Neighbor Address", "Logical Interface Handle"},
	"SSL":                       {"ContentType", "Version", "Length", "TLSClientHello", "Legacy Version", "Random", "Cipher Suites"},
	"IEEE 802.1X":               {"Protocol Version", "Packet Type", "Body Length", "Code", "Identifier", "Length", "Identity"},
	"SMB2":                      {"ProtocolId", "StructureSize", "Command", "DialectCount", "Dialects"},
	"DHCPv6":                    {"Message Type", "Transaction ID", "Options"},
	"EAPOL":                     {"Protocol Version", "Packet Type", "Body Length"},
	"EIGRP":                     {"Version", "Opcode", "Checksum", "AS Number", "TLVs"},
	"FTP-DATA":                  {"File Data"},
	"ICMPv6":                    {"Type", "Code", "Checksum", "Identifier", "Sequence Number"},
	"IEEE 802.1Q":               {"PCP", "DEI", "VID High", "VID Low", "Type"},
	"IEEE 802.1ad QinQ":         {"PCP", "DEI", "VID High", "VID Low", "Type", "CTag"},
	"IGMP":                      {"Type", "Checksum", "Group Address"},
	"MPLS":                      {"Label", "Exp", "Bottom", "TTL"},
	"MSRPC":                     {"RPC Vers", "PType", "Frag Length", "Call ID", "Abstract Syntax"},
	"NBNS":                      {"Header", "Questions", "Type", "Class"},
	"NBT NS":                    {"Header", "Questions", "Type", "Class"},
	"NBT DG":                    {"Message Type", "Datagram ID", "Source IP", "Source Port", "Length"},
	"ONC RPC":                   {"XID", "Message Type", "Program", "Procedure"},
	"RIP":                       {"Command", "Version", "Address Family", "Address", "Metric"},
	"SOCKS4":                    {"Version", "Command", "Port", "IP"},
	"WPAD proxy":                {"HTTP Request", "Method", "Path", "Headers"},
	"IEEE 802.11":               {"Frame Control", "Duration", "Addr1", "Addr2", "Addr3", "Seq"},
	"LDAP":                      {"Identifier", "MessageID", "ProtocolOp Tag", "Version", "Auth Tag", "Auth Length"},
	"CDP":                       {"Version", "TTL", "Checksum", "TLVs"},
	"CHAP":                      {"Code", "Identifier", "Length", "Value Size", "Value"},
	"LCP":                       {"Code", "Identifier", "Length"},
	"6in4":                      {"Total Length", "Protocol", "IPv6", "Payload Length", "Next Header", "ICMPv6", "Type", "Identifier", "Sequence Number", "Echo Data"},
	"AFP":                       {"DSI Command", "Request ID", "Function"},
	"AJP":                       {"Magic", "Code"},
	"AMQP":                      {"Type", "Frame End"},
	"ARP":                       {"Hardware type", "Opcode"},
	"ActiveMQ OpenWire":         {"Frame Length", "Data Type", "Magic", "Version"},
	"AnyDesk":                   {"ContentType", "Version", "Length"},
	"BGP":                       {"Marker", "Type"},
	"BFD":                       {"Version Diagnostic", "State Flags", "My Discriminator"},
	"BACnet":                    {"BVLC Type", "BVLC Function", "NPDU Version", "NPDU Control"},
	"Beckhoff ADS":              {"Target Net ID", "Command ID", "Invoke ID"},
	"BitTorrent":                {"Pstr", "Info Hash"},
	"eMule/ED2K":                {"Protocol", "Message Length", "Opcode", "User Hash Length", "User Hash", "Client ID", "TCP Port", "Tag Count", "Tag Type", "Name Length", "Name ID", "String Length", "String Value", "Unsigned Value", "Server Address", "Server Port"},
	"XMPP":                      {"XML Text"},
	"CAN/ISO-TP":                {"Magic", "Version", "Frame Count", "Options"},
	"Cassandra CQL":             {"Version", "Stream", "Opcode", "Body Length"},
	"Ceph":                      {"Protocol Banner", "Server Identity", "Client Identity"},
	"Citrix ICA":                {"Magic", "Terminator"},
	"Collectd":                  {"Parts", "Part", "Type", "Length", "Value"},
	"DCE/RPC":                   {"RPC Vers", "Packet Type", "Object ID", "Operation Number", "Stub"},
	"DB2 DRDA":                  {"Messages", "Magic", "Correlation ID", "Code Point"},
	"DHCP":                      {"Operation", "Hardware Type", "Hardware Length", "Xid", "Client MAC", "Magic Cookie", "Options", "Code", "Message Type"},
	"DNS":                       {"ID", "Questions"},
	"DLMS/COSEM":                {"Opening Flag", "Frame Format and Length", "Control", "Closing Flag"},
	"DNP3":                      {"Start", "Control", "Destination", "Source"},
	"DTLS":                      {"Content Type", "Fragment"},
	"DingTalk":                  {"ContentType", "TLSClientHello"},
	"Diameter":                  {"Version", "Command Code", "Application ID", "Hop-by-Hop ID"},
	"DoH":                       {"ContentType", "Length"},
	"DoQ":                       {"First Byte", "Version"},
	"DoT":                       {"ContentType", "Length"},
	"Elasticsearch":             {"Magic", "Request ID", "Action"},
	"EtherNet/IP CIP":           {"Item Count", "Connection ID", "Encapsulation Sequence", "Sequence Count"},
	"FTP":                       {"Code", "Message"},
	"FTPS":                      {"Command", "Mechanism"},
	"FastCGI":                   {"Version", "Type", "Request ID"},
	"FIX":                       {"Begin String", "Message Type", "Checksum"},
	"GRE":                       {"Flags And Version", "Protocol Type"},
	"GTP Prime":                 {"Flags", "Message Type", "Sequence Number"},
	"GTP-C":                     {"Flags", "Message Type", "TEID", "Sequence Number"},
	"GTP-U":                     {"Flags", "Message Type", "TEID"},
	"GLBP":                      {"Version", "Group", "Owner ID", "TLV Type", "TLV Length"},
	"Git daemon":                {"Packet Length", "Service Path", "Host"},
	"Gnutella":                  {"Start Line", "Headers"},
	"H.323":                     {"TPKT Version", "TPKT Length", "Q931 Protocol Discriminator", "Q931 Message Type", "H323-UserInformation", "h323-message-body Choice", "protocolIdentifier Bytes", "h323-ID Bytes", "manufacturerCode", "callIdentifier Value"},
	"HSRP":                      {"TLVs", "Virtual IP"},
	"Hart-IP":                   {"Version", "Message Type", "Sequence Number", "Byte Count"},
	"HTTP":                      {"HTTP Request", "Headers"},
	"HTTP Proxy CONNECT":        {"HTTP Request", "Method", "Path"},
	"HTTP/2":                    {"Frames", "Type"},
	"HTTP/3":                    {"First Byte", "Version"},
	"ICMP":                      {"Type", "Checksum"},
	"IIOP/GIOP":                 {"Magic", "Message Type", "Message Size", "Request ID", "Operation", "Service Context Count", "Context ID", "Context Data", "Stub Data"},
	"IKEv1":                     {"Initiator SPI", "Exchange Type", "Payloads"},
	"IKEv2":                     {"Initiator SPI", "Exchange Type", "Payloads"},
	"IMAP":                      {"Tag", "Command"},
	"IMAPS":                     {"ContentType", "Length"},
	"IEC 60870-5-104":           {"Start", "APDU Length", "Control"},
	"IEC 61850 MMS":             {"PDU Tag", "PDU Length", "Local Detail Calling", "Proposed Calling Limit", "Proposed Called Limit", "Proposed Nesting Level", "Proposed Version", "Parameter CBB Bits", "Services Supported Bits"},
	"IPMI":                      {"Version", "Class", "Session"},
	"IPP":                       {"Version Major", "Operation ID", "Request ID", "Attribute Groups"},
	"IPsec AH":                  {"SPI", "Sequence", "ICV"},
	"IPsec ESP":                 {"SPI", "Sequence", "Ciphertext"},
	"JSON-RPC":                  {"Brace", "Pairs"},
	"JSON-RPC 2.0":              {"JSON Text"},
	"IAX2":                      {"Source Word", "Destination Word", "Timestamp", "Outbound Sequence", "Inbound Sequence", "Frame Type", "Subclass", "Information Elements", "IE Type", "IE Length", "Unsigned 16", "Unsigned 32", "Text"},
	"XML-RPC":                   {"XML Text"},
	"Prometheus exposition":     {"Exposition Text"},
	"Kafka":                     {"API Key", "Correlation ID", "Body"},
	"Kerberos":                  {"Application Tag", "Seq Tag", "Seq Length"},
	"IRC":                       {"Command", "Parameters"},
	"KNX/IP":                    {"Header Length", "Service Type", "Total Length"},
	"LDP":                       {"Version", "PDU Length", "LSR ID", "Messages"},
	"LLDP":                      {"TLVs", "TypeLen", "Chassis ID", "Port ID", "TTL"},
	"MELSEC":                    {"Subheader", "Network Number", "PC Number", "IO Number"},
	"MGCP":                      {"Request Line", "Headers"},
	"MPEG-TS":                   {"Packets", "Sync Byte", "PID High", "Flags and Continuity"},
	"MQTT":                      {"Packet Type", "RL0"},
	"MSSQL TDS":                 {"Type", "Length", "PacketID"},
	"Memcached":                 {"Text Line"},
	"MongoDB":                   {"Message Length", "Op Code"},
	"Modbus TCP":                {"Transaction ID", "Protocol ID", "Function Code"},
	"MySQL":                     {"Payload Length", "Sequence ID"},
	"NATS":                      {"Operation", "Arguments"},
	"NFS":                       {"XID", "Message Type", "Program"},
	"NNTP":                      {"Status Code", "Message"},
	"NTP":                       {"Version", "Mode"},
	"NetBIOS":                   {"ID", "Questions"},
	"NetFlow v9":                {"Version", "Count", "Sequence Number", "FlowSets", "FlowSet ID"},
	"OCSP":                      {"Request Tag", "TBS Request", "Request List", "Certificate ID", "Algorithm OID", "Issuer Name Hash", "Issuer Key Hash", "Serial Number"},
	"OPC UA":                    {"Message Type", "Message Size", "Endpoint URL"},
	"OSPF":                      {"Version", "Type", "LSAs"},
	"OpenVPN":                   {"Opcode", "Key ID"},
	"Omron FINS":                {"ICF", "Service ID", "Main Request Code", "Sub Request Code"},
	"Oracle TNS":                {"Packet Length", "Packet Type"},
	"POP3":                      {"Status", "Arg", "LF"},
	"PPP":                       {"Address", "Protocol"},
	"PPPoE Discovery":           {"VersionType", "Code", "Payload"},
	"PPTP":                      {"Length", "ControlMessageType"},
	"PFCP":                      {"Flags", "Message Type", "Sequence Number", "Information Elements"},
	"PIM":                       {"Version Type", "Checksum", "Message Data"},
	"PTP":                       {"Message Type", "Clock Identity", "Sequence ID"},
	"PostgreSQL":                {"Length", "Protocol"},
	"Protobuf":                  {"Fields", "Tag"},
	"Profinet IO":               {"RPC Version", "Packet Type", "Interface ID", "Operation Number", "Fragment Length", "Block Type", "Block Length", "Sequence Number", "API", "Slot Number", "Subslot Number", "Index", "Record Data Length"},
	"RADIUS":                    {"Code", "Identifier", "Length", "Authenticator", "Attributes", "Type"},
	"RMI/JRMP":                  {"Magic", "Version"},
	"RSH":                       {"Secondary Port"},
	"RTCP":                      {"Packet Type", "SSRC"},
	"RDP":                       {"PacketLength", "TPDUCode", "RequestedProtocols"},
	"RTMP":                      {"Version", "Time", "Zero", "Random"},
	"RTP":                       {"VersionPXPCC", "Sequence", "SSRC"},
	"RTSP":                      {"RTSP Request", "Headers"},
	"Rsync daemon":              {"Magic", "Major", "Minor"},
	"SCTP":                      {"Source Port", "Destination Port", "Chunks"},
	"DCCP":                      {"DCCP", "Source Port", "Destination Port", "Data Offset", "CCVal", "Checksum Coverage", "Checksum", "Packet Type", "Extended Sequence Numbers", "Sequence Number", "Service Code", "Timestamp", "Application Data"},
	"SCCP/Skinny":               {"Data Length", "Header Version", "Message ID"},
	"S7comm":                    {"TPKT Version", "Protocol ID", "ROSCTR", "Parameter Length"},
	"S7comm-plus":               {"TPKT Version", "Protocol ID", "Protocol Version", "Message Data"},
	"SIP":                       {"SIP Request", "Headers"},
	"SMB":                       {"ProtocolId", "Command"},
	"SMTP":                      {"Code", "Message"},
	"SMPP":                      {"Command Length", "Command ID", "System ID", "Interface Version"},
	"SMTPS":                     {"ContentType", "Length"},
	"SNMP":                      {"Version", "Community", "PDU Tag"},
	"SOAP":                      {"HTTP Response", "Status", "Headers"},
	"SOCKS5":                    {"Version", "Command", "AddressType"},
	"SSDP":                      {"Method", "Target", "Version", "Headers", "Line", "Message End"},
	"SRVLOC/SLP":                {"Version", "Function ID", "XID", "Language Tag"},
	"STOMP":                     {"Command", "Headers", "Terminator"},
	"SSH":                       {"Identification"},
	"STUN":                      {"Message Type", "Magic Cookie", "Transaction ID"},
	"Syslog":                    {"PRI", "Message"},
	"TFTP":                      {"Opcode", "Filename", "Mode"},
	"Teredo":                    {"Indicator Type", "Nonce", "Confirmation", "Next Header", "Source", "Destination"},
	"T.38":                      {"SDP Session 0", "SDP Session 1", "SDP Version", "Connection Network Type", "Connection Address Type", "Connection Address", "Media Type", "Media Port", "Media Transport", "Media Format 0", "Attribute Name", "Attribute Value"},
	"TLS":                       {"ContentType", "Version", "Length"},
	"Telnet":                    {"Items", "IACByte", "Command", "Option"},
	"TeamViewer":                {"Magic", "Command", "Body Length 16", "Opaque Body"},
	"Thrift":                    {"Version", "Name", "Seq ID"},
	"VNC/RFB":                   {"Magic", "Major", "Minor"},
	"VRRP":                      {"VersionType", "VRID", "Priority"},
	"VXLAN":                     {"I", "VNI", "Inner"},
	"UPnP":                      {"Method", "Target", "Version", "Headers", "Line", "Message End"},
	"WeChat/MicroMsg":           {"ContentType", "TLSClientHello"},
	"WebDAV":                    {"HTTP Request", "Method", "Headers"},
	"WebSocket":                 {"Opcode", "Payload Len"},
	"WireGuard":                 {"Type", "Receiver", "Counter", "Ciphertext"},
	"XDMCP":                     {"Version", "Opcode", "Message Length"},
	"Zabbix agent":              {"Magic", "Flags", "Length"},
	"mDNS":                      {"ID", "Questions"},
	"sFlow":                     {"Version", "Agent Address Type", "Sequence Number", "Sample Count", "Samples"},
}

// Exact values for newly added authoritative samples. RequiredNodes above
// protects the shape of every direct result; these checks additionally anchor
// the values of stable header fields against the independent decoder output
// recorded when each sample was selected.
var protocolCorpusExactValues = map[string]map[string]any{
	"ndpi-t38/T.38": {
		"SDP Version": "0", "Connection Network Type": "IN", "Connection Address Type": "IP4", "Connection Address": "$", "Media Type": "audio", "Media Port": "$", "Media Transport": "RTP/AVP", "Media Format 0": "8",
	},
	"gen-redfish-valid/Redfish": {
		"Method": "GET", "Path": "/redfish/v1/", "Version": "HTTP/1.1",
	},
	"gen-minio-s3-valid/MinIO/S3": {
		"Method": "GET", "Request Target": "/bucket/object", "HTTP Version": "HTTP/1.1", "Algorithm": "AWS4-HMAC-SHA256", "Access Key ID": "AKIA", "Scope Date": "20200101", "Region": "us-east-1", "Service": "s3", "Scope Terminator": "aws4_request", "Signed Header 0": "host", "Signed Header 1": "x-amz-date", "Signature Hex": strings.Repeat("0", 64),
	},
	"gen-etcd-version-valid/ETCD": {
		"Method": "GET", "Path": "/version", "Version": "HTTP/1.1",
	},
	"gen-acme-jws-valid/ACMEv2": {
		"Method": "POST", "Path": "/acme/order/1", "Version": "HTTP/1.1",
	},
	"gen-k8s-list-options-valid/Kubernetes API": {
		"Method": "GET", "API Prefix": "api", "API Version": "v1", "Resource": "namespaces", "Version": "HTTP/1.1",
	},
	"pr5023-gen-k8s-api/Kubernetes API": {
		"Method": "GET", "API Prefix": "api", "API Version": "v1", "Resource": "namespaces", "Version": "HTTP/1.1", "Authentication Scheme": "Bearer", "Credential": []byte("lab"),
	},
	"gen-docker-api-valid/Docker API": {
		"Method": "GET", "Path": "/v1.41/containers/json", "Version": "HTTP/1.1",
	},
	"gen-winrm-identify-valid/WinRM HTTP": {
		"Method": "POST", "Path": "/wsman", "Version": "HTTP/1.1", "Content Length": uint64(190), "HTTP host": "wsman.example", "HTTP content-type": "application/soap+xml;charset=UTF-8",
	},
	"gen-gssapi-valid/GSS-API": {
		"Method": "GET", "Path": "/", "Version": "HTTP/1.1",
	},
	"ndpi-rmi/RMI/JRMP": {
		"Magic": []byte("JRMI"), "Version": uint64(2), "Protocol": uint64(0x4b),
	},
	"gen-rmi-valid/RMI": {
		"Magic": []byte("JRMI"), "Version": uint64(2), "Protocol": uint64(0x4c), "Type": uint64(0x50), "Ser Magic": uint64(0xaced), "Ser Version": uint64(5), "Block Length": uint64(34), "Object Number": uint64(0), "Method Hash": uint64(0x0102030405060708), "String": "sample",
	},
	"gen-smb3-valid/SMB3": {
		"ProtocolId": uint64(0x424d53fe), "Header StructureSize": uint64(64), "Command": uint64(0), "StructureSize": uint64(36), "DialectCount": uint64(2), "Capabilities": uint64(0x7f),
	},
	"gen-cifs-valid/CIFS": {
		"Direct TCP Zero": uint64(0), "Direct TCP Length": uint64(71), "ProtocolId": uint64(0x424d53ff), "Command": uint64(0x72), "Flags": uint64(0x18), "Flags2": uint64(0xc853), "PID": uint64(0x1234), "MID": uint64(7), "WordCount": uint64(0), "ByteCount": uint64(36), "BufferFormat": uint64(2), "Dialect Name": "PC NETWORK PROGRAM 1.0",
	},
	"gen-xtp-valid/XTP": {
		"Key": uint64(0x8001020304050607), "Version": uint64(1), "Packet Format": uint64(0), "Data Length": uint64(0), "Checksum": uint64(0x4d81), "Sort": uint64(0x1234), "Synchronization": uint64(0x01020304), "Sequence": uint64(0x0102030405060708),
	},
	"gen-h225-valid/H.225": {
		"RasMessage Choice": uint64(0), "requestSeqNum": uint64(1), "protocolIdentifier Bytes": "0.0.8.2250.0.4", "ip Bytes": []byte{127, 0, 0, 1}, "port": uint64(1719),
	},
	"ndpi-h323/H.323": {
		"TPKT Version": uint64(3), "TPKT Length": uint64(160), "Q931 Protocol Discriminator": uint64(8), "Q931 Message Type": uint64(5), "h323-message-body Choice": uint64(0), "protocolIdentifier Bytes": "0.0.8.2250.0.4", "h323-ID Bytes": "m.jemec", "manufacturerCode": uint64(61),
	},
	"gen-xyplex-valid/XYPLEX": {
		"Protocol Type": uint64(1), "Padding": uint64(0), "Server Port": uint64(7), "Return Port": uint64(8080), "Reserved": uint64(0),
	},
	"pr5023-gen-xyplex/XYPLEX": {
		"Protocol Type": uint64(0), "Padding": uint64(0), "Server Port": uint64(0), "Return Port": uint64(0), "Reserved": uint64(0), "Uninterpreted Request Tail": make([]byte, 8),
	},
	"gen-sna-valid/SNA": {
		"SNA Ethernet Length": uint64(16), "SNA Ethernet Padding": uint64(0), "DSAP": uint64(4), "SSAP": uint64(4), "LLC Control": uint64(4), "LLC Extended Control": uint64(7), "Format Identification": uint64(2), "Begin BIU": uint64(1), "End BIU": uint64(1), "Destination Local Address": uint64(1), "Origin Local Address": uint64(2), "Sequence Number": uint64(0x0102), "Response Indicator": uint64(0), "RU Category": uint64(0), "Uninterpreted RU Bytes": []byte{1, 0, 0xff},
	},
	"gen-swipe-valid/swIPe": {
		"Packet Type": uint64(0), "Header Length Words": uint64(1), "Policy Identifier": uint64(1), "Inner IP Version": uint64(4), "Inner Header Length Words": uint64(5), "Inner Total Length": uint64(23), "Inner Source": []byte{192, 0, 2, 10}, "Inner Destination": []byte{198, 51, 100, 20}, "Inner Payload": []byte("dat"),
	},
	"gen-megaco-valid/Megaco/H.248": {
		"Protocol Marker": "MEGACO", "Version": "1", "Address": "192.0.2.10", "Port": "2944", "Transaction ID": "1", "Context ID": "-", "Termination ID": "ROOT", "Service Change Method": "Restart", "Service Change Reason": `"901 restart"`,
	},
	"scapy-zigbee-join/Zigbee": {
		"MAC Frame Control": uint64(0x8841), "MAC Sequence Number": uint64(51), "MAC Destination PAN": uint64(0x01ff), "MAC Destination Short": uint64(0xffff), "MAC Source Short": uint64(0), "NWK Frame Control": uint64(0x1209), "NWK Destination": uint64(0xfffc), "NWK Source": uint64(0), "NWK Radius": uint64(1), "NWK Sequence Number": uint64(209), "Auxiliary Control": uint64(0x28), "Auxiliary Frame Counter": uint64(1), "Auxiliary Key Sequence Number": uint64(0), "Protected Payload and MIC": []byte{0x40, 0x15, 0xcd, 0x19, 0xab, 0x20},
	},
	"scapy-zigbee-skke/Zigbee": {
		"MAC Frame Control": uint64(0x8861), "MAC Sequence Number": uint64(48), "MAC Destination PAN": uint64(0x3359), "MAC Destination Short": uint64(0x9090), "NWK Frame Control": uint64(8), "NWK Destination": uint64(0x9090), "NWK Source": uint64(0), "NWK Radius": uint64(30), "NWK Sequence Number": uint64(221), "APS Frame Control": uint64(1), "APS Counter": uint64(220), "APS Command ID": uint64(5), "Transport Key Type": uint64(1), "Transport Key Sequence Number": uint64(0), "Frame Check Sequence": uint64(0x244f),
	},
	"gen-vines-valid/VINES": {
		"Checksum": uint64(0xffff), "Packet Length": uint64(27), "Control High Bit": uint64(0), "Control Flags": uint64(0), "Hop Count": uint64(15), "Protocol": uint64(1), "Destination Network": uint64(0x10203040), "Destination Subnetwork": uint64(0x8001), "Source Network": uint64(0x50607080), "Source Subnetwork": uint64(1), "Source Port": uint64(0x1234), "Destination Port": uint64(0x5678), "IPC Packet Type": uint64(0), "Datagram Padding": uint64(0), "IPC Data": []byte{0x61, 0, 0xff},
	},
	"gen-sigcomp-valid/SIGCOMP": {
		"Signature": uint64(31), "Feedback Present": uint64(0), "State ID Length Selector": uint64(1), "Partial State Identifier": []byte{1, 2, 3, 4, 5, 6}, "Compressed Data": []byte{0xaa, 0xbb},
	},
	"pr5023-gen-wep/WEP": {
		"Initialization Vector": []byte{0, 0, 1}, "Key Index": uint64(0), "Extended IV": uint64(0), "Reserved": uint64(0), "Encrypted Data": make([]byte, 20), "Encrypted ICV": make([]byte, 4),
	},
	"gen-ncp-valid/NCP": {
		"Packet Type": uint64(0x1111), "Sequence Number": uint64(0), "Connection Number Low": uint64(255), "Task Number": uint64(0), "Reserved": uint64(0), "Request Code": uint64(0),
	},
	"gen-lat-valid/LAT": {
		"Message Type": uint64(0), "Master": uint64(0), "Response Required": uint64(1), "Slot Count": uint64(0), "Destination Circuit ID": uint64(0x1234), "Source Circuit ID": uint64(0x5678), "Message Sequence Number": uint64(254), "Message Acknowledgment Number": uint64(255),
	},
	"pr5023-gen-sebek/SEBEK": {
		"Magic": uint64(0xd0d0d0d0), "Version": uint64(3), "Record Type": uint64(0), "Counter": uint64(0), "Parent Process ID": uint64(0), "Process ID": uint64(0), "Data Length": uint64(3), "Command Name": "bash\x00\x00\x00\x00\x00\x00\x00\x00", "Record Data": []byte("ls\n"),
	},
	"gen-sebek-valid/SEBEK": {
		"Magic": uint64(0xa1b2c3d4), "Version": uint64(2), "Record Type": uint64(1), "Counter": uint64(0x01020304), "Time Seconds": uint64(0x10203040), "Time Microseconds": uint64(123456), "Process ID": uint64(8), "User ID": uint64(9), "File Descriptor": uint64(10), "Command Name": "record-label", "Data Length": uint64(3), "Record Data": []byte{0x41, 0, 0xff},
	},
	"gen-alljoyn-valid/AllJoyn": {
		"Sender Version": uint64(0), "Message Version": uint64(0), "Question Count": uint64(1), "Answer Count": uint64(0), "Timer": uint64(0),
		"Question Type": uint64(2), "Question Reserved": uint64(0), "T Flag": uint64(1), "U Flag": uint64(0), "S Flag": uint64(0), "F Flag": uint64(1),
		"Name Count": uint64(1), "Byte Length": uint64(18), "Text": "org.example.Sensor",
	},
	"gen-fcoe-valid/FCoE": {
		"FCoE Version": uint64(0), "SOF": uint64(0x2e), "FC CRC32": uint64(0xe116194a), "EOF": uint64(0x42),
		"R_CTL": uint64(4), "D_ID": uint64(0x010203), "S_ID": uint64(0x040506), "FC Type": uint64(0xff), "F_CTL": uint64(0x380000), "SEQ_ID": uint64(7), "OX_ID": uint64(0x1234), "RX_ID": uint64(0xffff), "FC Data": []byte{0xde, 0xad, 0xbe, 0xef},
	},
	"gen-fcoe-valid/Fibre Channel": {
		"R_CTL": uint64(4), "D_ID": uint64(0x010203), "CS_CTL": uint64(0), "S_ID": uint64(0x040506), "FC Type": uint64(0xff), "F_CTL": uint64(0x380000),
		"SEQ_ID": uint64(7), "DF_CTL": uint64(0), "SEQ_CNT": uint64(0), "OX_ID": uint64(0x1234), "RX_ID": uint64(0xffff), "Parameter": uint64(0), "FC Data": []byte{0xde, 0xad, 0xbe, 0xef},
	},
	"gen-aarp-valid/AARP": {
		"Hardware Type": uint64(1), "Protocol Type": uint64(0x809b), "Hardware Length": uint64(6), "Protocol Length": uint64(4), "Function": uint64(1),
		"Source Hardware Address": []byte{2, 0, 0, 0, 0, 1}, "Source Protocol Reserved": uint64(0), "Source Network": uint64(0x1234), "Source Node": uint64(0x2a),
		"Destination Hardware Address": []byte{0, 0, 0, 0, 0, 0}, "Destination Protocol Reserved": uint64(0), "Destination Network": uint64(0x1234), "Destination Node": uint64(0x56),
	},
	"ndpi-c37118/IEEE C37.118 synchrophasor": {
		"Sync": uint64(0xaa), "Frame Type": uint64(4), "Version": uint64(1),
		"Frame Size": uint64(18), "ID Code": uint64(241), "Second Of Century": uint64(0),
		"Time Quality": uint64(0), "Fraction Of Second": uint64(0), "Command": uint64(5), "Checksum": uint64(0xd7d0),
	},
	"ndpi-dicom/DICOM Upper Layer": {
		"PDU Type": uint64(1), "PDU Length": uint64(677), "Protocol Version": uint64(1),
		"Called AE Title": "testserver      ", "Calling AE Title": "testclient      ",
		"UID": "1.2.840.10008.3.1.1.1", "Maximum PDU Length": uint64(4194304),
	},
	"gen-powerlink-valid/Powerlink": {
		"Message Type": uint64(1), "Destination Node": uint64(255), "Source Node": uint64(240),
		"Cycle Flags": uint64(0xc0), "Net Time Seconds": uint64(0x12345678),
		"Net Time Nanoseconds": uint64(123456789), "Relative Time": uint64(0x0102030405060708),
	},
	"gen-isis-valid/IS-IS": {
		"Intradomain Routing Protocol Discriminator": uint64(0x83), "Length Indicator": uint64(27),
		"PDU Type": uint64(15), "PDU Length": uint64(53), "Holding Timer": uint64(30),
		"Priority": uint64(1), "Circuit Type": uint64(1),
	},
	"ndpi-netbeui/NetBEUI": {
		"Header Length": uint64(44), "Delimiter": uint64(0xefff), "Command": uint64(1),
	},
	"ndpi-ethersio/Ether-S-I/O": {
		"Magic": "ESIO", "Telegram Type": uint64(1), "Version": uint64(0), "Length": uint64(49),
		"Transaction ID": uint64(0x7616), "Telegram ID": uint64(24), "Source Station ID": uint64(13),
		"Transfer Count": uint64(1), "Transfer Flags": uint64(0), "Transfer ID": uint64(92),
		"Destination Station ID": uint64(1), "Data Length": uint64(17),
	},
	"ndpi-ethersbus/Ether-S-Bus": {
		"Length": uint64(13), "Version": uint64(1), "Protocol": uint64(0),
		"Sequence": uint64(1), "Attribute": uint64(0), "Destination": uint64(10),
		"Command": uint64(0x20), "Checksum": uint64(0x5318),
	},
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

func protocolCorpusIsPositive(capture protocolCorpusCapture) bool {
	return capture.EvidenceKind == "upstream-positive" || capture.EvidenceKind == "generated-positive"
}

func protocolCorpusIsNegative(capture protocolCorpusCapture) bool {
	return capture.EvidenceKind == "upstream-negative" || capture.EvidenceKind == "generated-negative"
}

func protocolCorpusRequireCaptureParseSpec(t *testing.T, capture protocolCorpusCapture, spec protocolCorpusCaptureParseSpec) {
	t.Helper()
	require.NotNil(t, capture.RepresentativeFrame, "%s has a capture-specific parse contract but no representative frame", capture.ID)
	require.True(t, protocolCorpusIsPositive(capture), "%s capture-specific parse contract cannot validate %s material", capture.ID, capture.EvidenceKind)
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
		if capture.RoadmapName == nil || !protocolCorpusIsPositive(capture) {
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
			if capture.ID == "ndpi-wsd-original" {
				info, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
				require.True(t, ok)
				message, ok := info["WS-Discovery Message"].(*stream_parser.WSDiscoveryMessage)
				require.True(t, ok, "a raw XML field is not a discovery-message parse")
				require.Equal(t, "2005/04", message.Version)
				require.Equal(t, "Resolve", message.Kind)
				require.Equal(t, "http://schemas.xmlsoap.org/ws/2005/04/discovery/Resolve", message.Header.Action)
				require.Equal(t, "urn:uuid:3f42dc9a-24ce-48d1-88f9-16b96a137d71", message.Header.MessageID)
				require.Equal(t, "urn:schemas-xmlsoap-org:ws:2005:04:discovery", message.Header.To)
				require.Len(t, message.Endpoints, 1)
				require.Equal(t, "urn:uuid:e3248000-80ce-11db-8000-001ba99ec956", message.Endpoints[0].EPR.Address)
				require.Nil(t, message.Header.ReplyTo)
				require.Nil(t, message.Header.AppSequence)
				require.Nil(t, message.Endpoints[0].MetadataVersion)
			}
			if capture.ID == "ndpi-xmpp-jabber" {
				// The original representative is only a declaration. The exact
				// contiguous frame-6 + frame-8 opening supplies real namespaces;
				// the 376-record test separately accounts for every other record.
				require.Len(t, input, 138)
				message := xmppCorpusMessage(t, node, false)
				require.Len(t, message.Events, 1)
				require.Len(t, message.Declarations, 1)
				require.Equal(t, "stream-open", message.Events[0].Kind)
				header := message.Events[0].Element
				require.Equal(t, "http://etherx.jabber.org/streams", header.Name.Space)
				require.Equal(t, "stream", header.Name.Local)
				require.Equal(t, "jabber:client", header.Namespaces[""])
				require.Equal(t, "cs-xmpp.lan", xmppCorpusAttr(header, "to"))
				require.Equal(t, "1.0", xmppCorpusAttr(header, "version"))
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
			require.True(t, protocolCorpusIsNegative(capture))
			protocolCorpusRequireRuleEntry(t, id, spec.Contract)
			require.NotEmpty(t, spec.ControlCaptureID, "%s rejection contract has no positive control capture", id)
			controlCapture, ok := captures[spec.ControlCaptureID]
			require.True(t, ok, "%s positive control points to missing capture %s", id, spec.ControlCaptureID)
			require.True(t, protocolCorpusIsPositive(controlCapture), "%s positive control must use positive material", id)
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
	diagnostic := protocolCorpusFailureDiagnostic(err)
	require.Contains(t, diagnostic, spec.ExpectedErrorContains, "%s failed for an unexpected reason: %v", id, err)
	switch spec.ExpectedFailureClass {
	case protocolCorpusFailureInvalidValue:
		require.ErrorContains(t, err, "YakVM Panic:", "%s was not rejected by a field-value invariant", id)
	case protocolCorpusFailureLengthBounds:
		require.Contains(t, diagnostic, "over max size", "%s was not rejected by an input-length bound", id)
	case protocolCorpusFailureTruncated:
		message := diagnostic
		require.True(t, strings.Contains(message, "truncated") || strings.Contains(message, "EOF"), "%s was not rejected as truncated: %v", id, err)
	default:
		t.Fatalf("%s rejection contract has unknown failure class %q", id, spec.ExpectedFailureClass)
	}
}

// Yak errors include annotated operator source. Only inspect the innermost
// diagnostic, so a panic string merely present in that source cannot satisfy
// an expected-failure contract.
func protocolCorpusFailureDiagnostic(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if index := strings.LastIndex(message, "YakVM Panic: "); index >= 0 {
		message = message[index+len("YakVM Panic: "):]
	}
	return strings.TrimSpace(strings.SplitN(message, "\n", 2)[0])
}

func TestProtocolCorpusFailureDiagnosticIgnoresOperatorSource(t *testing.T) {
	err := errors.New("parse node Message error: Panic Stack:\n  1 | panic(\"decoy failure\")\nYakVM Panic: actual failure\n")
	require.Equal(t, "actual failure", protocolCorpusFailureDiagnostic(err))
	require.NotContains(t, protocolCorpusFailureDiagnostic(err), "decoy failure")
	require.Equal(t, "", protocolCorpusFailureDiagnostic(nil))
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
	dispositions := map[string]string{}
	for _, capture := range manifest.Captures {
		captureSpec, captureParse := protocolCorpusCaptureParseSpecs[capture.ID]
		if captureParse {
			protocolCorpusRequireCaptureParseSpec(t, capture, captureSpec)
		}
		direct := captureParse
		if !direct && protocolCorpusIsPositive(capture) && capture.RoadmapName != nil && capture.RepresentativeFrame != nil {
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
		if categoryCount != 1 {
			dispositions[capture.ID] = fmt.Sprintf("invalid-category-count-%d", categoryCount)
			t.Errorf("%s must have exactly one validation category; got %d", capture.ID, categoryCount)
			continue
		}

		switch {
		case direct:
			dispositions[capture.ID] = "registered-direct"
			counts["parsed"]++
		case classifier:
			dispositions[capture.ID] = "classifier-only"
			counts["classifier"]++
			require.True(t, protocolCorpusIsPositive(capture) || capture.EvidenceKind == "generated-identification",
				"%s classifier fixture cannot validate %s material", capture.ID, capture.EvidenceKind)
			info := ProtocolInfo{Name: capture.ID, Layer: classifierSpec.Layer}
			input := protocolCorpusParseInput(t, corpusDir, capture, info, protocolCorpusParseContract{
				Layer: classifierSpec.Layer, UseFullFrame: classifierSpec.UseFullFrame,
			})
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
			dispositions[capture.ID] = "expected-rejection"
			counts["rejected"]++
			require.True(t, protocolCorpusIsNegative(capture))
		case structural:
			dispositions[capture.ID] = "structural-only"
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

	// Registration is not a passing parse, nor proof of every application field.
	// Emit all rows before failing so a report cannot lose the uncovered inputs.
	encoded, err := json.Marshal(dispositions)
	require.NoError(t, err)
	t.Logf("corpus capture dispositions JSON: %s", encoded)
	t.Logf("matrix dispositions: total=%d registered-direct=%d classifier=%d rejected=%d structural=%d", len(manifest.Captures), counts["parsed"], counts["classifier"], counts["rejected"], counts["structural"])
	require.Equal(t, len(manifest.Captures), counts["parsed"]+counts["classifier"]+counts["rejected"]+counts["structural"])
	require.Equal(t, 434, counts["parsed"], "registered-direct category changed; review every added or reclassified capture")
	require.Equal(t, 14, counts["classifier"], "classifier validation category changed; review every added or reclassified capture")
	require.Equal(t, 102, counts["rejected"], "rejected-capture validation category changed; review every added or reclassified capture")
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
		if !protocolCorpusIsPositive(capture) || capture.RoadmapName == nil || capture.RepresentativeFrame == nil {
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
	encoded, err := json.Marshal(map[string][]string{
		"registered-field-level": protocolCorpusSortedNames(semantic),
		"registered-outer-only":  protocolCorpusSortedNames(outer),
		"classifier-only":        protocolCorpusSortedNames(classifier),
		"missing":                missing,
	})
	require.NoError(t, err)
	t.Logf("corpus roadmap dispositions JSON: %s", encoded)
	t.Logf("roadmap dispositions: total=%d semantic=%d outer=%d classifier=%d missing=%d", len(roadmapNames), len(semantic), len(outer), len(classifier), len(missing))
	require.Len(t, roadmapNames, 341, "the pinned corpus roadmap-name set changed")
	require.Len(t, semantic, 327, "field-level roadmap contract set changed; review every added or reclassified name")
	require.Len(t, outer, 10, "outer-format roadmap contract set changed; review every added or reclassified name")
	require.Len(t, classifier, 4, "classifier-only roadmap validation set changed; review every added or reclassified name")
	require.Empty(t, missing, "roadmap names without executable positive validation")
	require.Equal(t, len(roadmapNames), len(semantic)+len(outer)+len(classifier))
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
		through := capture.RepresentativeFrame.Number
		if contract.ReassembleThroughFrame != 0 {
			require.GreaterOrEqual(t, contract.ReassembleThroughFrame, through)
			require.LessOrEqual(t, contract.ReassembleThroughFrame, capture.PacketCount)
			through = contract.ReassembleThroughFrame
		}
		input = protocolCorpusTCPStreamThroughFrame(t, captureData, capture.LinkType, through)
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
	if len(contract.Base64After) > 0 {
		offset := bytes.Index(input, contract.Base64After)
		require.NotEqual(t, -1, offset, "%s does not contain the expected encoded-message prefix", capture.ID)
		encoded := input[offset+len(contract.Base64After):]
		if end := bytes.IndexAny(encoded, " \t\r\n"); end >= 0 {
			encoded = encoded[:end]
		}
		require.NotEmpty(t, encoded, "%s has an empty encoded-message value", capture.ID)
		decoded, err := base64.StdEncoding.Strict().DecodeString(string(encoded))
		require.NoError(t, err, "%s has an invalid base64 message", capture.ID)
		input = decoded
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
	if node.Name == name && protocolCorpusNodeHasResult(node) {
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
	case "FDDI":
		return gopacket.DecodeFunc(func(data []byte, builder gopacket.PacketBuilder) error {
			if len(data) < 13 {
				return fmt.Errorf("short FDDI frame")
			}
			return layers.LayerTypeLLC.Decode(data[13:], builder)
		}), nil
	case "Token Ring":
		return gopacket.DecodeFunc(func(data []byte, builder gopacket.PacketBuilder) error {
			if len(data) < 14 {
				return fmt.Errorf("short Token Ring frame")
			}
			offset := 14
			if data[8]&128 != 0 {
				if len(data) < 16 {
					return fmt.Errorf("short Token Ring route")
				}
				length := int(data[14] & 31)
				if length < 2 || length%2 != 0 || len(data) < offset+length {
					return fmt.Errorf("invalid Token Ring route")
				}
				offset += length
			}
			return layers.LayerTypeLLC.Decode(data[offset:], builder)
		}), nil
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
			// Framing belongs to this field only when it was actually consumed.
			// A matching byte at the next field's start is not sufficient evidence.
			if current.Cfg.Has(stream_parser.CfgConsumedBits) {
				end = position[0] + current.Cfg.GetUint64(stream_parser.CfgConsumedBits)
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
// extend such a range only when the recorded wire length and the exact input
// bytes both prove that this field consumed its delimiter.
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
					consumed := current.Cfg.GetUint64(stream_parser.CfgConsumedBits)
					if !current.Cfg.Has(stream_parser.CfgConsumedBits) {
						problems = append(problems, fmt.Sprintf("terminal %q has no recorded delimiter consumption", path))
					} else if consumed != end-start && consumed != end-start+delimiterBits {
						problems = append(problems, fmt.Sprintf("terminal %q has inconsistent consumed length %d", path, consumed))
					}
					if consumed == end-start+delimiterBits && delimiter != "" && end%8 == 0 && delimiterEnd <= inputBits {
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
		field.Cfg.SetItem(stream_parser.CfgConsumedBits, uint64(16))
		count, err := protocolCorpusTerminalCoverage(field, []byte("a:"))
		require.NoError(t, err)
		require.Equal(t, 1, count)

		_, err = protocolCorpusTerminalCoverage(field, []byte("ab"))
		require.ErrorContains(t, err, "uncovered bits [8,16)")
	})

	t.Run("optional delimiter cannot claim the next field", func(t *testing.T) {
		field := terminal("Optional", 0, 8)
		field.Cfg.SetItem(stream_parser.CfgDelimiter, ":")
		field.Cfg.SetItem(stream_parser.CfgDelimiterOptional, true)
		field.Cfg.SetItem(stream_parser.CfgConsumedBits, uint64(8))
		_, err := protocolCorpusTerminalCoverage(field, []byte("a:"))
		require.ErrorContains(t, err, "uncovered bits [8,16)")
		root := &base.Node{Name: "Message", Cfg: base.NewEmptyConfig(), Children: []*base.Node{field, terminal("Tail", 8, 16)}}
		_, err = protocolCorpusTerminalCoverage(root, []byte("a:"))
		require.NoError(t, err)
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

	var segments []soapCorpusTCPSegment
	for _, frame := range decoded {
		if frame.flow == targetFlow && frame.number <= targetFrame {
			segments = append(segments, soapCorpusTCPSegment{frame: frame.number, seq: frame.seq, payload: frame.data})
		}
	}
	require.NotEmpty(t, segments, "TCP direction has no payload through frame %d", targetFrame)
	stream, err := soapCorpusReassembleTCP(segments)
	require.NoError(t, err, "capture TCP extraction must reject gaps and conflicting retransmissions")
	return stream
}
