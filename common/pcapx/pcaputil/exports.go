package pcaputil

import (
	"github.com/yaklang/yaklang/common/utils"
)

var Exports = map[string]any{
	"StartSniff":   Sniff,
	"OpenPcapFile": OpenPcapFile,

	"pcap_bpfFilter":                    WithBPFFilter,
	"pcap_onFlowCreated":                WithOnTrafficFlowCreated,
	"pcap_onFlowClosed":                 WithOnTrafficFlowClosed,
	"pcap_onFlowDataFrameNoReassembled": WithOnTrafficFlowOnDataFrameArrived,
	"pcap_onFlowDataFrame":              WithOnTrafficFlowOnDataFrameReassembled,
	"pcap_onTLSClientHello":             WithTLSClientHello,
	"pcap_onHTTPRequest":                WithHTTPRequest,
	"pcap_onHTTPFlow":                   WithHTTPFlow,
	"pcap_everyPacket":                  WithEveryPacket,
	"pcap_debug":                        WithDebug,
}

func Sniff(iface string, opts ...CaptureOption) error {
	opts = append(opts, WithDevice(utils.PrettifyListFromStringSplited(iface, ",")...), WithFile(""))
	return Start(opts...)
}

func OpenPcapFile(filename string, opts ...CaptureOption) error {
	opts = append(opts, WithDevice(), WithFile(filename))
	return Start(opts...)
}
