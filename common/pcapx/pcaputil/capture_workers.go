package pcaputil

import (
	"context"
	"errors"
	"io"

	"github.com/gopacket/gopacket"
	"github.com/yaklang/pcap"
)

// Exclusive bin-parser/worker captures own a finite-timeout native reader. No producer goroutine
// remains inside libpcap when final device statistics are collected.
func openLiveWorkers(conf *CaptureConfig, ctx context.Context, h *PcapHandleWrapper) error {
	private := len(conf.onEveryPacket) == 0 && conf.Output == nil && !conf.Debug
	read := h.ReadPacketData
	if private {
		read = h.handle.ZeroCopyReadPacketData
	}
	link := h.LinkType()
	d := offlineDecoder{conf: conf, link: link}
	for ctx.Err() == nil {
		raw, ci, err := read()
		if err == pcap.NextErrorTimeoutExpired {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		conf.trafficPool.observeCapture(len(raw))
		if conf.recorder != nil {
			if err := conf.recorder.write(raw, ci, link); err != nil {
				return err
			}
		}
		if private {
			if conf.trafficPool.parallel == nil {
				if ci.CaptureLength < ci.Length {
					conf.trafficPool.singleDiagnostics.truncated.Add(1)
					conf.trafficPool.reassemblyFailure("truncated live capture")
				}
				d.feed(ctx, raw, ci)
				continue
			}
			conf.trafficPool.parallel.checkTruncation(ci)
			if !conf.DisableAssembly {
				if key, ok, err := rawFlowKey(raw, link); err != nil {
					conf.trafficPool.malformedPacket(err.Error())
				} else if ok {
					conf.trafficPool.parallel.submit(workerPacket{data: raw, ts: ci.Timestamp, link: link, raw: true, key: key})
				} else if conf.binParser != nil {
					d.feed(ctx, raw, ci)
				}
			}
		} else {
			packet := gopacket.NewPacket(raw, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true, DecodeStreamsAsDatagrams: true})
			packet.Metadata().CaptureInfo = ci
			conf.packetHandler(ctx, packet)
		}
	}
	return nil
}
