// pcap-inspect captures/replays packets with full fields and bounded viewing.
package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync/atomic"
	"text/tabwriter"
	"time"
)

type options struct {
	read, device, bpf, write, report, protocol string
	cpuProfile                                 string
	duration, interval                         time.Duration
	workers, history, memory, rows, snaplen    int
	captureBuffer                              int
	flow, detail                               uint64
	list, full, deferred, follow, quiet        bool
}

func parseOptions(args []string, stderr io.Writer) (o options, err error) {
	f := flag.NewFlagSet("pcap-inspect", flag.ContinueOnError)
	f.SetOutput(stderr)
	f.StringVar(&o.read, "read", "", "pcap/pcapng input file")
	f.StringVar(&o.device, "interface", "", "capture device name or index from -list")
	f.BoolVar(&o.list, "list", false, "list interfaces and exit")
	f.StringVar(&o.bpf, "bpf", "", "capture filter; file filtering also requires libpcap/Npcap")
	f.StringVar(&o.write, "write", "", "save original packets to a NEW nanosecond pcap file")
	f.StringVar(&o.report, "report", "", "write final metrics to a NEW JSON file after capture")
	f.StringVar(&o.cpuProfile, "cpu-profile", "", "save Go CPU profile to a NEW file; profiling affects throughput")
	f.DurationVar(&o.duration, "duration", 30*time.Second, "live duration; 0 runs until Ctrl-C")
	f.DurationVar(&o.interval, "interval", time.Second, "console refresh interval, minimum 100ms")
	f.IntVar(&o.workers, "workers", 1, "TCP workers and GOMAXPROCS: 1, 2 or 4")
	f.IntVar(&o.history, "history", 4096, "retained messages; 0 disables history")
	f.IntVar(&o.memory, "memory-mib", 32, "retained raw-message MiB")
	f.IntVar(&o.rows, "rows", 30, "maximum displayed rows per refresh/final table")
	f.IntVar(&o.snaplen, "snaplen", 262144, "live snapshot length in bytes")
	f.IntVar(&o.captureBuffer, "capture-buffer-mib", 32, "native buffer MiB per live interface; 0 uses backend default, maximum 256")
	f.StringVar(&o.protocol, "protocol", "", "message display filter, e.g. http, tls, mqtt, dns")
	f.Uint64Var(&o.flow, "flow", 0, "display one TCP flow")
	f.Uint64Var(&o.detail, "detail", 0, "show retained message raw bytes and full fields at exit")
	f.BoolVar(&o.full, "full", true, "parse full structured fields during capture (default)")
	f.BoolVar(&o.deferred, "deferred", false, "capture first; parse only requested details")
	f.BoolVar(&o.follow, "follow", false, "show new messages in bounded refresh batches")
	f.BoolVar(&o.quiet, "quiet", false, "disable periodic console output")
	if err = f.Parse(args); err != nil {
		return
	}
	if f.NArg() != 0 {
		return o, errors.New("unexpected positional arguments")
	}
	if o.list {
		return
	}
	if (o.read == "") == (o.device == "") {
		return o, errors.New("specify exactly one of -read or -interface; use -list to find devices")
	}
	if o.workers != 1 && o.workers != 2 && o.workers != 4 {
		return o, errors.New("workers must be 1, 2 or 4")
	}
	if o.captureBuffer < 0 || o.captureBuffer > 256 {
		return o, errors.New("capture-buffer-mib must be 0 through 256")
	}
	if o.duration < 0 || o.interval < 100*time.Millisecond || o.history < 0 || o.history > 1000000 || o.memory < 1 || o.memory > 1024 || o.rows < 1 || o.rows > 1000 || o.snaplen < 64 || o.snaplen > 16<<20 {
		return o, errors.New("invalid duration, refresh, history, row or capture size limit")
	}
	if o.history == 0 && (o.detail != 0 || o.follow) {
		return o, errors.New("detail/follow requires history > 0")
	}
	return
}

type liveCounts struct{ decoded, bytes, events atomic.Uint64 }
type report struct {
	Mode                                                    string
	Workers                                                 int
	ElapsedSeconds, CPUSeconds                              float64
	CPUAvailable                                            bool
	RateAvailable                                           bool
	CPUCores, DecodedMbps, DeliveredMbps, MessagesPerSecond float64
	HistoryEvicted                                          uint64
	DecodedBytes                                            uint64
	CapturedMbps                                            float64
	Analysis                                                pcaputil.BinParserStats
	Reassembly                                              pcaputil.TCPReassemblyStats
	Error                                                   string `json:",omitempty"`
}

func showRows(out io.Writer, rows []*pcaputil.BinParserEvent) {
	if len(rows) == 0 {
		return
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tFLOW\tTIME\tPROTOCOL\tSOURCE -> DESTINATION\tBYTES\tSTATUS\tINFO")
	for _, e := range rows {
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s -> %s\t%d\t%s\t%q\n", e.ID, e.FlowID, e.Timestamp.Format("15:04:05.000000"), e.Protocol, e.Source, e.Destination, e.Length, e.Status, e.Summary)
	}
	w.Flush()
}
func run(ctx context.Context, args []string, stdout, stderr io.Writer) (resultErr error) {
	o, err := parseOptions(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if o.list || o.device != "" {
		devs, err := pcap.FindAllDevs()
		if err != nil {
			return fmt.Errorf("capture interfaces unavailable: %w; Windows: install Npcap from https://npcap.com", err)
		}
		if o.list {
			fmt.Fprintln(stdout, "INDEX  DEVICE  DESCRIPTION  ADDRESSES")
			for i, d := range devs {
				fmt.Fprintf(stdout, "%d  %s  %q  %v\n", i+1, d.Name, d.Description, d.Addresses)
			}
			return nil
		}
		if index, err := strconv.Atoi(o.device); err == nil {
			if index < 1 || index > len(devs) {
				return fmt.Errorf("interface index %d unavailable; use -list", index)
			}
			o.device = devs[index-1].Name
		}
	}
	old := runtime.GOMAXPROCS(o.workers)
	defer runtime.GOMAXPROCS(old)
	var view *pcaputil.BinParserInspector
	if o.history > 0 {
		view, err = pcaputil.NewBinParserInspector(o.history, o.memory<<20)
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if o.device != "" && o.duration > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, o.duration)
		defer stop()
	}
	var metrics report
	metrics.Workers, metrics.Mode = o.workers, "full"
	if o.deferred || !o.full {
		metrics.Mode = "deferred"
	}
	var counts liveCounts
	opts := []pcaputil.CaptureOption{
		pcaputil.WithContext(ctx), pcaputil.WithTCPReassemblyWorkers(o.workers),
		pcaputil.WithBinParserDeferred(metrics.Mode == "deferred"),
		pcaputil.WithBinParser(func(e *pcaputil.BinParserEvent) {
			counts.events.Add(1)
			if e.Status == "decoded" {
				counts.decoded.Add(1)
				counts.bytes.Add(uint64(e.Length))
			}
			if view != nil {
				view.OnEvent(e)
			}
		}),
		pcaputil.WithBinParserStats(func(s pcaputil.BinParserStats) { metrics.Analysis = s }),
		pcaputil.WithTCPReassemblyStats(func(s pcaputil.TCPReassemblyStats) { metrics.Reassembly = s }),
	}
	var recording *os.File
	var buffer *bufio.Writer
	if o.write != "" {
		recording, err = os.OpenFile(o.write, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("create recording: %w", err)
		}
		defer recording.Close()
		buffer = bufio.NewWriterSize(recording, 256<<10)
		opts = append(opts, pcaputil.WithCaptureWriter(buffer))
	}
	var reportFile *os.File
	if o.report != "" {
		reportFile, err = os.OpenFile(o.report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("create report: %w", err)
		}
		defer reportFile.Close()
	}
	var cpuStart float64
	var profileFile *os.File
	if o.cpuProfile != "" {
		profileFile, err = os.OpenFile(o.cpuProfile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("create CPU profile: %w", err)
		}
		defer profileFile.Close()
		if err := pprof.StartCPUProfile(profileFile); err != nil {
			return err
		}
		defer pprof.StopCPUProfile()
	}
	proc, procErr := process.NewProcess(int32(os.Getpid()))
	if procErr == nil {
		if cpu, err := proc.Times(); err == nil {
			cpuStart, metrics.CPUAvailable = cpu.User+cpu.System, true
		}
	}
	start := time.Now()
	stopDisplay, displayDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(displayDone)
		if o.quiet {
			return
		}
		ticker := time.NewTicker(o.interval)
		defer ticker.Stop()
		lastBytes, lastMessages, lastTime := uint64(0), uint64(0), start
		var cursor uint64
		for {
			select {
			case <-stopDisplay:
				return
			case now := <-ticker.C:
				b, n := counts.bytes.Load(), counts.decoded.Load()
				dt := now.Sub(lastTime).Seconds()
				fmt.Fprintf(stderr, "[%s] decoded=%.2f Mbps %.0f msg/s total=%d events=%d\n", now.Format("15:04:05"), float64(b-lastBytes)*8/dt/1e6, float64(n-lastMessages)/dt, n, counts.events.Load())
				lastBytes, lastMessages, lastTime = b, n, now
				if o.follow {
					rows, next, omitted := view.RowsAfter(cursor, o.rows, o.protocol, o.flow)
					cursor = next
					showRows(stdout, rows)
					if omitted > 0 {
						fmt.Fprintf(stderr, "view: %d rows omitted by history/display limits (not analysis loss)\n", omitted)
					}
				}
			}
		}
	}()
	if o.read != "" {
		if o.bpf != "" {
			resultErr = pcaputil.OpenPcapFile(o.read, append(opts, pcaputil.WithBPFFilter(o.bpf))...)
		} else {
			resultErr = pcaputil.ReplayPcapFile(o.read, opts...)
		}
	} else {
		opts = append(opts, pcaputil.WithCaptureBufferSize(o.captureBuffer<<20))
		opts = append(opts, pcaputil.WithDeviceAdapter(&pcaputil.DeviceAdapter{DeviceName: o.device, BPF: o.bpf, Snaplen: int32(o.snaplen), Timeout: 20 * time.Millisecond}))
		resultErr = pcaputil.Start(opts...)
	}
	close(stopDisplay)
	<-displayDone
	if buffer != nil {
		resultErr = errors.Join(resultErr, buffer.Flush(), recording.Close())
	}
	metrics.ElapsedSeconds = time.Since(start).Seconds()
	metrics.RateAvailable = metrics.ElapsedSeconds > 0
	if metrics.CPUAvailable {
		if cpu, err := proc.Times(); err != nil {
			metrics.CPUAvailable = false
		} else {
			metrics.CPUSeconds = cpu.User + cpu.System - cpuStart
			if metrics.RateAvailable {
				metrics.CPUCores = metrics.CPUSeconds / metrics.ElapsedSeconds
			}
		}
	}
	if profileFile != nil {
		pprof.StopCPUProfile()
	}
	metrics.DecodedBytes = counts.bytes.Load()
	if metrics.RateAvailable {
		metrics.DecodedMbps = float64(metrics.DecodedBytes) * 8 / metrics.ElapsedSeconds / 1e6
		metrics.CapturedMbps = float64(metrics.Reassembly.CapturedBytes) * 8 / metrics.ElapsedSeconds / 1e6
		metrics.DeliveredMbps = float64(metrics.Analysis.InputBytes) * 8 / metrics.ElapsedSeconds / 1e6
		metrics.MessagesPerSecond = float64(metrics.Analysis.Decoded) / metrics.ElapsedSeconds
	}
	if view != nil {
		metrics.HistoryEvicted = view.Evicted()
		rows, _, omitted := view.RowsAfter(0, o.rows, o.protocol, o.flow)
		showRows(stdout, rows)
		if omitted > 0 {
			fmt.Fprintf(stderr, "view: %d rows omitted by history/display limits\n", omitted)
		}
		if o.detail != 0 {
			e, decodeErr := view.Details(o.detail)
			resultErr = errors.Join(resultErr, decodeErr)
			if e != nil {
				fmt.Fprintf(stdout, "message %d: %s\n%s\n", e.ID, e.Summary, hex.Dump(e.Raw))
				encoded, err := json.MarshalIndent(e.Structured, "", "  ")
				resultErr = errors.Join(resultErr, err)
				fmt.Fprintln(stdout, string(encoded))
			}
		}
	}
	if resultErr != nil {
		metrics.Error = resultErr.Error()
	}
	fmt.Fprintf(stderr, "finished: mode=%s workers=%d elapsed=%.3fs decoded=%.3f Mbps delivered=%.3f Mbps CPU=%.3fs cores=%.3f cpu_available=%v\nanalysis: %+v\nreassembly: %+v\nhistory_evicted=%d\n", metrics.Mode, metrics.Workers, metrics.ElapsedSeconds, metrics.DecodedMbps, metrics.DeliveredMbps, metrics.CPUSeconds, metrics.CPUCores, metrics.CPUAvailable, metrics.Analysis, metrics.Reassembly, metrics.HistoryEvicted)
	if reportFile != nil {
		enc := json.NewEncoder(reportFile)
		enc.SetIndent("", "  ")
		resultErr = errors.Join(resultErr, enc.Encode(metrics))
	}
	return
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
