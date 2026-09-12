package pcaputil

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// ReplayPcapFile replays pcap or pcapng without opening a native capture handle.
// It preserves nanosecond timestamps. BPF and native handle hooks require
// OpenPcapFile instead; they are never silently ignored here.
func ReplayPcapFile(filename string, options ...CaptureOption) error {
	conf := NewDefaultConfig()
	for _, option := range options {
		if err := option(conf); err != nil {
			return err
		}
	}
	return replayFileWithConfig(filename, conf)
}

func replayFileWithConfig(filename string, conf *CaptureConfig) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return replayWithConfig(f, conf)
}

// ReplayPcap borrows the reader until return, drains accepted worker jobs at EOF
// or cancellation, and does not close the caller's reader. A blocking custom
// reader must provide its own cancellation (ordinary file reads are bounded).
func ReplayPcap(input io.Reader, options ...CaptureOption) error {
	conf := NewDefaultConfig()
	for _, option := range options {
		if err := option(conf); err != nil {
			return err
		}
	}
	return replayWithConfig(input, conf)
}

func replayWithConfig(input io.Reader, conf *CaptureConfig) (resultErr error) {
	if conf.captureBuffer > 0 || conf.BPFFilter != "" || conf.onNetInterfaceCreated != nil || conf.EnableCache || len(conf.DeviceAdapter) != 0 || len(conf.Device) != 0 {
		return fmt.Errorf("ReplayPcap: BPF, devices, native handle callbacks and capture cache require a native capture")
	}
	if err := conf.prepareBinParser(); err != nil {
		return err
	}
	if conf.reassemblyOptions.Stream && conf.requiresFullStream {
		return fmt.Errorf("TCP streaming cannot be combined with built-in HTTP/TLS parsers")
	}
	br := bufio.NewReader(input)
	header, err := br.Peek(4)
	if err != nil {
		return err
	}
	var read func() ([]byte, gopacket.CaptureInfo, error)
	var link layers.LinkType
	var ng *pcapgo.NgReader
	if binary.LittleEndian.Uint32(header) == 0x0a0d0d0a {
		ng, err = pcapgo.NewNgReader(&boundedNgInput{input: br}, pcapgo.NgReaderOptions{WantMixedLinkType: true})
		if err != nil {
			return err
		}
		read = ng.ZeroCopyReadPacketData
	} else {
		r, err := newClassicPcapReader(br)
		if err != nil {
			return err
		}
		read, link = r.read, r.link
	}
	closeOutput, err := conf.openCaptureOutput()
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeOutput()) }()
	if conf.recorder != nil && ng == nil {
		if err := conf.recorder.init(link); err != nil {
			return err
		}
	}
	parent := conf.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	conf.Context = ctx
	pool := newTrafficPool(ctx, conf.reassemblyOptions)
	pool.captureAccountingAvailable = true
	conf.trafficPool, pool.captureConf = pool, conf
	defer func() {
		pool.Close()
		cancel()
		resultErr = errors.Join(resultErr, pool.Err())
		if conf.onReassemblyStats != nil {
			conf.onReassemblyStats(pool.Stats())
		}
		resultErr = errors.Join(resultErr, conf.finishBinParser())
	}()
	for _, init := range conf.onPoolCreated {
		init(pool)
	}
	pool.startWorkers()
	d := offlineDecoder{conf: conf}
	for ctx.Err() == nil {
		raw, ci, err := read()
		if errors.Is(err, io.EOF) {
			if conf.recorder != nil && ng != nil && conf.recorder.writer == nil {
				iface, err := ng.Interface(0)
				if err != nil {
					return fmt.Errorf("empty pcapng recording has no interface: %w", err)
				}
				return conf.recorder.init(iface.LinkType)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if ng != nil {
			iface, err := ng.Interface(ci.InterfaceIndex)
			if err != nil {
				return err
			}
			link = iface.LinkType
		}
		d.link = link
		pool.observeCapture(len(raw))
		if conf.recorder != nil {
			if err := conf.recorder.write(raw, ci, link); err != nil {
				return err
			}
		}
		if len(conf.onEveryPacket) != 0 || conf.Output != nil || conf.Debug {
			packet := gopacket.NewPacket(raw, link, gopacket.DecodeOptions{Lazy: true, NoCopy: false, DecodeStreamsAsDatagrams: true})
			packet.Metadata().CaptureInfo = ci
			conf.packetHandler(ctx, packet)
		} else if pool.parallel != nil && !conf.DisableAssembly {
			pool.parallel.checkTruncation(ci)
			if key, ok, err := rawFlowKey(raw, link); err != nil {
				pool.malformedPacket(err.Error())
			} else if ok {
				pool.parallel.submit(workerPacket{data: raw, ts: ci.Timestamp, link: link, raw: true, key: key})
			} else if conf.binParser != nil {
				d.feed(ctx, raw, ci) // datagrams; TCP still uses the private worker path
			}
		} else {
			d.feed(ctx, raw, ci)
		}
	}
	return nil
}
