package pcapx

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

var icmpLayerExports = map[string]any{
	"ICMP_TYPE_ECHO_REQUEST":                          layers.ICMPv4TypeEchoRequest,
	"ICMP_TYPE_ECHO_REPLY":                            layers.ICMPv4TypeEchoReply,
	"ICMP_TYPE_DEST_UNREACH":                          layers.ICMPv4TypeDestinationUnreachable,
	"ICMP_TYPE_SRC_QUENCH":                            layers.ICMPv4TypeSourceQuench,
	"ICMP_TYPE_REDIRECT":                              layers.ICMPv4TypeRedirect,
	"ICMP_TYPE_ROUTER_ADVERTISEMENT":                  layers.ICMPv4TypeRouterAdvertisement,
	"ICMP_TYPE_ROUTER_SOLICITATION":                   layers.ICMPv4TypeRouterSolicitation,
	"ICMP_TYPE_TIME_EXCEEDED":                         layers.ICMPv4TypeTimeExceeded,
	"ICMP_TYPE_PARAM_PROBLEM":                         layers.ICMPv4TypeParameterProblem,
	"ICMP_TYPE_TIMESTAMP":                             layers.ICMPv4TypeTimestampRequest,
	"ICMP_TYPE_TIMESTAMP_REPLY":                       layers.ICMPv4TypeTimestampReply,
	"ICMP_TYPE_INFO_REQUEST":                          layers.ICMPv4TypeInfoRequest,
	"ICMP_TYPE_INFO_REPLY":                            layers.ICMPv4TypeInfoReply,
	"ICMP_TYPE_ADDRESS_MASK_REQUEST":                  layers.ICMPv4TypeAddressMaskRequest,
	"ICMP_TYPE_ADDRESS_MASK_REPLY":                    layers.ICMPv4TypeAddressMaskReply,
	"ICMP_CODE_UNREACH_NET":                           layers.ICMPv4CodeNet,
	"ICMP_CODE_UNREACH_HOST":                          layers.ICMPv4CodeHost,
	"ICMP_CODE_UNREACH_PROTOCOL":                      layers.ICMPv4CodeProtocol,
	"ICMP_CODE_UNREACH_PORT":                          layers.ICMPv4CodePort,
	"ICMP_CODE_UNREACH_FRAGMENTATION_NEEDED":          layers.ICMPv4CodeFragmentationNeeded,
	"ICMP_CODE_UNREACH_SRC_ROUTE_FAIL":                layers.ICMPv4CodeSourceRoutingFailed,
	"ICMP_CODE_UNREACH_NET_UNKNOWN":                   layers.ICMPv4CodeNetUnknown,
	"ICMP_CODE_UNREACH_HOST_UNKNOWN":                  layers.ICMPv4CodeHostUnknown,
	"ICMP_CODE_UNREACH_SRC_ISOLATED":                  layers.ICMPv4CodeSourceIsolated,
	"ICMP_CODE_UNREACH_NET_ADMIN":                     layers.ICMPv4CodeNetAdminProhibited,
	"ICMP_CODE_UNREACH_HOST_ADMIN":                    layers.ICMPv4CodeHostAdminProhibited,
	"ICMP_CODE_UNREACH_NET_TOS":                       layers.ICMPv4CodeNetTOS,
	"ICMP_CODE_UNREACH_HOST_TOS":                      layers.ICMPv4CodeHostTOS,
	"ICMP_CODE_UNREACH_COMM_ADMIN":                    layers.ICMPv4CodeCommAdminProhibited,
	"ICMP_CODE_UNREACH_HOST_PRECEDENCE":               layers.ICMPv4CodeHostPrecedence,
	"ICMP_CODE_UNREACH_PRECEDENCE_CUTOFF":             layers.ICMPv4CodePrecedenceCutoff,
	"ICMP_CODE_TIME_EXCEEDED_TTL":                     layers.ICMPv4CodeTTLExceeded,
	"ICMP_CODE_TIME_EXCEEDED_FRAG_REASS":              layers.ICMPv4CodeFragmentReassemblyTimeExceeded,
	"ICMP_CODE_PARAM_PROBLEM_POINTER_INDICATES_ERROR": layers.ICMPv4CodePointerIndicatesError,
	"ICMP_CODE_PARAM_PROBLEM_MISSING_OPTION":          layers.ICMPv4CodeMissingOption,
	"ICMP_CODE_PARAM_PROBLEM_BAD_LENGTH":              layers.ICMPv4CodeBadLength,
	"ICMP_CODE_REDIRECT_NET":                          layers.ICMPv4CodeNet,
	"ICMP_CODE_REDIRECT_HOST":                         layers.ICMPv4CodeHost,
	"ICMP_CODE_REDIRECT_TOS_NET":                      layers.ICMPv4CodeNetTOS,
	"ICMP_CODE_REDIRECT_TOS_HOST":                     layers.ICMPv4CodeHostTOS,

	"icmp_type":    WithICMP_Type,
	"icmp_id":      WithICMP_Id,
	"icmp_seq":     WithICMP_Sequence,
	"icmp_payload": WithICMP_Payload,
}

func init() {
	for k, v := range icmpLayerExports {
		Exports[k] = v
	}
}

type ICMPOption func(pv4 *layers.ICMPv4) error

func WithICMP_Type(icmpType any, icmpCode any) ICMPOption {
	return func(pv4 *layers.ICMPv4) error {
		if funk.IsEmpty(icmpCode) {
			icmpCode = 0
		}
		pv4.TypeCode = layers.CreateICMPv4TypeCode(uint8(utils.InterfaceToInt(icmpType)), uint8(utils.InterfaceToInt(icmpCode)))
		return nil
	}
}

func WithICMP_Id(id any) ICMPOption {
	return func(pv4 *layers.ICMPv4) error {
		pv4.Id = uint16(utils.InterfaceToInt(id))
		return nil
	}
}

func WithICMP_Sequence(sequence any) ICMPOption {
	return func(pv4 *layers.ICMPv4) error {
		pv4.Seq = uint16(utils.InterfaceToInt(sequence))
		return nil
	}
}

func WithICMP_Payload(i []byte) ICMPOption {
	return func(pv4 *layers.ICMPv4) error {
		pv4.Payload = i
		return nil
	}
}
