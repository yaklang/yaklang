package pcaputil

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

var tsconst = uuid.New().String()

type DeviceAdapter struct {
	DeviceName string
	BPF        string
	Snaplen    int32
	Promisc    bool
	Timeout    time.Duration
}

type CaptureConfig struct {
	Context               context.Context
	mock                  PcapHandleOperation // TEST MOCK
	trafficPool           *TrafficPool
	Output                *pcapgo.Writer // output debug info
	onNetInterfaceCreated func(handle *PcapHandleWrapper)
	onFlowCreated         func(*TrafficFlow)
	wg                    *sync.WaitGroup
	Filename              string
	OverrideCacheId       string
	BPFFilter             string
	onPoolCreated         []func(*TrafficPool)
	Device                []string
	onEveryPacket         []func(packet gopacket.Packet)
	Debug                 bool // output debug info
	EnableCache           bool //  cache for handler cache
	EmptyDeviceStop       bool
	DisableAssembly       bool
	deviceAdapter         *DeviceAdapter
	DeviceAdapter         []*DeviceAdapter
}

type CaptureOption func(*CaptureConfig) error

func emptyOption(_ *CaptureConfig) error {
	return nil
}

func WithDeviceAdapter(devices ...*DeviceAdapter) CaptureOption {
	return func(c *CaptureConfig) error {
		c.DeviceAdapter = devices
		return nil
	}

}

func WithEveryPacket(h func(packet gopacket.Packet)) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.onEveryPacket == nil {
			c.onEveryPacket = make([]func(packet gopacket.Packet), 0)
		}
		c.onEveryPacket = append(c.onEveryPacket, h)
		return nil
	}
}

func WithNetInterfaceCreated(h func(handle *PcapHandleWrapper)) CaptureOption {
	return func(c *CaptureConfig) error {
		c.onNetInterfaceCreated = h
		return nil
	}
}

func WithEmptyDeviceStop(b bool) CaptureOption {
	return func(c *CaptureConfig) error {
		c.EmptyDeviceStop = b
		return nil
	}
}

func WithEnableCache(b bool) CaptureOption {
	return func(c *CaptureConfig) error {
		c.EnableCache = b
		return nil
	}
}

func WithDisableAssembly(b bool) CaptureOption {
	return func(c *CaptureConfig) error {
		c.DisableAssembly = b
		return nil
	}
}

func WithOverrideCacheId(id string) CaptureOption {
	return func(c *CaptureConfig) error {
		c.OverrideCacheId = id
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

func WithMockPcapOperation(op PcapHandleOperation) CaptureOption {
	return func(config *CaptureConfig) error {
		config.mock = op
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
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0o644)
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
	runner := omap.NewOrderedMap(make(map[string]*sync.Once))
	return withPool(func(pool *TrafficPool) {
		pool._onHTTPFlow = h
		pool.onFlowFrameDataFrameReassembled = append(pool.onFlowFrameDataFrameReassembled, func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame) {
			if len(frame.Payload) <= 0 {
				return
			}

			if !conn.IsMarkedAsHttpPacket() {
				if _, err := utils.ReadHTTPRequestFromBytes(frame.Payload); err == nil {
					flow.httpflowWg.Add(2)
					if flow.ClientConn == conn {
						flow.ClientConn.MarkAsHttpRequestConn(true)
						flow.ServerConn.MarkAsHttpRequestConn(false)
					} else {
						flow.ClientConn.MarkAsHttpRequestConn(false)
						flow.ServerConn.MarkAsHttpRequestConn(true)
					}
				} else if rsp, err := utils.ReadHTTPResponseFromBytes(frame.Payload, nil); err == nil && strings.HasPrefix(rsp.Proto, "HTTP/") {
					flow.httpflowWg.Add(2)
					if flow.ClientConn == conn {
						flow.ClientConn.MarkAsHttpRequestConn(false)
						flow.ServerConn.MarkAsHttpRequestConn(true)
					} else {
						flow.ClientConn.MarkAsHttpRequestConn(true)
						flow.ServerConn.MarkAsHttpRequestConn(false)
					}
				}
			}

			if conn.IsMarkedAsHttpPacket() && !runner.Have(flow.Hash) {
				// recognized http packet direction
				once := new(sync.Once)
				runner.Set(flow.Hash, once)
				once.Do(func() {
					requestConn := flow.GetHTTPRequestConnection()
					responseConn := flow.GetHTTPResponseConnection()
					go func() {
						defer flow.httpflowWg.Done()
						reader := bufio.NewReader(requestConn.reader)
						for {
							req, err := utils.ReadHTTPRequestFromBufioReader(reader)
							if err != nil {
								return
							}
							offset := requestConn.reader.Count() - reader.Buffered()
							httpctx.SetRequestReaderOffset(req, offset)
							flow.StashHTTPRequest(req)
							flow.AutoTriggerHTTPFlow(h)
						}
					}()
					go func() {
						defer flow.httpflowWg.Done()
						defer func() {
							if err := recover(); err != nil {
								log.Errorf("http flow panic with: %v", err)
							}
						}()
						reader := bufio.NewReader(responseConn.reader)
						for {
							rsp, err := utils.ReadHTTPResponseFromBufioReader(reader, nil)
							if err != nil {
								return
							}
							offset := responseConn.reader.Count() - reader.Buffered()
							rsp.Header.Set(tsconst, fmt.Sprint(offset))
							flow.StashHTTPResponse(rsp)
							flow.AutoTriggerHTTPFlow(h)
						}
					}()
				})
			}
		})
	})
}

func (c *CaptureConfig) assemblyWithTS(flow gopacket.Packet, networkLayer gopacket.SerializableLayer, tcp *layers.TCP, ts time.Time) {
	if c.DisableAssembly {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("assembly panic with: %s\n    FLOW: %v\n    TCP: \n%v\n    Payload:\n%v", err, flow.String(), spew.Sdump(tcp.LayerContents()), spew.Sdump(tcp.Payload))
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	raw, _ := flow.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
	c.trafficPool.Feed(raw, networkLayer, tcp, ts)
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
	if packet != nil && packet.Metadata() != nil {
		ts = packet.Metadata().Timestamp
	} else {
		ts = time.Now()
	}

	defer func() {
		if c.onEveryPacket != nil {
			for _, f := range c.onEveryPacket {
				f(packet)
			}
		}
	}()

	if packet == nil {
		return
	}

	var matched bool
	ret, isOk := packet.TransportLayer().(*layers.TCP)
	if !isOk || ret == nil {
		return
	}

	if netIPv4Layer, ipv4ok := packet.NetworkLayer().(*layers.IPv4); ipv4ok {
		c.assemblyWithTS(packet, netIPv4Layer, ret, ts)
	} else if netIPv6Layer, ipv6ok := packet.NetworkLayer().(*layers.IPv6); ipv6ok {
		c.assemblyWithTS(packet, netIPv6Layer, ret, ts)
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
	return &CaptureConfig{wg: new(sync.WaitGroup)}
}

func WithCaptureStartedCallback(callback func()) CaptureOption {
	return func(c *CaptureConfig) error {
		// 保存原始的onPoolCreated回调
		c.onPoolCreated = append(c.onPoolCreated, func(pool *TrafficPool) {
			// 在流量池创建后立即调用回调
			if callback != nil {
				callback()
			}
		})
		return nil
	}
}
