// Package sharkcli provides the interactive packet analyzer behind yak shark.
package sharkcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"golang.org/x/term"
)

var Command = &cli.Command{
	Name:  "shark",
	Usage: "Capture and inspect network packets in a Wireshark-style terminal",
	UsageText: "yak shark [options]\n\n" +
		"  yak shark\n  yak shark -i en0 --protocol dns,tls --bpf 'host 1.1.1.1'\n" +
		"  yak shark --pcap-file traffic.pcapng\n  yak shark --output-file capture.pcapng\n" +
		"  yak shark --plain --protocol tcp --count 100\n  yak shark --pcap-file traffic.pcap --json\n\n" +
		"Interactive: wheel/trackpad scrolls the hovered pane; click selects; drag scrollbars.\n" +
		"Arrows/j/k select, Tab switches panes, Enter pins flat fields, / searches, f edits BPF,\n" +
		"p builds a protocol filter, Space freezes/resumes the list, g/G first/follow, q quits.\n" +
		"Click a field to highlight its HEX/ASCII bytes; click bytes to locate their field.\n" +
		"4 or s opens Stream: h beginning, l recent, e evidence, t text, x hex,\n" +
		"d decoded fields, v direction, r refresh.\n" +
		"Click Search or press /: proto:http ip:10.223 content:hello hex:00ff stream:hello.\n" +
		"c changes packet cache capacity (default 20000); --max-packets sets it at startup.\n" +
		"Application protocol shortcuts use conventional ports. BPF is a capture filter;\n" +
		"interactive filters affect the display only. TCP streams are reassembled; TLS decryption requires an authorized --tls-keylog file. Search: fields dns.qry.name contains \"example\".",
	Flags: []cli.Flag{
		cli.StringFlag{Name: "interface,i", Usage: "Capture interface (default: physical interface with router gateway; excludes VPN/TUN)"},
		cli.StringFlag{Name: "pcap-file,r", Usage: "Read a pcap or pcapng capture instead of live traffic"},
		cli.StringFlag{Name: "output-file,w", Usage: "Save captured packets as .pcap or .pcapng (new file only)"},
		cli.StringFlag{Name: "tls-keylog", Usage: "Authorized NSS key log (bounded TLS AES-128-GCM profiles; never exported)"},
		cli.StringFlag{Name: "bpf,f", Usage: "BPF capture filter, combined with --protocol using AND"},
		cli.StringSliceFlag{Name: "protocol", Usage: "Quick protocol filter, comma-separated or repeatable (OR)"},
		cli.BoolFlag{Name: "list-interfaces,D", Usage: "List capture interfaces and exit"},
		cli.BoolFlag{Name: "list-protocols", Usage: "List protocol shortcuts and their BPF expressions"},
		cli.BoolFlag{Name: "plain", Usage: "Print packet summaries without the TUI"},
		cli.BoolFlag{Name: "json", Usage: "Print one JSON packet summary per line without the TUI"},
		cli.IntFlag{Name: "count,c", Usage: "Stop capture after this many matching packets (0: unlimited)"},
		cli.DurationFlag{Name: "duration", Usage: "Stop capture after this duration, e.g. 30s (0: unlimited)"},
		cli.IntFlag{Name: "max-packets", Value: defaultPacketCapacity, Usage: "Packets retained in the TUI (also editable with c); output file keeps all captured packets"},
		cli.IntFlag{Name: "max-streams", Value: 256, Usage: "Maximum TCP streams retained for inspection"},
		cli.IntFlag{Name: "stream-bytes", Value: defaultStreamBytes, Usage: "Recent reassembled bytes retained per TCP stream (global history limit: 32 MiB)"},
		cli.IntFlag{Name: "snaplen,s", Value: 65535, Usage: "Maximum bytes captured per packet"},
		cli.BoolFlag{Name: "promisc", Usage: "Enable promiscuous capture"},
	},
	Action: run,
}

func run(c *cli.Context) error {
	out := c.App.Writer
	if out == nil {
		out = os.Stdout
	}
	if c.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q; use --protocol dns or --bpf 'tcp port 80'", c.Args().First())
	}
	if c.Bool("list-protocols") {
		for _, name := range protocolNames() {
			if _, err := fmt.Fprintf(out, "%-12s %s\n", name, protocolFilters[name]); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintln(out, "Application shortcuts select conventional ports; protocol decoding is performed separately.")
		return err
	}
	if c.Bool("list-interfaces") {
		devices, err := pcap.FindAllDevs()
		if err != nil {
			return err
		}
		for _, dev := range devices {
			var addresses []string
			for _, address := range dev.Addresses {
				addresses = append(addresses, address.IP.String())
			}
			if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", dev.Name, strings.Join(addresses, ", "), dev.Description); err != nil {
				return err
			}
		}
		return nil
	}
	if c.Int("count") < 0 || c.Duration("duration") < 0 {
		return fmt.Errorf("count and duration must be non-negative")
	}
	if c.Int("max-packets") < 1 || c.Int("max-packets") > 100000 {
		return fmt.Errorf("max-packets must be between 1 and 100000")
	}
	if c.Int("snaplen") < 64 || c.Int("snaplen") > 262144 {
		return fmt.Errorf("snaplen must be between 64 and 262144")
	}
	if c.Int("max-streams") < 1 || c.Int("max-streams") > 4096 || c.Int("stream-bytes") < 4096 || c.Int("stream-bytes") > 8<<20 {
		return fmt.Errorf("max-streams must be 1..4096; stream-bytes must be 4096..8388608")
	}
	if c.String("pcap-file") != "" && c.String("interface") != "" {
		return fmt.Errorf("--pcap-file and --interface cannot be combined")
	}
	filter, err := buildFilter(c.StringSlice("protocol"), c.String("bpf"))
	if err != nil {
		return err
	}
	cfg := captureConfig{device: c.String("interface"), input: c.String("pcap-file"), output: c.String("output-file"), filter: filter, snaplen: c.Int("snaplen"), count: c.Int("count"), duration: c.Duration("duration"), promisc: c.Bool("promisc")}
	cfg.maxStreams, cfg.streamBytes = c.Int("max-streams"), c.Int("stream-bytes")
	if path := c.String("tls-keylog"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		f.Close()
		if err != nil {
			return err
		}
		cfg.tlsSecrets, err = pcaputil.ParseTLSKeyLog(string(data))
		if err != nil {
			return err
		}
	}

	src, err := openCapture(cfg)
	if err != nil {
		return err
	}
	defer src.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	interactive := !c.Bool("plain") && !c.Bool("json") && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("TERM") != "dumb"
	session := startCapture(ctx, src, cfg, interactive)
	defer session.Stop()
	if interactive {
		err = runTUI(ctx, session, src, cfg, c.Int("max-packets"))
		session.Stop()
		if err != nil {
			return err
		}
		return session.result()
	}
	return runPlain(ctx, session, out, c.Bool("json"))
}

func runPlain(ctx context.Context, s *captureSession, out io.Writer, asJSON bool) error {
	encoder := json.NewEncoder(out)
	for {
		select {
		case packet, ok := <-s.packets:
			if !ok {
				return s.result()
			}
			row := summarize(packet)
			var err error
			if asJSON {
				for _, e := range packet.protocolEvents() {
					row.PDUs = append(row.PDUs, map[string]any{"id": e.ID, "protocol": e.Protocol, "status": e.Status, "completeness": e.Completeness, "response_to": e.ResponseTo, "byte_source": e.SourceBytes, "fields": e.DisplayFields()})
				}
				err = encoder.Encode(row)
			} else {
				_, err = fmt.Fprintf(out, "%6d %s %-24s → %-24s %-12s %6d %s\n", row.Number, row.Time.Format("15:04:05.000000"), row.Source, row.Destination, row.Protocol, row.Length, row.Info)
			}
			if err != nil {
				return err
			}
		case <-ctx.Done():
			s.Stop()
			return s.result()
		}
	}
}
