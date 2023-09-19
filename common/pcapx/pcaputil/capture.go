package pcaputil

import (
	"context"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
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

type Capturer struct {
	Device          []string
	Filename        string
	Output          *pcapgo.Writer
	BPFFilter       string
	SuricataFilters []*SuricataFilter
	Context         context.Context

	trafficPool *trafficPool
	//Assembler *tcpassembly.Assembler

	// output debug info
	Debug bool

	onFlowCreated func(*TrafficFlow)
}

type CaptureOption func(*Capturer) error

func emptyOption(_ *Capturer) error {
	return nil
}

func WithBPFFilter(bpf string) CaptureOption {
	return func(c *Capturer) error {
		c.BPFFilter = bpf
		return nil
	}
}

func WithOnTrafficFlow(h func(flow *TrafficFlow)) CaptureOption {
	return func(capturer *Capturer) error {
		capturer.onFlowCreated = h
		return nil
	}
}

func WithSuricataFilter(filename string) CaptureOption {
	return func(c *Capturer) error {
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
	return func(c *Capturer) error {
		c.Filename = filename
		return nil
	}
}

func WithContext(ctx context.Context) CaptureOption {
	return func(c *Capturer) error {
		c.Context = ctx
		return nil
	}
}

func WithDebug(b bool) CaptureOption {
	return func(c *Capturer) error {
		c.Debug = b
		return nil
	}
}

func WithDevice(devs ...string) CaptureOption {
	return func(c *Capturer) error {
		c.Device = devs
		return nil
	}
}

func WithOutput(filename string) CaptureOption {
	return func(c *Capturer) error {
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return errors.New("open file failed")
		}
		c.Output = pcapgo.NewWriter(file)
		return c.Output.WriteFileHeader(65535, layers.LinkTypeEthernet)
	}
}

func (c *Capturer) Save(pk gopacket.Packet) {
	if c.Output != nil {
		err := c.Output.WritePacket(pk.Metadata().CaptureInfo, pk.Data())
		if err != nil {
			log.Errorf("write packet data failed: %s", err)
		}
	}
}

func (c *Capturer) assemblyWithTS(flow gopacket.Flow, networkLayer gopacket.SerializableLayer, tcp *layers.TCP, ts time.Time) {
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

func (c *Capturer) packetHandler(ctx context.Context, packet gopacket.Packet) {
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

func NewDefaultConfig() *Capturer {
	return &Capturer{}
}

func _open(ctx context.Context, handler *pcap.Handle, bpf string, packetEntry func(context.Context, gopacket.Packet)) error {
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if bpf != "" {
		if err := handler.SetBPFFilter(bpf); err != nil {
			return utils.Errorf("set bpf filter failed: %s", err)
		}
	}

	packetSource := gopacket.NewPacketSource(handler, handler.LinkType())

	for {
		select {
		case <-ctx.Done():
			return nil
		case packet := <-packetSource.Packets():
			if packet == nil {
				return nil
			}
			if packetEntry != nil {
				packetEntry(innerCtx, packet)
			} else {
				fmt.Println(packet.String())
			}
		}
	}
}

func Start(opt ...CaptureOption) error {
	conf := NewDefaultConfig()
	for _, i := range opt {
		if err := i(conf); err != nil {
			return utils.Errorf("set option failed: %s", err)
		}
	}

	var handlers []*pcap.Handle
	if conf.Filename != "" {
		handler, err := OpenFile(conf.Filename)
		if err != nil {
			log.Errorf("open file (%v) failed: %s", conf.Filename, err)
		} else {
			handlers = append(handlers, handler)
		}
	} else if len(conf.Device) == 0 {
		var ifs, err = pcap.FindAllDevs()
		if err != nil {
			return utils.Errorf("(pcap) find all devs failed: %s", err)
		}

		if len(ifs) > 128 {
			return utils.Errorf("too many devices: %d", len(ifs))
		}

		if len(ifs) == 0 {
			return utils.Errorf("no pcap devices found")
		}

		for _, iface := range ifs {
			handler, err := OpenIfaceLive(iface.Name)
			if err != nil {
				log.Errorf("open device (%v) failed: %s", iface.Name, err)
				continue
			}
			handlers = append(handlers, handler)
		}
	} else {
		for _, i := range conf.Device {
			pcapIface, err := IfaceNameToPcapIfaceName(i)
			if err != nil {
				log.Warnf("convert iface name (%v) failed: %s, use default", i, err)
				pcapIface = i
			}
			handler, err := OpenIfaceLive(pcapIface)
			if err != nil {
				log.Errorf("open device (%v) failed: %s", pcapIface, err)
				continue
			}
			handlers = append(handlers, handler)
		}
	}

	if conf.Context == nil {
		conf.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(conf.Context)
	defer func() {
		log.Info("pcapx.utils.capture context done")
		cancel()
	}()

	conf.trafficPool = NewTrafficPool(ctx)
	conf.trafficPool.onFlowCreated = conf.onFlowCreated

	utils.WaitRoutinesFromSlice(handlers, func(handler *pcap.Handle) {
		if err := _open(ctx, handler, "", conf.packetHandler); err != nil {
			log.Errorf("open device failed: %s", err)
		}
	})

	return nil
}
