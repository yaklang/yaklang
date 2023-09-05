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

	var ts time.Time
	if packet.Metadata() != nil {
		ts = packet.Metadata().Timestamp
	} else {
		ts = time.Now()
	}

	// todo: suricata group matcher
	for _, filter := range c.SuricataFilters {
		if filter.Matcher.Match(packet.Data()) {
			fmt.Printf("[%s] Alert %s\n", ts.String(), filter.Rule.Message)
			fmt.Println(packet.String())
			c.Save(packet)
			break
		}
	}

	// clear it?
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

	if c.Debug {
		fmt.Println(packet.String())
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

	var devs []string
	if len(conf.Device) == 0 {
		var ifs, err = pcap.FindAllDevs()
		if err != nil {
			return utils.Errorf("(pcap) find all devs failed: %s", err)
		}
		for _, iface := range ifs {
			devs = append(devs, iface.Name)
		}
	} else {
		for _, i := range conf.Device {
			pcapIface, err := IfaceNameToPcapIfaceName(i)
			if err != nil {
				log.Warnf("convert iface name (%v) failed: %s, use default", i, err)
				pcapIface = i
			}
			devs = append(devs, pcapIface)
		}
	}

	// TODO: check devs length, 128 is enough...
	if len(devs) > 128 {
		return utils.Errorf("too many devices: %d", len(devs))
	} else if len(devs) == 0 {
		return utils.Errorf("no pcap devices found")
	}

	if conf.Context == nil {
		conf.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(conf.Context)
	defer cancel()

	// create stream factory and pool
	//streamFactory := NewStreamFactory(ctx)
	//streamPool := tcpassembly.NewStreamPool(streamFactory)
	//assembler := tcpassembly.NewAssembler(streamPool)
	// conf.Assembler = assembler
	conf.trafficPool = NewTrafficPool(ctx)

	var handlers []*pcap.Handle
	for _, i := range devs {
		handler, err := OpenIfaceLive(i)
		if err != nil {
			log.Errorf("open device (%v) failed: %s", i, err)
			continue
		}
		handlers = append(handlers, handler)
	}

	if conf.Filename != "" {
		handler, err := OpenFile(conf.Filename)
		if err != nil {
			log.Errorf("open file (%v) failed: %s", conf.Filename, err)
		} else {
			handlers = append(handlers, handler)
		}
	}

	utils.WaitRoutinesFromSlice(handlers, func(handler *pcap.Handle) {
		if err := _open(ctx, handler, "", conf.packetHandler); err != nil {
			log.Errorf("open device failed: %s", err)
		}
	})

	return nil
}
