//go:build !linux

package netstackvm

import "github.com/yaklang/pcap"

func captureWithChecksumMetadata(_ string, _ int, _ bool, handle *pcap.Handle) packetCaptureHandle {
	return handle
}
