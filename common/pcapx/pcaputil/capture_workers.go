package pcaputil

import (
	"context"
	"errors"
	"io"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
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
	return readLiveWorkerPackets(conf, ctx, read, link, private)
}

// Keep the native reader separate so offline regressions exercise live dispatch.
func readLiveWorkerPackets(conf *CaptureConfig, ctx context.Context, read func() ([]byte, gopacket.CaptureInfo, error), link layers.LinkType, private bool) error {
	d := offlineDecoder{conf: conf, link: link}
	var number uint64
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
		number++
		evidence := evidenceFrom(ci)
		if evidence.Ref.Number == 0 {
			evidence.Ref.Number = number
		}
		ci = withEvidence(ci, evidence)
		conf.trafficPool.observeCaptureInfo(len(raw), ci)
		if conf.recorder != nil {
			if err := conf.recorder.write(raw, ci, link); err != nil {
				return err
			}
		}
		if private {
			if conf.trafficPool.parallel == nil {
				d.feed(ctx, raw, ci)
				continue
			}
			if !conf.DisableAssembly {
				if key, ok, err := rawFlowKey(raw, link); err != nil {
					if conf.binParser != nil {
						d.feed(ctx, raw, ci)
					} else {
						conf.trafficPool.malformedPacket(err.Error())
					}
				} else if ok {
					key.domain = evidence.Ref.Domain
					conf.trafficPool.parallel.submit(workerPacket{data: raw, ts: ci.Timestamp, captureLength: ci.CaptureLength, originalLength: ci.Length, link: link, raw: true, key: key, evidence: evidence})
				} else if conf.binParser != nil {
					d.feed(ctx, raw, ci)
				}
			}
		} else {
			packet := gopacket.NewPacket(raw, captureLinkDecoder(link), gopacket.DecodeOptions{Lazy: true, NoCopy: true, DecodeStreamsAsDatagrams: conf.binParser == nil})
			packet.Metadata().CaptureInfo = ci
			conf.packetHandlerWithLink(ctx, packet, link)
		}
	}
	return nil
}
