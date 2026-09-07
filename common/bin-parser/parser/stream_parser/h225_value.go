package stream_parser

// This is an aligned-PER reader for the explicitly listed H.225 schema, not a
// general ASN.1 compiler. Schema source: ITU H.225.0 (12/2009), H323-MESSAGES,
// mirrored by Wireshark at revision 411f78566bed622a2ca2e6480c659486175db129:
// https://github.com/wireshark/wireshark/blob/411f78566bed622a2ca2e6480c659486175db129/epan/dissectors/asn1/h225/H323-MESSAGES.asn
// Encoding rules: ITU-T X.691, aligned variant. Unknown extension additions
// are length-delimited and retained; unknown root alternatives are rejected.
import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"unicode/utf16"
)

type h225Field struct {
	Name, Type string
	Start, End int // bit offsets from the bounded message, including PER padding
	Children   []h225Field
	List       bool
	Value      any
	Decoded    bool
}

type h225Member struct {
	name, typ string
	optional  bool
}
type h225Schema struct {
	kind                string
	lo, hi              uint64
	extensible          bool
	members, extensions []h225Member
	element             string
}

// A schema is immutable after package initialization. Each decode owns all
// cursor state, output values and recursion/resource counters.
var h225Schemas = h225BuildSchemas()

func h225BuildSchemas() map[string]h225Schema {
	m := map[string]h225Schema{}
	integer := func(n string, lo, hi uint64) { m[n] = h225Schema{kind: "integer", lo: lo, hi: hi} }
	integer("u8", 0, 255)
	integer("u16", 0, 65535)
	integer("u32", 0, 4294967295)
	integer("seqnum", 1, 65535)
	integer("ttl", 1, 4294967295)
	integer("hop", 1, 31)
	integer("priority", 0, 127)
	integer("multiplier", 1, 256)
	integer("standard", 0, 16383)
	m["standardExt"] = h225Schema{kind: "extInteger", element: "standard"}
	m["bool"] = h225Schema{kind: "bool"}
	m["null"] = h225Schema{kind: "null"}
	m["oid"] = h225Schema{kind: "oid"}
	for _, n := range []uint64{2, 4, 6, 16} {
		m[fmt.Sprint("oct", n)] = h225Schema{kind: "octets", lo: n, hi: n}
	}
	m["octets"] = h225Schema{kind: "octets", hi: 65535}
	m["product"] = h225Schema{kind: "octets", lo: 1, hi: 256}
	m["nsap"] = h225Schema{kind: "octets", lo: 1, hi: 20}
	m["userdata"] = h225Schema{kind: "octets", lo: 1, hi: 131}
	m["bmp"] = h225Schema{kind: "bmp", lo: 1, hi: 128}
	m["unboundedBMP"] = h225Schema{kind: "bmp", hi: 65535}
	m["group"] = h225Schema{kind: "ia5", lo: 1, hi: 128}
	m["aliasbmp"] = h225Schema{kind: "bmp", lo: 1, hi: 256}
	m["digits"] = h225Schema{kind: "digits", lo: 1, hi: 128}
	m["ia5"] = h225Schema{kind: "ia5", hi: 65535}
	m["url"] = h225Schema{kind: "ia5", lo: 1, hi: 512}
	m["language"] = h225Schema{kind: "ia5", lo: 1, hi: 32}
	parse := func(s string) []h225Member {
		var out []h225Member
		for _, f := range strings.Fields(s) {
			p := strings.SplitN(f, ":", 2)
			optional := strings.HasSuffix(p[1], "?")
			out = append(out, h225Member{p[0], strings.TrimSuffix(p[1], "?"), optional})
		}
		return out
	}
	seq := func(n, root, ext string, extensible bool) {
		m[n] = h225Schema{kind: "sequence", members: parse(root), extensions: parse(ext), extensible: extensible}
	}
	choice := func(n, root, ext string) {
		m[n] = h225Schema{kind: "choice", members: parse(root), extensions: parse(ext), extensible: true}
	}
	list := func(n, elem string) { m[n] = h225Schema{kind: "list", element: elem, hi: 65535} }
	list("addresses", "TransportAddress")
	list("aliases", "AliasAddress")
	list("rawlist", "octets")
	list("nons", "NonStandardParameter")
	list("references", "u16")
	list("languages", "language")
	list("protocols", "SupportedProtocols")
	list("routes", "oct4")
	list("alternateGK", "AlternateGK")
	list("integrities", "IntegrityMechanism")
	list("genericDatas", "GenericData")
	list("callsAvailable", "CallsAvailable")
	list("dataRates", "DataRate")
	list("prefixes", "SupportedPrefix")
	m["parameters"] = h225Schema{kind: "list", element: "EnumeratedParameter", lo: 1, hi: 512}
	m["nested"] = h225Schema{kind: "list", element: "GenericData", lo: 1, hi: 16}
	seq("GenericData", "id:GenericIdentifier parameters:parameters?", "", true)
	choice("GenericIdentifier", "standard:standardExt oid:oid nonStandard:oct16", "")
	seq("EnumeratedParameter", "id:GenericIdentifier content:Content?", "", true)
	choice("Content", "raw:octets text:ia5 unicode:unboundedBMP bool:bool number8:u8 number16:u16 number32:u32 id:GenericIdentifier alias:AliasAddress transport:TransportAddress compound:parameters nested:nested", "")
	seq("FeatureSet", "replacementFeatureSet:bool neededFeatures:genericDatas? desiredFeatures:genericDatas? supportedFeatures:genericDatas?", "", true)
	seq("DataRate", "nonStandardData:NonStandardParameter? channelRate:u32 channelMultiplier:multiplier?", "", true)
	seq("SupportedPrefix", "nonStandardData:NonStandardParameter? prefix:AliasAddress", "", true)
	seq("CallsAvailable", "calls:u32 group:group?", "carrier:opaque", true)
	seq("CallCapacity", "maximumCallCapacity:CallCapacityInfo? currentCallCapacity:CallCapacityInfo?", "", true)
	seq("CallCapacityInfo", "voiceGwCallsAvailable:callsAvailable? h310GwCallsAvailable:callsAvailable? h320GwCallsAvailable:callsAvailable? h321GwCallsAvailable:callsAvailable? h322GwCallsAvailable:callsAvailable? h323GwCallsAvailable:callsAvailable? h324GwCallsAvailable:callsAvailable? t120OnlyGwCallsAvailable:callsAvailable? t38FaxAnnexbOnlyGwCallsAvailable:callsAvailable? terminalCallsAvailable:callsAvailable? mcuCallsAvailable:callsAvailable?", "sipGwCallsAvailable:callsAvailable", true)
	choice("IntegrityMechanism", "nonStandard:NonStandardParameter digSig:null iso9797:oid nonIsoIM:unsupported", "")
	seq("IPv4", "ip:oct4 port:u16", "", false)
	seq("IPv6", "ip:oct16 port:u16", "", true)
	seq("IPX", "node:oct6 netnum:oct4 port:oct2", "", false)
	choice("Routing", "strict:null loose:null", "")
	seq("IPSourceRoute", "ip:oct4 port:u16 route:routes routing:Routing", "", true)
	choice("TransportAddress", "ipAddress:IPv4 ipSourceRoute:IPSourceRoute ipxAddress:IPX ip6Address:IPv6 netBios:oct16 nsap:nsap nonStandardAddress:NonStandardParameter", "")
	seq("H221NonStandard", "t35CountryCode:u8 t35Extension:u8 manufacturerCode:u16", "", true)
	choice("NonStandardIdentifier", "object:oid h221NonStandard:H221NonStandard", "")
	seq("NonStandardParameter", "nonStandardIdentifier:NonStandardIdentifier data:octets", "", false)
	seq("VendorIdentifier", "vendor:H221NonStandard productId:product? versionId:product?", "enterpriseNumber:oid", true)
	seq("TerminalInfo", "nonStandardData:NonStandardParameter?", "", true)
	seq("GatekeeperInfo", "nonStandardData:NonStandardParameter?", "", true)
	seq("McuInfo", "nonStandardData:NonStandardParameter?", "protocol:protocols", true)
	seq("GatewayInfo", "protocol:protocols? nonStandardData:NonStandardParameter?", "", true)
	seq("Capability", "nonStandardData:NonStandardParameter?", "dataRatesSupported:opaque supportedPrefixes:opaque", true)
	choice("SupportedProtocols", "nonStandardData:NonStandardParameter h310:Capability h320:Capability h321:Capability h322:Capability h323:Capability h324:Capability voice:Capability t120-only:Capability", "")
	seq("EndpointType", "nonStandardData:NonStandardParameter? vendor:VendorIdentifier? gatekeeper:GatekeeperInfo? gateway:GatewayInfo? mcu:McuInfo? terminal:TerminalInfo? mc:bool undefinedNode:bool", "set:opaque supportedTunnelledProtocols:opaque", true)
	choice("AliasAddress", "dialledDigits:digits h323-ID:aliasbmp", "url-ID:url transportID:TransportAddress email-ID:url partyNumber:opaque mobileUIM:opaque isupNumber:opaque")
	seq("CallIdentifier", "guid:oct16", "", true)
	seq("CallLinkage", "globalCallId:oct16? threadId:oct16?", "", true)
	choice("CallType", "pointToPoint:null oneToN:null nToOne:null nToN:null", "")
	choice("CallModel", "direct:null gatekeeperRouted:null", "")
	choice("ConferenceGoal", "create:null join:null invite:null", "capability-negotiation:null callIndependentSupplementaryService:null")
	choice("TransportQOS", "endpointControlled:null gatekeeperControlled:null noControl:null", "qOSCapabilities:opaque")
	seq("Q954Details", "conferenceCalling:bool threePartyService:bool", "", true)
	seq("QseriesOptions", "q932Full:bool q951Full:bool q952Full:bool q953Full:bool q955Full:bool q956Full:bool q957Full:bool q954Info:Q954Details", "", true)
	seq("AlternateGK", "rasAddress:TransportAddress gatekeeperIdentifier:bmp? needToRegister:bool priority:priority", "", true)
	seq("PreGrantedARQ", "makeCall:bool useGKCallSignalAddressToMakeCall:bool answerCall:bool useGKCallSignalAddressToAnswer:bool", "irrFrequencyInCall:seqnum totalBandwidthRestriction:u32 alternateTransportAddresses:opaque useSpecifiedTransport:opaque", true)
	seq("UUIEsRequested", "setup:bool callProceeding:bool connect:bool alerting:bool information:bool releaseComplete:bool facility:bool progress:bool empty:bool", "status:bool statusInquiry:bool setupAcknowledge:bool notify:bool", true)
	seq("GatekeeperRequest", "requestSeqNum:seqnum protocolIdentifier:oid nonStandardData:NonStandardParameter? rasAddress:TransportAddress endpointType:EndpointType gatekeeperIdentifier:bmp? callServices:QseriesOptions? endpointAlias:aliases?", "alternateEndpoints:opaque tokens:opaque cryptoTokens:opaque authenticationCapability:opaque algorithmOIDs:opaque integrity:opaque integrityCheckValue:opaque supportsAltGK:null featureSet:opaque genericData:opaque supportsAssignedGK:bool assignedGatekeeper:opaque", true)
	seq("GatekeeperConfirm", "requestSeqNum:seqnum protocolIdentifier:oid nonStandardData:NonStandardParameter? gatekeeperIdentifier:bmp? rasAddress:TransportAddress", "alternateGatekeeper:alternateGK authenticationMode:opaque tokens:opaque cryptoTokens:opaque algorithmOID:oid integrity:opaque integrityCheckValue:opaque featureSet:opaque genericData:opaque assignedGatekeeper:opaque rehomingModel:opaque", true)
	seq("RegistrationRequest", "requestSeqNum:seqnum protocolIdentifier:oid nonStandardData:NonStandardParameter? discoveryComplete:bool callSignalAddress:addresses rasAddress:addresses terminalType:EndpointType terminalAlias:aliases? gatekeeperIdentifier:bmp? endpointVendor:VendorIdentifier", "alternateEndpoints:opaque timeToLive:ttl tokens:opaque cryptoTokens:opaque integrityCheckValue:opaque keepAlive:bool endpointIdentifier:bmp willSupplyUUIEs:bool maintainConnection:bool alternateTransportAddresses:opaque additiveRegistration:null terminalAliasPattern:opaque supportsAltGK:null usageReportingCapability:opaque multipleCalls:bool supportedH248Packages:opaque callCreditCapability:opaque capacityReportingCapability:opaque capacity:opaque featureSet:opaque genericData:opaque restart:null supportsACFSequences:null supportsAssignedGK:bool assignedGatekeeper:opaque transportQOS:TransportQOS language:languages", true)
	seq("RegistrationConfirm", "requestSeqNum:seqnum protocolIdentifier:oid nonStandardData:NonStandardParameter? callSignalAddress:addresses terminalAlias:aliases? gatekeeperIdentifier:bmp? endpointIdentifier:bmp", "alternateGatekeeper:alternateGK timeToLive:ttl tokens:opaque cryptoTokens:opaque integrityCheckValue:opaque willRespondToIRR:bool preGrantedARQ:PreGrantedARQ maintainConnection:bool serviceControl:opaque supportsAdditiveRegistration:null terminalAliasPattern:opaque supportedPrefixes:opaque usageSpec:opaque featureServerAlias:opaque capacityReportingSpec:opaque featureSet:opaque genericData:opaque assignedGatekeeper:opaque rehomingModel:opaque transportQOS:TransportQOS", true)
	seq("AdmissionRequest", "requestSeqNum:seqnum callType:CallType callModel:CallModel? endpointIdentifier:bmp destinationInfo:aliases? destCallSignalAddress:TransportAddress? destExtraCallInfo:aliases? srcInfo:aliases srcCallSignalAddress:TransportAddress? bandWidth:u32 callReferenceValue:u16 nonStandardData:NonStandardParameter? callServices:QseriesOptions? conferenceID:oct16 activeMC:bool answerCall:bool", "canMapAlias:bool callIdentifier:CallIdentifier srcAlternatives:opaque destAlternatives:opaque gatekeeperIdentifier:bmp tokens:opaque cryptoTokens:opaque integrityCheckValue:opaque transportQOS:TransportQOS willSupplyUUIEs:bool callLinkage:CallLinkage gatewayDataRate:opaque capacity:opaque circuitInfo:opaque desiredProtocols:protocols desiredTunnelledProtocol:opaque featureSet:opaque genericData:opaque canMapSrcAlias:bool", true)
	seq("AdmissionConfirm", "requestSeqNum:seqnum bandWidth:u32 callModel:CallModel destCallSignalAddress:TransportAddress irrFrequency:seqnum? nonStandardData:NonStandardParameter?", "destinationInfo:aliases destExtraCallInfo:aliases destinationType:EndpointType remoteExtensionAddress:AliasAddress alternateEndpoints:opaque tokens:opaque cryptoTokens:opaque integrityCheckValue:opaque transportQOS:TransportQOS willRespondToIRR:bool uuiesRequested:UUIEsRequested language:languages alternateTransportAddresses:opaque useSpecifiedTransport:opaque circuitInfo:opaque usageSpec:opaque supportedProtocols:protocols serviceControl:opaque multipleCalls:bool featureSet:opaque genericData:opaque modifiedSrcInfo:aliases assignedGatekeeper:opaque", true)
	choice("DisengageReason", "forcedDrop:null normalDrop:null undefinedReason:null", "")
	seq("DisengageRequest", "requestSeqNum:seqnum endpointIdentifier:bmp conferenceID:oct16 callReferenceValue:u16 disengageReason:DisengageReason nonStandardData:NonStandardParameter?", "callIdentifier:CallIdentifier gatekeeperIdentifier:bmp tokens:opaque cryptoTokens:opaque integrityCheckValue:opaque answeredCall:bool callLinkage:CallLinkage capacity:opaque circuitInfo:opaque usageInformation:opaque terminationCause:opaque serviceControl:opaque genericData:opaque", true)
	seq("InfoRequest", "requestSeqNum:seqnum callReferenceValue:u16 nonStandardData:NonStandardParameter? replyAddress:TransportAddress?", "callIdentifier:CallIdentifier tokens:opaque cryptoTokens:opaque integrityCheckValue:opaque uuiesRequested:UUIEsRequested callLinkage:CallLinkage usageInfoRequested:opaque segmentedResponseSupported:null nextSegmentRequested:u16 capacityInfoRequested:null genericData:opaque assignedGatekeeper:opaque", true)
	choice("RasMessage", "gatekeeperRequest:GatekeeperRequest gatekeeperConfirm:GatekeeperConfirm gatekeeperReject:unsupported registrationRequest:RegistrationRequest registrationConfirm:RegistrationConfirm registrationReject:unsupported unregistrationRequest:unsupported unregistrationConfirm:unsupported unregistrationReject:unsupported admissionRequest:AdmissionRequest admissionConfirm:AdmissionConfirm admissionReject:unsupported bandwidthRequest:unsupported bandwidthConfirm:unsupported bandwidthReject:unsupported disengageRequest:DisengageRequest disengageConfirm:unsupported disengageReject:unsupported locationRequest:unsupported locationConfirm:unsupported locationReject:unsupported infoRequest:InfoRequest infoRequestResponse:unsupported nonStandardMessage:unsupported unknownMessageResponse:unsupported", "")
	common := "callIdentifier:CallIdentifier h245SecurityMode:opaque tokens:opaque cryptoTokens:opaque fastStart:rawlist multipleCalls:bool maintainConnection:bool"
	seq("CallProceeding-UUIE", "protocolIdentifier:oid destinationInfo:EndpointType h245Address:TransportAddress?", common+" fastConnectRefused:null featureSet:opaque", true)
	seq("Alerting-UUIE", "protocolIdentifier:oid destinationInfo:EndpointType h245Address:TransportAddress?", common+" alertingAddress:aliases presentationIndicator:opaque screeningIndicator:opaque fastConnectRefused:null serviceControl:opaque capacity:opaque featureSet:opaque displayName:opaque", true)
	seq("Connect-UUIE", "protocolIdentifier:oid h245Address:TransportAddress? destinationInfo:EndpointType conferenceID:oct16", common+" language:languages connectedAddress:aliases presentationIndicator:opaque screeningIndicator:opaque fastConnectRefused:null serviceControl:opaque capacity:opaque featureSet:opaque displayName:opaque", true)
	seq("Information-UUIE", "protocolIdentifier:oid", "callIdentifier:CallIdentifier tokens:opaque cryptoTokens:opaque fastStart:rawlist fastConnectRefused:null circuitInfo:opaque", true)
	choice("ReleaseCompleteReason", "noBandwidth:null gatekeeperResources:null unreachableDestination:null destinationRejection:null invalidRevision:null noPermission:null unreachableGatekeeper:null gatewayResources:null badFormatAddress:null adaptiveBusy:null inConf:null undefinedReason:null", "")
	seq("ReleaseComplete-UUIE", "protocolIdentifier:oid reason:ReleaseCompleteReason?", "callIdentifier:CallIdentifier tokens:opaque cryptoTokens:opaque busyAddress:aliases presentationIndicator:opaque screeningIndicator:opaque capacity:opaque serviceControl:opaque featureSet:opaque destinationInfo:EndpointType displayName:opaque", true)
	seq("Setup-UUIE", "protocolIdentifier:oid h245Address:TransportAddress? sourceAddress:aliases? sourceInfo:EndpointType destinationAddress:aliases? destCallSignalAddress:TransportAddress? destExtraCallInfo:aliases? destExtraCRV:references? activeMC:bool conferenceID:oct16 conferenceGoal:ConferenceGoal callServices:QseriesOptions? callType:CallType", "sourceCallSignalAddress:TransportAddress remoteExtensionAddress:AliasAddress callIdentifier:CallIdentifier h245SecurityCapability:opaque tokens:opaque cryptoTokens:opaque fastStart:rawlist mediaWaitForConnect:bool canOverlapSend:bool endpointIdentifier:bmp multipleCalls:bool maintainConnection:bool connectionParameters:opaque language:languages presentationIndicator:opaque screeningIndicator:opaque serviceControl:opaque symmetricOperationRequired:null capacity:opaque circuitInfo:opaque desiredProtocols:protocols neededFeatures:opaque desiredFeatures:opaque supportedFeatures:opaque parallelH245Control:rawlist additionalSourceAddresses:aliases hopCount:hop displayName:opaque", true)
	choice("CallBody", "setup:Setup-UUIE callProceeding:CallProceeding-UUIE connect:Connect-UUIE alerting:Alerting-UUIE information:Information-UUIE releaseComplete:ReleaseComplete-UUIE facility:unsupported", "progress:opaque empty:null status:opaque statusInquiry:opaque setupAcknowledge:opaque notify:opaque")
	seq("H323-UU-PDU", "h323-message-body:CallBody nonStandardData:NonStandardParameter?", "h4501SupplementaryService:rawlist h245Tunnelling:bool h245Control:rawlist nonStandardControl:nons callLinkage:CallLinkage tunnelledSignallingMessage:opaque provisionalRespToH245Tunnelling:null stimulusControl:opaque genericData:opaque", true)
	seq("UserData", "protocol-discriminator:u8 user-information:userdata", "", true)
	seq("H323-UserInformation", "h323-uu-pdu:H323-UU-PDU user-data:UserData?", "", true)
	// These additions are shared schema types, not arbitrary payload guesses.
	known := map[string]string{"integrity": "integrities", "featureSet": "FeatureSet", "genericData": "genericDatas", "capacity": "CallCapacity", "gatewayDataRate": "DataRate", "dataRatesSupported": "dataRates", "supportedPrefixes": "prefixes"}
	for name, schema := range m {
		for i, member := range schema.extensions {
			if member.typ == "opaque" {
				if typ, ok := known[member.name]; ok {
					schema.extensions[i].typ = typ
				}
			}
		}
		m[name] = schema
	}
	return m
}

type h225PER struct {
	wire                            []byte
	pos, end, fields, depth, opaque int
}

func (r *h225PER) fail(s string) error { return fmt.Errorf("h225: PER bit %d: %s", r.pos, s) }
func (r *h225PER) value(n int) (uint64, error) {
	if n < 0 || n > 64 || n > r.end-r.pos {
		return 0, r.fail("truncated bit field")
	}
	var v uint64
	for i := 0; i < n; i++ {
		v = v<<1 | uint64((r.wire[r.pos/8]>>uint(7-r.pos%8))&1)
		r.pos++
	}
	return v, nil
}
func (r *h225PER) scalar(out *[]h225Field, name, typ string, n int) (uint64, error) {
	start := r.pos
	v, e := r.value(n)
	if e != nil {
		return 0, e
	}
	*out = append(*out, h225Field{Name: name, Type: typ, Start: start, End: r.pos})
	return v, nil
}
func (r *h225PER) align(out *[]h225Field) error {
	n := (-r.pos) & 7
	if n == 0 {
		return nil
	}
	v, e := r.scalar(out, "PER Padding", "raw", n)
	if e != nil {
		return e
	}
	if v != 0 {
		return r.fail("nonzero alignment padding")
	}
	return nil
}
func (r *h225PER) constrained(out *[]h225Field, name string, lo, hi uint64) (uint64, error) {
	if hi < lo {
		return 0, r.fail("invalid constraint")
	}
	count := hi - lo + 1
	if count == 1 {
		*out = append(*out, h225Field{Name: name, Type: "uint64", Start: r.pos, End: r.pos, Value: lo, Decoded: true})
		return lo, nil
	}
	n := bits.Len64(count - 1)
	if count >= 256 {
		if count <= 65536 {
			if e := r.align(out); e != nil {
				return 0, e
			}
			if count == 256 {
				n = 8
			} else {
				n = 16
			}
		} else {
			k, e := r.constrained(out, name+" Octet Count", 1, uint64((n+7)/8))
			if e != nil {
				return 0, e
			}
			n = int(k) * 8
			if e = r.align(out); e != nil {
				return 0, e
			}
		}
	}
	typ := "uint8"
	if n > 8 {
		typ = "uint16"
	}
	if n > 16 {
		typ = "uint32"
	}
	if n > 32 {
		typ = "uint64"
	}
	v, e := r.scalar(out, name, typ, n)
	if e != nil {
		return 0, e
	}
	if v >= count {
		return 0, r.fail("constrained integer out of range")
	}
	v += lo
	if lo != 0 {
		p := &(*out)[len(*out)-1]
		p.Value = v
		p.Decoded = true
	}
	return v, nil
}
func (r *h225PER) length(out *[]h225Field, name string) (int, error) {
	if e := r.align(out); e != nil {
		return 0, e
	}
	v, e := r.scalar(out, name, "uint8", 8)
	if e != nil {
		return 0, e
	}
	if v < 128 {
		return int(v), nil
	}
	if v >= 192 {
		return 0, r.fail("fragmented length outside bounded profile")
	}
	w, e := r.scalar(out, name+" Low", "uint8", 8)
	if e != nil {
		return 0, e
	}
	n := int(v&63)<<8 | int(w)
	if n < 128 {
		return 0, r.fail("nonminimal length determinant")
	}
	return n, nil
}
func (r *h225PER) small(out *[]h225Field, name string) (int, error) {
	v, e := r.scalar(out, name+" Large", "uint8", 1)
	if e != nil {
		return 0, e
	}
	if v == 0 {
		x, e := r.scalar(out, name, "uint8", 6)
		return int(x), e
	}
	n, e := r.length(out, name+" Octets")
	if e != nil {
		return 0, e
	}
	if n < 1 || n > 2 {
		return 0, r.fail("normally small integer exceeds profile")
	}
	x, e := r.scalar(out, name, "uint16", n*8)
	if e != nil {
		return 0, e
	}
	if x < 64 {
		return 0, r.fail("nonminimal normally small integer")
	}
	return int(x), nil
}
func (r *h225PER) raw(out *[]h225Field, name string, n int) error {
	if n < 0 || n > r.end-r.pos {
		return r.fail("truncated content")
	}
	*out = append(*out, h225Field{Name: name, Type: "raw", Start: r.pos, End: r.pos + n})
	r.pos += n
	return nil
}
func (r *h225PER) finish(out *[]h225Field) error {
	if r.end-r.pos > 7 {
		return r.fail("unconsumed PER octets")
	}
	if e := r.align(out); e != nil {
		return e
	}
	if r.pos != r.end {
		return r.fail("unaligned message boundary")
	}
	return nil
}
func (r *h225PER) open(out *[]h225Field, name, typ string) error {
	start := r.pos
	f := h225Field{Name: name, Start: start}
	n, e := r.length(&f.Children, "Open Type Length")
	if e != nil {
		return e
	}
	if n < 1 || n > (r.end-r.pos)/8 {
		return r.fail("invalid open type length")
	}
	end := r.pos + n*8
	if typ == "opaque" || typ == "" {
		if e = r.raw(&f.Children, "Uninterpreted Extension", n*8); e != nil {
			return e
		}
		r.opaque++
	} else {
		sub := *r
		sub.end = end
		if typ == "null" {
			if n != 1 {
				return r.fail("NULL open type must contain one zero octet")
			}
			if e = sub.raw(&f.Children, "NULL Open Type Padding", n*8); e != nil {
				return e
			}
			for _, b := range r.wire[r.pos/8 : end/8] {
				if b != 0 {
					return r.fail("nonzero NULL open type")
				}
			}
		} else {
			if e = sub.decode(&f.Children, name+" Value", typ); e != nil {
				return e
			}
			if e = sub.finish(&f.Children); e != nil {
				return e
			}
		}
		r.pos = end
		r.fields = sub.fields
		r.opaque = sub.opaque
	}
	f.End = r.pos
	*out = append(*out, f)
	return nil
}
func (r *h225PER) decode(out *[]h225Field, name, typ string) error {
	r.depth++
	defer func() { r.depth-- }()
	r.fields++
	if r.depth > 48 || r.fields > 8192 {
		return r.fail("schema resource limit")
	}
	s, ok := h225Schemas[typ]
	if !ok {
		return r.fail("unsupported root schema " + typ)
	}
	f := h225Field{Name: name, Start: r.pos}
	var e error
	switch s.kind {
	case "null":
		return nil
	case "bool":
		_, e = r.scalar(out, name, "uint8", 1)
		return e
	case "integer":
		_, e = r.constrained(out, name, s.lo, s.hi)
		return e
	case "extInteger":
		v, e := r.scalar(&f.Children, "Integer Extension", "uint8", 1)
		if e != nil {
			return e
		}
		if v != 0 {
			return r.fail("extended integer outside supported profile")
		}
		if e = r.decode(&f.Children, name+" Value", s.element); e != nil {
			return e
		}
	case "sequence":
		ext := uint64(0)
		if s.extensible {
			ext, e = r.scalar(&f.Children, "Extension Present", "uint8", 1)
			if e != nil {
				return e
			}
		}
		present := make([]bool, len(s.members))
		for i, v := range s.members {
			present[i] = true
			if v.optional {
				x, e := r.scalar(&f.Children, v.name+" Present", "uint8", 1)
				if e != nil {
					return e
				}
				present[i] = x != 0
			}
		}
		for i, v := range s.members {
			if present[i] {
				if e = r.decode(&f.Children, v.name, v.typ); e != nil {
					return e
				}
			}
		}
		if ext != 0 {
			n, e := r.small(&f.Children, "Extension Bitmap Length Minus One")
			if e != nil {
				return e
			}
			n++
			if n > 256 {
				return r.fail("too many extension additions")
			}
			p := make([]bool, n)
			for i := range p {
				v, e := r.scalar(&f.Children, fmt.Sprintf("Extension %d Present", i), "uint8", 1)
				if e != nil {
					return e
				}
				p[i] = v != 0
			}
			for i, v := range p {
				if v {
					nm, tp := fmt.Sprintf("Extension %d", i), "opaque"
					if i < len(s.extensions) {
						nm, tp = s.extensions[i].name, s.extensions[i].typ
					}
					if e = r.open(&f.Children, nm, tp); e != nil {
						return e
					}
				}
			}
		}
	case "choice":
		ext, e := r.scalar(&f.Children, "Choice Extension", "uint8", 1)
		if e != nil {
			return e
		}
		if ext != 0 {
			i, e := r.small(&f.Children, name+" Extension Choice")
			if e != nil {
				return e
			}
			nm, tp := fmt.Sprintf("Extension Choice %d", i), "opaque"
			if i < len(s.extensions) {
				nm, tp = s.extensions[i].name, s.extensions[i].typ
			}
			if e = r.open(&f.Children, nm, tp); e != nil {
				return e
			}
		} else {
			i, e := r.constrained(&f.Children, name+" Choice", 0, uint64(len(s.members)-1))
			if e != nil {
				return e
			}
			v := s.members[i]
			if e = r.decode(&f.Children, v.name, v.typ); e != nil {
				return e
			}
		}
	case "list":
		var n int
		if s.hi == 65535 {
			n, e = r.length(&f.Children, name+" Count")
		} else {
			var k uint64
			k, e = r.constrained(&f.Children, name+" Count", s.lo, s.hi)
			n = int(k)
		}
		if e != nil {
			return e
		}
		if n > 1024 {
			return r.fail("sequence-of count exceeds profile")
		}
		items := h225Field{Name: name + " Items", Start: r.pos, List: true}
		for i := 0; i < n; i++ {
			item := h225Field{Name: fmt.Sprintf("Item %d", i), Start: r.pos}
			if e = r.decode(&item.Children, "Value", s.element); e != nil {
				return e
			}
			item.End = r.pos
			items.Children = append(items.Children, item)
		}
		items.End = r.pos
		f.Children = append(f.Children, items)
	case "octets", "bmp", "ia5", "digits":
		n := s.lo
		if s.lo != s.hi {
			if s.hi == 65535 {
				k, e := r.length(&f.Children, name+" Length")
				if e != nil {
					return e
				}
				n = uint64(k)
			} else {
				n, e = r.constrained(&f.Children, name+" Length", s.lo, s.hi)
				if e != nil {
					return e
				}
			}
		}
		width := 8
		if s.kind == "bmp" {
			width = 16
		}
		if s.kind == "digits" {
			width = 4
		}
		if n > 65535 {
			return r.fail("string length exceeds profile")
		}
		if s.hi*uint64(width) > 16 {
			if e = r.align(&f.Children); e != nil {
				return e
			}
		}
		start := r.pos
		if e = r.raw(&f.Children, name+" Bytes", int(n)*width); e != nil {
			return e
		}
		if s.kind != "octets" {
			leaf := &f.Children[len(f.Children)-1]
			leaf.Type = "string"
			leaf.Decoded = true
			sub := h225PER{wire: r.wire, pos: start, end: r.pos}
			var text strings.Builder
			if s.kind == "bmp" {
				units := make([]uint16, n)
				for i := range units {
					x, _ := sub.value(16)
					units[i] = uint16(x)
					if x >= 0xd800 && x <= 0xdfff {
						return r.fail("BMPString contains surrogate")
					}
				}
				text.WriteString(string(utf16.Decode(units)))
			} else {
				for i := uint64(0); i < n; i++ {
					x, _ := sub.value(width)
					if s.kind == "digits" {
						const alphabet = ",#*0123456789"
						if x >= uint64(len(alphabet)) {
							return r.fail("restricted alphabet index out of range")
						}
						text.WriteByte(alphabet[x])
					} else {
						if x > 127 {
							return r.fail("non-IA5 character")
						}
						text.WriteByte(byte(x))
					}
				}
			}
			leaf.Value = text.String()
		}
	case "oid":
		n, e := r.length(&f.Children, "OID Length")
		if e != nil {
			return e
		}
		if n < 1 || n > 64 {
			return r.fail("OID length outside profile")
		}
		start := r.pos
		if e = r.raw(&f.Children, name+" Bytes", n*8); e != nil {
			return e
		}
		var arcs []string
		var v uint64
		first := true
		inArc := false
		for _, b := range r.wire[start/8 : r.pos/8] {
			if !inArc && b == 128 {
				return r.fail("nonminimal OID arc")
			}
			if v > 0x1ffffffffffffff {
				return r.fail("OID arc overflow")
			}
			v = v<<7 | uint64(b&127)
			inArc = b&128 != 0
			if !inArc {
				if first {
					a := uint64(2)
					if v < 80 {
						a = v / 40
					}
					arcs = append(arcs, strconv.FormatUint(a, 10), strconv.FormatUint(v-a*40, 10))
					first = false
				} else {
					arcs = append(arcs, strconv.FormatUint(v, 10))
				}
				v = 0
			}
		}
		if inArc {
			return r.fail("truncated OID arc")
		}
		leaf := &f.Children[len(f.Children)-1]
		leaf.Type = "string"
		leaf.Decoded = true
		leaf.Value = strings.Join(arcs, ".")
		if name == "protocolIdentifier" {
			if len(arcs) != 6 || strings.Join(arcs[:5], ".") != "0.0.8.2250.0" {
				return r.fail("not an H225 protocol identifier")
			}
			version, e := strconv.Atoi(arcs[5])
			if e != nil || version < 1 || version > 7 {
				return r.fail("H225 version outside 1..7 profile")
			}
		}
	default:
		return r.fail("unsupported schema kind")
	}
	f.End = r.pos
	*out = append(*out, f)
	return nil
}

func decodeH225Message(wire []byte, mode string) ([]h225Field, map[string]any, error) {
	if len(wire) == 0 || len(wire) > 65535 {
		return nil, nil, fmt.Errorf("h225: message must contain 1..65535 bytes")
	}
	info := map[string]any{"Profile": "H225 aligned PER bounded fields", "Mode": mode, "Application Semantics": "Not interpreted", "Unknown Extensions": 0}
	if mode == "ras" {
		r := h225PER{wire: wire, end: len(wire) * 8}
		var f []h225Field
		if e := r.decode(&f, "RasMessage", "RasMessage"); e != nil {
			return nil, nil, e
		}
		if e := r.finish(&f); e != nil {
			return nil, nil, e
		}
		info["Unknown Extensions"] = r.opaque
		return f, info, nil
	}
	if mode != "call" {
		return nil, nil, fmt.Errorf("h225: unknown explicit mode")
	}
	if len(wire) < 8 || wire[0] != 3 || wire[1] != 0 || int(binary.BigEndian.Uint16(wire[2:4])) != len(wire) {
		return nil, nil, fmt.Errorf("h225: invalid exact TPKT boundary")
	}
	var f []h225Field
	add := func(name, typ string, start, end int) {
		f = append(f, h225Field{Name: name, Type: typ, Start: start * 8, End: end * 8})
	}
	add("TPKT Version", "uint8", 0, 1)
	add("TPKT Reserved", "uint8", 1, 2)
	add("TPKT Length", "uint16", 2, 4)
	if wire[4] != 8 {
		return nil, nil, fmt.Errorf("h225: Q931 discriminator must be 8")
	}
	add("Q931 Protocol Discriminator", "uint8", 4, 5)
	n := int(wire[5])
	if n > 15 || n > len(wire)-7 {
		return nil, nil, fmt.Errorf("h225: invalid Q931 call reference length")
	}
	add("Q931 Call Reference Length", "uint8", 5, 6)
	add("Q931 Call Reference", "raw", 6, 6+n)
	pos := 6 + n
	msg := wire[pos]
	add("Q931 Message Type", "uint8", pos, pos+1)
	pos++
	expected := map[byte]uint64{5: 0, 2: 1, 7: 2, 1: 3, 0x7b: 4, 0x5a: 5, 0x62: 6}
	want, ok := expected[msg]
	if !ok {
		return nil, nil, fmt.Errorf("h225: unsupported Q931 message type %x", msg)
	}
	seen := false
	for pos < len(wire) {
		start := pos
		id := wire[pos]
		pos++
		ie := h225Field{Name: fmt.Sprintf("Q931 IE 0x%02x", id), Start: start * 8}
		ie.Children = append(ie.Children, h225Field{Name: "Q931 IE Identifier", Type: "uint8", Start: start * 8, End: pos * 8})
		if id&128 != 0 {
			ie.End = pos * 8
			f = append(f, ie)
			continue
		}
		width := 1
		if id == 0x7e {
			width = 2
		}
		if len(wire)-pos < width {
			return nil, nil, fmt.Errorf("h225: truncated Q931 IE length")
		}
		length := int(wire[pos])
		if width == 2 {
			length = int(binary.BigEndian.Uint16(wire[pos : pos+2]))
		}
		ie.Children = append(ie.Children, h225Field{Name: "Q931 IE Length", Type: map[int]string{1: "uint8", 2: "uint16"}[width], Start: pos * 8, End: (pos + width) * 8})
		pos += width
		if length > len(wire)-pos {
			return nil, nil, fmt.Errorf("h225: truncated Q931 IE")
		}
		end := pos + length
		if id == 0x7e {
			if seen || length < 2 || wire[pos] != 5 {
				return nil, nil, fmt.Errorf("h225: invalid H323 user information IE")
			}
			seen = true
			ie.Children = append(ie.Children, h225Field{Name: "User Information Discriminator", Type: "uint8", Start: pos * 8, End: (pos + 1) * 8})
			pos++
			r := h225PER{wire: wire, pos: pos * 8, end: end * 8}
			if e := r.decode(&ie.Children, "H323-UserInformation", "H323-UserInformation"); e != nil {
				return nil, nil, e
			}
			if e := r.finish(&ie.Children); e != nil {
				return nil, nil, e
			}
			info["Unknown Extensions"] = r.opaque
			// Q.931 and H323-UU-PDU must describe the same message alternative.
			var actual uint64
			found := false
			var walk func([]h225Field)
			walk = func(fs []h225Field) {
				for _, v := range fs {
					if v.Name == "h323-message-body Choice" {
						q := h225PER{wire: wire, pos: v.Start, end: v.End}
						actual, _ = q.value(v.End - v.Start)
						found = true
					}
					walk(v.Children)
				}
			}
			walk(ie.Children)
			if !found || actual != want {
				return nil, nil, fmt.Errorf("h225: Q931 and H323 message choices disagree")
			}
		} else {
			ie.Children = append(ie.Children, h225Field{Name: "Uninterpreted Q931 IE Data", Type: "raw", Start: pos * 8, End: end * 8})
		}
		pos = end
		ie.End = pos * 8
		f = append(f, ie)
	}
	if !seen {
		return nil, nil, fmt.Errorf("h225: Q931 has no H323 user information")
	}
	return f, info, nil
}
