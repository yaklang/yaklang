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
	"github.com/yaklang/yaklang/common/suricata/match"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"time"
)

type SuricataFilter struct {
	Rule    *surirule.Rule
	Matcher *match.Matcher
}

type CaptureConfig struct {
	Device          []string
	Filename        string
	Output          *pcapgo.Writer
	BPFFilter       string
	SuricataFilters []*SuricataFilter
	Context         context.Context

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
		pool.onFlowFrameDataFrameArrived = h
	})
}

func WithOnTrafficFlowOnDataFrameReassembled(h func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)) CaptureOption {
	return withPool(func(pool *TrafficPool) {
		pool.onFlowFrameDataFrameReassembled = h
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

func WithSuricataFilter(filename string) CaptureOption {
	return func(c *CaptureConfig) error {
		file, err := os.Open(filename)
		if err != nil {
			return err
		}

		data, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		rules, err := surirule.Parse(string(data))
		if err != nil {
			return err
		}

		var filters []*SuricataFilter
		for _, rule := range rules {
			filters = append(filters, &SuricataFilter{
				Rule:    rule,
				Matcher: match.New(rule),
			})
		}
		c.SuricataFilters = append(c.SuricataFilters, filters...)
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

	// todo: suricata group matcher
	var matched bool
	for _, filter := range c.SuricataFilters {
		if filter.Matcher.Match(packet.Data()) {
			fmt.Printf("[%s] Alert %s\n", ts.String(), filter.Rule.Message)
			fmt.Println(packet.String())
			matched = true
			break
		}
	}
	if len(c.SuricataFilters) != 0 && !matched {
		save = false
	}

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
