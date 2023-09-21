package pcaputil

import (
	"context"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net/http"
	"os"
	"strings"
	"time"
)

type CaptureConfig struct {
	Device    []string
	Filename  string
	Output    *pcapgo.Writer
	BPFFilter string
	Context   context.Context

	trafficPool *TrafficPool

	// output debug info
	Debug         bool
	onPoolCreated []func(*TrafficPool)
	onFlowCreated func(*TrafficFlow)
	onEveryPacket func(any)
}

type CaptureOption func(*CaptureConfig) error

func emptyOption(_ *CaptureConfig) error {
	return nil
}

func WithEveryPacket(h func(any)) CaptureOption {
	return func(c *CaptureConfig) error {
		c.onEveryPacket = h
		return nil
	}
}

func WithBPFFilter(bpf string) CaptureOption {
	return func(c *CaptureConfig) error {
		c.BPFFilter = bpf
		return nil
	}
}

func WithOnTrafficFlowCreated(h func(flow *TrafficFlow)) CaptureOption {
	return func(capturer *CaptureConfig) error {
		capturer.onPoolCreated = append(capturer.onPoolCreated, func(pool *TrafficPool) {
			pool.onFlowCreated = h
		})
		return nil
	}
}

func WithOnTrafficFlowClosed(h func(reason TrafficFlowCloseReason, flow *TrafficFlow)) CaptureOption {
	return withPool(func(pool *TrafficPool) {
		pool.onFlowClosed = h
	})
}

func WithOnTrafficFlowOnDataFrameArrived(h func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)) CaptureOption {
	return withPool(func(pool *TrafficPool) {
		pool.onFlowFrameDataFrameArrived = append(pool.onFlowFrameDataFrameArrived, h)
	})
}

func WithOnTrafficFlowOnDataFrameReassembled(h func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)) CaptureOption {
	return withPool(func(pool *TrafficPool) {
		pool.onFlowFrameDataFrameReassembled = append(pool.onFlowFrameDataFrameReassembled, h)
	})
}

func withPool(h func(pool *TrafficPool)) CaptureOption {
	return func(config *CaptureConfig) error {
		config.onPoolCreated = append(config.onPoolCreated, func(pool *TrafficPool) {
			h(pool)
		})
		return nil
	}
}

func WithFile(filename string) CaptureOption {
	return func(c *CaptureConfig) error {
		c.Filename = filename
		return nil
	}
}

func WithContext(ctx context.Context) CaptureOption {
	return func(c *CaptureConfig) error {
		c.Context = ctx
		return nil
	}
}

func WithDebug(b bool) CaptureOption {
	return func(c *CaptureConfig) error {
		c.Debug = b
		return nil
	}
}

func WithDevice(devs ...string) CaptureOption {
	return func(c *CaptureConfig) error {
		c.Device = devs
		return nil
	}
}

func WithOutput(filename string) CaptureOption {
	return func(c *CaptureConfig) error {
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return errors.New("open file failed")
		}
		c.Output = pcapgo.NewWriter(file)
		return c.Output.WriteFileHeader(65535, layers.LinkTypeEthernet)
	}
}

func (c *CaptureConfig) Save(pk gopacket.Packet) {
	if c.Output != nil {
		err := c.Output.WritePacket(pk.Metadata().CaptureInfo, pk.Data())
		if err != nil {
			log.Errorf("write packet data failed: %s", err)
		}
	}
}

func WithTLSClientHello(h func(flow *TrafficFlow, hello *tlsutils.HandshakeClientHello)) CaptureOption {
	return withPool(func(pool *TrafficPool) {
		pool.onFlowFrameDataFrameReassembled = append(pool.onFlowFrameDataFrameReassembled, func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame) {
			if len(frame.Payload) <= 0 {
				return
			}

			if hello, err := tlsutils.ParseClientHello(frame.Payload); err == nil {
				h(flow, hello)
			}
		})
	})
}

func WithHTTPRequest(h func(flow *TrafficFlow, req *http.Request)) CaptureOption {
	return withPool(func(pool *TrafficPool) {
		pool.onFlowFrameDataFrameReassembled = append(pool.onFlowFrameDataFrameReassembled, func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame) {
			if len(frame.Payload) <= 0 {
				return
			}

			if req, err := utils.ReadHTTPRequestFromBytes(frame.Payload); err == nil && utils.IsCommonHTTPRequestMethod(req) {
				h(flow, req)
			}
		})
	})
}

func WithHTTPFlow(h func(flow *TrafficFlow, req *http.Request, rsp *http.Response)) CaptureOption {
	return withPool(func(pool *TrafficPool) {
		pool.onFlowFrameDataFrameReassembled = append(pool.onFlowFrameDataFrameReassembled, func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame) {
			if len(frame.Payload) <= 0 {
				return
			}

			if req, err := utils.ReadHTTPRequestFromBytes(frame.Payload); err == nil && utils.IsCommonHTTPRequestMethod(req) {
				flow.StashHTTPRequest(req)
			} else if rsp, err := utils.ReadHTTPResponseFromBytes(frame.Payload, nil); err == nil && strings.HasPrefix(rsp.Proto, "HTTP/") {
				rsp.Request = flow.FetchStashedHTTPRequest()
				h(flow, rsp.Request, rsp)
				if rsp.Request == nil {
					log.Warnf("no request found for response: %v %v", rsp.Proto, rsp.Status)
				}
			}
		})
	})
}

func (c *CaptureConfig) assemblyWithTS(flow gopacket.Flow, networkLayer gopacket.SerializableLayer, tcp *layers.TCP, ts time.Time) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("assembly panic with: %s\n    FLOW: %v\n    TCP: \n%v\n    Payload:\n%v", err, flow.String(), spew.Sdump(tcp.LayerContents()), spew.Sdump(tcp.Payload))
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	c.trafficPool.Feed(flow, networkLayer, tcp)
	//if c.Assembler != nil {
	//	if tcp.Payload == nil {
	//		return
	//	}
	//	c.Assembler.AssembleWithTimestamp(flow, tcp, ts)
	//}
}

func (c *CaptureConfig) packetHandler(ctx context.Context, packet gopacket.Packet) {
	defer func() {
		if err := recover(); err != nil {
			spew.Dump(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	save := true
	var ts time.Time
	if packet.Metadata() != nil {
		ts = packet.Metadata().Timestamp
	} else {
		ts = time.Now()
	}

	var matched bool
	ret, isOk := packet.TransportLayer().(*layers.TCP)
	if !isOk || ret == nil {
		return
	}

	if netIPv4Layer, ipv4ok := packet.NetworkLayer().(*layers.IPv4); ipv4ok {
		c.assemblyWithTS(netIPv4Layer.NetworkFlow(), netIPv4Layer, ret, ts)
	} else if netIPv6Layer, ipv6ok := packet.NetworkLayer().(*layers.IPv6); ipv6ok {
		c.assemblyWithTS(netIPv6Layer.NetworkFlow(), netIPv6Layer, ret, ts)
	} else {
		log.Warnf("unknown network layer: %v", packet.NetworkLayer())
	}

	if c.Debug && !matched {
		fmt.Println(packet.String())
	}

	if save {
		c.Save(packet)
	}
}

func NewDefaultConfig() *CaptureConfig {
	return &CaptureConfig{}
}
