package model

import (
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ToLowhttpTraceInfoGRPCModel(l *lowhttp.LowhttpTraceInfo) *ypb.TraceInfo {
	if l == nil {
		return &ypb.TraceInfo{
			DurationMs:             -1,
			DNSDurationMs:          -1,
			ConnDurationMs:         -1,
			TotalDurationMs:        -1,
			TLSHandshakeDurationMs: -1,
			ConnectDurationMs:      -1,
			TCPDurationMs:          -1,
		}
	}
	return &ypb.TraceInfo{
		AvailableDNSServers:    l.AvailableDNSServers,
		DurationMs:             l.ServerTime.Milliseconds(),
		DNSDurationMs:          l.DNSTime.Milliseconds(),
		ConnDurationMs:         l.ConnTime.Milliseconds(),
		TotalDurationMs:        l.TotalTime.Milliseconds(),
		TLSHandshakeDurationMs: l.TLSHandshakeTime.Milliseconds(),
		ConnectDurationMs:      l.ConnTime.Milliseconds(),
		TCPDurationMs:          l.TCPTime.Milliseconds(),
	}
}
